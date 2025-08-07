package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/git"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/internal/run"
	"github.com/speakeasy-api/sdk-generation-action/internal/utils"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
)

func Release() error {
	logging.Info("SDK_CHANGELOG_JULY_2025: %s", os.Getenv("SDK_CHANGELOG_JULY_2025"))
	logging.Info("GITHUB_REPOSITORY: %s", os.Getenv("GITHUB_REPOSITORY"))
	logging.Info("GITHUB_ACTION_REPOSITORY: %s", os.Getenv("GITHUB_ACTION_REPOSITORY"))
	logging.Info("GITHUB_REPOSITORY_OWNER: %s", os.Getenv("GITHUB_REPOSITORY_OWNER"))

	accessToken := environment.GetAccessToken()
	if accessToken == "" {
		return errors.New("github access token is required")
	}
	repoURL := os.Getenv("GITHUB_REPOSITORY")
	if strings.Contains(strings.ToLower(repoURL), "speakeasy-api") || strings.Contains(strings.ToLower(repoURL), "speakeasy-sdks") || strings.Contains(strings.ToLower(repoURL), "ryan-timothy-albert") {
		os.Setenv("SDK_CHANGELOG_JULY_2025", "true")
	}

	g, err := initAction()
	if err != nil {
		return err
	}

	dir := "."
	usingReleasesMd := false
	var providesExplicitTarget bool
	logging.Info("specificTarget: %s", environment.SpecifiedTarget())
	if specificTarget := environment.SpecifiedTarget(); specificTarget != "" {
		logging.Info("inside if condition")
		workflow, err := configuration.GetWorkflowAndValidateLanguages(true)
		logging.Info("error: %v", err)
		if err != nil {
			return err
		}
		logging.Info("about to check target")
		if target, ok := workflow.Targets[specificTarget]; ok {
			logging.Info("inside if condition 2")
			logging.Info("target: %v", target)
			if target.Output != nil {
				logging.Info("inside if condition 3")
				dir = strings.TrimPrefix(*target.Output, "./")
			}

			logging.Info("dir: %v", dir)
			dir = filepath.Join(environment.GetWorkingDirectory(), dir)
			logging.Info("dir after join: %v", dir)
			providesExplicitTarget = true
		}
	}

	logging.Info("providesExplicitTarget: %v", providesExplicitTarget)

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

		logging.Info("files: %v", files)
		logging.Info("dir: %v", dir)
		logging.Info("usingReleasesMd: %v", usingReleasesMd)
		dir, usingReleasesMd = GetDirAndShouldUseReleasesMD(files, dir, usingReleasesMd)

	}

	logging.Info("usingReleasesMd outside if condition: %v", usingReleasesMd)
	var languages map[string]releases.LanguageReleaseInfo
	var latestRelease *releases.ReleasesInfo
	var targetSpecificReleaseNotes releases.TargetReleaseNotes = nil
	oldReleaseContent := ""

	// Old way of getting release Info (uses RELEASES.md)
	if usingReleasesMd {
		logging.Info("Using RELEASES.md to get release info")
		logging.Debug("Using RELEASES.md to get release info")
		latestRelease, err = releases.GetLastReleaseInfo(dir)
	} else {
		logging.Info("Using gen lockfile to get release info")
		logging.Debug("Using gen lockfile to get release info")
		latestRelease, err = releases.GetReleaseInfoFromGenerationFiles(dir)
		if err != nil {
			fmt.Printf("Error getting release info from generation files: %v\n", err)
			return err
		}
		// targetSpecificReleaseNotes variable is present only if SDK_CHANGELOG_JULY_2025 env is true
		targetSpecificReleaseNotes, err = releases.GetTargetSpecificReleaseNotes(dir)
		if err != nil {
			fmt.Printf("Error getting target specific release notes: %v\n", err)
		}

	}
	if err != nil {
		return err
	}
	languages = latestRelease.Languages
	oldReleaseContent = latestRelease.String()

	outputs := map[string]string{}
	for lang, info := range languages {
		outputs[utils.OutputTargetRegenerated(lang)] = "true"
		outputs[utils.OutputTargetDirectory(lang)] = info.Path
	}

	if err = addPublishOutputs(dir, outputs); err != nil {
		return err
	}

	if err := g.CreateRelease(oldReleaseContent, languages, outputs, targetSpecificReleaseNotes); err != nil {
		return err
	}

	if err = setOutputs(outputs); err != nil {
		return err
	}

	if os.Getenv("SPEAKEASY_API_KEY") != "" {
		if err = addCurrentBranchTagging(g, languages); err != nil {
			return errors.Wrap(err, "failed to tag registry images")
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
	// a tag that is applied if the target contributing is published
	var isPublished bool
	branch := strings.TrimPrefix(os.Getenv("GITHUB_REF"), "refs/heads/")
	workflow, err := configuration.GetWorkflowAndValidateLanguages(true)
	if err != nil {
		return err
	}

	// the tagging library treats targets synonymously with code samples
	if specificTarget := environment.SpecifiedTarget(); specificTarget != "" {
		if target, ok := workflow.Targets[specificTarget]; ok {
			isPublished = target.IsPublished()
			if source, ok := workflow.Sources[target.Source]; ok && source.Registry != nil {
				sources = append(sources, target.Source)
			}

			if target.CodeSamples != nil && target.CodeSamples.Registry != nil {
				targets = append(targets, specificTarget)
			}
		}
	} else {
		for name, target := range workflow.Targets {
			if releaseInfo, ok := latestRelease[target.Target]; ok {
				var targetIsMatched bool
				releasePath, err := filepath.Rel(".", releaseInfo.Path)
				if err != nil {
					return err
				}

				// check for no SDK output path
				if (releasePath == "" || releasePath == ".") && target.Output == nil {
					targetIsMatched = true
				}

				if target.Output != nil {
					outputPath, err := filepath.Rel(".", *target.Output)
					if err != nil {
						return err
					}
					outputPath = filepath.Join(environment.GetWorkingDirectory(), outputPath)
					if outputPath == releasePath {
						targetIsMatched = true
					}
				}

				if targetIsMatched {
					isPublished = isPublished || target.IsPublished()
					if source, ok := workflow.Sources[target.Source]; ok && source.Registry != nil {
						sources = append(sources, target.Source)
					}

					if target.CodeSamples != nil && target.CodeSamples.Registry != nil {
						targets = append(targets, name)
					}
				}
			}
		}
	}

	if (len(sources) > 0 || len(targets) > 0) && branch != "" {
		tags := []string{branch}
		if isPublished {
			tags = append(tags, "published")
		}
		return cli.Tag(tags, sources, targets)
	}

	return nil
}
