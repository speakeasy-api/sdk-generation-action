package main

import (
	"context"
	"fmt"
	"github.com/speakeasy-api/sdk-generation-action/internal/telemetry"
	"github.com/speakeasy-api/speakeasy-client-sdk-go/v3/pkg/models/shared"
	"os"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/actions"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"golang.org/x/exp/slices"
)

func main() {
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
	case environment.ActionLog:
		return actions.LogActionResult()
	default:
		return fmt.Errorf("unknown action: %s", environment.GetAction())
		}
	})


	if err != nil {
		fmt.Printf("::error title=failed::%v\n", err)
		os.Exit(1)
	}
}
