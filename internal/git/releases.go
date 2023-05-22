package git

import (
	"context"
	"fmt"
	"os"

	"github.com/google/go-github/v48/github"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
)

func (g *Git) CreateRelease(releaseInfo releases.ReleasesInfo) error {
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
		} else {
			_, _, err = g.client.Repositories.CreateRelease(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), &github.RepositoryRelease{
				TagName:         github.String(tag),
				TargetCommitish: github.String(commitHash),
				Name:            github.String(fmt.Sprintf("%s - %s - %s", lang, tag, environment.GetInvokeTime().Format("2006-01-02 15:04:05"))),
				Body:            github.String(fmt.Sprintf(`# Generated by Speakeasy CLI%s`, releaseInfo)),
			})
			if err != nil {
				return fmt.Errorf("failed to create release: %w", err)
			}
		}
	}

	return nil
}
