package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/utils/merkletrie"
	genConfig "github.com/speakeasy-api/sdk-gen-config"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"

	"github.com/google/go-github/v54/github"
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

	logging.Info("Cloning repo: %s from ref: %s", repoPath, ref)

	workspace := environment.GetWorkspace()

	r, err := git.PlainClone(path.Join(workspace, "repo"), false, &git.CloneOptions{
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

func (g *Git) CheckDirDirty(dir string, ignoreChangePatterns map[string]string) (bool, string, error) {
	if g.repo == nil {
		return false, "", fmt.Errorf("repo not cloned")
	}

	w, err := g.repo.Worktree()
	if err != nil {
		return false, "", fmt.Errorf("error getting worktree: %w", err)
	}

	status, err := w.Status()
	if err != nil {
		return false, "", fmt.Errorf("error getting status: %w", err)
	}

	cleanedDir := path.Clean(dir)
	if cleanedDir == "." {
		cleanedDir = ""
	}

	changesFound := false
	fileChangesFound := false
	newFiles := []string{}

	for f, s := range status {
		if strings.Contains(f, "gen.yaml") || strings.Contains(f, "gen.lock") {
			continue
		}

		if strings.HasPrefix(f, cleanedDir) {
			switch s.Worktree {
			case git.Added:
				fallthrough
			case git.Deleted:
				fallthrough
			case git.Untracked:
				newFiles = append(newFiles, f)
				fileChangesFound = true
			case git.Modified:
				fallthrough
			case git.Renamed:
				fallthrough
			case git.Copied:
				fallthrough
			case git.UpdatedButUnmerged:
				changesFound = true
			case git.Unmodified:
			}

			if changesFound && fileChangesFound {
				break
			}
		}
	}

	if fileChangesFound {
		return true, fmt.Sprintf("new file found: %#v", newFiles), nil
	}

	if !changesFound {
		return false, "", nil
	}

	diffOutput, err := runGitCommand("diff", "--word-diff=porcelain")
	if err != nil {
		return false, "", fmt.Errorf("error running git diff: %w", err)
	}

	return IsGitDiffSignificant(diffOutput, ignoreChangePatterns)
}

func (g *Git) FindExistingPR(branchName string, action environment.Action) (string, *github.PullRequest, error) {
	if g.repo == nil {
		return "", nil, fmt.Errorf("repo not cloned")
	}

	prs, _, err := g.client.PullRequests.List(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), nil)
	if err != nil {
		return "", nil, fmt.Errorf("error getting pull requests: %w", err)
	}

	var prTitle string
	if action == environment.ActionGenerate || action == environment.ActionFinalize {
		prTitle = getGenPRTitle()
	} else if action == environment.ActionFinalize || action == environment.ActionFinalizeSuggestion {
		prTitle = getSuggestPRTitle()
	}

	if environment.IsDocsGeneration() {
		prTitle = getDocsPRTitle()
	}

	for _, p := range prs {
		if strings.Compare(p.GetTitle(), prTitle) == 0 {
			logging.Info("Found existing PR %s", *p.Title)

			if branchName != "" && p.GetHead().GetRef() != branchName {
				return "", nil, fmt.Errorf("existing PR has different branch name: %s than expected: %s", p.GetHead().GetRef(), branchName)
			}

			return p.GetHead().GetRef(), p, nil
		}
	}

	logging.Info("Existing PR not found")

	return branchName, nil, nil
}

func (g *Git) FindBranch(branchName string) (string, error) {
	if g.repo == nil {
		return "", fmt.Errorf("repo not cloned")
	}

	w, err := g.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("error getting worktree: %w", err)
	}

	r, err := g.repo.Remote("origin")
	if err != nil {
		return "", fmt.Errorf("error getting remote: %w", err)
	}
	if err := r.Fetch(&git.FetchOptions{
		Auth: getGithubAuth(g.accessToken),
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branchName, branchName)),
		},
	}); err != nil && err != git.NoErrAlreadyUpToDate {
		return "", fmt.Errorf("error fetching remote: %w", err)
	}

	branchRef := plumbing.NewBranchReferenceName(branchName)

	if err := w.Checkout(&git.CheckoutOptions{
		Branch: branchRef,
	}); err != nil {
		return "", fmt.Errorf("error checking out branch: %w", err)
	}

	logging.Info("Found existing branch %s", branchName)

	return branchName, nil
}

func (g *Git) FindOrCreateBranch(branchName string, action environment.Action) (string, error) {
	if g.repo == nil {
		return "", fmt.Errorf("repo not cloned")
	}

	w, err := g.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("error getting worktree: %w", err)
	}

	if branchName != "" {
		return g.FindBranch(branchName)
	}

	if action == environment.ActionGenerate {
		branchName = fmt.Sprintf("speakeasy-sdk-regen-%d", time.Now().Unix())
	} else if action == environment.ActionSuggest {
		branchName = fmt.Sprintf("speakeasy-openapi-suggestion-%d", time.Now().Unix())
	}

	if environment.IsDocsGeneration() {
		branchName = fmt.Sprintf("speakeasy-sdk-docs-regen-%d", time.Now().Unix())
	}

	logging.Info("Creating branch %s", branchName)

	localRef := plumbing.NewBranchReferenceName(branchName)

	if err := w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName(localRef.String()),
		Create: true,
	}); err != nil {
		return "", fmt.Errorf("error checking out branch: %w", err)
	}

	return branchName, nil
}

func (g *Git) DeleteBranch(branchName string) error {
	if g.repo == nil {
		return fmt.Errorf("repo not cloned")
	}

	logging.Info("Deleting branch %s", branchName)

	r, err := g.repo.Remote("origin")
	if err != nil {
		return fmt.Errorf("error getting remote: %w", err)
	}

	ref := plumbing.NewBranchReferenceName(branchName)

	if err := r.Push(&git.PushOptions{
		Auth: getGithubAuth(g.accessToken),
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf(":%s", ref.String())),
		},
	}); err != nil {
		return fmt.Errorf("error deleting branch: %w", err)
	}

	return nil
}

func (g *Git) CommitAndPush(openAPIDocVersion, speakeasyVersion, doc string, action environment.Action) (string, error) {
	if g.repo == nil {
		return "", fmt.Errorf("repo not cloned")
	}

	w, err := g.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("error getting worktree: %w", err)
	}

	logging.Info("Commit and pushing changes to git")

	if _, err := w.Add("."); err != nil {
		return "", fmt.Errorf("error adding changes: %w", err)
	}

	var commitMessage string
	if action == environment.ActionGenerate {
		commitMessage = fmt.Sprintf("ci: regenerated with OpenAPI Doc %s, Speakeasy CLI %s", openAPIDocVersion, speakeasyVersion)
	} else if action == environment.ActionSuggest {
		commitMessage = fmt.Sprintf("ci: suggestions for OpenAPI doc %s", doc)
	}
	commitHash, err := w.Commit(commitMessage, &git.CommitOptions{
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

func (g *Git) CreateOrUpdatePR(branchName string, releaseInfo releases.ReleasesInfo, previousGenVersion string, pr *github.PullRequest) error {
	var changelog string
	var err error

	var previousGenVersions []string

	if previousGenVersion != "" {
		previousGenVersions = strings.Split(previousGenVersion, ";")
	}

	for language, info := range releaseInfo.LanguagesGenerated {
		genPath := path.Join(environment.GetWorkspace(), "repo", info.Path)

		var targetVersions map[string]string

		cfg, err := genConfig.Load(genPath)
		if err != nil {
			logging.Debug("failed to load gen config for retrieving granular versions for changelog at path %s: %v", genPath, err)
			continue
		} else {
			ok := false
			targetVersions, ok = cfg.LockFile.Features[language]
			if !ok {
				logging.Debug("failed to find language %s in gen config for retrieving granular versions for changelog at path %s", language, genPath)
				continue
			}
		}

		var previousVersions map[string]string

		if len(previousGenVersions) > 0 {
			for _, previous := range previousGenVersions {
				langVersions := strings.Split(previous, ":")

				if len(langVersions) == 2 && langVersions[0] == language {
					previousVersions = map[string]string{}

					pairs := strings.Split(langVersions[1], ",")
					for i := 0; i < len(pairs); i += 2 {
						previousVersions[pairs[i]] = pairs[i+1]
					}
				}
			}
		}

		versionChangelog, err := cli.GetChangelog(language, releaseInfo.GenerationVersion, "", targetVersions, previousVersions)
		if err != nil {
			return fmt.Errorf("failed to get changelog for language %s: %w", language, err)
		}

		changelog += fmt.Sprintf("\n\n## %s CHANGELOG\n\n%s", strings.ToUpper(language), versionChangelog)
	}

	if changelog == "" {
		// Not using granular version, grab old changelog
		changelog, err = cli.GetChangelog("", releaseInfo.GenerationVersion, previousGenVersion, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to get changelog: %w", err)
		}
		if strings.TrimSpace(changelog) != "" {
			changelog = "\n\n\n## CHANGELOG\n\n" + changelog
		}
	} else {
		changelog = "\n" + changelog
	}

	body := fmt.Sprintf(`# SDK update
Based on:
- OpenAPI Doc %s %s
- Speakeasy CLI %s (%s) https://github.com/speakeasy-api/speakeasy%s`, releaseInfo.DocVersion, releaseInfo.DocLocation, releaseInfo.SpeakeasyVersion, releaseInfo.GenerationVersion, changelog)

	// TODO: To be removed after we start blocking on usage limits.
	if accessAllowed, err := cli.CheckFreeUsageAccess(); err == nil && !accessAllowed {
		warningDate := time.Date(2024, time.March, 18, 0, 0, 0, 0, time.UTC)
		daysToLimit := int(math.Round(warningDate.Sub(time.Now().Truncate(24*time.Hour)).Hours() / 24))
		body = fmt.Sprintf(`# 🚀 Time to Upgrade 🚀
You have exceeded the limit of one free generated SDK. Please reach out to the Speakeasy team in the next %d days to ensure continued access`, daysToLimit) + "\n\n" + body
	} else {
		return err
	}

	const maxBodyLength = 65536

	if len(body) > maxBodyLength {
		body = body[:maxBodyLength-3] + "..."
	}

	if pr != nil {
		logging.Info("Updating PR")

		pr.Body = github.String(body)
		pr, _, err = g.client.PullRequests.Edit(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), pr.GetNumber(), pr)
		if err != nil {
			return fmt.Errorf("failed to update PR: %w", err)
		}
	} else {
		logging.Info("Creating PR")

		title := getGenPRTitle()
		if environment.IsDocsGeneration() {
			title = getDocsPRTitle()
		}

		pr, _, err = g.client.PullRequests.Create(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), &github.NewPullRequest{
			Title:               github.String(title),
			Body:                github.String(body),
			Head:                github.String(branchName),
			Base:                github.String(environment.GetRef()),
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

	logging.Info("PR: %s", url)

	return nil
}

func (g *Git) CreateOrUpdateDocsPR(branchName string, releaseInfo releases.ReleasesInfo, previousGenVersion string, pr *github.PullRequest) error {
	var err error

	body := fmt.Sprintf(`# SDK Docs update
Based on:
- OpenAPI Doc %s %s
- Speakeasy CLI %s (%s) https://github.com/speakeasy-api/speakeasy`, releaseInfo.DocVersion, releaseInfo.DocLocation, releaseInfo.SpeakeasyVersion, releaseInfo.GenerationVersion)

	const maxBodyLength = 65536

	if len(body) > maxBodyLength {
		body = body[:maxBodyLength-3] + "..."
	}

	if pr != nil {
		logging.Info("Updating PR")

		pr.Body = github.String(body)
		pr, _, err = g.client.PullRequests.Edit(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), pr.GetNumber(), pr)
		if err != nil {
			return fmt.Errorf("failed to update PR: %w", err)
		}
	} else {
		logging.Info("Creating PR")

		pr, _, err = g.client.PullRequests.Create(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), &github.NewPullRequest{
			Title:               github.String(getDocsPRTitle()),
			Body:                github.String(body),
			Head:                github.String(branchName),
			Base:                github.String(environment.GetRef()),
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

	logging.Info("PR: %s", url)

	return nil
}

func (g *Git) CreateSuggestionPR(branchName, output string) (*int, string, error) {
	body := fmt.Sprintf(`Generated OpenAPI Suggestions by Speakeasy CLI. 
    Outputs changes to *%s*.`, output)

	logging.Info("Creating PR")

	fmt.Println(body, branchName, getSuggestPRTitle(), environment.GetRef())

	pr, _, err := g.client.PullRequests.Create(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), &github.NewPullRequest{
		Title:               github.String("Speakeasy OpenAPI Suggestions -" + environment.GetWorkflowName()),
		Body:                github.String(body),
		Head:                github.String(branchName),
		Base:                github.String(environment.GetRef()),
		MaintainerCanModify: github.Bool(true),
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to create PR: %w", err)
	}

	return pr.Number, pr.GetHead().GetSHA(), nil
}

func (g *Git) WritePRBody(prNumber int, body string) error {
	pr, _, err := g.client.PullRequests.Get(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), prNumber)
	if err != nil {
		return fmt.Errorf("failed to get PR: %w", err)
	}

	pr.Body = github.String(strings.Join([]string{*pr.Body, sanitizeExplanations(body)}, "\n\n"))
	if _, _, err = g.client.PullRequests.Edit(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), prNumber, pr); err != nil {
		return fmt.Errorf("failed to update PR: %w", err)
	}

	return nil
}

func (g *Git) WritePRComment(prNumber int, fileName, body string, line int) error {
	pr, _, err := g.client.PullRequests.Get(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), prNumber)
	if err != nil {
		return fmt.Errorf("failed to get PR: %w", err)
	}

	_, _, err = g.client.PullRequests.CreateComment(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), prNumber, &github.PullRequestComment{
		Body:     github.String(sanitizeExplanations(body)),
		Line:     github.Int(line),
		Path:     github.String(fileName),
		CommitID: github.String(pr.GetHead().GetSHA()),
	})
	if err != nil {
		return fmt.Errorf("failed to create PR comment: %w", err)
	}

	return nil
}

func sanitizeExplanations(str string) string {
	// Remove ANSI sequences
	ansiEscape := regexp.MustCompile(`\x1b[^m]*m`)
	str = ansiEscape.ReplaceAllString(str, "")
	// Escape ~ characters
	return strings.ReplaceAll(str, "~", "\\~")
}

func (g *Git) MergeBranch(branchName string) (string, error) {
	if g.repo == nil {
		return "", fmt.Errorf("repo not cloned")
	}

	w, err := g.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("error getting worktree: %w", err)
	}

	logging.Info("Merging branch %s", branchName)

	// Checkout target branch
	if err := w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName(environment.GetRef()),
		Create: false,
	}); err != nil {
		return "", fmt.Errorf("error checking out branch: %w", err)
	}

	output, err := runGitCommand("merge", branchName)
	if err != nil {
		// This can happen if a "compile" has changed something unexpectedly. Add a "git status --porcelain" into the action output
		debugOutput, _ := runGitCommand("status", "--porcelain")
		if len(debugOutput) > 0 {
			logging.Info("git status\n%s", debugOutput)
		}
		debugOutput, _ = runGitCommand("diff")
		if len(debugOutput) > 0 {
			logging.Info("git diff\n%s", debugOutput)
		}
		return "", fmt.Errorf("error merging branch: %w", err)
	}

	logging.Debug("Merge output: %s", output)

	headRef, err := g.repo.Head()
	if err != nil {
		return "", fmt.Errorf("error getting head ref: %w", err)
	}

	if err := g.repo.Push(&git.PushOptions{
		Auth: getGithubAuth(g.accessToken),
	}); err != nil {
		return "", fmt.Errorf("error pushing changes: %w", err)
	}

	return headRef.Hash().String(), nil
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

func (g *Git) GetDownloadLink(version string) (string, string, error) {
	page := 0

	// Iterate through pages until we find the release, or we run out of results
	for {
		releases, response, err := g.client.Repositories.ListReleases(context.Background(), "speakeasy-api", "speakeasy", &github.ListOptions{Page: page})
		if err != nil {
			return "", "", fmt.Errorf("failed to get speakeasy cli releases: %w", err)
		}

		if len(releases) == 0 {
			return "", "", fmt.Errorf("no speakeasy cli releases found")
		} else {
			link, tag := getDownloadLinkFromReleases(releases, version)
			if link == nil || tag == nil {
				page = response.NextPage
				continue
			}

			return *link, *tag, nil
		}
	}
}

func getDownloadLinkFromReleases(releases []*github.RepositoryRelease, version string) (*string, *string) {
	for _, release := range releases {
		for _, asset := range release.Assets {
			if version == "latest" || version == release.GetTagName() {
				curOS := runtime.GOOS
				curArch := runtime.GOARCH
				downloadUrl := asset.GetBrowserDownloadURL()

				// https://github.com/speakeasy-api/sdk-generation-action/pull/28#discussion_r1213129634
				if curOS == "linux" && (strings.Contains(strings.ToLower(asset.GetName()), "_linux_x86_64") || strings.Contains(strings.ToLower(asset.GetName()), "_linux_amd64")) {
					return &downloadUrl, release.TagName
				} else if strings.Contains(strings.ToLower(asset.GetName()), curOS) &&
					strings.Contains(strings.ToLower(asset.GetName()), curArch) {
					return &downloadUrl, release.TagName
				}
			}
		}
	}

	return nil, nil
}

func (g *Git) GetCommitedFiles() ([]string, error) {
	path := environment.GetWorkflowEventPayloadPath()

	if path == "" {
		return nil, fmt.Errorf("no workflow event payload path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow event payload: %w", err)
	}

	var payload struct {
		After  string `json:"after"`
		Before string `json:"before"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow event payload: %w", err)
	}

	if payload.After == "" {
		return nil, fmt.Errorf("no commit hash found in workflow event payload")
	}

	beforeCommit, err := g.repo.CommitObject(plumbing.NewHash(payload.Before))
	if err != nil {
		return nil, fmt.Errorf("failed to get before commit object: %w", err)
	}

	afterCommit, err := g.repo.CommitObject(plumbing.NewHash(payload.After))
	if err != nil {
		return nil, fmt.Errorf("failed to get after commit object: %w", err)
	}

	beforeState, err := beforeCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get before commit tree: %w", err)
	}

	afterState, err := afterCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get after commit tree: %w", err)
	}

	changes, err := beforeState.Diff(afterState)
	if err != nil {
		return nil, fmt.Errorf("failed to get diff between commits: %w", err)
	}

	files := []string{}

	for _, change := range changes {
		action, err := change.Action()
		if err != nil {
			return nil, fmt.Errorf("failed to get change action: %w", err)
		}
		if action == merkletrie.Delete {
			continue
		}

		files = append(files, change.To.Name)
	}

	logging.Info("Found %d files in commits", len(files))

	return files, nil
}

func (g *Git) CreateTag(tag string, hash string) error {
	logging.Info("Creating Tag %s from commit %s", tag, hash)

	if _, err := g.repo.CreateTag(tag, plumbing.NewHash(hash), &git.CreateTagOptions{
		Message: tag,
	}); err != nil {
		logging.Info("Failed to create tag: %s", err)
		return err
	}

	logging.Info("Tag %s created for commit %s", tag, hash)
	return nil
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

const (
	speakeasyGenPRTitle     = "chore: 🐝 Update SDK - "
	speakeasySuggestPRTitle = "chore: 🐝 Suggest OpenAPI changes - "
	speakeasyDocsPRTitle    = "chore: 🐝 Update SDK Docs - "
)

func getGenPRTitle() string {
	return speakeasyGenPRTitle + environment.GetWorkflowName()
}

func getDocsPRTitle() string {
	return speakeasyDocsPRTitle + environment.GetWorkflowName()
}

func getSuggestPRTitle() string {
	return speakeasySuggestPRTitle + environment.GetWorkflowName()
}

func runGitCommand(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = filepath.Join(environment.GetWorkspace(), "repo")
	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run git command: %w - %s", err, errb.String())
	}

	return outb.String(), nil
}
