package main

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
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

func commitAndPush(g *git.Repository, openAPIDocVersion, speakeasyVersion, accessToken string) (string, error) {
	w, err := g.Worktree()
	if err != nil {
		return "", fmt.Errorf("error getting worktree: %w", err)
	}

	fmt.Println("Commit and pushing changes to git")

	if _, err := w.Add("."); err != nil {
		return "", fmt.Errorf("error adding changes: %w", err)
	}

	commitHash, err := w.Commit(fmt.Sprintf("ci: regenerated with OpenAPI Doc %s, Speakeay CLI %s", openAPIDocVersion, speakeasyVersion), &git.CommitOptions{
		Author: &object.Signature{
			Name:  "speakeasybot",
			Email: "bot@speakeasyapi.dev",
			When:  time.Now(),
		},
		All: true,
	})
	if err != nil {
		return "", fmt.Errorf("error committing changes: %w", err)
	}

	if err := g.Push(&git.PushOptions{
		Auth: getGithubAuth(accessToken),
	}); err != nil {
		return "", fmt.Errorf("error pushing changes: %w", err)
	}

	return commitHash.String(), nil
}

func checkDirDirty(g *git.Repository, dir string) (bool, error) {
	w, err := g.Worktree()
	if err != nil {
		return false, fmt.Errorf("error getting worktree: %w", err)
	}

	status, err := w.Status()
	if err != nil {
		return false, fmt.Errorf("error getting status: %w", err)
	}

	cleanedDir := path.Clean(dir)
	if cleanedDir == "." {
		cleanedDir = ""
	}

	for f, s := range status {
		if strings.Contains(f, "gen.yaml") {
			continue
		}

		if strings.HasPrefix(f, cleanedDir) && s.Worktree != git.Unmodified {
			return true, nil
		}
	}

	return false, nil
}
