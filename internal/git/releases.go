package git

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v63/github"
	config "github.com/speakeasy-api/sdk-gen-config"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/telemetry"
	"github.com/speakeasy-api/sdk-generation-action/internal/utils"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
)

//go:embed goreleaser.yml
var tfGoReleaserConfig string

const PublishingCompletedString = "Publishing Completed"

func (g *Git) SetReleaseToPublished(version, directory string) error {
	if g.repo == nil {
		return fmt.Errorf("repo not cloned")
	}
	tag := "v" + version
	if directory != "" && directory != "." && directory != "./" {
		tag = fmt.Sprintf("%s/%s", directory, tag)
	}

	release, _, err := g.client.Repositories.GetReleaseByTag(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), tag)
	if err != nil {
		return fmt.Errorf("failed to get release for tag %s: %w", tag, err)
	}

	if release != nil && release.ID != nil {
		if release.Body != nil && !strings.Contains(*release.Body, PublishingCompletedString) {
			body := *release.Body + "\n\n" + PublishingCompletedString
			release.Body = &body
		}

		if _, _, err = g.client.Repositories.EditRelease(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), *release.ID, release); err != nil {
			return fmt.Errorf("failed to add to release body for tag %s: %w", tag, err)
		}
	}

	return nil
}

func (g *Git) CreateRelease(releaseInfo releases.ReleasesInfo, outputs map[string]string) error {
	if g.repo == nil {
		return fmt.Errorf("repo not cloned")
	}

	fmt.Println("Creating release")

	headRef, err := g.repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get head ref: %w", err)
	}

	commitHash := headRef.Hash().String()

	for lang, info := range releaseInfo.Languages {
		tag := "v" + info.Version
		if info.Path != "" && info.Path != "." && info.Path != "./" {
			tag = fmt.Sprintf("%s/%s", info.Path, tag)
		}

		if lang == "terraform" {
			// Terraform is a special case -- we use go releaser externally to turn this tag into a release.
			err = g.CreateTag("v"+info.Version, commitHash)
			if err != nil {
				return fmt.Errorf("failed to create tag: %w", err)
			}
			// Copy our standard terraform config into /tmp/.goreleaser.yml
			err = os.WriteFile("/tmp/.goreleaser.yml", []byte(tfGoReleaserConfig), 0644)
			if err != nil {
				return fmt.Errorf("failed to write goreleaser config: %w", err)
			}
			cmd := exec.Command("goreleaser", "release", "--clean", "--config", "/tmp/.goreleaser.yml")
			cmd.Dir = filepath.Join(environment.GetWorkspace(), "repo")
			cmd.Env = append(os.Environ(),
				"GORELEASER_PREVIOUS_TAG="+info.PreviousVersion,
				"GORELEASER_CURRENT_TAG="+tag,
				"GITHUB_TOKEN="+environment.GetAccessToken(),
				"GPG_FINGERPRINT="+environment.GetGPGFingerprint(),
			)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to run goreleaser: %w", err)
			}
		} else {
			tagName := github.String(tag)
			release, _, err := g.client.Repositories.CreateRelease(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), &github.RepositoryRelease{
				TagName:         tagName,
				TargetCommitish: github.String(commitHash),
				Name:            github.String(fmt.Sprintf("%s - %s - %s", lang, tag, environment.GetInvokeTime().Format("2006-01-02 15:04:05"))),
				Body:            github.String(fmt.Sprintf(`# Generated by Speakeasy CLI%s`, releaseInfo)),
			})

			if err != nil {
				if release, _, err := g.client.Repositories.GetReleaseByTag(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), *tagName); err == nil && release != nil {
					if release.Body != nil && strings.Contains(*release.Body, PublishingCompletedString) {
						fmt.Println(fmt.Sprintf("a github release with tag %s has already been published ... skipping publishing", *tagName))
						fmt.Println(fmt.Sprintf("to publish this version again please check with your package managed delete the github tag and release"))
						if _, ok := outputs[fmt.Sprintf("publish_%s", lang)]; ok {
							outputs[fmt.Sprintf("publish_%s", lang)] = "false"
						}
					}
					// TODO: Consider deleting and recreating the release if we are moving forward with publishing
					return nil
				}
				// If the release fails, trigger a failed publishing CLI event
				if _, publishEventErr := telemetry.TriggerPublishingEvent(info.Path, "failed", utils.GetRegistryName(lang)); publishEventErr != nil {
					fmt.Printf("failed to write publishing event: %v\n", publishEventErr)
				}

				return fmt.Errorf("failed to create release for tag %s: %w", *tagName, err)
			} else {
				if lang == "typescript" {
					if err := g.AttachMCPBinary(info.Path, release.ID); err != nil {
						fmt.Println(fmt.Sprintf("attempted building standalone MCP binary: %v", err))
					}
				}
				// Go has no publishing job, so we publish a CLI event on github release here
				if lang == "go" {
					if _, publishEventErr := telemetry.TriggerPublishingEvent(info.Path, "success", utils.GetRegistryName(lang)); publishEventErr != nil {
						fmt.Printf("failed to write publishing event: %v\n", publishEventErr)
					}
				}
			}
		}
	}

	return nil
}

func (g *Git) AttachMCPBinary(path string, releaseID *int64) error {
	if releaseID == nil {
		fmt.Println("No release ID present ... skipping MCP binary upload")
		return nil
	}
	loadedCfg, err := config.Load(filepath.Join(environment.GetWorkspace(), "repo", path))
	if err != nil {
		return err
	}
	if tsConfig, ok := loadedCfg.Config.Languages["typescript"]; ok {
		if enable, ok := tsConfig.Cfg["enableMCPServer"].(bool); ok && enable {
			binaryPath := "./bin/mcp-server"

			installCmd := exec.Command("npm", "install")
			installCmd.Dir = filepath.Join(environment.GetWorkspace(), "repo")
			installCmd.Env = os.Environ()
			installCmd.Stdout = os.Stdout
			installCmd.Stderr = os.Stderr

			if err := installCmd.Run(); err != nil {
				return fmt.Errorf("failed to install dependencies: %w", err)
			}

			buildCmd := exec.Command("bun", "build", "./src/mcp-server/mcp-server.ts", // TODO: Do we potentially need to worry about this path?
				"--compile", "--outfile", binaryPath)
			buildCmd.Dir = filepath.Join(environment.GetWorkspace(), "repo")
			buildCmd.Env = os.Environ()
			buildCmd.Stdout = os.Stdout
			buildCmd.Stderr = os.Stderr

			if err := buildCmd.Run(); err != nil {
				return fmt.Errorf("failed to build mcp-server binary: %w", err)
			}

			file, err := os.Open(binaryPath)
			if err != nil {
				return fmt.Errorf("failed to open binary file: %w", err)
			}
			defer file.Close()

			_, _, err = g.client.Repositories.UploadReleaseAsset(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), *releaseID, &github.UploadOptions{
				Name: "mcp-server",
			}, file)

			if err != nil {
				return fmt.Errorf("failed to upload mcp-server release asset: %w", err)
			}

		}
	}

	fmt.Println("No MCP server present ... skipping MCP binary upload")
	return nil
}
