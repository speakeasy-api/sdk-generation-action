package actions

import (
	"errors"
	"os"

	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
)

func Finalize() error {
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
		if !success || environment.GetMode() == environment.ModeDirect {
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

		branchName, pr, err := g.FindExistingPR(branchName)
		if err != nil {
			return err
		}

		releaseInfo, err := getReleasesInfo()
		if err != nil {
			return err
		}

		if err := g.CreateOrUpdatePR(branchName, *releaseInfo, environment.GetPreviousGenVersion(), pr); err != nil {
			return err
		}
	case environment.ModeDirect:
		releaseInfo, err := getReleasesInfo()
		if err != nil {
			return err
		}

		commitHash, err := g.MergeBranch(branchName)
		if err != nil {
			return err
		}

		if environment.CreateGitRelease() {
			if err := g.CreateRelease(*releaseInfo); err != nil {
				return err
			}
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

func getReleasesInfo() (*releases.ReleasesInfo, error) {
	// Find releases file
	langs, err := configuration.GetAndValidateLanguages(false)
	if err != nil {
		return nil, err
	}

	releasesDir := "."
	for _, dir := range langs {
		// If we are only generating one language and its not in the root directory we assume this is a multi-sdk repo
		if len(langs) == 1 && dir != "." {
			releasesDir = dir
		}
	}

	data, err := os.ReadFile(releases.GetReleasesPath(releasesDir))
	if err != nil {
		return nil, err
	}

	releasesInfo, err := releases.ParseReleases(string(data))
	if err != nil {
		return nil, err
	}

	return releasesInfo, nil
}
