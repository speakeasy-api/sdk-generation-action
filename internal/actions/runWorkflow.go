package actions

import (
	"fmt"

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

	// The top-level CLI can always use latest. The CLI itself manages pinned versions.
	resolvedVersion, err := cli.Download("latest", g)
	if err != nil {
		return err
	}

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
	if mode == environment.ModePR {
		var err error
		branchName, _, err = g.FindExistingPR("", environment.ActionRunWorkflow, sourcesOnly)
		if err != nil {
			return err
		}
	}

	branchName, err = g.FindOrCreateBranch(branchName, environment.ActionRunWorkflow)
	if err != nil {
		return err
	}

	success := false
	defer func() {
		if (!success || environment.GetMode() == environment.ModeDirect) && !environment.IsDebugMode() {
			if err := g.DeleteBranch(branchName); err != nil {
				logging.Debug("failed to delete branch %s: %v", branchName, err)
			}
		}
	}()

	runRes, outputs, err := run.Run(g, wf)
	if err != nil {
		if err := setOutputs(outputs); err != nil {
			logging.Debug("failed to set outputs: %v", err)
		}
		return err
	}
	outputs["resolved_speakeasy_version"] = resolvedVersion

	anythingRegenerated := false

	if runRes.GenInfo != nil {
		docVersion := runRes.GenInfo.OpenAPIDocVersion
		speakeasyVersion := runRes.GenInfo.SpeakeasyVersion

		releaseInfo := releases.ReleasesInfo{
			ReleaseTitle:       environment.GetInvokeTime().Format("2006-01-02 15:04:05"),
			DocVersion:         docVersion,
			SpeakeasyVersion:   speakeasyVersion,
			GenerationVersion:  runRes.GenInfo.GenerationVersion,
			DocLocation:        environment.GetOpenAPIDocLocation(),
			Languages:          map[string]releases.LanguageReleaseInfo{},
			LanguagesGenerated: map[string]releases.GenerationInfo{},
		}

		supportedLanguages, err := cli.GetSupportedLanguages()
		if err != nil {
			return err
		}

		for _, lang := range supportedLanguages {
			langGenInfo, ok := runRes.GenInfo.Languages[lang]

			if ok && outputs[fmt.Sprintf("%s_regenerated", lang)] == "true" {
				anythingRegenerated = true

				path := outputs[fmt.Sprintf("%s_directory", lang)]

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

		releasesDir, err := getReleasesDir()
		if err != nil {
			return err
		}

		if err := releases.UpdateReleasesFile(releaseInfo, releasesDir); err != nil {
			return err
		}

		if _, err := g.CommitAndPush(docVersion, speakeasyVersion, "", environment.ActionRunWorkflow, false); err != nil {
			return err
		}
	}

	if sourcesOnly {
		if _, err := g.CommitAndPush("", resolvedVersion, "", environment.ActionRunWorkflow, sourcesOnly); err != nil {
			return err
		}
	}

	if err := finalize(finalizeInputs{
		Outputs:              outputs,
		BranchName:           branchName,
		AnythingRegenerated:  anythingRegenerated,
		SourcesOnly:          sourcesOnly,
		Git:                  g,
		LintingReportURL:     runRes.LintingReportURL,
		ChangesReportURL:     runRes.ChangesReportURL,
		OpenAPIChangeSummary: runRes.OpenAPIChangeSummary,
	}); err != nil {
		return err
	}

	success = true

	return nil
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

		var releaseInfo *releases.ReleasesInfo
		if !inputs.SourcesOnly {
			releaseInfo, err = getReleasesInfo()
			if err != nil {
				return err
			}
		}

		if err := inputs.Git.CreateOrUpdatePR(git.PRInfo{
			BranchName:           branchName,
			ReleaseInfo:          releaseInfo,
			PreviousGenVersion:   inputs.Outputs["previous_gen_version"],
			PR:                   pr,
			SourceGeneration:     inputs.SourcesOnly,
			LintingReportURL:     inputs.LintingReportURL,
			ChangesReportURL:     inputs.ChangesReportURL,
			OpenAPIChangeSummary: inputs.OpenAPIChangeSummary,
		}); err != nil {
			return err
		}
	case environment.ModeDirect:
		var releaseInfo *releases.ReleasesInfo
		if !inputs.SourcesOnly {
			releaseInfo, err = getReleasesInfo()
			if err != nil {
				return err
			}
		}

		commitHash, err := inputs.Git.MergeBranch(branchName)
		if err != nil {
			return err
		}

		if !inputs.SourcesOnly && environment.CreateGitRelease() {
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
