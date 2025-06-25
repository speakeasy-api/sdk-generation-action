package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/internal/telemetry"
	"github.com/speakeasy-api/speakeasy-client-sdk-go/v3/pkg/models/shared"

	"github.com/speakeasy-api/sdk-generation-action/internal/actions"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"golang.org/x/exp/slices"
)

func main() {
	// Log action version and branch information
	logging.Info("=== SDK Generation Action Debug Info ===")
	logging.Info("Action Repository: %s", os.Getenv("GITHUB_ACTION_REPOSITORY"))
	logging.Info("Action Ref: %s", os.Getenv("GITHUB_ACTION_REF"))
	logging.Info("Action Path: %s", os.Getenv("GITHUB_ACTION_PATH"))
	logging.Info("Workflow Ref: %s", os.Getenv("GITHUB_REF"))
	logging.Info("Workflow SHA: %s", os.Getenv("GITHUB_SHA"))
	logging.Info("Repository: %s", os.Getenv("GITHUB_REPOSITORY"))
	logging.Info("Event Name: %s", os.Getenv("GITHUB_EVENT_NAME"))

	if environment.IsDebugMode() {
		envs := os.Environ()
		slices.SortFunc(envs, func(i, j string) int {
			iKey, iValue, _ := strings.Cut(i, "=")
			jKey, jValue, _ := strings.Cut(j, "=")

			comp := strings.Compare(iKey, jKey)

			if comp != 0 {
				return comp
			}

			return strings.Compare(iValue, jValue)
		})

		for _, env := range envs {
			fmt.Println(env)
		}
	}

	var err error
	// Don't fire CI_Exec telemetry on actions where we are only sending specific telemetry back.
	if environment.GetAction() == environment.ActionLog {
		err = actions.LogActionResult()
	} else if environment.GetAction() == environment.ActionPublishEvent {
		err = actions.PublishEventAction()
	} else {
		err = telemetry.Track(context.Background(), shared.InteractionTypeCiExec, func(ctx context.Context, event *shared.CliEvent) error {
			switch environment.GetAction() {
			case environment.ActionSuggest:
				return actions.Suggest()
			case environment.ActionRunWorkflow:
				return actions.RunWorkflow()
			case environment.ActionFinalizeSuggestion:
				return actions.FinalizeSuggestion()
			case environment.ActionRelease:
				return actions.Release()
			case environment.ActionTag:
				return actions.Tag()
			case environment.ActionTest:
				return actions.Test(ctx)
			default:
				return fmt.Errorf("unknown action: %s", environment.GetAction())
			}
		})
	}

	if err != nil {
		fmt.Printf("::error title=failed::%v\n", err)
		os.Exit(1)
	}
}
