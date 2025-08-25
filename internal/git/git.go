package git

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	prs, _, err := g.client.PullRequests.List(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), nil)
	if err != nil {
		return "", nil, fmt.Errorf("error getting pull requests: %w", err)
	}

	// Get source branch for context-aware PR matching
	sourceBranch := environment.GetSourceBranch()
	isMainBranch := environment.IsMainBranch(sourceBranch)

	var prTitle string
	switch action {
	case environment.ActionRunWorkflow, environment.ActionFinalize:
		prTitle = getGenPRTitlePrefix()
		if sourceGeneration {
			prTitle = getGenSourcesTitlePrefix()
		}
	case environment.ActionFinalizeSuggestion:
		prTitle = getSuggestPRTitlePrefix()
	}

	if environment.IsDocsGeneration() {
		prTitle = getDocsPRTitlePrefix()
	}

	// For feature branches, include source branch context in title matching
	if !isMainBranch {
		sanitizedSourceBranch := environment.SanitizeBranchName(sourceBranch)
		prTitle = prTitle + " [" + sanitizedSourceBranch + "]"
	}

	for _, p := range prs {
		if strings.HasPrefix(p.GetTitle(), prTitle) {
			logging.Info("Found existing PR %s", *p.Title)

			if branchName != "" && p.GetHead().GetRef() != branchName {
				return "", nil, fmt.Errorf("existing PR has different branch name: %s than expected: %s", p.GetHead().GetRef(), branchName)
			}

			// For feature branches, also verify the PR targets the correct base branch
			if !isMainBranch {
				expectedBaseBranch := environment.GetTargetBaseBranch()
				actualBaseBranch := p.GetBase().GetRef()

				// Handle the case where GetTargetBaseBranch returns a full ref
				if strings.HasPrefix(expectedBaseBranch, "refs/") {
					expectedBaseBranch = strings.TrimPrefix(expectedBaseBranch, "refs/heads/")
				}

				if actualBaseBranch != expectedBaseBranch {
					logging.Info("Found PR with matching title but wrong base branch: expected %s, got %s", expectedBaseBranch, actualBaseBranch)
					continue
				}
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

	// Get source branch for context-aware branch naming
	sourceBranch := environment.GetSourceBranch()
	isMainBranch := environment.IsMainBranch(sourceBranch)
	timestamp := time.Now().Unix()

	if action == environment.ActionRunWorkflow {
		if isMainBranch {
			// Maintain backward compatibility for main/master branches
			branchName = fmt.Sprintf("speakeasy-sdk-regen-%d", timestamp)
		} else {
			// Include source branch context for feature branches
			sanitizedSourceBranch := environment.SanitizeBranchName(sourceBranch)
			branchName = fmt.Sprintf("speakeasy-sdk-regen-%s-%d", sanitizedSourceBranch, timestamp)
		}
	} else if action == environment.ActionSuggest {
		if isMainBranch {
			branchName = fmt.Sprintf("speakeasy-openapi-suggestion-%d", timestamp)
		} else {
			sanitizedSourceBranch := environment.SanitizeBranchName(sourceBranch)
			branchName = fmt.Sprintf("speakeasy-openapi-suggestion-%s-%d", sanitizedSourceBranch, timestamp)
		}
	} else if environment.IsDocsGeneration() {
		if isMainBranch {
			branchName = fmt.Sprintf("speakeasy-sdk-docs-regen-%d", timestamp)
		} else {
			sanitizedSourceBranch := environment.SanitizeBranchName(sourceBranch)
			branchName = fmt.Sprintf("speakeasy-sdk-docs-regen-%s-%d", sanitizedSourceBranch, timestamp)
		}
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

func (g *Git) CommitAndPush(openAPIDocVersion, speakeasyVersion, doc string, action environment.Action, sourcesOnly bool, mergedVersionReport *versioning.MergedVersionReport) (string, error) {
	// Log git location
	if gitPath, err := exec.LookPath("git"); err != nil {
		fmt.Printf("git not found: %v\n", err)
	} else {
		fmt.Printf("git: %s\n", gitPath)
	}

	if mergedVersionReport == nil {
		logging.Info("mergedVersionReport is nil")
	} else if mergedVersionReport.GetCommitMarkdownSection() == "" {
		logging.Info("mergedVersionReport.GetCommitMarkdownSection is empty ")
	}

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
	logging.Info("INPUT_ENABLE_SDK_CHANGELOG is %s", environment.GetSDKChangelog())

	var commitMessage string
	switch action {
	case environment.ActionRunWorkflow:
		commitMessage = fmt.Sprintf("ci: regenerated with OpenAPI Doc %s, Speakeasy CLI %s", openAPIDocVersion, speakeasyVersion)
		if sourcesOnly {
			commitMessage = fmt.Sprintf("ci: regenerated with Speakeasy CLI %s", speakeasyVersion)
		} else if environment.GetSDKChangelog() == "true" && mergedVersionReport != nil && mergedVersionReport.GetCommitMarkdownSection() != "" {
			// For clients using older cli with new sdk-action, GetCommitMarkdownSection would be empty so we will use the old commit message
			commitMessage = mergedVersionReport.GetCommitMarkdownSection()
		}
	case environment.ActionSuggest:
		commitMessage = fmt.Sprintf("ci: suggestions for OpenAPI doc %s", doc)
	default:
		return "", errors.New("invalid action")
	}

	// Create commit message
	if !environment.GetSignedCommits() {
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

	branch, err := g.GetCurrentBranch()
	if err != nil {
		return "", fmt.Errorf("error getting current branch: %w", err)
	}

	// Get status of changed files
	status, err := w.Status()
	if err != nil {
		return "", fmt.Errorf("error getting status for branch: %w", err)
	}

	// Get repo head commit
	head, err := g.repo.Head()
	if err != nil {
		return "", fmt.Errorf("error getting repo head commit: %w", err)
	}

	// Create reference on remote if it doesn't exist
	ref, err := g.getOrCreateRef(string(head.Name()))
	if err != nil {
		return "", fmt.Errorf("error getting reference: %w", err)
	}

	// Create new tree with SHA of last commit
	tree, err := g.createAndPushTree(ref, status)
	if err != nil {
		return "", fmt.Errorf("error creating new tree: %w", err)
	}

	_, githubRepoLocation := g.getRepoMetadata()
	owner, repo := g.getOwnerAndRepo(githubRepoLocation)

	// Get parent commit
	parentCommit, _, err := g.client.Git.GetCommit(context.Background(), owner, repo, *ref.Object.SHA)
	if err != nil {
		return "", fmt.Errorf("error getting parent commit: %w", err)
	}

	// Commit changes
	commitResult, _, err := g.client.Git.CreateCommit(context.Background(), owner, repo, &github.Commit{
		Message: github.String(commitMessage),
		Tree:    &github.Tree{SHA: tree.SHA},
		Parents: []*github.Commit{parentCommit},
	}, &github.CreateCommitOptions{})
	if err != nil {
		return "", fmt.Errorf("error committing changes: %w", err)
	}

	// Update reference
	newRef := &github.Reference{
		Ref:    github.String("refs/heads/" + branch),
		Object: &github.GitObject{SHA: commitResult.SHA},
	}
	g.client.Git.UpdateRef(context.Background(), owner, repo, newRef, true)

	return *commitResult.SHA, nil
}

// getOrCreateRef returns the commit branch reference object if it exists or creates it
// from the base branch before returning it.
func (g *Git) getOrCreateRef(commitRef string) (ref *github.Reference, err error) {
	_, githubRepoLocation := g.getRepoMetadata()
	owner, repo := g.getOwnerAndRepo(githubRepoLocation)
	environmentRef := environment.GetRef()

	if ref, _, err = g.client.Git.GetRef(context.Background(), owner, repo, commitRef); err == nil {
		return ref, nil
	}

	// We consider that an error means the branch has not been found and needs to
	// be created.
	if commitRef == environmentRef {
		return nil, errors.New("the commit branch does not exist but `-base-branch` is the same as `-commit-branch`")
	}

	var baseRef *github.Reference
	if baseRef, _, err = g.client.Git.GetRef(context.Background(), owner, repo, environmentRef); err != nil {
		return nil, err
	}

	newRef := &github.Reference{Ref: github.String(commitRef), Object: &github.GitObject{SHA: baseRef.Object.SHA}}
	ref, _, err = g.client.Git.CreateRef(context.Background(), owner, repo, newRef)
	return ref, err
}

// Generates the tree to commit based on the commit reference and source files. If doesn't exist on the remote
// host, it will create and push it.
func (g *Git) createAndPushTree(ref *github.Reference, sourceFiles git.Status) (tree *github.Tree, err error) {
	_, githubRepoLocation := g.getRepoMetadata()
	owner, repo := g.getOwnerAndRepo(githubRepoLocation)
	w, _ := g.repo.Worktree()

	entries := []*github.TreeEntry{}
	for file, fileStatus := range sourceFiles {
		if fileStatus.Staging != git.Unmodified && fileStatus.Staging != git.Untracked && fileStatus.Staging != git.Deleted {
			filePath := w.Filesystem.Join(w.Filesystem.Root(), file)
			content, err := os.ReadFile(filePath)
			if err != nil {
				fmt.Println("Error getting file content", err, filePath)
				return nil, err
			}

			entries = append(entries, &github.TreeEntry{
				Path:    github.String(file),
				Type:    github.String("blob"),
				Content: github.String(string(content)),
				Mode:    github.String("100644"),
			})
		}
	}

	tree, _, err = g.client.Git.CreateTree(context.Background(), owner, repo, *ref.Object.SHA, entries)
	return tree, err
}

func (g *Git) Add(arg string) error {
	fmt.Println("=== git.Add() FUNCTION START - GIT DEBUG ===")
	gitPath, err := exec.LookPath("git")
	if err != nil {
		fmt.Println("Couldn't locate git on system")
		return err
	} else {
		fmt.Printf("Got gitpath %v\n", gitPath)
		
		// Enhanced debugging at the start of Add function
		if info, err := os.Stat(gitPath); err == nil {
			fmt.Printf("git.Add() - git file size: %d bytes\n", info.Size())
		}
		if output, err := exec.Command(gitPath, "--version").CombinedOutput(); err == nil {
			fmt.Printf("git.Add() - git version: %s\n", strings.TrimSpace(string(output)))
		} else {
			fmt.Printf("git.Add() - error getting git version: %v\n", err)
		}
		
		// Check if gitPath is a file or symlink
		fileInfo, err := os.Lstat(gitPath)
		if err != nil {
			fmt.Printf("Error getting file info for git binary: %v\n", err)
		} else if fileInfo.Mode()&os.ModeSymlink != 0 {
			// It's a symlink, get the target
			target, err := os.Readlink(gitPath)
			if err != nil {
				fmt.Printf("Git binary is a symlink but couldn't read target: %v\n", err)
			} else {
				fmt.Printf("Git binary is a symlink pointing to: %s\n", target)
			}
		} else {
			fmt.Printf("Git binary is a regular file\n")
		}
		
		// Compare checksums between CLI_LOCATION and gitPath
		cliLocation := os.Getenv("SPEAKEASY_CLI_LOCATION")
		if cliLocation != "" {
			// Check if CLI_LOCATION file exists and get its info
			cliInfo, err := os.Stat(cliLocation)
			if err != nil {
				fmt.Printf("Error getting CLI_LOCATION file info: %v\n", err)
			} else {
				fmt.Printf("CLI_LOCATION file exists, size: %d bytes\n", cliInfo.Size())
			}
			
			gitChecksum, err := calculateFileChecksum(gitPath)
			if err != nil {
				fmt.Printf("Error calculating checksum for git binary: %v\n", err)
			} else {
				cliChecksum, err := calculateFileChecksum(cliLocation)
				if err != nil {
					fmt.Printf("Error calculating checksum for CLI_LOCATION: %v\n", err)
				} else {
					fmt.Printf("Git binary checksum: %s\n", gitChecksum)
					fmt.Printf("CLI_LOCATION checksum: %s\n", cliChecksum)
					if gitChecksum == cliChecksum {
						fmt.Printf("Checksums match - same binary\n")
					} else {
						fmt.Printf("Checksums differ - different binaries\n")
					}
				}
			}
			
			// Check version output from both binaries
			gitVersion, err := getBinaryVersion(gitPath)
			if err != nil {
				fmt.Printf("Error getting version from git binary: %v\n", err)
			} else {
				fmt.Printf("Git binary version output: %s\n", gitVersion)
			}
			
			cliVersion, err := getBinaryVersion(cliLocation)
			if err != nil {
				fmt.Printf("Error getting version from CLI_LOCATION: %v\n", err)
				// Try to get more details about the failure
				cmd := exec.Command(cliLocation, "--version")
				output, cmdErr := cmd.CombinedOutput()
				fmt.Printf("CLI_LOCATION command output: %s\n", string(output))
				fmt.Printf("CLI_LOCATION command error: %v\n", cmdErr)
			} else {
				fmt.Printf("CLI_LOCATION version output: %s\n", cliVersion)
			}
			
			// Check what 'which git' returns vs our gitPath
			whichCmd := exec.Command("which", "git")
			whichOutput, err := whichCmd.CombinedOutput()
			if err != nil {
				fmt.Printf("Error running 'which git': %v\n", err)
			} else {
				whichPath := strings.TrimSpace(string(whichOutput))
				fmt.Printf("'which git' returns: %s\n", whichPath)
				fmt.Printf("exec.LookPath found: %s\n", gitPath)
				if whichPath == gitPath {
					fmt.Printf("Paths match\n")
				} else {
					fmt.Printf("Paths differ!\n")
				}
			}
			
			// Check file permissions and ownership
			gitInfo, err := os.Stat(gitPath)
			if err != nil {
				fmt.Printf("Error getting git binary file info: %v\n", err)
			} else {
				fmt.Printf("Git binary file size: %d bytes, mode: %v\n", gitInfo.Size(), gitInfo.Mode())
			}
			
		} else {
			fmt.Printf("SPEAKEASY_CLI_LOCATION not set, skipping checksum comparison\n")
		}
	}
	// We execute this manually because go-git doesn't properly support gitignore
	cmd := exec.Command(gitPath, "--help")
	cmd.Dir = filepath.Join(environment.GetWorkspace(), "repo", environment.GetWorkingDirectory())
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	fmt.Printf("combined output: %s\n", string(output))
	if err != nil {
		return fmt.Errorf("error running `git add %s`: %w %s", arg, err, string(output))
	}

	return nil
}

func calculateFileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func getBinaryVersion(binaryPath string) (string, error) {
	cmd := exec.Command(binaryPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get version: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
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

func (g *Git) getRepoMetadata() (string, string) {
	githubURL := os.Getenv("GITHUB_SERVER_URL")
	githubRepoLocation := os.Getenv("GITHUB_REPOSITORY")

	return githubURL, githubRepoLocation
}

func (g *Git) getOwnerAndRepo(githubRepoLocation string) (string, string) {
	ownerAndRepo := strings.Split(githubRepoLocation, "/")

	return ownerAndRepo[0], ownerAndRepo[1]
}

func (g *Git) CreateOrUpdatePR(info PRInfo) (*github.PullRequest, error) {
	logging.Info("Starting: Create or Update PR")
	labelTypes := g.UpsertLabelTypes(context.Background())
	var changelog string
	var err error
	body := ""
	var previousGenVersions []string
	var title string
	if info.PreviousGenVersion != "" {
		previousGenVersions = strings.Split(info.PreviousGenVersion, ";")
	}

	// Deprecated -- kept around for old CLI versions. VersioningReport is newer pathway
	if info.ReleaseInfo != nil && info.VersioningInfo.VersionReport == nil {
		changelog, err = g.generateGeneratorChangelogForOldCLIVersions(info, previousGenVersions, changelog)
		if err != nil {
			return nil, err
		}
	}

	// We will use the old PR body if the INPUT_ENABLE_SDK_CHANGELOG env is not set or set to false
	// We will use the new PR body if INPUT_ENABLE_SDK_CHANGELOG is set to true.
	// Backwards compatible: If a client uses new sdk-action with old cli we will not get new changelog body
	title, body = g.generatePRTitleAndBody(info, labelTypes, changelog)

	_, _, labels := PRVersionMetadata(info.VersioningInfo.VersionReport, labelTypes)

	const maxBodyLength = 65536

	if len(body) > maxBodyLength {
		body = body[:maxBodyLength-3] + "..."
	}

	prClient := g.client
	if providedPat := os.Getenv("PR_CREATION_PAT"); providedPat != "" {
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
		info.PR, _, err = prClient.PullRequests.Edit(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), info.PR.GetNumber(), info.PR)
		// Set labels MUST always follow updating the PR
		g.setPRLabels(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), info.PR.GetNumber(), labelTypes, info.PR.Labels, labels)
		if err != nil {
			return nil, fmt.Errorf("failed to update PR: %w", err)
		}
	} else {
		logging.Info("Creating PR")

		// Use source-branch-aware target base branch
		targetBaseBranch := environment.GetTargetBaseBranch()
		// Handle the case where GetTargetBaseBranch returns a full ref
		if strings.HasPrefix(targetBaseBranch, "refs/") {
			targetBaseBranch = strings.TrimPrefix(targetBaseBranch, "refs/heads/")
		}

		info.PR, _, err = prClient.PullRequests.Create(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), &github.NewPullRequest{
			Title:               github.String(title),
			Body:                github.String(body),
			Head:                github.String(info.BranchName),
			Base:                github.String(targetBaseBranch),
			MaintainerCanModify: github.Bool(true),
		})
		if err != nil {
			messageSuffix := ""
			if strings.Contains(err.Error(), "GitHub Actions is not permitted to create or approve pull requests") {
				messageSuffix += "\nNavigate to Settings > Actions > Workflow permissions and ensure that allow GitHub Actions to create and approve pull requests is checked. For more information see https://www.speakeasy.com/docs/advanced-setup/github-setup"
			}
			return nil, fmt.Errorf("failed to create PR: %w%s", err, messageSuffix)
		} else if info.PR != nil && len(labels) > 0 {
			g.setPRLabels(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), info.PR.GetNumber(), labelTypes, info.PR.Labels, labels)
		}
	}

	url := ""
	if info.PR.URL != nil {
		url = *info.PR.HTMLURL
	}

	logging.Info("PR: %s", url)

	return info.PR, nil
}

// --- Helper function for old PR title/body generation ---
func (g *Git) generatePRTitleAndBody(info PRInfo, labelTypes map[string]github.Label, changelog string) (string, string) {
	body := ""
	title := getGenPRTitlePrefix()
	if environment.IsDocsGeneration() {
		title = getDocsPRTitlePrefix()
	} else if info.SourceGeneration {
		title = getGenSourcesTitlePrefix()
	}

	// Add source branch context for feature branches
	sourceBranch := environment.GetSourceBranch()
	isMainBranch := environment.IsMainBranch(sourceBranch)
	if !isMainBranch {
		sanitizedSourceBranch := environment.SanitizeBranchName(sourceBranch)
		title = title + " [" + sanitizedSourceBranch + "]"
	}

	suffix, labelBumpType, _ := PRVersionMetadata(info.VersioningInfo.VersionReport, labelTypes)
	title += suffix

	if info.LintingReportURL != "" || info.ChangesReportURL != "" {
		body += `> [!IMPORTANT]
`
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
		body += "# SDK update\n"
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

		// New changelog is added here if speakeasy cli added a PR report
		// Text inserted here is controlled entirely by the speakeasy cli.
		// We want to move in a direction where the speakeasy CLI controls the messaging entirely
		body += stripCodes(info.VersioningInfo.VersionReport.GetMarkdownSection())

	} else {
		if len(info.OpenAPIChangeSummary) > 0 {
			body += fmt.Sprintf(`## OpenAPI Change Summary

%s
`, stripCodes(info.OpenAPIChangeSummary))
		}

		body += changelog
	}

	if !info.SourceGeneration {
		body += fmt.Sprintf(`
Based on [Speakeasy CLI](https://github.com/speakeasy-api/speakeasy) %s
`, info.ReleaseInfo.SpeakeasyVersion)
	}

	return title, body
}

// --- Helper function for changelog generation for old CLI versions ---
func (g *Git) generateGeneratorChangelogForOldCLIVersions(info PRInfo, previousGenVersions []string, changelog string) (string, error) {
	for language, genInfo := range info.ReleaseInfo.LanguagesGenerated {
		genPath := path.Join(environment.GetWorkspace(), "repo", genInfo.Path)

		var targetVersions map[string]string

		cfg, err := genConfig.Load(genPath)
		if err != nil {
			logging.Error("failed to load gen config for retrieving granular versions for changelog at path %s: %v", genPath, err)
			continue
		} else {
			ok := false
			targetVersions, ok = cfg.LockFile.Features[language]
			if !ok {
				logging.Error("failed to find language %s in gen config for retrieving granular versions for changelog at path %s", language, genPath)
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
			return changelog, fmt.Errorf("failed to get changelog for language %s: %w", language, err)
		}
		changelog += fmt.Sprintf("\n\n## Generator Changelog\n\n%s", versionChangelog)
	}

	if changelog == "" {
		// Not using granular version, grab old changelog
		var err error
		changelog, err = cli.GetChangelog("", info.ReleaseInfo.GenerationVersion, info.PreviousGenVersion, nil, nil)
		if err != nil {
			return changelog, fmt.Errorf("failed to get changelog: %w", err)
		}
		if strings.TrimSpace(changelog) != "" {
			changelog = "\n\n\n## Changelog\n\n" + changelog
		}
	} else {
		changelog = "\n" + changelog
	}
	return changelog, nil
}

func stripCodes(str string) string {
	const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"
	re := regexp.MustCompile(ansi)
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

	// Generate source-branch-aware title
	title := getDocsPRTitlePrefix()
	sourceBranch := environment.GetSourceBranch()
	isMainBranch := environment.IsMainBranch(sourceBranch)
	if !isMainBranch {
		sanitizedSourceBranch := environment.SanitizeBranchName(sourceBranch)
		title = title + " [" + sanitizedSourceBranch + "]"
	}

	if pr != nil {
		logging.Info("Updating PR")

		pr.Body = github.String(body)
		pr.Title = github.String(title)
		pr, _, err = g.client.PullRequests.Edit(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), pr.GetNumber(), pr)
		if err != nil {
			return fmt.Errorf("failed to update PR: %w", err)
		}
	} else {
		logging.Info("Creating PR")

		// Use source-branch-aware target base branch
		targetBaseBranch := environment.GetTargetBaseBranch()
		// Handle the case where GetTargetBaseBranch returns a full ref
		if strings.HasPrefix(targetBaseBranch, "refs/") {
			targetBaseBranch = strings.TrimPrefix(targetBaseBranch, "refs/heads/")
		}

		pr, _, err = g.client.PullRequests.Create(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), &github.NewPullRequest{
			Title:               github.String(title),
			Body:                github.String(body),
			Head:                github.String(branchName),
			Base:                github.String(targetBaseBranch),
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

	// Generate source-branch-aware title
	title := getSuggestPRTitlePrefix()
	sourceBranch := environment.GetSourceBranch()
	isMainBranch := environment.IsMainBranch(sourceBranch)
	if !isMainBranch {
		sanitizedSourceBranch := environment.SanitizeBranchName(sourceBranch)
		title = title + " [" + sanitizedSourceBranch + "]"
	}

	// Use source-branch-aware target base branch
	targetBaseBranch := environment.GetTargetBaseBranch()
	// Handle the case where GetTargetBaseBranch returns a full ref
	if strings.HasPrefix(targetBaseBranch, "refs/") {
		targetBaseBranch = strings.TrimPrefix(targetBaseBranch, "refs/heads/")
	}

	fmt.Println(body, branchName, title, targetBaseBranch)

	pr, _, err := g.client.PullRequests.Create(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), &github.NewPullRequest{
		Title:               github.String(title),
		Body:                github.String(body),
		Head:                github.String(branchName),
		Base:                github.String(targetBaseBranch),
		MaintainerCanModify: github.Bool(true),
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to create PR: %w", err)
	}

	return pr.Number, pr.GetHead().GetSHA(), nil
}

func (g *Git) WritePRBody(prNumber int, body string) error {
	pr, _, err := g.client.PullRequests.Get(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), prNumber)
	if err != nil {
		return fmt.Errorf("failed to get PR: %w", err)
	}

	pr.Body = github.String(strings.Join([]string{*pr.Body, sanitizeExplanations(body)}, "\n\n"))
	if _, _, err = g.client.PullRequests.Edit(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), prNumber, pr); err != nil {
		return fmt.Errorf("failed to update PR: %w", err)
	}

	return nil
}

func (g *Git) ListIssueComments(prNumber int) ([]*github.IssueComment, error) {
	comments, _, err := g.client.Issues.ListComments(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), prNumber, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR comments: %w", err)
	}

	return comments, nil
}

func (g *Git) DeleteIssueComment(commentID int64) error {
	_, err := g.client.Issues.DeleteComment(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), commentID)
	if err != nil {
		return fmt.Errorf("failed to delete issue comment: %w", err)
	}

	return nil
}

func (g *Git) WritePRComment(prNumber int, fileName, body string, line int) error {
	pr, _, err := g.client.PullRequests.Get(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), prNumber)
	if err != nil {
		return fmt.Errorf("failed to get PR: %w", err)
	}

	_, _, err = g.client.PullRequests.CreateComment(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), prNumber, &github.PullRequestComment{
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

func (g *Git) WriteIssueComment(prNumber int, body string) error {
	comment := &github.IssueComment{
		Body: github.String(sanitizeExplanations(body)),
	}

	_, _, err := g.client.Issues.CreateComment(context.Background(), os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), prNumber, comment)
	if err != nil {
		return fmt.Errorf("failed to create issue comment: %w", err)
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
	return g.client.Repositories.GetReleaseByTag(ctx, os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), tag)
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

func (g *Git) GetChangedFilesForPRorBranch() ([]string, *int, error) {
	ctx := context.Background()
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	if eventPath == "" {
		return nil, nil, fmt.Errorf("no workflow event payload path")
	}

	data, err := os.ReadFile(eventPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read workflow event payload: %w", err)
	}

	var payload struct {
		Number     int `json:"number"`
		Repository struct {
			DefaultBranch string `json:"default_branch"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal workflow event payload: %w", err)
	}

	prNumber := payload.Number
	// This occurs if we come from a non-PR event trigger
	if payload.Number == 0 {
		ref := strings.TrimPrefix(environment.GetRef(), "refs/heads/")
		if ref == "main" || ref == "master" {
			files, err := g.GetCommitedFiles()
			// We just need to get the commit diff since we are not in a separate branch of PR
			return files, nil, err
		}

		opts := &github.PullRequestListOptions{
			Head:  fmt.Sprintf("%s:%s", os.Getenv("GITHUB_REPOSITORY_OWNER"), environment.GetRef()),
			State: "open",
		}

		if prs, _, _ := g.client.PullRequests.List(ctx, os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), opts); len(prs) > 0 {
			prNumber = prs[0].GetNumber()
			os.Setenv("GH_PULL_REQUEST", prs[0].GetURL())
		}

		defaultBranch := "main"
		if payload.Repository.DefaultBranch != "" {
			fmt.Println("Default branch:", payload.Repository.DefaultBranch)
			defaultBranch = payload.Repository.DefaultBranch
		}

		// Get the feature branch reference
		branchRef, err := g.repo.Reference(plumbing.ReferenceName(environment.GetRef()), true)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get feature branch reference: %w", err)
		}

		// Get the latest commit on the feature branch
		latestCommit, err := g.repo.CommitObject(branchRef.Hash())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get latest commit of feature branch: %w", err)
		}

		var files []string
		opt := &github.ListOptions{PerPage: 100} // Fetch 100 files per page (max: 300)
		pageCount := 1                           // Track the number of API pages fetched

		for {
			comparison, resp, err := g.client.Repositories.CompareCommits(
				ctx,
				os.Getenv("GITHUB_REPOSITORY_OWNER"),
				GetRepo(),
				defaultBranch,
				latestCommit.Hash.String(),
				opt,
			)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to compare commits via GitHub API: %w", err)
			}

			// Collect filenames from this page
			for _, file := range comparison.Files {
				files = append(files, file.GetFilename())
			}

			// Check if there are more pages to fetch
			if resp.NextPage == 0 {
				break // No more pages, exit loop
			}

			opt.Page = resp.NextPage
			pageCount++
		}

		logging.Info("Found %d files", len(files))
		return files, &prNumber, nil

	} else {
		prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), prNumber)
		os.Setenv("GH_PULL_REQUEST", prURL)
		opts := &github.ListOptions{PerPage: 100}
		var allFiles []string

		// Fetch all changed files of the PR to determine testing coverage
		for {
			files, resp, err := g.client.PullRequests.ListFiles(ctx, os.Getenv("GITHUB_REPOSITORY_OWNER"), GetRepo(), prNumber, opts)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get changed files: %w", err)
			}

			for _, file := range files {
				allFiles = append(allFiles, file.GetFilename())
			}

			if resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}

		logging.Info("Found %d files", len(allFiles))

		return allFiles, &prNumber, nil
	}
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

func GetRepo() string {
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
