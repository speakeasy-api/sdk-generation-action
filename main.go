package main

import (
	"fmt"
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
	switch environment.GetAction() {
	case environment.ActionSuggest:
		err = actions.Suggest()
	case environment.ActionRunWorkflow:
		err = actions.RunWorkflow()
	case environment.ActionFinalizeSuggestion:
		err = actions.FinalizeSuggestion()
	case environment.ActionRelease:
		err = actions.Release()
	case environment.ActionLog:
		actions.LogActionResult()
	}

	if err != nil {
		fmt.Printf("::error title=failed::%v\n", err)
		os.Exit(1)
	}
}
