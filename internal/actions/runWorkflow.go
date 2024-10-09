package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v63/github"
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

	setVersion := environment.SetVersion()
	if setVersion != "" {
		tagName := setVersion
		if !strings.HasPrefix(tagName, "v") {
			tagName = "v" + tagName
		}
		if release, _, err := g.GetReleaseByTag(context.Background(), tagName); err == nil && release != nil {
			logging.Debug("cannot manually set a version: %s that has already been released", setVersion)
			return fmt.Errorf("cannot manually set a version: %s that has already been released", setVersion)
		}
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
		VersioningReport:     runRes.VersioningReport,
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

		if err := inputs.Git.CreateOrUpdatePR(git.PRInfo{
			BranchName:           branchName,
			ReleaseInfo:          inputs.currentRelease,
			PreviousGenVersion:   inputs.Outputs["previous_gen_version"],
			PR:                   pr,
			SourceGeneration:     inputs.SourcesOnly,
			LintingReportURL:     inputs.LintingReportURL,
			ChangesReportURL:     inputs.ChangesReportURL,
			VersioningReport:     inputs.VersioningReport,
			OpenAPIChangeSummary: inputs.OpenAPIChangeSummary,
		}); err != nil {
			return err
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
