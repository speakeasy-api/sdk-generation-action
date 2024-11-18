package actions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/git"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/internal/run"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
)

func Release() error {
	accessToken := environment.GetAccessToken()
	if accessToken == "" {
		return errors.New("github access token is required")
	}

	g, err := initAction()
	if err != nil {
		return err
	}

	dir := "."
	usingReleasesMd := false
	var providesExplicitTarget bool
	if specificTarget := environment.SpecifiedTarget(); specificTarget != "" {
		workflow, err := configuration.GetWorkflowAndValidateLanguages(true)
		if err != nil {
			return err
		}
		if target, ok := workflow.Targets[specificTarget]; ok {
			if target.Output != nil {
				dir = strings.TrimPrefix(*target.Output, "./")
			}

			dir = filepath.Join(environment.GetWorkingDirectory(), dir)

			providesExplicitTarget = true
		}
	}

	if !providesExplicitTarget {
		// This searches for files that would be referenced in the GH Action trigger
		files, err := g.GetCommitedFiles()
		if err != nil {
			fmt.Printf("Failed to get commited files: %s\n", err.Error())
		}

		if environment.IsDebugMode() {
			for _, file := range files {
				logging.Debug("Found commited file: %s", file)
			}
		}

		dir, usingReleasesMd = GetDirAndShouldUseReleasesMD(files, dir, usingReleasesMd)
	}

	var latestRelease *releases.ReleasesInfo
	if usingReleasesMd {
		latestRelease, err = releases.GetLastReleaseInfo(dir)
		if err != nil {
			return err
		}
	} else {
		latestRelease, err = releases.GetReleaseInfoFromGenerationFiles(dir)
		if err != nil {
			return err
		}
	}

	outputs := map[string]string{}
	for lang, info := range latestRelease.Languages {
		outputs[fmt.Sprintf("%s_regenerated", lang)] = "true"
		outputs[fmt.Sprintf("%s_directory", lang)] = info.Path
	}

	if err = addPublishOutputs(dir, outputs); err != nil {
		return err
	}

	if err := g.CreateRelease(*latestRelease, outputs); err != nil {
		return err
	}

	if err = setOutputs(outputs); err != nil {
		return err
	}

	fmt.Println("WE ENTERED")
	if os.Getenv("SPEAKEASY_API_KEY") != "" {
		fmt.Println("WE HAVE API KEY")
		if err = addCurrentBranchTagging(g, latestRelease.Languages); err != nil {
			logging.Debug("failed to tag registry images: %v", err)
		}
	}

	return nil
}

func GetDirAndShouldUseReleasesMD(files []string, dir string, usingReleasesMd bool) (string, bool) {
	for _, file := range files {
		// Maintain Support for RELEASES.MD for backward compatibility with existing publishing actions
		if strings.Contains(file, "RELEASES.md") {
			// file = ./RELEASES.md
			// dir = .
			dir = filepath.Dir(file)
			logging.Info("Found RELEASES.md in %s\n", dir)
			usingReleasesMd = true
			break
		}

		if strings.Contains(file, "gen.lock") {
			// file = .speakeasy/gen.lock
			dir = filepath.Dir(file)
			if strings.Contains(dir, ".speakeasy") {
				dir = filepath.Dir(dir)
			}

			logging.Info("Found gen.lock in %s\n", dir)
		}
	}
	return dir, usingReleasesMd
}

func addPublishOutputs(dir string, outputs map[string]string) error {
	wf, err := configuration.GetWorkflowAndValidateLanguages(false)
	if err != nil {
		return err
	}

	for _, target := range wf.Targets {
		// Only add outputs for the target that was regenerated, based on output directory
		if dir != "." && target.Output != nil {
			output, err := filepath.Rel(".", *target.Output)
			if err != nil {
				return err
			}

			if environment.GetWorkingDirectory() != "" {
				output = filepath.Join(environment.GetWorkingDirectory(), output)
			}

			if output != dir {
				continue
			}
		}

		run.AddTargetPublishOutputs(target, outputs, nil)
	}

	return nil
}

func addCurrentBranchTagging(g *git.Git, latestRelease map[string]releases.LanguageReleaseInfo) error {
	_, err := cli.Download("latest", g)
	if err != nil {
		return err
	}

	var sources, targets []string
	branch := strings.TrimPrefix(os.Getenv("GITHUB_REF"), "refs/heads/")
	workflow, err := configuration.GetWorkflowAndValidateLanguages(true)
	if err != nil {
		return err
	}

	if specificTarget := environment.SpecifiedTarget(); specificTarget != "" {
		fmt.Println("WE HAVE SPECIFIC TARGET")
		if target, ok := workflow.Targets[specificTarget]; ok {
			sources = append(sources, target.Source)
			targets = append(targets, specificTarget)
		}
	} else {
		for name, target := range workflow.Targets {
			if releaseInfo, ok := latestRelease[target.Target]; ok {
				releasePath, err := filepath.Rel(".", releaseInfo.Path)
				if err != nil {
					return err
				}

				fmt.Println("WE HAVE RELEASE INFO")
				fmt.Println(target.Output)
				fmt.Println(releasePath)

				if releasePath == "" && target.Output == nil {
					sources = append(sources, target.Source)
					targets = append(targets, name)
				}

				if target.Output != nil {
					if outputPath, err := filepath.Rel(".", *target.Output); err != nil && outputPath == releasePath {
						sources = append(sources, target.Source)
						targets = append(targets, name)
					}
				}
			}
		}
	}

	fmt.Println("BRANCH: ", branch)
	fmt.Println("SOURCES: ", sources)
	fmt.Println("TARGETS: ", targets)

	if len(sources) > 0 && len(targets) > 0 && branch != "" {
		return cli.Tag([]string{branch}, sources, targets)
	}

	return nil
}
