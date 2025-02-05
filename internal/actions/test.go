package actions

import (
	"fmt"
	"path/filepath"
	"strings"

	config "github.com/speakeasy-api/sdk-gen-config"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
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

	// We do accept 'all' as a validate special case
	providedTargetName := environment.SpecifiedTarget()

	if providedTargetName == "" {
		files, err := g.GetCommitedFiles()
		if err != nil {
			fmt.Printf("Failed to get commited files: %s\n", err.Error())
		}

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
					if targetOutput == outDir {
						providedTargetName = name
					}
				}
			}
		}
	}
	if providedTargetName == "" {
		return fmt.Errorf("no target was provided")
	}

	// TODO: Once we have stable test reports we will probably want to use GH API to leave a PR comment/clean up old comments
	return cli.Test(providedTargetName)
}
