package actions

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/run"

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

	dir := "."
	usingReleasesMd := false
	// TODO: Maybe instead check for workflow dispatch event
	providesExplicitTarget := environment.SpecifiedTarget() != ""
	if providesExplicitTarget {
		workflow, err := configuration.GetWorkflowAndValidateLanguages(true)
		if err != nil {
			return err
		}
		if target, ok := workflow.Targets[environment.SpecifiedTarget()]; ok && target.Output != nil {
			dir = strings.TrimPrefix(*target.Output, "./")
		}
		dir = filepath.Join(environment.GetWorkingDirectory(), dir)
		fmt.Println("THIS PATH")
		fmt.Println(dir)
	}

	if !providesExplicitTarget {
		// This searches for files that would be referenced in the GH Action trigger
		files, err := g.GetCommitedFiles()
		if err != nil {
			fmt.Printf("Failed to get commited files: %s\n", err.Error())
		}

		if environment.IsDebugMode() {
			for _, file := range files {
				logging.Debug("Found commited file: %s", file)
			}
		}

		fmt.Println("THESE FILES")
		fmt.Println(files)
		for _, file := range files {
			// Maintain Support for RELEASES.MD for backward compatibility with existing publishing actions
			if strings.Contains(file, "RELEASES.md") {
				dir = filepath.Dir(file)
				logging.Info("Found RELEASES.md in %s\n", dir)
				usingReleasesMd = true
				break
			}

			if strings.Contains(file, "gen.lock") {
				dir = filepath.Dir(file)
				dir = filepath.Dir(strings.ReplaceAll(dir, "/.speakeasy", ""))
				dir = filepath.Dir(strings.ReplaceAll(dir, ".speakeasy", ""))
				logging.Info("Found gen.lock in %s\n", dir)
				break
			}
		}
	}

	var latestRelease *releases.ReleasesInfo
	if usingReleasesMd {
		fmt.Println("USING RELEASES.MD")
		latestRelease, err = releases.GetLastReleaseInfo(dir)
		if err != nil {
			return err
		}
	} else {
		latestRelease, err = releases.GetReleaseInfoFromGenerationFiles(dir)
		if err != nil {
			return err
		}
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
