package actions

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/v63/github"
	"github.com/pkg/errors"
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

		supportedLanguages := cli.GetSupportedLanguages()
		for _, lang := range supportedLanguages {
			langGenInfo, ok := runRes.GenInfo.Languages[lang]

			if ok && outputs[fmt.Sprintf("%s_regenerated", lang)] == "true" {
				anythingRegenerated = true

				path := outputs[fmt.Sprintf("%s_directory", lang)]
				path = strings.TrimPrefix(path, "./")

				releaseInfo.LanguagesGenerated[lang] = releases.GenerationInfo{
					Version: langGenInfo.Version,
					Path:    path,
				}

				if published, ok := outputs[fmt.Sprintf("publish_%s", lang)]; ok && published == "true" {
					releaseInfo.Languages[lang] = releases.LanguageReleaseInfo{
						PackageName: langGenInfo.PackageName,
						Version:     langGenInfo.Version,
						Path:        path,
					}
				}
			}
		}

		if environment.PushCodeSamplesOnly() {
			// If we're just pushing code samples we don't want to raise a PR
			return nil
		}

		releasesDir, err := getReleasesDir()
		if err != nil {
			return err
		}

		if err := releases.UpdateReleasesFile(releaseInfo, releasesDir); err != nil {
			return err
		}

		if _, err := g.CommitAndPush(docVersion, resolvedVersion, "", environment.ActionRunWorkflow, false); err != nil {
			return err
		}
	}

	outputs["resolved_speakeasy_version"] = resolvedVersion

	if sourcesOnly {
		if _, err := g.CommitAndPush("", resolvedVersion, "", environment.ActionRunWorkflow, sourcesOnly); err != nil {
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
		currentRelease:       &releaseInfo,
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
	currentRelease       *releases.ReleasesInfo
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
		})

		if err != nil {
			return err
		}

		if pr != nil {
			os.Setenv("GH_PULL_REQUEST", *pr.URL)
		}

	case environment.ModeDirect:
		var releaseInfo *releases.ReleasesInfo
		if !inputs.SourcesOnly {
			releaseInfo = inputs.currentRelease
			// We still read from releases info for terraform generations since they use the goreleaser
			if inputs.Outputs["terraform_regenerated"] == "true" {
				releaseInfo, err = getReleasesInfo()
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
			if err := inputs.Git.CreateRelease(*releaseInfo, inputs.Outputs); err != nil {
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
		return cli.Tag([]string{branch}, sources, targets)
	}

	return nil
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
