package actions

import (
	"fmt"
	"github.com/speakeasy-api/sdk-generation-action/internal/suggestions"
	"io"
	"os"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/document"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
)

func Validate() error {
	g, err := initAction()
	if err != nil {
		return err
	}

	if err := cli.Download(environment.GetPinnedSpeakeasyVersion(), g); err != nil {
		return err
	}

	docPath, _, _, err := document.GetOpenAPIFileInfo()
	if err != nil {
		return err
	}
	docPathPrefix := environment.GetWorkspace()
	if !strings.HasSuffix(docPathPrefix, "/") {
		docPathPrefix += "/"
	}
	if err := setOutputs(map[string]string{
		"openapi_doc": strings.TrimPrefix(docPath, docPathPrefix),
	}); err != nil {
		logging.Debug("failed to set outputs: %v", err)
	}

	isLocalFile := false
	if _, err := os.Stat(docPath); err == nil {
		isLocalFile = true
	}

	// We will write suggestions to a new PR if the input flag is set and we are dealing with a local OpenAPI file
	if os.Getenv("INPUT_WRITE_SUGGESTIONS") == "true" && cli.IsAtLeastVersion(cli.LLMSuggestionVersion) && isLocalFile {
		out, suggestionErr := cli.Suggest(docPath)
		output := suggestions.ParseOutput(out)
		if len(output) > 0 {
			// creates a branch for our suggestion PR
			branch, err := g.CreateSuggestionBranch()
			if err != nil {
				return suggestionErr
			}

			// writes the OpenAPI doc into a new file for comments
			file, err := createTempSuggestionFile(docPath)
			if err != nil {
				return suggestionErr
			}

			// commits and pushes our new OpenAPI doc
			_, err = g.CommitAndPushSuggestions(docPath)
			if err != nil {
				return suggestionErr
			}

			// Creates a PR to layer OpenAPI suggestions on
			prNumber, err := g.CreateSuggestionPR(branch)
			if err != nil {
				return suggestionErr
			}

			// Writes suggestion comments inline on the PR
			if prNumber != nil {
				g.WriteSuggestionComments(file, prNumber, output)
			}

		}

		return suggestionErr
	} else {
		if err := cli.Validate(docPath); err != nil {
			return err
		}
	}

	return nil
}

// We need to create a temp suggestion file to layer our PR comments
func createTempSuggestionFile(doc string) (string, error) {
	srcFile, err := os.Open(doc)
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	fileName := fmt.Sprintf("./%s-speakeasy-temp", doc)
	dstFile, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return "", fmt.Errorf("error opening releases file: %w", err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return "", err
	}

	err = dstFile.Sync()
	if err != nil {
		return "", err
	}

	return fileName, nil
}
