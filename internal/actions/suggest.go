package actions

import (
	"fmt"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/internal/suggestions"
)

func Suggest() error {
	if !cli.IsAtLeastVersion(cli.LLMSuggestionVersion) {
		return fmt.Errorf("suggestion action requires at least version %s of the speakeasy CLI", cli.LLMSuggestionVersion)
	}

	g, err := initAction()
	if err != nil {
		return err
	}

	var outputs map[string]string

	if err := cli.Download(environment.GetPinnedSpeakeasyVersion(), g); err != nil {
		return err
	}

	branchName := ""

	branchName, _, err = g.FindExistingPR("", environment.ActionSuggest)
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

	out, err := suggestions.Suggest()
	if err != nil {
		return err
	}

	outputs["cli_output"] = out

	if _, err := g.CommitAndPush("", "", environment.GetOpenAPIDocOutput(), environment.ActionSuggest); err != nil {
		return err
	}

	outputs["branch_name"] = branchName

	if err := setOutputs(outputs); err != nil {
		return err
	}

	success = true

	return nil
}
