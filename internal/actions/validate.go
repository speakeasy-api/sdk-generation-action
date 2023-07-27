package actions

import (
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

	// We will write suggestions to a new PR if the input flag is set to true, and we are parsing a local OpenAPI file.
	//if os.Getenv("INPUT_WRITE_SUGGESTIONS") == "true" && cli.IsAtLeastVersion(cli.LLMSuggestionVersion) {
	//	out, suggestionErr := cli.Suggest(docPath)
	//	if err := writeSuggestions(g, out, docPath); err != nil {
	//		logging.Info("error writing suggestions to PR %s", err.Error())
	//	}
	//
	//	return suggestionErr
	//} else {
	if err := cli.Validate(docPath); err != nil {
		return err
	}
	//}

	return nil
}

//func writeSuggestions(g *git.Git, out string, docPath string) error {
//	output := suggestions.ParseOutput(out)
//	if len(output) > 0 {
//		// creates a branch for our suggestion PR
//		branch, err := g.CreateSuggestionBranch()
//		if err != nil {
//			return err
//		}
//
//		// writes the OpenAPI doc into a new file for comments
//		file, err := createTempSuggestionFile(docPath)
//		if err != nil {
//			return err
//		}
//
//		// commits and pushes our new OpenAPI doc to the new branch
//		_, err = g.CommitAndPushSuggestions(strings.Join(strings.Split(docPath, "/")[1:], "/"))
//		if err != nil {
//			return err
//		}
//
//		// Creates a PR to layer OpenAPI suggestions on to the new branch
//		prNumber, commitSha, err := g.CreateSuggestionPR(branch, strings.Join(strings.Split(docPath, "/")[1:], "/"))
//		if err != nil {
//			return err
//		}
//
//		// Writes suggestion comments inline on the PR
//		if prNumber != nil {
//			fileName := strings.Join(strings.Split(file, "/")[1:], "/")
//			return g.WriteSuggestionComments(fileName, prNumber, commitSha, output)
//		}
//	}
//
//	return nil
//}

// We need to create a temp suggestion file to layer our PR comments
//func createTempSuggestionFile(doc string) (string, error) {
//	srcFile, err := os.Open(doc)
//	if err != nil {
//		return "", err
//	}
//	defer srcFile.Close()
//
//	// Handle the case where the file comes from a remote URL
//	doc = strings.TrimPrefix(doc, "openapi/")
//	if !strings.HasPrefix(doc, "repo/") {
//		doc = "repo/" + doc
//	}
//
//	fileName := strings.TrimSuffix(doc, filepath.Ext(doc)) + "-speakeasytemp" + filepath.Ext(doc)
//	dstFile, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
//	if err != nil {
//		return "", fmt.Errorf("error opening releases file: %w", err)
//	}
//	defer dstFile.Close()
//
//	_, err = io.Copy(dstFile, srcFile)
//	if err != nil {
//		return "", err
//	}
//
//	err = dstFile.Sync()
//	if err != nil {
//		return "", err
//	}
//
//	return fileName, nil
//}
