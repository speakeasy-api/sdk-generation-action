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

	if err := cli.Validate(docPath); err != nil {
		return err
	}
	//}

	return nil
}
