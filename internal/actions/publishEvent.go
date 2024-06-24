package actions

import (
	"os"

	"github.com/speakeasy-api/sdk-generation-action/internal/telemetry"
)

func PublishEventAction() error {
	g, err := initAction()
	if err != nil {
		return err
	}

	if err := telemetry.TriggerPublishingEvent(g, os.Getenv("INPUT_TARGET_DIRECTORY"), os.Getenv("GH_ACTION_RESULT"), os.Getenv("INPUT_REGISTRY_NAME")); err != nil {
		return err
	}

	return nil
}
