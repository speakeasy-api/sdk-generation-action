package actions

import (
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

func getReleasesDir() (string, error) {
	releasesDir := "."
	// For SDK Docs the Release Directory will always be root for now.
	if environment.GetAction() == environment.ActionFinalizeDocs || environment.GetAction() == environment.ActionGenerateDocs {
		return releasesDir, nil
	}

	// Find releases file
	langs, err := configuration.GetAndValidateLanguages(false)
	if err != nil {
		return "", err
	}

	for _, dir := range langs {
		// If we are only generating one language and its not in the root directory we assume this is a multi-sdk repo
		if len(langs) == 1 && dir != "." {
			releasesDir = dir
		}
	}

	return releasesDir, nil
}
