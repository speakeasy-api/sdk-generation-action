package actions

import (
	"fmt"
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

	resolvedVersion, err := cli.Download(environment.GetPinnedSpeakeasyVersion(), g)
	if err != nil {
		return err
	}

	if !cli.IsAtLeastVersion(cli.MinimumSupportedCLIVersion) {
		return fmt.Errorf("action requires at least version %s of the speakeasy CLI", cli.MinimumSupportedCLIVersion)
	}

	docPath, _, err := document.GetOpenAPIFileInfo()
	if err != nil {
		return err
	}
	docPathPrefix := environment.GetWorkspace()
	if !strings.HasSuffix(docPathPrefix, "/") {
		docPathPrefix += "/"
	}
	if err := setOutputs(map[string]string{
		"resolved_speakeasy_version": resolvedVersion,
		"openapi_doc":                strings.TrimPrefix(docPath, docPathPrefix),
	}); err != nil {
		logging.Debug("failed to set outputs: %v", err)
	}

	// Errors from GetMaxValidation{Warnings,Errors} are very non-fatal, but should be logged.

	var maxWarns, maxErrors int
	if maxWarns, err = environment.GetMaxValidationWarnings(); err != nil {
		logging.Info("%v", err)
	}

	if maxErrors, err = environment.GetMaxValidationErrors(); err != nil {
		logging.Info("%v", err)
	}

	if err := cli.Validate(docPath, maxWarns, maxErrors); err != nil {
		return err
	}

	return nil
}
