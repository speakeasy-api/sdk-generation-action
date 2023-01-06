package generate

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/invopop/yaml"
	"github.com/speakeasy-api/sdk-generation-action/internal/download"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

func getOpenAPIFileInfo(openAPIPath string) (string, string, string, error) {
	var filePath string

	baseDir := environment.GetBaseDir()

	localPath := filepath.Join(baseDir, "repo", openAPIPath)

	if _, err := os.Stat(localPath); err == nil {
		fmt.Println("Using local OpenAPI file: ", localPath)

		filePath = localPath
	} else {
		u, err := url.Parse(openAPIPath)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to parse openapi url: %w", err)
		}

		fmt.Println("Downloading openapi file from: ", u.String())

		filePath, err = download.DownloadFile(u.String(), "openapi", environment.GetOpenAPIDocAuthHeader(), environment.GetOpenAPIDocAuthToken())
		if err != nil {
			return "", "", "", fmt.Errorf("failed to download openapi file: %w", err)
		}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read openapi file: %w", err)
	}

	var doc struct {
		Info struct {
			Version string
		}
	}

	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", "", "", fmt.Errorf("failed to parse openapi file: %w", err)
	}

	hash := md5.Sum(data)
	checksum := hex.EncodeToString(hash[:])

	return filePath, checksum, doc.Info.Version, nil
}
