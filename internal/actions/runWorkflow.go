package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/v63/github"
	"github.com/pkg/errors"
	"github.com/speakeasy-api/sdk-generation-action/internal/utils"
	"github.com/speakeasy-api/sdk-generation-action/internal/versionbumps"
	"github.com/speakeasy-api/versioning-reports/versioning"

	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/git"
	"github.com/speakeasy-api/sdk-generation-action/internal/run"

	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
)

func RunWorkflow() error {
	g, err := initAction()
	if err != nil {
		return err
	}

	if err := SetupEnvironment(); err != nil {
		return fmt.Errorf("failed to setup environment: %w", err)
	}

	// The top-level CLI can always use latest. The CLI itself manages pinned versions.
	resolvedVersion, err := cli.Download("latest", g)
	if err != nil {
		return err
	}

	// This flag is generally deprecated, it will not be provided on new action instances
	pinnedVersion := cli.GetVersion(environment.GetPinnedSpeakeasyVersion())
	if pinnedVersion != "latest" {
		resolvedVersion = pinnedVersion
		// This environment variable is read by the CLI to determine which version should be used to execute `run`
		if err := environment.SetCLIVersionToUse(pinnedVersion); err != nil {
			return fmt.Errorf("failed to set pinned speakeasy version: %w", err)
		}
	}

	mode := environment.GetMode()

	wf, err := configuration.GetWorkflowAndValidateLanguages(true)

	if err != nil {
		return err
	}

	sourcesOnly := wf.Targets == nil || len(wf.Targets) == 0

	branchName := ""
	var pr *github.PullRequest
	if mode == environment.ModePR {
		var err error
		branchName, pr, err = g.FindExistingPR("", environment.ActionRunWorkflow, sourcesOnly)
		if err != nil {
			return err
		}

		if pr != nil {
			os.Setenv("GH_PULL_REQUEST", *pr.URL)
		}
	}

	// We want to stay on main if we're pushing code samples because we want to tag the code samples with `main`
	if !environment.PushCodeSamplesOnly() && !environment.IsTestMode() {
		branchName, err = g.FindOrCreateBranch(branchName, environment.ActionRunWorkflow)
		if err != nil {
			return err
		}
	}

	success := false
	defer func() {
		if shouldDeleteBranch(success) {
			if err := g.DeleteBranch(branchName); err != nil {
				logging.Debug("failed to delete branch %s: %v", branchName, err)
			}
		}
	}()

	if branchName != "" {
		os.Setenv("SPEAKEASY_ACTIVE_BRANCH", branchName)
	}

	runRes, outputs, err := run.Run(g, pr, wf)
	if err != nil {
		if err := setOutputs(outputs); err != nil {
			logging.Debug("failed to set outputs: %v", err)
		}
		return err
	}

	anythingRegenerated := false

	var releaseInfo releases.ReleasesInfo
	runResultInfo, err := json.MarshalIndent(runRes, "", "  ")
	if err != nil {
		logging.Debug("failed to marshal runRes : %s\n", err)
	} else {
		logging.Debug("Result of running the command is: %s\n", runResultInfo)
	}
	if runRes.GenInfo != nil {
		docVersion := runRes.GenInfo.OpenAPIDocVersion
		resolvedVersion = runRes.GenInfo.SpeakeasyVersion

		releaseInfo = releases.ReleasesInfo{
			ReleaseTitle:       environment.GetInvokeTime().Format("2006-01-02 15:04:05"),
			DocVersion:         docVersion,
			SpeakeasyVersion:   resolvedVersion,
			GenerationVersion:  runRes.GenInfo.GenerationVersion,
			DocLocation:        environment.GetOpenAPIDocLocation(),
			Languages:          map[string]releases.LanguageReleaseInfo{},
			LanguagesGenerated: map[string]releases.GenerationInfo{},
		}

		supportedTargetNames := cli.GetSupportedTargetNames()
		for _, supportedTargetName := range supportedTargetNames {
			langGenInfo, ok := runRes.GenInfo.Languages[supportedTargetName]
			if ok && outputs[utils.OutputTargetRegenerated(supportedTargetName)] == "true" {
				anythingRegenerated = true

				path := outputs[utils.OutputTargetDirectory(supportedTargetName)]
				path = strings.TrimPrefix(path, "./")

				releaseInfo.LanguagesGenerated[supportedTargetName] = releases.GenerationInfo{
					Version: langGenInfo.Version,
					Path:    path,
				}

				if published, ok := outputs[utils.OutputTargetPublish(supportedTargetName)]; ok && published == "true" {
					releaseInfo.Languages[supportedTargetName] = releases.LanguageReleaseInfo{
						PackageName: langGenInfo.PackageName,
						Version:     langGenInfo.Version,
						Path:        path,
					}
				}
			}
		}

		var commitMessages map[string]string
		// We set commit message from persisted reports.
		// The reports were persisted by the speakeasy cli.
		updateCommitMessages(commitMessages, runRes)

		if environment.PushCodeSamplesOnly() {
			// If we're just pushing code samples we don't want to raise a PR
			return nil
		}

		releasesDir, err := getReleasesDir()
		if err != nil {
			return err
		}

		err = releases.UpdateReleasesFile(releaseInfo, releasesDir)
		if err != nil {
			logging.Debug("error while updating releases file: %v", err.Error())
			return err
		}

		if _, err := g.CommitAndPush(docVersion, resolvedVersion, "", environment.ActionRunWorkflow, false, runRes.VersioningReport); err != nil {
			return err
		}
	}

	outputs["resolved_speakeasy_version"] = resolvedVersion
	if sourcesOnly {
		if _, err := g.CommitAndPush("", resolvedVersion, "", environment.ActionRunWorkflow, sourcesOnly, nil); err != nil {
			return err
		}
	}

	// If test mode is successful to this point, exit here
	if environment.IsTestMode() {
		success = true
		return nil
	}

	if err := finalize(finalizeInputs{
		Outputs:              outputs,
		BranchName:           branchName,
		AnythingRegenerated:  anythingRegenerated,
		SourcesOnly:          sourcesOnly,
		Git:                  g,
		VersioningInfo:       runRes.VersioningInfo,
		LintingReportURL:     runRes.LintingReportURL,
		ChangesReportURL:     runRes.ChangesReportURL,
		OpenAPIChangeSummary: runRes.OpenAPIChangeSummary,
		GenInfo:              runRes.GenInfo,
		currentRelease:       &releaseInfo,
		releaseNotes:         runRes.ReleaseNotes,
	}); err != nil {
		return err
	}

	success = true

	return nil
}

func shouldDeleteBranch(isSuccess bool) bool {
	isDirectMode := environment.GetMode() == environment.ModeDirect
	return !environment.IsDebugMode() && !environment.IsTestMode() && (isDirectMode || !isSuccess)
}

type finalizeInputs struct {
	Outputs              map[string]string
	BranchName           string
	AnythingRegenerated  bool
	SourcesOnly          bool
	Git                  *git.Git
	LintingReportURL     string
	ChangesReportURL     string
	OpenAPIChangeSummary string
	VersioningReport     *versioning.MergedVersionReport
	VersioningInfo       versionbumps.VersioningInfo
	GenInfo              *run.GenerationInfo
	currentRelease       *releases.ReleasesInfo
	releaseNotes         map[string]string
}

// Sets outputs and creates or adds releases info
func finalize(inputs finalizeInputs) error {
	// If nothing was regenerated, we don't need to do anything
	if !inputs.AnythingRegenerated && !inputs.SourcesOnly {
		return nil
	}

	branchName, err := inputs.Git.FindAndCheckoutBranch(inputs.BranchName)
	if err != nil {
		return err
	}

	defer func() {
		inputs.Outputs["branch_name"] = branchName

		if err := setOutputs(inputs.Outputs); err != nil {
			logging.Debug("failed to set outputs: %v", err)
		}
	}()

	logging.Debug("getMode from the environment: %s\n", environment.GetMode())
	switch environment.GetMode() {
	case environment.ModePR:
		branchName, pr, err := inputs.Git.FindExistingPR(branchName, environment.ActionFinalize, inputs.SourcesOnly)
		if err != nil {
			return err
		}
		pr, err = inputs.Git.CreateOrUpdatePR(git.PRInfo{
			BranchName:           branchName,
			ReleaseInfo:          inputs.currentRelease,
			PreviousGenVersion:   inputs.Outputs["previous_gen_version"],
			PR:                   pr,
			SourceGeneration:     inputs.SourcesOnly,
			LintingReportURL:     inputs.LintingReportURL,
			ChangesReportURL:     inputs.ChangesReportURL,
			VersioningInfo:       inputs.VersioningInfo,
			OpenAPIChangeSummary: inputs.OpenAPIChangeSummary,
			GenInfo:              inputs.GenInfo,
		})

		if err != nil {
			return err
		}

		if pr != nil {
			os.Setenv("GH_PULL_REQUEST", *pr.URL)
		}

		// If we are in PR mode and testing should be triggered by this PR we will attempt to fire an empty commit from our app so trigger github actions checks
		// for more info on why this is necessary see https://github.com/peter-evans/create-pull-request/blob/main/docs/concepts-guidelines.md#workarounds-to-trigger-further-workflow-runs
		// If the customer has manually set up a PR_CREATION_PAT we will not do this
		if inputs.GenInfo != nil && inputs.GenInfo.HasTestingEnabled && os.Getenv("PR_CREATION_PAT") == "" {
			sanitizedBranchName := strings.TrimPrefix(branchName, "refs/heads/")
			if err := cli.FireEmptyCommit(os.Getenv("GITHUB_REPOSITORY_OWNER"), git.GetRepo(), sanitizedBranchName); err != nil {
				fmt.Println("Failed to create empty commit to trigger testing workflow", err)
			}
		}

	case environment.ModeDirect:
		var releaseInfo *releases.ReleasesInfo
		var oldReleaseInfo string
		var languages map[string]releases.LanguageReleaseInfo
		var newReleaseInfo map[string]string = nil
		if !inputs.SourcesOnly {
			releaseInfo = inputs.currentRelease
			languages = releaseInfo.Languages
			oldReleaseInfo = releaseInfo.String()
			if os.Getenv("SDK_CHANGELOG_JULY_2025") == "true" && inputs.releaseNotes != nil {
				newReleaseInfo = inputs.releaseNotes
			}

			// We still read from releases info for terraform generations since they use the goreleaser
			// Read from Releases.md for terraform generations
			if inputs.Outputs[utils.OutputTargetRegenerated("terraform")] == "true" {
				releaseInfo, err = getReleasesInfo()
				oldReleaseInfo = releaseInfo.String()
				newReleaseInfo = nil
				languages = releaseInfo.Languages
				if err != nil {
					return err
				}
			}
		}

		commitHash, err := inputs.Git.MergeBranch(branchName)
		if err != nil {
			return err
		}

		if !inputs.SourcesOnly {
			if err := inputs.Git.CreateRelease(oldReleaseInfo, languages, inputs.Outputs, newReleaseInfo); err != nil {
				return err
			}
		}

		inputs.Outputs["commit_hash"] = commitHash

		// add merging branch registry tag
		if err = addDirectModeBranchTagging(); err != nil {
			return errors.Wrap(err, "failed to tag registry images")
		}

	}

	return nil
}

func addDirectModeBranchTagging() error {
	wf, err := configuration.GetWorkflowAndValidateLanguages(true)
	if err != nil {
		return err
	}

	branch := strings.TrimPrefix(os.Getenv("GITHUB_REF"), "refs/heads/")

	var sources, targets []string
	// a tag that is applied if the target contributing is published
	var isPublished bool
	// the tagging library treats targets synonymously with code samples
	if specificTarget := environment.SpecifiedTarget(); specificTarget != "" {
		if target, ok := wf.Targets[specificTarget]; ok {
			isPublished = target.IsPublished()
			if source, ok := wf.Sources[target.Source]; ok && source.Registry != nil {
				sources = append(sources, target.Source)
			}

			if target.CodeSamples != nil && target.CodeSamples.Registry != nil {
				targets = append(targets, specificTarget)
			}
		}
	} else {
		for name, target := range wf.Targets {
			isPublished = isPublished || target.IsPublished()
			if source, ok := wf.Sources[target.Source]; ok && source.Registry != nil {
				sources = append(sources, target.Source)
			}

			if target.CodeSamples != nil && target.CodeSamples.Registry != nil {
				targets = append(targets, name)
			}
		}
	}
	if (len(sources) > 0 || len(targets) > 0) && branch != "" {
		tags := []string{branch}
		if isPublished {
			tags = append(tags, "published")
		}
		return cli.Tag(tags, sources, targets)
	}

	return nil
}

func updateCommitMessages(commitMessages map[string]string, runRes *run.RunResult) {
	if runRes.VersioningInfo.VersionReport != nil {
		reports := runRes.VersioningInfo.VersionReport.Reports
		for _, target := range cli.DefaultSupportedTargetsForChangelog {
			key := fmt.Sprintf("%s_commit_message", strings.ToLower(target))
			commitMessage := releases.FindPRReportByKey(reports, key)
			logging.Debug("lang is: %s, key is: %s, commitMessage is: %s", target, key, commitMessage)
			if commitMessage != "" {
				commitMessages[target] = commitMessage
			}
		}
	}
}

func getReleasesInfo() (*releases.ReleasesInfo, error) {
	releasesDir, err := getReleasesDir()
	if err != nil {
		return nil, err
	}

	releasesInfo, err := releases.GetLastReleaseInfo(releasesDir)
	if err != nil {
		return nil, err
	}

	return releasesInfo, nil
}
