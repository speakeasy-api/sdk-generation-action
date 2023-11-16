package actions

import (
	"errors"

	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
)

func FinalizeDocs() error {
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

	switch environment.GetMode() {
	case environment.ModePR:
		if err := cli.Download(environment.GetPinnedSpeakeasyVersion(), g); err != nil {
			return err
		}

		branchName, pr, err := g.FindExistingPR(branchName, environment.ActionFinalizeDocs)
		if err != nil {
			return err
		}

		releaseInfo, err := getReleasesInfo()
		if err != nil {
			return err
		}

		if err := g.CreateOrUpdateDocsPR(branchName, *releaseInfo, environment.GetPreviousGenVersion(), pr); err != nil {
			return err
		}
	case environment.ModeDirect:
		commitHash, err := g.MergeBranch(branchName)
		if err != nil {
			return err
		}

		outputs := map[string]string{
			"commit_hash": commitHash,
		}

		if err := setOutputs(outputs); err != nil {
			return err
		}
	}

	success = true

	return nil
}
