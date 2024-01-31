package configuration

import (
	"fmt"
	"github.com/speakeasy-api/sdk-gen-config/workflow"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"golang.org/x/exp/slices"
	"path/filepath"
)

func GetWorkflowAndValidateLanguages(checkLangSupported bool) (*workflow.Workflow, []string, error) {
	wf, err := getWorkflow()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load workflow file: %w", err)
	}

	var langs []string
	for _, target := range wf.Targets {
		langs = append(langs, target.Target)
	}

	if checkLangSupported {
		if err := AssertLangsSupported(langs); err != nil {
			return nil, nil, err
		}
	}

	return wf, langs, nil
}

func getWorkflow() (*workflow.Workflow, error) {
	workspace := environment.GetWorkspace()

	localPath := filepath.Join(workspace, "repo")

	wf, _, err := workflow.Load(localPath)
	if err != nil {
		return nil, err
	}

	return wf, err
}

func AssertLangsSupported(langs []string) error {
	supportedLangs, err := cli.GetSupportedLanguages()
	if err != nil {
		return fmt.Errorf("failed to get supported languages: %w", err)
	}

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
