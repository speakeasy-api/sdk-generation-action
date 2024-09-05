package actions

import (
	"errors"
	"fmt"
	"github.com/speakeasy-api/sdk-generation-action/internal/run"
	"path/filepath"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"

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

	outputs := map[string]string{}

	for lang, info := range latestRelease.Languages {
		outputs[fmt.Sprintf("%s_regenerated", lang)] = "true"
		outputs[fmt.Sprintf("%s_directory", lang)] = info.Path
	}

	if err = addPublishOutputs(dir, outputs); err != nil {
		return err
	}

	if environment.CreateGitRelease() {
		if err := g.CreateRelease(*latestRelease, outputs); err != nil {
			return err
		}
	}

	if err = setOutputs(outputs); err != nil {
		return err
	}

	return nil
}

func addPublishOutputs(dir string, outputs map[string]string) error {
	wf, err := configuration.GetWorkflowAndValidateLanguages(false)
	if err != nil {
		return err
	}

	for _, target := range wf.Targets {
		// Only add outputs for the target that was regenerated, based on output directory
		if dir != "." && target.Output != nil {
			output, err := filepath.Rel(".", *target.Output)
			if err != nil {
				return err
			}

			if environment.GetWorkingDirectory() != "" {
				output = filepath.Join(environment.GetWorkingDirectory(), output)
			}

			if output != dir {
				continue
			}
		}

		run.AddTargetPublishOutputs(target, outputs, nil)
	}

	return nil
}
