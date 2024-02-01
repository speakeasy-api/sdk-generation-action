package actions

import (
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
)

func getReleasesDir() (string, error) {
	releasesDir := "."
	// Find releases file
	wf, err := configuration.GetWorkflowAndValidateLanguages(false)
	if err != nil {
		return "", err
	}

	for _, target := range wf.Targets {
		// If we are only generating one language and its not in the root directory we assume this is a multi-sdk repo
		if len(wf.Targets) == 1 && *target.Output != "." {
			releasesDir = *target.Output
		}
	}

	return releasesDir, nil
}
