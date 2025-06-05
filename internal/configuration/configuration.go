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
		if err := AssertTargetNamesSupported(langs); err != nil {
			return nil, err
		}
	}

	return wf, nil
}

func getWorkflow() (*workflow.Workflow, error) {
	workspace := environment.GetWorkspace()

	localPath := filepath.Join(workspace, "repo", environment.GetWorkingDirectory())

	wf, _, err := workflow.Load(localPath)
	if err != nil {
		return nil, err
	}

	return wf, err
}

func AssertTargetNamesSupported(workflowTargetNames []string) error {
	supportedTargetNames := cli.GetSupportedTargetNames()
	for _, workflowTargetName := range workflowTargetNames {
		if !slices.Contains(supportedTargetNames, workflowTargetName) {
			return fmt.Errorf("unsupported target: %s", workflowTargetName)
		}
	}

	return nil
}
