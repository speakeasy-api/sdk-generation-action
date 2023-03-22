package actions

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
)

func Release() error {
	accessToken := environment.GetAccessToken()
	if accessToken == "" {
		return errors.New("github access token is required")
	}

	g, err := initAction()
	if err != nil {
		return err
	}

	files, err := g.GetCommitedFiles()
	if err != nil {
		fmt.Printf("Failed to get commited files: %s\n", err.Error())
	}

	if environment.IsDebugMode() {
		for _, file := range files {
			logging.Debug("Found commited file: %s", file)
		}
	}

	dir := "."

	for _, file := range files {
		if strings.Contains(file, "RELEASES.md") {
			dir = filepath.Dir(file)
			logging.Info("Found RELEASES.md in %s\n", dir)
			break
		}
	}

	latestRelease, err := releases.GetLastReleaseInfo(dir)
	if err != nil {
		return err
	}

	if environment.CreateGitRelease() {
		if err := g.CreateRelease(*latestRelease); err != nil {
			return err
		}
	}

	outputs := map[string]string{}

	for lang, info := range latestRelease.Languages {
		outputs[fmt.Sprintf("%s_regenerated", lang)] = "true"
		outputs[fmt.Sprintf("%s_directory", lang)] = info.Path
	}

	if err := setOutputs(outputs); err != nil {
		return err
	}

	return nil
}
