package actions

import (
	"errors"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/document"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

func FinalizeSuggestion() error {
	g, err := initAction()
	if err != nil {
		return err
	}

	docPath, _, _, err := document.GetOpenAPIFileInfo()
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

	if _, _, err := g.CreateSuggestionPR(branchName, docPath, environment.GetOpenAPIDocOutput()); err != nil {
		return err
	}

	return nil
}
