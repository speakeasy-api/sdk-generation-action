package actions

import (
	"errors"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
)

func FinalizeSuggestion() error {
	g, err := initAction()
	if err != nil {
		return err
	}

	branchName := environment.GetBranchName()
	if branchName == "" {
		return errors.New("branch name is required")
	}

	success := false

	defer func() {
		if !success {
			if err := g.DeleteBranch(branchName); err != nil {
				logging.Debug("failed to delete branch %s: %v", branchName, err)
			}
		}
	}()

	branchName, err = g.FindBranch(branchName)
	if err != nil {
		return err
	}

	if err = cli.Download(environment.GetPinnedSpeakeasyVersion(), g); err != nil {
		return err
	}

	branchName, _, err = g.FindExistingPR(branchName, environment.ActionFinalize)
	if err != nil {
		return err
	}

	_, _, err = g.CreateSuggestionPR(branchName, environment.GetOpenAPIDocOutput())
	if err != nil {
		return err
	}

	//out := environment.GetCliOutput()
	//if out != "" {
	//	if err = suggestions.WriteSuggestions(g, prNumber, out); err != nil {
	//		return err
	//	}
	//}

	success = true

	return nil
}
