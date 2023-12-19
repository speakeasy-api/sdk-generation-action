package document

import (
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

func GetOpenAPIFileInfo() (string, string, error) {
	// TODO OPENAPI_DOC_LOCATION is deprecated and should be removed in the future
	openapiFiles, err := getFiles(environment.GetOpenAPIDocs(), environment.GetOpenAPIDocLocation())
	if err != nil {
		return "", "", err
	}

	if len(openapiFiles) > 1 && !cli.IsAtLeastVersion(cli.MergeVersion) {
		return "", "", fmt.Errorf("multiple openapi files are only supported in speakeasy version %s or higher", cli.MergeVersion.String())
	}

	resolvedOpenAPIFiles, err := resolveFiles(openapiFiles, "openapi")
	if err != nil {
		return "", "", err
	}

	basePath := ""
	filePath := ""

	if len(resolvedOpenAPIFiles) == 1 {
		filePath = resolvedOpenAPIFiles[0]
		basePath = filepath.Dir(filePath)
	} else {
		basePath = filepath.Dir(resolvedOpenAPIFiles[0])
		filePath, err = mergeFiles(resolvedOpenAPIFiles)
		if err != nil {
			return "", "", err
		}
	}

	overlayFiles, err := getFiles(environment.GetOverlayDocs(), "")
	if err != nil {
		return "", "", err
	}

	if len(overlayFiles) > 1 && !cli.IsAtLeastVersion(cli.OverlayVersion) {
		return "", "", fmt.Errorf("overlay files are only supported in speakeasy version %s or higher", cli.OverlayVersion.String())
	}

	resolvedOverlayFiles, err := resolveFiles(overlayFiles, "overlay")
	if err != nil {
		return "", "", err
	}

	if len(resolvedOverlayFiles) > 0 {
		filePath, err = applyOverlay(filePath, resolvedOverlayFiles)
		if err != nil {
			return "", "", err
		}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read openapi file: %w", err)
	}

	doc, err := libopenapi.NewDocumentWithConfiguration(data, &datamodel.DocumentConfiguration{
		AllowRemoteReferences:               true,
		AllowFileReferences:                 true,
		BasePath:                            basePath, // TODO possiblity this is set incorrectly for multiple input files but it is assumed currently any local references are relative to the first file
		IgnorePolymorphicCircularReferences: true,
		IgnoreArrayCircularReferences:       true,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to parse openapi file: %w", err)
	}

	model, errs := doc.BuildV3Model()
	if len(errs) > 0 {
		return "", "", fmt.Errorf("failed to build openapi model: %w", errs[0])
	}
	if model == nil {
		return "", "", fmt.Errorf("failed to build openapi model: model is nil")
	}

	version := "0.0.0"
	if model.Model.Info != nil {
		version = model.Model.Info.Version
	}

	return filePath, version, nil
}

func mergeFiles(files []string) (string, error) {
	outPath := filepath.Join(environment.GetWorkspace(), ".openapi", "openapi_merged")

	if err := os.MkdirAll(filepath.Dir(outPath), os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create openapi directory: %w", err)
	}

	absOutPath, err := filepath.Abs(outPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for openapi file: %w", err)
	}

	if err := cli.MergeDocuments(files, absOutPath); err != nil {
		return "", fmt.Errorf("failed to merge openapi files: %w", err)
	}

	return absOutPath, nil
}

func applyOverlay(filePath string, overlayFiles []string) (string, error) {
	for i, overlayFile := range overlayFiles {
		outPath := filepath.Join(environment.GetWorkspace(), "openapi", fmt.Sprintf("openapi_overlay_%v", i))

		if err := os.MkdirAll(filepath.Dir(outPath), os.ModePerm); err != nil {
			return "", fmt.Errorf("failed to create openapi directory: %w", err)
		}

		outPathAbs, err := filepath.Abs(outPath)
		if err != nil {
			return "", fmt.Errorf("failed to get absolute path for openapi overlay file: %w", err)
		}

		if err := cli.ApplyOverlay(overlayFile, filePath, outPathAbs); err != nil {
			return "", fmt.Errorf("failed to apply overlay: %w", err)
		}

		filePath = outPathAbs
	}

	return filePath, nil
}

func resolveFiles(files []file, typ string) ([]string, error) {
	workspace := environment.GetWorkspace()

	outFiles := []string{}

	for i, file := range files {
		localPath := filepath.Join(workspace, "repo", file.Location)

		if _, err := os.Stat(localPath); err == nil {
			fmt.Printf("Found local %s file: %s\n", typ, localPath)
			absPath, err := filepath.Abs(localPath)
			if err != nil {
				return nil, fmt.Errorf("failed to get absolute path for %s file: %w", localPath, err)
			}

			outFiles = append(outFiles, absPath)
		} else {
			u, err := url.Parse(file.Location)
			if err != nil {
				return nil, fmt.Errorf("failed to parse %s url: %w", typ, err)
			}

			fmt.Printf("Downloading %s file from: %s\n", typ, u.String())

			filePath := filepath.Join(environment.GetWorkspace(), typ, fmt.Sprintf("%s_%d", typ, i))

			if environment.GetAction() == environment.ActionValidate {
				if extension := path.Ext(u.Path); extension != "" {
					filePath = filePath + extension
				}
			}

			if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				return nil, fmt.Errorf("failed to create %s directory: %w", typ, err)
			}

			absPath, err := filepath.Abs(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to get absolute path for %s file: %w", filePath, err)
			}

			if err := download.DownloadFile(u.String(), absPath, file.Header, file.Token); err != nil {
				return nil, fmt.Errorf("failed to download %s file: %w", typ, err)
			}

			outFiles = append(outFiles, absPath)
		}
	}

	return outFiles, nil
}

func getFiles(filesYaml string, defaultFile string) ([]file, error) {
	var fileLocations []string
	if err := yaml.Unmarshal([]byte(filesYaml), &fileLocations); err != nil {
		return nil, fmt.Errorf("failed to parse openapi_docs input: %w", err)
	}

	if len(fileLocations) == 0 && defaultFile != "" {
		fileLocations = append(fileLocations, defaultFile)
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
