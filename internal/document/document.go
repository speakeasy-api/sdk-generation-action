package document

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pb33f/libopenapi"
	"github.com/speakeasy-api/sdk-generation-action/internal/download"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

func GetOpenAPIFileInfo(openAPIPath string) (string, string, string, error) {
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

	doc, err := libopenapi.NewDocument(data)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to parse openapi file: %w", err)
	}

	model, errs := doc.BuildV3Model()
	if len(errs) > 0 {
		return "", "", "", fmt.Errorf("failed to build openapi model: %w", errs[0])
	}
	if model == nil {
		return "", "", "", fmt.Errorf("failed to build openapi model: model is nil")
	}

	hash := md5.Sum(data)
	checksum := hex.EncodeToString(hash[:])
	version := "0.0.0"
	if model.Model.Info != nil {
		version = model.Model.Info.Version
	}

	return filePath, checksum, version, nil
}
