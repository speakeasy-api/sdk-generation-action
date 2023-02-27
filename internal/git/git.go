package git

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"

	"github.com/google/go-github/v48/github"
	"golang.org/x/oauth2"
)

type Git struct {
	accessToken string
	repo        *git.Repository
	client      *github.Client
}

func New(accessToken string) *Git {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(ctx, ts)

	return &Git{
		accessToken: accessToken,
		client:      github.NewClient(tc),
	}
}

func (g *Git) CloneRepo() error {
	githubURL := os.Getenv("GITHUB_SERVER_URL")
	githubRepoLocation := os.Getenv("GITHUB_REPOSITORY")

	repoPath, err := url.JoinPath(githubURL, githubRepoLocation)
	if err != nil {
		return fmt.Errorf("failed to construct repo url: %w", err)
	}

	ref := os.Getenv("GITHUB_REF")

	fmt.Printf("Cloning repo: %s from ref: %s\n", repoPath, ref)

	baseDir := environment.GetBaseDir()

	r, err := git.PlainClone(path.Join(baseDir, "repo"), false, &git.CloneOptions{
		URL:           repoPath,
		Progress:      os.Stdout,
		Auth:          getGithubAuth(g.accessToken),
		ReferenceName: plumbing.ReferenceName(ref),
		SingleBranch:  true,
	})
	if err != nil {
		return fmt.Errorf("failed to clone repo: %w", err)
	}
	g.repo = r

	return nil
}

func (g *Git) CheckDirDirty(dir string) (bool, error) {
	if g.repo == nil {
		return false, fmt.Errorf("repo not cloned")
	}

	w, err := g.repo.Worktree()
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

func (g *Git) FindOrCreateBranch() (string, *github.PullRequest, error) {
	if g.repo == nil {
		return "", nil, fmt.Errorf("repo not cloned")
	}

	w, err := g.repo.Worktree()
	if err != nil {
		return "", nil, fmt.Errorf("error getting worktree: %w", err)
	}

	prs, _, err := g.client.PullRequests.List(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), nil)
	if err != nil {
		return "", nil, fmt.Errorf("error getting pull requests: %w", err)
	}

	var pr *github.PullRequest

	for _, p := range prs {
		if strings.Compare(p.GetTitle(), getPRTitle()) == 0 {
			pr = p
			break
		}
	}

	if pr != nil {
		branchName := pr.GetHead().GetRef()

		fmt.Printf("Found existing branch %s\n", branchName)

		r, err := g.repo.Remote("origin")
		if err != nil {
			return "", nil, fmt.Errorf("error getting remote: %w", err)
		}
		if err := r.Fetch(&git.FetchOptions{
			Auth: getGithubAuth(g.accessToken),
			RefSpecs: []config.RefSpec{
				config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branchName, branchName)),
			},
		}); err != nil && err != git.NoErrAlreadyUpToDate {
			return "", nil, fmt.Errorf("error fetching remote: %w", err)
		}

		branchRef := plumbing.NewBranchReferenceName(branchName)

		if err := w.Checkout(&git.CheckoutOptions{
			Branch: branchRef,
		}); err != nil {
			return "", nil, fmt.Errorf("error checking out branch: %w", err)
		}

		return branchName, pr, nil
	}

	branchName := fmt.Sprintf("speakeasy-sdk-regen-%d", time.Now().Unix())

	fmt.Printf("Creating branch %s\n", branchName)

	localRef := plumbing.NewBranchReferenceName(branchName)

	if err := w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName(localRef.String()),
		Create: true,
	}); err != nil {
		return "", nil, fmt.Errorf("error checking out branch: %w", err)
	}

	return branchName, nil, nil
}

func (g *Git) CommitAndPush(openAPIDocVersion, speakeasyVersion string) (string, error) {
	if g.repo == nil {
		return "", fmt.Errorf("repo not cloned")
	}

	w, err := g.repo.Worktree()
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

	if err := g.repo.Push(&git.PushOptions{
		Auth: getGithubAuth(g.accessToken),
	}); err != nil {
		return "", fmt.Errorf("error pushing changes: %w", err)
	}

	return commitHash.String(), nil
}

func (g *Git) CreateOrUpdatePR(branchName string, releaseInfo releases.ReleasesInfo, pr *github.PullRequest) error {
	var err error
	body := fmt.Sprintf(`# Generated by Speakeasy CLI
Based on:
- OpenAPI Doc %s %s
- Speakeasy CLI %s https://github.com/speakeasy-api/speakeasy`, releaseInfo.DocVersion, releaseInfo.DocLocation, releaseInfo.SpeakeasyVersion)

	if pr != nil {
		fmt.Println("Updating PR")

		pr.Body = github.String(body)
		pr, _, err = g.client.PullRequests.Edit(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), pr.GetNumber(), pr)
		if err != nil {
			return fmt.Errorf("failed to update PR: %w", err)
		}
	} else {
		fmt.Println("Creating PR")

		pr, _, err = g.client.PullRequests.Create(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), &github.NewPullRequest{
			Title:               github.String(getPRTitle()),
			Body:                github.String(body),
			Head:                github.String(branchName),
			Base:                github.String(os.Getenv("GITHUB_REF")),
			MaintainerCanModify: github.Bool(true),
		})
		if err != nil {
			return fmt.Errorf("failed to create PR: %w", err)
		}
	}

	url := ""
	if pr.URL != nil {
		url = *pr.HTMLURL
	}

	fmt.Printf("PR: %s\n", url)

	return nil
}

func (g *Git) GetLatestTag() (string, error) {
	tags, _, err := g.client.Repositories.ListTags(context.Background(), "speakeasy-api", "speakeasy", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get speakeasy cli tags: %w", err)
	}

	if len(tags) == 0 {
		return "", fmt.Errorf("no speakeasy cli tags found")
	}

	return tags[0].GetName(), nil
}

func (g *Git) GetCommitedFiles() ([]string, error) {
	path := environment.GetWorkflowEventPayloadPath()

	fmt.Printf("Workflow event payload path: %s\n", path)

	if path == "" {
		return nil, fmt.Errorf("no workflow event payload path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow event payload: %w", err)
	}

	fmt.Printf("Workflow event payload: %s\n", string(data))

	var payload struct {
		Commits []struct {
			Added    []string `json:"added"`
			Modified []string `json:"modified"`
		}
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow event payload: %w", err)
	}

	files := append(payload.Commits[0].Added, payload.Commits[0].Modified...)

	fmt.Printf("Found %d files in commits\n", len(files))

	return files, nil
}

func getGithubAuth(accessToken string) *gitHttp.BasicAuth {
	return &gitHttp.BasicAuth{
		Username: "gen",
		Password: accessToken,
	}
}

func getRepo() string {
	repoPath := os.Getenv("GITHUB_REPOSITORY")
	parts := strings.Split(repoPath, "/")
	return parts[len(parts)-1]
}

const speakeasyPRTitle = "chore: speakeasy sdk regeneration - "

func getPRTitle() string {
	return speakeasyPRTitle + environment.GetWorkflowName()
}
