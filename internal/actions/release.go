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
	accessToken := environment.GetAccessToken()
	if accessToken == "" {
		return errors.New("github access token is required")
	}
	fmt.Println("INPUT_ENABLE_SDK_CHANGELOG: ", environment.GetSDKChangelog())

	g, err := initAction()
	if err != nil {
		return err
	}

	dir := "."
	usingReleasesMd := false
	var providesExplicitTarget bool
	logging.Info("specificTarget: %s", environment.SpecifiedTarget())
	if specificTarget := environment.SpecifiedTarget(); specificTarget != "" {
		workflow, err := configuration.GetWorkflowAndValidateLanguages(true)
		if err != nil {
			logging.Error("error: %v", err)
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

	logging.Info("providesExplicitTarget is set as: %v", providesExplicitTarget)

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

	var languages map[string]releases.LanguageReleaseInfo
	var latestRelease *releases.ReleasesInfo
	var targetSpecificReleaseNotes releases.TargetReleaseNotes = nil
	oldReleaseContent := ""

	// Old way of getting release Info (uses RELEASES.md)
	if usingReleasesMd {
		logging.Info("Using RELEASES.md to get release info")
		latestRelease, err = releases.GetLastReleaseInfo(dir)
	} else {
		logging.Info("Using gen lockfile to get release info")
		latestRelease, err = releases.GetReleaseInfoFromGenerationFiles(dir)
		if err != nil {
			fmt.Printf("Error getting release info from generation files: %v\n", err)
			return err
		}
		// targetSpecificReleaseNotes variable is present only if INPUT_ENABLE_SDK_CHANGELOG env is true
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
		if strings.Contains(file, "gen.lock") {
			// file = .speakeasy/gen.lock
			dir = filepath.Dir(file)
			if strings.Contains(dir, ".speakeasy") {
				dir = filepath.Dir(dir)
			}

			logging.Info("Found gen.lock in %s\n", dir)
			break
		}
		// Maintain Support for RELEASES.MD for backward compatibility with existing publishing actions
		if strings.Contains(file, "RELEASES.md") {
			// file = ./RELEASES.md
			// dir = .
			dir = filepath.Dir(file)
			logging.Info("Found RELEASES.md in %s\n", dir)
			usingReleasesMd = true
			break
		}
	}
	return dir, usingReleasesMd
}

func addPublishOutputs(dir string, outputs map[string]string) error {
	logging.Info("DEBUG: addPublishOutputs called with dir=%s", dir)

	wf, err := configuration.GetWorkflowAndValidateLanguages(false)
	if err != nil {
		return err
	}

	logging.Info("DEBUG: Found %d targets in workflow", len(wf.Targets))

	for targetID, target := range wf.Targets {
		logging.Info("DEBUG: Checking target %s (lang=%s)", targetID, target.Target)

		// Only add outputs for the target that was regenerated, based on output directory
		if dir != "." && target.Output != nil {
			output, err := filepath.Rel(".", *target.Output)
			if err != nil {
				return err
			}

			if environment.GetWorkingDirectory() != "" {
				output = filepath.Join(environment.GetWorkingDirectory(), output)
			}

			logging.Info("DEBUG: dir=%s, output=%s", dir, output)
			if output != dir {
				logging.Info("DEBUG: Skipping target %s because output doesn't match dir", targetID)
				continue
			}
		}

		// Debug: Print publishing config
		logging.Info("DEBUG: Processing target %s", target.Target)
		if target.Publishing != nil {
			logging.Info("DEBUG: Publishing config found for %s", target.Target)
			if target.Publishing.PyPi != nil {
				if target.Publishing.PyPi.UseTrustedPublishing != nil {
					logging.Info("DEBUG: PyPi.UseTrustedPublishing=%v", *target.Publishing.PyPi.UseTrustedPublishing)
				} else {
					logging.Info("DEBUG: PyPi.UseTrustedPublishing is nil")
				}
			} else {
				logging.Info("DEBUG: PyPi config is nil")
			}
		} else {
			logging.Info("DEBUG: Publishing config is nil for %s", target.Target)
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
		tags := []string{environment.SanitizeBranchName(branch)}
		if isPublished {
			tags = append(tags, "published")
		}
		return cli.Tag(tags, sources, targets)
	}

	return nil
}
