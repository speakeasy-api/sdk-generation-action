package actions

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	config "github.com/speakeasy-api/sdk-gen-config"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"golang.org/x/exp/slices"
)

func Test() error {
	g, err := initAction()
	if err != nil {
		return err
	}

	if _, err = cli.Download("latest", g); err != nil {
		return err
	}

	wf, err := configuration.GetWorkflowAndValidateLanguages(false)
	if err != nil {
		return err
	}

	// This will only come in via workflow dispatch, we do accept 'all' as a validate special case
	var testedTargets []string
	if providedTargetName := environment.SpecifiedTarget(); providedTargetName != "" {
		testedTargets = append(testedTargets, providedTargetName)
	}

	if len(testedTargets) == 0 {
		files, err := g.GetCommittedFilesFromBaseBranch()
		if err != nil {
			fmt.Printf("Failed to get commited files: %s\n", err.Error())
		}

		fmt.Println("Files: ", files)

		for _, file := range files {
			if strings.Contains(file, "gen.yaml") || strings.Contains(file, "gen.lock") {
				cfgDir := filepath.Dir(file)
				_, err := config.Load(filepath.Dir(file))
				if err != nil {
					return fmt.Errorf("failed to load config: %w", err)
				}

				outDir, err := filepath.Abs(filepath.Dir(cfgDir))
				if err != nil {
					return err
				}
				for name, target := range wf.Targets {
					targetOutput := ""
					if target.Output != nil {
						targetOutput = *target.Output
					}
					targetOutput, err := filepath.Abs(filepath.Join(environment.GetWorkingDirectory(), targetOutput))
					if err != nil {
						return err
					}
					// If there are multiple SDKs in a workflow we ensure output path is unique
					if targetOutput == outDir && !slices.Contains(testedTargets, name) {
						testedTargets = append(testedTargets, name)
					}
				}
			}
		}
	}
	if len(testedTargets) == 0 {
		return fmt.Errorf("no target was provided")
	}

	// we will pretty much never have a test action for multiple targets
	// but if a customer manually setup their triggers in this way, we will run test sequentially for clear output
	var errs []error
	for _, target := range testedTargets {
		// TODO: Once we have stable test reports we will probably want to use GH API to leave a PR comment/clean up old comments
		if err := cli.Test(target); err != nil {
			errs = append(errs, err)
		}
	}

	return fmt.Errorf("test failures occured: %w", errors.Join(errs...))
}
