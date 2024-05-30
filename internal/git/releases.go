package git

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/google/go-github/v54/github"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
)

//go:embed goreleaser.yml
var tfGoReleaserConfig string

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
		if info.Path != "." {
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
			_, _, err = g.client.Repositories.CreateRelease(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), &github.RepositoryRelease{
				TagName:         tagName,
				TargetCommitish: github.String(commitHash),
				Name:            github.String(fmt.Sprintf("%s - %s - %s", lang, tag, environment.GetInvokeTime().Format("2006-01-02 15:04:05"))),
				Body:            github.String(fmt.Sprintf(`# Generated by Speakeasy CLI%s`, releaseInfo)),
			})
			if err != nil {
				if release, _, err := g.client.Repositories.GetReleaseByTag(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), *tagName); err == nil && release != nil {
					fmt.Println(fmt.Sprintf("a github release with tag %s already existing ... skipping publishing", *tagName))
					fmt.Println(fmt.Sprintf("to publish this version again delete the github tag and release"))
					if _, ok := outputs[fmt.Sprintf("publish_%s", lang)]; ok {
						outputs[fmt.Sprintf("publish_%s", lang)] = "false"
					}

					return nil
				}

				return fmt.Errorf("failed to create release for tag %s: %w", *tagName, err)
			}
		}
	}

	return nil
}
