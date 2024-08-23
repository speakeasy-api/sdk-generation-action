package configuration

import (
	"fmt"
	"path/filepath"

	"github.com/speakeasy-api/sdk-gen-config/workflow"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"golang.org/x/exp/slices"
)

func GetWorkflowAndValidateLanguages(checkLangSupported bool) (*workflow.Workflow, error) {
	wf, err := getWorkflow()
	if err != nil {
		return nil, fmt.Errorf("failed to load workflow file: %w", err)
	}

	var langs []string
	for _, target := range wf.Targets {
		langs = append(langs, target.Target)
	}

	if checkLangSupported {
		if err := AssertLangsSupported(langs); err != nil {
			return nil, err
		}
	}

	return wf, nil
}

func getWorkflow() (*workflow.Workflow, error) {
	workspace := environment.GetWorkspace()
	if environment.GetWorkingDirectory() != "" {
		workspace = filepath.Join(workspace, environment.GetWorkingDirectory())
	}

	localPath := filepath.Join(workspace, "repo")

	wf, _, err := workflow.Load(localPath)
	if err != nil {
		return nil, err
	}

	return wf, err
}

func AssertLangsSupported(langs []string) error {
	supportedLangs := cli.GetSupportedLanguages()
	for _, l := range langs {
		if l == "docs" {
			return nil
		}

		if !slices.Contains(supportedLangs, l) {
			return fmt.Errorf("unsupported language: %s", l)
		}
	}

	return nil
}
