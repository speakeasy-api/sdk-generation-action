package actions

import (
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

func getReleasesDir() (string, error) {
	releasesDir := "."
	// Find releases file
	wf, err := configuration.GetWorkflowAndValidateLanguages(false)
	if err != nil {
		return "", err
	}

	// Checking for multiple targets ensures backward compatibility with the code below
	if len(wf.Targets) > 1 && environment.SpecifiedTarget() != "" && environment.SpecifiedTarget() != "all" {
		if target, ok := wf.Targets[environment.SpecifiedTarget()]; ok && target.Output != nil {
			return *target.Output, nil
		}
	}

	for _, target := range wf.Targets {
		// If we are only generating one language and its not in the root directory we assume this is a multi-sdk repo
		if len(wf.Targets) == 1 && target.Output != nil && *target.Output != "." {
			releasesDir = *target.Output
		}
	}

	return releasesDir, nil
}
