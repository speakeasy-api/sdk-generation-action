package actions

import (
	"fmt"

	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/document"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/internal/suggestions"
)

func Suggest() error {
	g, err := initAction()
	if err != nil {
		return err
	}

	_, err = cli.Download(environment.GetPinnedSpeakeasyVersion(), g)
	if err != nil {
		return err
	}

	if !cli.IsAtLeastVersion(cli.MinimumSupportedCLIVersion) {
		return fmt.Errorf("suggestion action requires at least version %s of the speakeasy CLI", cli.MinimumSupportedCLIVersion)
	}

	docPath, _, err := document.GetOpenAPIFileInfo()
	if err != nil {
		return err
	}

	outputs := make(map[string]string)

	branchName := ""

	branchName, _, err = g.FindExistingPR("", environment.ActionSuggest, false)
	if err != nil {
		return err
	}

	branchName, err = g.FindOrCreateBranch(branchName, environment.ActionSuggest)
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

	out, err := suggestions.Suggest(docPath, environment.GetMaxSuggestions())
	if err != nil {
		return err
	}

	outputs["cli_output"] = out

	if _, err := g.CommitAndPush("", "", environment.GetOpenAPIDocOutput(), environment.ActionSuggest, false, nil, nil, nil); err != nil {
		return err
	}

	outputs["branch_name"] = branchName

	if err := setOutputs(outputs); err != nil {
		return err
	}

	success = true

	return nil
}
