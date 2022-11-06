package main

import (
	"fmt"
	"net/url"
	"os"
	"path"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

func cloneRepo(accessToken string) (*git.Repository, error) {
	githubURL := os.Getenv("GITHUB_SERVER_URL")
	githubRepoLocation := os.Getenv("GITHUB_REPOSITORY")

	repoPath, err := url.JoinPath(githubURL, githubRepoLocation)
	if err != nil {
		return nil, fmt.Errorf("failed to construct repo url: %w", err)
	}

	ref := os.Getenv("GITHUB_HEAD_REF")
	if ref == "" {
		ref = os.Getenv("GITHUB_REF")
	} else {
		ref = string(plumbing.NewBranchReferenceName(ref))
	}

	fmt.Printf("Cloning repo: %s from ref: %s\n", repoPath, ref)

	g, err := git.PlainClone(path.Join(baseDir, "repo"), false, &git.CloneOptions{
		URL:           repoPath,
		Progress:      os.Stdout,
		Auth:          getGithubAuth(accessToken),
		ReferenceName: plumbing.ReferenceName(ref),
		SingleBranch:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to clone repo: %w", err)
	}

	return g, nil
}

func getGithubAuth(accessToken string) *gitHttp.BasicAuth {
	return &gitHttp.BasicAuth{
		Username: "gen",
		Password: accessToken,
	}
}
