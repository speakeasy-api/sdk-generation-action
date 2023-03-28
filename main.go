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
		slices.SortFunc(envs, func(i, j string) bool {
			iKey, iValue, _ := strings.Cut(i, "=")
			jKey, jValue, _ := strings.Cut(j, "=")

			return iKey < jKey || (iKey == jKey && iValue < jValue)
		})

		for _, env := range envs {
			fmt.Println(env)
		}
	}

	var err error
	switch environment.GetAction() {
	case environment.ActionValidate:
		err = actions.Validate()
	case environment.ActionGenerate:
		err = actions.Generate()
	case environment.ActionFinalize:
		err = actions.Finalize()
	case environment.ActionRelease:
		err = actions.Release()
	}

	if err != nil {
		fmt.Printf("::error title=failed::%v\n", err)
		os.Exit(1)
	}
}
