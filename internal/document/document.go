package document

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pb33f/libopenapi"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/download"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"gopkg.in/yaml.v3"
)

type file struct {
	Location string `yaml:"location"`
	Header   string `yaml:"auth_header"`
	Token    string `yaml:"auth_token"`
}

func GetOpenAPIFileInfo() (string, string, string, error) {
	files, err := getFiles()
	if err != nil {
		return "", "", "", err
	}

	if len(files) > 1 && !cli.IsAtLeastVersion(cli.MergeVersion) {
		return "", "", "", fmt.Errorf("multiple openapi files are only supported in speakeasy version %s or higher", cli.MergeVersion.String())
	}

	filePaths, err := resolveFiles(files)
	if err != nil {
		return "", "", "", err
	}

	filePath := ""

	if len(filePaths) == 1 {
		filePath = filePaths[0]
	} else {
		filePath, err = mergeFiles(filePaths)
		if err != nil {
			return "", "", "", err
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

func mergeFiles(files []string) (string, error) {
	outPath := filepath.Join(os.TempDir(), "openapi_merged")

	if err := cli.MergeDocuments(files, outPath); err != nil {
		return "", fmt.Errorf("failed to merge openapi files: %w", err)
	}

	return outPath, nil
}

func resolveFiles(files []file) ([]string, error) {
	baseDir := environment.GetBaseDir()

	outFiles := []string{}

	for i, file := range files {
		localPath := filepath.Join(baseDir, "repo", file.Location)

		if _, err := os.Stat(localPath); err == nil {
			fmt.Println("Found local OpenAPI file: ", localPath)

			outFiles = append(outFiles, localPath)
		} else {
			u, err := url.Parse(file.Location)
			if err != nil {
				return nil, fmt.Errorf("failed to parse openapi url: %w", err)
			}

			fmt.Println("Downloading openapi file from: ", u.String())

			filePath, err := download.DownloadFile(u.String(), fmt.Sprintf("openapi_%d", i), file.Header, file.Token)
			if err != nil {
				return nil, fmt.Errorf("failed to download openapi file: %w", err)
			}

			outFiles = append(outFiles, filePath)
		}
	}

	return outFiles, nil
}

func getFiles() ([]file, error) {
	docsYaml := environment.GetOpenAPIDocs()

	var files []file
	if err := yaml.Unmarshal([]byte(docsYaml), &files); err != nil {
		return nil, fmt.Errorf("failed to parse openapi_docs input: %w", err)
	}

	if len(files) > 0 {
		return files, nil
	}

	// TODO below inputs are deprecated and should be removed in the future
	fileLoc := environment.GetOpenAPIDocLocation()
	if fileLoc == "" {
		return nil, fmt.Errorf("no openapi files found")
	}

	return []file{
		{
			Location: fileLoc,
			Header:   environment.GetOpenAPIDocAuthHeader(),
			Token:    environment.GetOpenAPIDocAuthToken(),
		},
	}, nil
}
