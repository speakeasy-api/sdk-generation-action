package actions

import (
	"errors"
	"fmt"

	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/internal/suggestions"
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

	branchName, err = g.FindAndCheckoutBranch(branchName)
	if err != nil {
		return err
	}

	if _, err = cli.Download(environment.GetPinnedSpeakeasyVersion(), g); err != nil {
		return err
	}

	if !cli.IsAtLeastVersion(cli.MinimumSupportedCLIVersion) {
		return fmt.Errorf("suggestion action requires at least version %s of the speakeasy CLI", cli.MinimumSupportedCLIVersion)
	}

	branchName, _, err = g.FindExistingPR(branchName, environment.ActionFinalize)
	if err != nil {
		return err
	}

	prNumber, _, err := g.CreateSuggestionPR(branchName, environment.GetOpenAPIDocOutput())
	if err != nil {
		return err
	}

	out := environment.GetCliOutput()
	if out != "" {
		if err = suggestions.WriteSuggestions(g, *prNumber, out); err != nil {
			return err
		}
	}

	success = true

	return nil
}
