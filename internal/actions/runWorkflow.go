package actions

import (
	"fmt"
	"github.com/hashicorp/go-version"
	"github.com/speakeasy-api/sdk-gen-config/workflow"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/git"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/internal/run"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
	"gopkg.in/yaml.v3"
	"os"
)

func RunWorkflow() error {
	g, err := initAction()
	if err != nil {
		return err
	}

	wf, workflowPath, err := configuration.GetWorkflowAndValidateLanguages(true)
	if err != nil {
		return err
	}

	// Override workflow.yaml speakeasyVersion with GH action speakeasyVersion
	if v := cli.GetVersion(environment.GetPinnedSpeakeasyVersion()); v != "latest" {
		resolvedVersion, err := cli.Download(v, g)
		if err != nil {
			return err
		}

		return runWorkflow(g, resolvedVersion, wf)
	}

	pinnedVersion := wf.SpeakeasyVersion.String()
	firstRunVersion := pinnedVersion
	if environment.ShouldAutoUpgradeSpeakeasyVersion() {
		firstRunVersion = "latest"
	}

	resolvedVersion, err := cli.Download(firstRunVersion, g)
	if err != nil {
		return err
	}

	// We will only bother doing the second run if it will be with a different version than the first
	attemptingAutoUpgrade := pinnedVersion != "latest" && pinnedVersion != resolvedVersion
	if attemptingAutoUpgrade {
		logging.Info("Attempting auto-upgrade from Speakeasy version %s to %s", pinnedVersion, resolvedVersion)
	}

	err = runWorkflow(g, resolvedVersion, wf)
	if attemptingAutoUpgrade {
		// If we tried to upgrade and the run succeeded, update the workflow file with the new version
		if err == nil {
			logging.Info("Successfully ran workflow with updated version %s", resolvedVersion)
			if err := updateSpeakeasyVersion(wf, resolvedVersion, workflowPath); err != nil {
				return err
			}

			_, err = g.CommitAndPushWithMessage(fmt.Sprintf("ci: update speakeasyVersion to %s", resolvedVersion))
			return err
		} else {
			logging.Info("Error running workflow with version %s: %v", firstRunVersion, err)
			logging.Info("Trying again with pinned version %s", pinnedVersion)

			resolvedVersion, err := cli.Download(firstRunVersion, g)
			if err != nil {
				return err
			}

			return runWorkflow(g, resolvedVersion, wf)
		}
	}

	return err
}

func runWorkflow(g *git.Git, resolvedVersion string, wf *workflow.Workflow) error {
	minimumVersionForRun := version.Must(version.NewVersion("1.161.0"))
	if !cli.IsAtLeastVersion(minimumVersionForRun) {
		return fmt.Errorf("action requires at least version %s of the speakeasy CLI", minimumVersionForRun)
	}

	mode := environment.GetMode()

	sourcesOnly := wf.Targets == nil || len(wf.Targets) == 0

	branchName := ""
	if mode == environment.ModePR {
		var err error
		branchName, _, err = g.FindExistingPR("", environment.ActionRunWorkflow, sourcesOnly)
		if err != nil {
			return err
		}
	}

	branchName, err := g.FindOrCreateBranch(branchName, environment.ActionRunWorkflow)
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

	genInfo, outputs, err := run.Run(g, wf)
	if err != nil {
		if err := setOutputs(outputs); err != nil {
			logging.Debug("failed to set outputs: %v", err)
		}
		return err
	}
	outputs["resolved_speakeasy_version"] = resolvedVersion

	anythingRegenerated := false

	if genInfo != nil {
		docVersion := genInfo.OpenAPIDocVersion
		speakeasyVersion := genInfo.SpeakeasyVersion

		releaseInfo := releases.ReleasesInfo{
			ReleaseTitle:       environment.GetInvokeTime().Format("2006-01-02 15:04:05"),
			DocVersion:         docVersion,
			SpeakeasyVersion:   speakeasyVersion,
			GenerationVersion:  genInfo.GenerationVersion,
			DocLocation:        environment.GetOpenAPIDocLocation(),
			Languages:          map[string]releases.LanguageReleaseInfo{},
			LanguagesGenerated: map[string]releases.GenerationInfo{},
		}

		supportedLanguages, err := cli.GetSupportedLanguages()
		if err != nil {
			return err
		}

		for _, lang := range supportedLanguages {
			langGenInfo, ok := genInfo.Languages[lang]

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

	if err = finalize(outputs, branchName, anythingRegenerated, sourcesOnly, g); err != nil {
		return err
	}

	success = true

	return nil
}

// Sets outputs and creates or adds releases info
func finalize(outputs map[string]string, branchName string, anythingRegenerated bool, sourcesOnly bool, g *git.Git) error {
	// If nothing was regenerated, we don't need to do anything
	if !anythingRegenerated && !sourcesOnly {
		return nil
	}

	branchName, err := g.FindAndCheckoutBranch(branchName)
	if err != nil {
		return err
	}

	defer func() {
		outputs["branch_name"] = branchName

		if err := setOutputs(outputs); err != nil {
			logging.Debug("failed to set outputs: %v", err)
		}
	}()

	switch environment.GetMode() {
	case environment.ModePR:
		branchName, pr, err := g.FindExistingPR(branchName, environment.ActionFinalize, sourcesOnly)
		if err != nil {
			return err
		}

		var releaseInfo *releases.ReleasesInfo
		if !sourcesOnly {
			releaseInfo, err = getReleasesInfo()
			if err != nil {
				return err
			}
		}

		if err := g.CreateOrUpdatePR(branchName, releaseInfo, environment.GetPreviousGenVersion(), pr, sourcesOnly); err != nil {
			return err
		}
	case environment.ModeDirect:
		var releaseInfo *releases.ReleasesInfo
		if !sourcesOnly {
			releaseInfo, err = getReleasesInfo()
			if err != nil {
				return err
			}
		}

		commitHash, err := g.MergeBranch(branchName)
		if err != nil {
			return err
		}

		if !sourcesOnly && environment.CreateGitRelease() {
			if err := g.CreateRelease(*releaseInfo); err != nil {
				return err
			}
		}

		outputs["commit_hash"] = commitHash
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

func updateSpeakeasyVersion(wf *workflow.Workflow, newVersion, workflowFilepath string) error {
	wf.SpeakeasyVersion = workflow.Version(newVersion)

	f, err := os.OpenFile(workflowFilepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("error opening workflow file: %w", err)
	}
	defer f.Close()

	out, err := yaml.Marshal(wf)
	if err != nil {
		return fmt.Errorf("error marshalling workflow file: %w", err)
	}

	_, err = f.Write(out)
	if err != nil {
		return fmt.Errorf("error writing to workflow file: %w", err)
	}

	return nil
}
