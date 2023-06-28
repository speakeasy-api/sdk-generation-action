package document

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel"
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

	basePath := ""
	filePath := ""

	if len(filePaths) == 1 {
		filePath = filePaths[0]
		basePath = filepath.Dir(filePath)
	} else {
		basePath = filepath.Dir(filePaths[0])
		filePath, err = mergeFiles(filePaths)
		if err != nil {
			return "", "", "", err
		}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read openapi file: %w", err)
	}

	doc, err := libopenapi.NewDocumentWithConfiguration(data, &datamodel.DocumentConfiguration{
		AllowRemoteReferences: true,
		AllowFileReferences:   true,
		BasePath:              basePath, // TODO possiblity this is set incorrectly for multiple input files but it is assumed currently any local references are relative to the first file
	})
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
	outPath := filepath.Join(environment.GetWorkspace(), "openapi", "openapi_merged")

	if err := os.MkdirAll(filepath.Dir(outPath), os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create openapi directory: %w", err)
	}

	if err := cli.MergeDocuments(files, outPath); err != nil {
		return "", fmt.Errorf("failed to merge openapi files: %w", err)
	}

	return outPath, nil
}

func resolveFiles(files []file) ([]string, error) {
	workspace := environment.GetWorkspace()

	outFiles := []string{}

	for i, file := range files {
		localPath := filepath.Join(workspace, "repo", file.Location)

		if _, err := os.Stat(localPath); err == nil {
			fmt.Println("Found local OpenAPI file: ", localPath)

			outFiles = append(outFiles, localPath)
		} else {
			u, err := url.Parse(file.Location)
			if err != nil {
				return nil, fmt.Errorf("failed to parse openapi url: %w", err)
			}

			fmt.Println("Downloading openapi file from: ", u.String())

			filePath := filepath.Join(environment.GetWorkspace(), "openapi", fmt.Sprintf("openapi_%d", i))

			if environment.GetAction() == environment.ActionValidate {
				if extension := path.Ext(u.Path); extension != "" {
					filePath = filePath + extension
				}
			}

			if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				return nil, fmt.Errorf("failed to create openapi directory: %w", err)
			}

			if err := download.DownloadFile(u.String(), filePath, file.Header, file.Token); err != nil {
				return nil, fmt.Errorf("failed to download openapi file: %w", err)
			}

			outFiles = append(outFiles, filePath)
		}
	}

	return outFiles, nil
}

func getFiles() ([]file, error) {
	docsYaml := environment.GetOpenAPIDocs()

	var fileLocations []string
	if err := yaml.Unmarshal([]byte(docsYaml), &fileLocations); err != nil {
		return nil, fmt.Errorf("failed to parse openapi_docs input: %w", err)
	}

	// TODO OPENAPI_DOC_LOCATION is deprecated and should be removed in the future
	if len(fileLocations) == 0 {
		fileLocations = append(fileLocations, environment.GetOpenAPIDocLocation())
	}

	files := []file{}

	for _, fileLoc := range fileLocations {
		files = append(files, file{
			Location: fileLoc,
			Header:   environment.GetOpenAPIDocAuthHeader(),
			Token:    environment.GetOpenAPIDocAuthToken(),
		})
	}

	return files, nil
}
