package actions

import (
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
)

func getReleasesDir() (string, error) {
	releasesDir := "."
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
