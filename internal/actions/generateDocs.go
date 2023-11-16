package actions

import (
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/generate"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
)

func GenerateDocs() error {
	g, err := initAction()
	if err != nil {
		return err
	}

	if err := cli.Download(environment.GetPinnedSpeakeasyVersion(), g); err != nil {
		return err
	}

	mode := environment.GetMode()

	branchName := ""

	if mode == environment.ModePR {
		var err error
		branchName, _, err = g.FindExistingPR("", environment.ActionGenerateDocs)
		if err != nil {
			return err
		}
	}

	branchName, err = g.FindOrCreateBranch(branchName, environment.ActionGenerateDocs)
	if err != nil {
		return err
	}

	success := false
	defer func() {
		if !success && !environment.IsDebugMode() {
			if err := g.DeleteBranch(branchName); err != nil {
				logging.Debug("failed to delete branch %s: %v", branchName, err)
			}
		}
	}()

	genInfo, outputs, err := generate.GenerateDocs(g)
	if err != nil {
		if err := setOutputs(outputs); err != nil {
			logging.Debug("failed to set outputs: %v", err)
		}
		return err
	}

	if genInfo != nil {
		docVersion := genInfo.OpenAPIDocVersion
		speakeasyVersion := genInfo.SpeakeasyVersion

		releaseInfo := releases.ReleasesInfo{
			ReleaseTitle:      environment.GetInvokeTime().Format("2006-01-02 15:04:05"),
			DocVersion:        docVersion,
			SpeakeasyVersion:  speakeasyVersion,
			GenerationVersion: genInfo.GenerationVersion,
			DocLocation:       environment.GetOpenAPIDocLocation(),
		}

		releasesDir, err := getReleasesDir()
		if err != nil {
			return err
		}

		if err := releases.UpdateReleasesFile(releaseInfo, releasesDir); err != nil {
			return err
		}

		if _, err := g.CommitAndPush(docVersion, speakeasyVersion, "", environment.ActionGenerate); err != nil {
			return err
		}
	}

	outputs["branch_name"] = branchName

	if err := setOutputs(outputs); err != nil {
		return err
	}

	success = true

	return nil
}
