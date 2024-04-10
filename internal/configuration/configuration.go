package configuration

import (
	"fmt"
	"github.com/speakeasy-api/sdk-gen-config/workflow"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"golang.org/x/exp/slices"
	"path/filepath"
)

func GetWorkflowAndValidateLanguages(checkLangSupported bool) (*workflow.Workflow, string, error) {
	wf, path, err := getWorkflow()
	if err != nil {
		return nil, path, fmt.Errorf("failed to load workflow file: %w", err)
	}

	var langs []string
	for _, target := range wf.Targets {
		langs = append(langs, target.Target)
	}

	if checkLangSupported {
		if err := AssertLangsSupported(langs); err != nil {
			return nil, path, err
		}
	}

	return wf, path, nil
}

func getWorkflow() (*workflow.Workflow, string, error) {
	workspace := environment.GetWorkspace()

	localPath := filepath.Join(workspace, "repo")

	return workflow.Load(localPath)
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
