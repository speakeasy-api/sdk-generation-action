package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
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
	"github.com/speakeasy-api/sdk-generation-action/internal/versionbumps"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
	"github.com/speakeasy-api/versioning-reports/versioning"

	"github.com/google/go-github/v63/github"
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

	ref := environment.GetRef()

	logging.Info("Cloning repo: %s from ref: %s", repoPath, ref)

	workspace := environment.GetWorkspace()

	// Remove the repo if it exists
	// Flow is useful when testing locally, but we're usually in a fresh image so unnecessary most of the time
	repoDir := path.Join(workspace, "repo")
	if err := os.RemoveAll(repoDir); err != nil {
		return err
	}

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

	filesToIgnore := []string{"gen.yaml", "gen.lock", "workflow.yaml", "workflow.lock"}

	for f, s := range status {
		shouldSkip := slices.ContainsFunc(filesToIgnore, func(fileToIgnore string) bool {
			return strings.Contains(f, fileToIgnore)
		})
		if shouldSkip {
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

func (g *Git) FindExistingPR(branchName string, action environment.Action, sourceGeneration bool) (string, *github.PullRequest, error) {
	if g.repo == nil {
		return "", nil, fmt.Errorf("repo not cloned")
	}

	prs, _, err := g.client.PullRequests.List(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), nil)
	if err != nil {
		return "", nil, fmt.Errorf("error getting pull requests: %w", err)
	}

	var prTitle string
	if action == environment.ActionRunWorkflow || action == environment.ActionFinalize {
		prTitle = getGenPRTitlePrefix()
		if sourceGeneration {
			prTitle = getGenSourcesTitlePrefix()
		}
	} else if action == environment.ActionFinalize || action == environment.ActionFinalizeSuggestion {
		prTitle = getSuggestPRTitlePrefix()
	}

	if environment.IsDocsGeneration() {
		prTitle = getDocsPRTitlePrefix()
	}

	for _, p := range prs {
		if strings.HasPrefix(p.GetTitle(), prTitle) {
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

func (g *Git) FindAndCheckoutBranch(branchName string) (string, error) {
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

func (g *Git) Reset(args ...string) error {
	// We execute this manually because go-git doesn't support all the options we need
	args = append([]string{"reset"}, args...)

	logging.Info("Running git  %s", strings.Join(args, " "))

	cmd := exec.Command("git", args...)
	cmd.Dir = filepath.Join(environment.GetWorkspace(), "repo", environment.GetWorkingDirectory())
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error running `git %s`: %w %s", strings.Join(args, " "), err, string(output))
	}

	return nil
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
		defaultBranch, err := g.GetCurrentBranch()
		if err != nil {
			// Swallow this error for now. Functionality will be unchanged from previous behavior if it fails
			logging.Info("failed to get default branch: %s", err.Error())
		}

		branchName, err := g.FindAndCheckoutBranch(branchName)
		if err != nil {
			return "", err
		}

		origin := fmt.Sprintf("origin/%s", defaultBranch)
		if err = g.Reset("--hard", origin); err != nil {
			// Swallow this error for now. Functionality will be unchanged from previous behavior if it fails
			logging.Info("failed to reset branch: %s", err.Error())
		}

		return branchName, nil
	}

	if action == environment.ActionRunWorkflow {
		branchName = fmt.Sprintf("speakeasy-sdk-regen-%d", time.Now().Unix())
	} else if action == environment.ActionSuggest {
		branchName = fmt.Sprintf("speakeasy-openapi-suggestion-%d", time.Now().Unix())
	} else if environment.IsDocsGeneration() {
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

func (g *Git) GetCurrentBranch() (string, error) {
	if g.repo == nil {
		return "", fmt.Errorf("repo not cloned")
	}

	head, err := g.repo.Head()
	if err != nil {
		return "", fmt.Errorf("error getting head: %w", err)
	}

	return head.Name().Short(), nil
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

func (g *Git) CommitAndPush(openAPIDocVersion, speakeasyVersion, doc string, action environment.Action, sourcesOnly bool) (string, error) {
	if g.repo == nil {
		return "", fmt.Errorf("repo not cloned")
	}

	// In test mode do not commit and push, just move forward
	if environment.IsTestMode() {
		return "", nil
	}

	w, err := g.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("error getting worktree: %w", err)
	}

	logging.Info("Commit and pushing changes to git")

	if err := g.Add("."); err != nil {
		return "", fmt.Errorf("error adding changes: %w", err)
	}

	var commitMessage string
	if action == environment.ActionRunWorkflow {
		commitMessage = fmt.Sprintf("ci: regenerated with OpenAPI Doc %s, Speakeasy CLI %s", openAPIDocVersion, speakeasyVersion)
		if sourcesOnly {
			commitMessage = fmt.Sprintf("ci: regenerated with Speakeasy CLI %s", speakeasyVersion)
		}
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
		Auth:  getGithubAuth(g.accessToken),
		Force: true, // This is necessary because at the beginning of the workflow we reset the branch
	}); err != nil {
		return "", pushErr(err)
	}

	return commitHash.String(), nil
}

func (g *Git) Add(arg string) error {
	// We execute this manually because go-git doesn't properly support gitignore
	cmd := exec.Command("git", "add", arg)
	cmd.Dir = filepath.Join(environment.GetWorkspace(), "repo", environment.GetWorkingDirectory())
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error running `git add %s`: %w %s", arg, err, string(output))
	}

	return nil
}

type PRInfo struct {
	BranchName           string
	ReleaseInfo          *releases.ReleasesInfo
	PreviousGenVersion   string
	PR                   *github.PullRequest
	SourceGeneration     bool
	LintingReportURL     string
	ChangesReportURL     string
	OpenAPIChangeSummary string
	VersioningInfo       versionbumps.VersioningInfo
}

func (g *Git) CreateOrUpdatePR(info PRInfo) (*github.PullRequest, error) {
	var changelog string
	var err error

	labelTypes := g.UpsertLabelTypes(context.Background())

	var previousGenVersions []string

	if info.PreviousGenVersion != "" {
		previousGenVersions = strings.Split(info.PreviousGenVersion, ";")
	}

	// Deprecated -- kept around for old CLI versions. VersioningReport is newer pathway
	if info.ReleaseInfo != nil && info.VersioningInfo.VersionReport == nil {
		for language, genInfo := range info.ReleaseInfo.LanguagesGenerated {
			genPath := path.Join(environment.GetWorkspace(), "repo", genInfo.Path)

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

			versionChangelog, err := cli.GetChangelog(language, info.ReleaseInfo.GenerationVersion, "", targetVersions, previousVersions)
			if err != nil {
				return nil, fmt.Errorf("failed to get changelog for language %s: %w", language, err)
			}

			changelog += fmt.Sprintf("\n\n## %s CHANGELOG\n\n%s", strings.ToUpper(language), versionChangelog)
		}

		if changelog == "" {
			// Not using granular version, grab old changelog
			changelog, err = cli.GetChangelog("", info.ReleaseInfo.GenerationVersion, info.PreviousGenVersion, nil, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get changelog: %w", err)
			}
			if strings.TrimSpace(changelog) != "" {
				changelog = "\n\n\n## CHANGELOG\n\n" + changelog
			}
		} else {
			changelog = "\n" + changelog
		}
	}

	title := getGenPRTitlePrefix()
	if environment.IsDocsGeneration() {
		title = getDocsPRTitlePrefix()
	} else if info.SourceGeneration {
		title = getGenSourcesTitlePrefix()
	}

	suffix, labelBumpType, labels := PRVersionMetadata(info.VersioningInfo.VersionReport, labelTypes)
	title += suffix

	body := ""

	if info.LintingReportURL != "" || info.ChangesReportURL != "" {
		body += fmt.Sprintf(`> [!IMPORTANT]
`)
	}

	if info.LintingReportURL != "" {
		body += fmt.Sprintf(`> Linting report available at: <%s>
`, info.LintingReportURL)
	}

	if info.ChangesReportURL != "" {
		body += fmt.Sprintf(`> OpenAPI Change report available at: <%s>
`, info.ChangesReportURL)
	}

	if info.SourceGeneration {
		body += "Update of compiled sources"
	} else {
		body += fmt.Sprintf(`# SDK update
Based on:
- OpenAPI Doc %s %s
- Speakeasy CLI %s (%s) https://github.com/speakeasy-api/speakeasy
`, info.ReleaseInfo.DocVersion, info.ReleaseInfo.DocLocation, info.ReleaseInfo.SpeakeasyVersion, info.ReleaseInfo.GenerationVersion)
	}

	if info.VersioningInfo.VersionReport != nil {

		// We keep track of explicit bump types and whether that bump type is manual or automated in the PR body
		if labelBumpType != nil && *labelBumpType != versioning.BumpCustom && *labelBumpType != versioning.BumpNone {
			// be very careful if changing this it critically aligns with a regex in parseBumpFromPRBody
			versionBumpMsg := "Version Bump Type: " + fmt.Sprintf("[%s]", string(*labelBumpType)) + " - "
			if info.VersioningInfo.ManualBump {
				versionBumpMsg += string(versionbumps.BumpMethodManual) + " (manual)"
				// if manual we bold the message
				versionBumpMsg = "**" + versionBumpMsg + "**"
				versionBumpMsg += fmt.Sprintf("\n\nThis PR will stay on the current version until the %s label is removed and/or modified.", string(*labelBumpType))
			} else {
				versionBumpMsg += string(versionbumps.BumpMethodAutomated) + " (automated)"
			}
			body += fmt.Sprintf(`## Versioning

%s
`, versionBumpMsg)
		}

		body += stripCodes(info.VersioningInfo.VersionReport.GetMarkdownSection())

	} else {
		if len(info.OpenAPIChangeSummary) > 0 {
			body += fmt.Sprintf(`## OpenAPI Change Summary

%s
`, stripCodes(info.OpenAPIChangeSummary))
		}

		body += changelog
	}

	const maxBodyLength = 65536

	if len(body) > maxBodyLength {
		body = body[:maxBodyLength-3] + "..."
	}

	prClient := g.client
	if providedPat := os.Getenv("ACTION_GITHUB_PATH"); providedPat != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: providedPat},
		)
		tc := oauth2.NewClient(context.Background(), ts)
		prClient = github.NewClient(tc)
	}

	if info.PR != nil {
		logging.Info("Updating PR")

		info.PR.Body = github.String(body)
		info.PR.Title = &title
		info.PR, _, err = prClient.PullRequests.Edit(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), info.PR.GetNumber(), info.PR)
		// Set labels MUST always follow updating the PR
		g.setPRLabels(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), info.PR.GetNumber(), labelTypes, info.PR.Labels, labels)
		if err != nil {
			return nil, fmt.Errorf("failed to update PR: %w", err)
		}
	} else {
		logging.Info("Creating PR")

		info.PR, _, err = prClient.PullRequests.Create(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), &github.NewPullRequest{
			Title:               github.String(title),
			Body:                github.String(body),
			Head:                github.String(info.BranchName),
			Base:                github.String(environment.GetRef()),
			MaintainerCanModify: github.Bool(true),
		})
		if err != nil {
			messageSuffix := ""
			if strings.Contains(err.Error(), "GitHub Actions is not permitted to create or approve pull requests") {
				messageSuffix += "\nNavigate to Settings > Actions > Workflow permissions and ensure that allow GitHub Actions to create and approve pull requests is checked. For more information see https://www.speakeasy.com/docs/advanced-setup/github-setup"
			}
			return nil, fmt.Errorf("failed to create PR: %w%s", err, messageSuffix)
		} else if info.PR != nil && len(labels) > 0 {
			g.setPRLabels(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), info.PR.GetNumber(), labelTypes, info.PR.Labels, labels)
		}
	}

	url := ""
	if info.PR.URL != nil {
		url = *info.PR.HTMLURL
	}

	logging.Info("PR: %s", url)

	return info.PR, nil
}

func notEquivalent(desired []*github.Label, actual []*github.Label) bool {
	desiredByName := make(map[string]bool)
	for _, label := range desired {
		desiredByName[label.GetName()] = true
	}
	if len(desiredByName) != len(actual) {
		return true
	}
	for _, label := range actual {
		_, ok := desiredByName[label.GetName()]
		if !ok {
			return true
		}
	}
	return false
}

func reapplySuffix(title *string, suffix string) *string {
	if title == nil {
		return nil
	}
	split := strings.Split(*title, "🐝")
	if len(split) < 2 {
		return title
	}
	// take first two sections
	*title = strings.Join(split[:2], "🐝") + suffix
	return title
}

func stripCodes(str string) string {
	const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"
	var re = regexp.MustCompile(ansi)
	return re.ReplaceAllString(str, "")
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
			Title:               github.String(getDocsPRTitlePrefix()),
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

	fmt.Println(body, branchName, getSuggestPRTitlePrefix(), environment.GetRef())

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
		return "", pushErr(err)
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

func (g *Git) GetReleaseByTag(ctx context.Context, tag string) (*github.RepositoryRelease, *github.Response, error) {
	return g.client.Repositories.GetReleaseByTag(ctx, os.Getenv("GITHUB_REPOSITORY_OWNER"), getRepo(), tag)
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

func ArtifactMatchesRelease(assetName, goos, goarch string) bool {
	assetNameLower := strings.ToLower(assetName)

	// Ignore non-zip files
	if !strings.HasSuffix(assetNameLower, ".zip") {
		return false
	}

	// Remove the .zip suffix and split into segments
	assetNameLower = strings.ToLower(strings.TrimSuffix(assetNameLower, ".zip"))
	segments := strings.Split(assetNameLower, "_")

	// Ensure we have at least 3 segments (name_os_arch)
	if len(segments) < 3 {
		return false
	}

	// Check if the second segment (OS) matches
	if segments[1] != goos {
		return false
	}

	// Check if the third segment (arch) is a prefix of goarch
	// This handles cases like "arm64" matching "arm64/v8"
	return strings.HasPrefix(goarch, segments[2])
}

func getDownloadLinkFromReleases(releases []*github.RepositoryRelease, version string) (*string, *string) {
	defaultAsset := "speakeasy_linux_amd64.zip"
	var defaultDownloadUrl *string
	var defaultTagName *string

	for _, release := range releases {
		for _, asset := range release.Assets {
			if version == "latest" || version == release.GetTagName() {
				downloadUrl := asset.GetBrowserDownloadURL()
				// default one is linux/amd64 which represents ubuntu-latest github actions
				if asset.GetName() == defaultAsset {
					defaultDownloadUrl = &downloadUrl
					defaultTagName = release.TagName
				}

				if ArtifactMatchesRelease(asset.GetName(), strings.ToLower(runtime.GOOS), strings.ToLower(runtime.GOARCH)) {
					return &downloadUrl, release.TagName
				}
			}
		}
	}

	return defaultDownloadUrl, defaultTagName
}

func (g *Git) GetCommittedFilesFromBaseBranch() ([]string, error) {
	baseBranch := "main" // Default base branch
	if os.Getenv("GITHUB_BASE_REF") != "" {
		baseBranch = os.Getenv("GITHUB_BASE_REF")
	}

	// Read event payload
	path := environment.GetWorkflowEventPayloadPath()
	if path == "" {
		return nil, fmt.Errorf("no workflow event payload path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow event payload: %w", err)
	}

	var payload struct {
		After string `json:"after"` // PR Head commit
	}

	fmt.Println("Data: ", string(data))

	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow event payload: %w", err)
	}

	if payload.After == "" {
		return nil, fmt.Errorf("missing commit hash in workflow event payload")
	}

	// ✅ Get the latest commit from main (instead of using Before)
	baseBranchRef, err := g.repo.Reference(plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", baseBranch)), true)
	if err != nil {
		return nil, fmt.Errorf("failed to get base branch reference: %w", err)
	}

	baseCommit, err := g.repo.CommitObject(baseBranchRef.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get base commit object: %w", err)
	}

	// ✅ Get the PR commit (After commit)
	currentCommit, err := g.repo.CommitObject(plumbing.NewHash(payload.After))
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch commit object: %w", err)
	}

	// Get tree objects
	baseTree, err := baseCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get base branch tree: %w", err)
	}

	currentTree, err := currentCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch tree: %w", err)
	}

	// Compute diff
	changes, err := baseTree.Diff(currentTree)
	if err != nil {
		return nil, fmt.Errorf("failed to get diff between base and current branch: %w", err)
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

	logging.Info("Found %d files changed from base branch %s", len(files), baseBranch)

	return files, nil
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

	fmt.Println("Data: ", string(data))

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
	speakeasyGenSpecsTitle  = "chore: 🐝 Update Specs - "
	speakeasySuggestPRTitle = "chore: 🐝 Suggest OpenAPI changes - "
	speakeasyDocsPRTitle    = "chore: 🐝 Update SDK Docs - "
)

func getGenPRTitlePrefix() string {
	title := speakeasyGenPRTitle + environment.GetWorkflowName()
	if environment.SpecifiedTarget() != "" && !strings.Contains(title, strings.ToUpper(environment.SpecifiedTarget())) {
		title += " " + strings.ToUpper(environment.SpecifiedTarget())
	}
	return title
}

func getGenSourcesTitlePrefix() string {
	return speakeasyGenSpecsTitle + environment.GetWorkflowName()
}

func getDocsPRTitlePrefix() string {
	return speakeasyDocsPRTitle + environment.GetWorkflowName()
}

func getSuggestPRTitlePrefix() string {
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

func pushErr(err error) error {
	if err != nil {
		if strings.Contains(err.Error(), "protected branch hook declined") {
			return fmt.Errorf("error pushing changes: %w\nThis is likely due to a branch protection rule. Please ensure that the branch is not protected (repo > settings > branches).", err)
		}
		return fmt.Errorf("error pushing changes: %w", err)
	}
	return nil
}
