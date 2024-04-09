package actions

import (
	"fmt"
	"os"

	"github.com/hashicorp/go-version"
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

	fmt.Println("WE ARE HERE")
	if os.Getenv("INPUT_OPENAPI_DOC_AUTH_TOKEN") != "" {
		fmt.Println("WE ARE HERE1")
	}

	if os.Getenv("OPENAPI_DOC_AUTH_TOKEN") != "" {
		fmt.Println("WE ARE HERE2")
	}

	if os.Getenv("INPUT_openapi_doc_auth_token") != "" {
		fmt.Println("WE ARE HERE3")
	}

	if os.Getenv("openapi_doc_auth_token") != "" {
		fmt.Println("WE ARE HERE4")
	}

	resolvedVersion, err := cli.Download(environment.GetPinnedSpeakeasyVersion(), g)
	if err != nil {
		return err
	}

	minimumVersionForRun := version.Must(version.NewVersion("1.161.0"))
	if !cli.IsAtLeastVersion(minimumVersionForRun) {
		return fmt.Errorf("action requires at least version %s of the speakeasy CLI", minimumVersionForRun)
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
