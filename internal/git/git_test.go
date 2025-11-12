package git

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/google/go-github/v63/github"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRepo(t *testing.T) (*git.Repository, billy.Filesystem) {
	t.Helper()

	mfs := memfs.New()

	err := filepath.WalkDir("./fixtures", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		fixture, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fixture.Close()

		f, err := mfs.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(f, fixture)
		if err != nil {
			return err
		}

		return nil
	})
	require.NoError(t, err, "expected to walk the fixture directory")

	storage := memory.NewStorage()
	repo, err := git.Init(storage, mfs)
	require.NoError(t, err, "expected empty repo to be initialized")

	wt, err := repo.Worktree()
	require.NoError(t, err, "expected to get worktree")

	_, err = wt.Add(".")
	require.NoError(t, err, "expected to add all files")

	_, err = wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Unix(0, 0),
		},
	})
	require.NoError(t, err, "expected to commit all files")

	return repo, mfs
}

func TestGit_CheckDirDirty(t *testing.T) {
	repo, mfs := newTestRepo(t)

	f, err := mfs.Create("dirty-file")
	require.NoError(t, err, "expected to create a dirty file")
	defer f.Close()
	fmt.Fprintln(f, "sample content")

	g := Git{repo: repo}
	dirty, str, err := g.CheckDirDirty(".", map[string]string{})
	require.NoError(t, err, "expected to check the directory")

	require.Equal(t, `new file found: []string{"dirty-file"}`, str)
	require.True(t, dirty, "expected the directory to be dirty")
}

func TestGit_CheckDirDirty_IgnoredFiles(t *testing.T) {
	repo, mfs := newTestRepo(t)

	f, err := mfs.Create("workflow.lock")
	require.NoError(t, err, "expected to create a dirty file")
	defer f.Close()
	fmt.Fprintln(f, "sample content")

	g := Git{repo: repo}
	dirty, str, err := g.CheckDirDirty(".", map[string]string{})
	require.NoError(t, err, "expected to check the directory")

	require.Equal(t, "", str, "expected no dirty files reported")
	require.False(t, dirty, "expected the directory to be clean")
}

func TestArtifactMatchesRelease(t *testing.T) {
	tests := []struct {
		name      string
		assetName string
		goos      string
		goarch    string
		want      bool
	}{
		{
			name:      "Linux amd64",
			assetName: "speakeasy_linux_amd64.zip",
			goos:      "linux",
			goarch:    "amd64",
			want:      true,
		},
		{
			name:      "Linux 386",
			assetName: "speakeasy_linux_386.zip",
			goos:      "linux",
			goarch:    "386",
			want:      true,
		},
		{
			name:      "Linux arm64",
			assetName: "speakeasy_linux_arm64.zip",
			goos:      "linux",
			goarch:    "arm64",
			want:      true,
		},
		{
			name:      "macOS amd64",
			assetName: "speakeasy_darwin_amd64.zip",
			goos:      "darwin",
			goarch:    "amd64",
			want:      true,
		},
		{
			name:      "Linux arm64/v8",
			assetName: "speakeasy_linux_arm64.zip",
			goos:      "linux",
			goarch:    "arm64/v8",
			want:      true,
		},
		{
			name:      "macOS arm64",
			assetName: "speakeasy_darwin_arm64.zip",
			goos:      "darwin",
			goarch:    "arm64",
			want:      true,
		},
		{
			name:      "Windows amd64",
			assetName: "speakeasy_windows_amd64.zip",
			goos:      "windows",
			goarch:    "amd64",
			want:      true,
		},
		{
			name:      "Windows 386",
			assetName: "speakeasy_windows_386.zip",
			goos:      "windows",
			goarch:    "386",
			want:      true,
		},
		{
			name:      "Windows arm64",
			assetName: "speakeasy_windows_arm64.zip",
			goos:      "windows",
			goarch:    "arm64",
			want:      true,
		},
		{
			name:      "Mismatched OS",
			assetName: "speakeasy_linux_amd64.zip",
			goos:      "darwin",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Mismatched arch",
			assetName: "speakeasy_linux_amd64.zip",
			goos:      "linux",
			goarch:    "arm64",
			want:      false,
		},
		{
			name:      "Checksums file",
			assetName: "checksums.txt",
			goos:      "linux",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Source code zip",
			assetName: "Source code (zip)",
			goos:      "linux",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Source code tar.gz",
			assetName: "Source code (tar.gz)",
			goos:      "linux",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Incorrect file extension",
			assetName: "speakeasy_linux_amd64.tar.gz",
			goos:      "linux",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Missing architecture",
			assetName: "speakeasy_linux.zip",
			goos:      "linux",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Wrong order of segments",
			assetName: "speakeasy_amd64_linux.zip",
			goos:      "linux",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Partial match in OS",
			assetName: "speakeasy_darwin_amd64.zip",
			goos:      "dar",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Partial match in arch",
			assetName: "speakeasy_linux_amd64.zip",
			goos:      "linux",
			goarch:    "amd",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ArtifactMatchesRelease(tt.assetName, tt.goos, tt.goarch); got != tt.want {
				t.Errorf("ArtifactMatchesRelease() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test source-branch-aware branch naming
func TestGit_FindOrCreateBranch_SourceBranchAware(t *testing.T) {
	tests := []struct {
		name           string
		sourceBranch   string
		action         environment.Action
		expectedPrefix string
	}{
		{
			name:           "main branch - SDK regen",
			sourceBranch:   "main",
			action:         environment.ActionRunWorkflow,
			expectedPrefix: "speakeasy-sdk-regen-",
		},
		{
			name:           "master branch - SDK regen",
			sourceBranch:   "master",
			action:         environment.ActionRunWorkflow,
			expectedPrefix: "speakeasy-sdk-regen-",
		},
		{
			name:           "feature branch - SDK regen",
			sourceBranch:   "feature/new-api",
			action:         environment.ActionRunWorkflow,
			expectedPrefix: "speakeasy-sdk-regen-feature-new-api-",
		},
		{
			name:           "feature branch with special chars - SDK regen",
			sourceBranch:   "feature/user-auth_v2",
			action:         environment.ActionRunWorkflow,
			expectedPrefix: "speakeasy-sdk-regen-feature-user-auth-v2-",
		},
		{
			name:           "feature branch - suggestion",
			sourceBranch:   "feature/api-changes",
			action:         environment.ActionSuggest,
			expectedPrefix: "speakeasy-openapi-suggestion-feature-api-changes-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables for the test
			originalGithubRef := os.Getenv("GITHUB_REF")
			originalGithubHeadRef := os.Getenv("GITHUB_HEAD_REF")
			originalGithubBaseRef := os.Getenv("GITHUB_BASE_REF")
			originalGithubEventName := os.Getenv("GITHUB_EVENT_NAME")

			defer func() {
				os.Setenv("GITHUB_REF", originalGithubRef)
				os.Setenv("GITHUB_HEAD_REF", originalGithubHeadRef)
				os.Setenv("GITHUB_BASE_REF", originalGithubBaseRef)
				os.Setenv("GITHUB_EVENT_NAME", originalGithubEventName)
			}()

			// Set up test environment
			if tt.sourceBranch == "main" || tt.sourceBranch == "master" {
				os.Setenv("GITHUB_REF", "refs/heads/"+tt.sourceBranch)
				os.Setenv("GITHUB_HEAD_REF", "")
				os.Setenv("GITHUB_EVENT_NAME", "push")
			} else {
				os.Setenv("GITHUB_REF", "refs/pull/123/merge")
				os.Setenv("GITHUB_HEAD_REF", tt.sourceBranch)
				os.Setenv("GITHUB_BASE_REF", "main")
				os.Setenv("GITHUB_EVENT_NAME", "pull_request")
			}

			repo, _ := newTestRepo(t)
			g := Git{repo: repo}

			branchName, err := g.FindOrCreateBranch("", "", tt.action)
			require.NoError(t, err)

			assert.True(t, len(branchName) > len(tt.expectedPrefix), "Branch name should be longer than prefix")
			assert.True(t, len(branchName) > 0, "Branch name should not be empty")

			// For main/master branches, should not include source branch in name
			if tt.sourceBranch == "main" || tt.sourceBranch == "master" {
				assert.Contains(t, branchName, tt.expectedPrefix)
				assert.NotContains(t, branchName, "main-")
				assert.NotContains(t, branchName, "master-")
			} else {
				// For feature branches, should include sanitized source branch name
				assert.Contains(t, branchName, tt.expectedPrefix)
			}
		})
	}
}

// Test source-branch-aware PR title generation
func TestGit_generatePRTitleAndBody_SourceBranchAware(t *testing.T) {
	tests := []struct {
		name               string
		sourceBranch       string
		sourceGeneration   bool
		expectedTitleParts []string
		notExpectedParts   []string
	}{
		{
			name:               "main branch - regular generation",
			sourceBranch:       "main",
			sourceGeneration:   false,
			expectedTitleParts: []string{"chore: 🐝 Update SDK"},
			notExpectedParts:   []string{"[main]"},
		},
		{
			name:               "master branch - regular generation",
			sourceBranch:       "master",
			sourceGeneration:   false,
			expectedTitleParts: []string{"chore: 🐝 Update SDK"},
			notExpectedParts:   []string{"[master]"},
		},
		{
			name:               "feature branch - regular generation",
			sourceBranch:       "feature/new-api",
			sourceGeneration:   false,
			expectedTitleParts: []string{"chore: 🐝 Update SDK", "[feature-new-api]"},
			notExpectedParts:   []string{},
		},
		{
			name:               "feature branch with special chars - regular generation",
			sourceBranch:       "feature/user-auth_v2",
			sourceGeneration:   false,
			expectedTitleParts: []string{"chore: 🐝 Update SDK", "[feature-user-auth-v2]"},
			notExpectedParts:   []string{},
		},
		{
			name:               "feature branch - source generation",
			sourceBranch:       "feature/specs-update",
			sourceGeneration:   true,
			expectedTitleParts: []string{"chore: 🐝 Update Specs", "[feature-specs-update]"},
			notExpectedParts:   []string{},
		},
		{
			name:               "main branch - source generation",
			sourceBranch:       "main",
			sourceGeneration:   true,
			expectedTitleParts: []string{"chore: 🐝 Update Specs"},
			notExpectedParts:   []string{"[main]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables for the test
			originalGithubRef := os.Getenv("GITHUB_REF")
			originalGithubHeadRef := os.Getenv("GITHUB_HEAD_REF")
			originalGithubBaseRef := os.Getenv("GITHUB_BASE_REF")
			originalGithubEventName := os.Getenv("GITHUB_EVENT_NAME")

			defer func() {
				os.Setenv("GITHUB_REF", originalGithubRef)
				os.Setenv("GITHUB_HEAD_REF", originalGithubHeadRef)
				os.Setenv("GITHUB_BASE_REF", originalGithubBaseRef)
				os.Setenv("GITHUB_EVENT_NAME", originalGithubEventName)
			}()

			// Set up test environment
			if tt.sourceBranch == "main" || tt.sourceBranch == "master" {
				os.Setenv("GITHUB_REF", "refs/heads/"+tt.sourceBranch)
				os.Setenv("GITHUB_HEAD_REF", "")
				os.Setenv("GITHUB_EVENT_NAME", "push")
			} else {
				os.Setenv("GITHUB_REF", "refs/pull/123/merge")
				os.Setenv("GITHUB_HEAD_REF", tt.sourceBranch)
				os.Setenv("GITHUB_BASE_REF", "main")
				os.Setenv("GITHUB_EVENT_NAME", "pull_request")
			}

			g := Git{}
			prInfo := PRInfo{
				SourceGeneration: tt.sourceGeneration,
				ReleaseInfo: &releases.ReleasesInfo{
					SpeakeasyVersion: "1.0.0",
				},
			}

			title, _ := g.generatePRTitleAndBody(prInfo, map[string]github.Label{}, "")

			// Check that expected parts are in the title
			for _, expectedPart := range tt.expectedTitleParts {
				assert.Contains(t, title, expectedPart, "Title should contain: %s", expectedPart)
			}

			// Check that not expected parts are NOT in the title
			for _, notExpectedPart := range tt.notExpectedParts {
				assert.NotContains(t, title, notExpectedPart, "Title should NOT contain: %s", notExpectedPart)
			}
		})
	}
}

// Test backward compatibility for main/master branches
func TestGit_BackwardCompatibility_MainBranches(t *testing.T) {
	tests := []struct {
		name         string
		sourceBranch string
	}{
		{
			name:         "main branch",
			sourceBranch: "main",
		},
		{
			name:         "master branch",
			sourceBranch: "master",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables for the test
			originalGithubRef := os.Getenv("GITHUB_REF")
			originalGithubHeadRef := os.Getenv("GITHUB_HEAD_REF")
			originalGithubEventName := os.Getenv("GITHUB_EVENT_NAME")

			defer func() {
				os.Setenv("GITHUB_REF", originalGithubRef)
				os.Setenv("GITHUB_HEAD_REF", originalGithubHeadRef)
				os.Setenv("GITHUB_EVENT_NAME", originalGithubEventName)
			}()

			// Set up test environment for main/master branch
			os.Setenv("GITHUB_REF", "refs/heads/"+tt.sourceBranch)
			os.Setenv("GITHUB_HEAD_REF", "")
			os.Setenv("GITHUB_EVENT_NAME", "push")

			repo, _ := newTestRepo(t)
			g := Git{repo: repo}

			// Test branch naming - should NOT include source branch context
			branchName, err := g.FindOrCreateBranch("", "", environment.ActionRunWorkflow)
			require.NoError(t, err)

			// Should follow old naming pattern without source branch context
			assert.Contains(t, branchName, "speakeasy-sdk-regen-")
			assert.NotContains(t, branchName, "main-")
			assert.NotContains(t, branchName, "master-")

			// Test PR title generation - should NOT include source branch context
			prInfo := PRInfo{
				SourceGeneration: false,
				ReleaseInfo: &releases.ReleasesInfo{
					SpeakeasyVersion: "1.0.0",
				},
			}
			title, _ := g.generatePRTitleAndBody(prInfo, map[string]github.Label{}, "")

			// Should follow old title pattern without source branch context
			assert.Contains(t, title, "chore: 🐝 Update SDK")
			assert.NotContains(t, title, "[main]")
			assert.NotContains(t, title, "[master]")
		})
	}
}

func TestCreateOrUpdateDocsPR_SourceBranchAware(t *testing.T) {
	tests := []struct {
		name                  string
		sourceBranch          string
		expectedTitleContains []string
		expectedBaseBranch    string
	}{
		{
			name:                  "main branch - no source context",
			sourceBranch:          "main",
			expectedTitleContains: []string{"chore: 🐝 Update SDK Docs"},
			expectedBaseBranch:    "main",
		},
		{
			name:                  "master branch - no source context",
			sourceBranch:          "master",
			expectedTitleContains: []string{"chore: 🐝 Update SDK Docs"},
			expectedBaseBranch:    "master",
		},
		{
			name:                  "feature branch - includes source context",
			sourceBranch:          "feature/new-api",
			expectedTitleContains: []string{"chore: 🐝 Update SDK Docs", "[feature-new-api]"},
			expectedBaseBranch:    "feature/new-api",
		},
		{
			name:                  "feature branch with special chars",
			sourceBranch:          "feature/api-v2.1_update",
			expectedTitleContains: []string{"chore: 🐝 Update SDK Docs", "[feature-api-v2.1-update]"},
			expectedBaseBranch:    "feature/api-v2.1_update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			originalGithubRef := os.Getenv("GITHUB_REF")
			originalGithubHeadRef := os.Getenv("GITHUB_HEAD_REF")
			originalGithubBaseRef := os.Getenv("GITHUB_BASE_REF")
			originalGithubEventName := os.Getenv("GITHUB_EVENT_NAME")

			defer func() {
				os.Setenv("GITHUB_REF", originalGithubRef)
				os.Setenv("GITHUB_HEAD_REF", originalGithubHeadRef)
				os.Setenv("GITHUB_BASE_REF", originalGithubBaseRef)
				os.Setenv("GITHUB_EVENT_NAME", originalGithubEventName)
			}()

			// Set up test environment
			if tt.sourceBranch == "main" || tt.sourceBranch == "master" {
				os.Setenv("GITHUB_REF", "refs/heads/"+tt.sourceBranch)
				os.Setenv("GITHUB_HEAD_REF", "")
				os.Setenv("GITHUB_EVENT_NAME", "push")
			} else {
				os.Setenv("GITHUB_REF", "refs/pull/123/merge")
				os.Setenv("GITHUB_HEAD_REF", tt.sourceBranch)
				os.Setenv("GITHUB_BASE_REF", "main")
				os.Setenv("GITHUB_EVENT_NAME", "pull_request")
			}

			// Test the title generation logic directly
			title := getDocsPRTitlePrefix()
			sourceBranch := environment.GetSourceBranch()
			isMainBranch := environment.IsMainBranch(sourceBranch)
			if !isMainBranch {
				sanitizedSourceBranch := environment.SanitizeBranchName(sourceBranch)
				title = title + " [" + sanitizedSourceBranch + "]"
			}

			targetBaseBranch := environment.GetTargetBaseBranch()
			if strings.HasPrefix(targetBaseBranch, "refs/") {
				targetBaseBranch = strings.TrimPrefix(targetBaseBranch, "refs/heads/")
			}

			// Verify title contains expected parts
			for _, expectedPart := range tt.expectedTitleContains {
				assert.Contains(t, title, expectedPart, "Title should contain: %s", expectedPart)
			}

			// Verify base branch
			assert.Equal(t, tt.expectedBaseBranch, targetBaseBranch)
		})
	}
}

func TestCreateSuggestionPR_SourceBranchAware(t *testing.T) {
	tests := []struct {
		name                  string
		sourceBranch          string
		expectedTitleContains []string
		expectedBaseBranch    string
	}{
		{
			name:                  "main branch - no source context",
			sourceBranch:          "main",
			expectedTitleContains: []string{"chore: 🐝 Suggest OpenAPI changes"},
			expectedBaseBranch:    "main",
		},
		{
			name:                  "master branch - no source context",
			sourceBranch:          "master",
			expectedTitleContains: []string{"chore: 🐝 Suggest OpenAPI changes"},
			expectedBaseBranch:    "master",
		},
		{
			name:                  "feature branch - includes source context",
			sourceBranch:          "feature/openapi-updates",
			expectedTitleContains: []string{"chore: 🐝 Suggest OpenAPI changes", "[feature-openapi-updates]"},
			expectedBaseBranch:    "feature/openapi-updates",
		},
		{
			name:                  "bugfix branch with special chars",
			sourceBranch:          "bugfix/api-v1.2_fix",
			expectedTitleContains: []string{"chore: 🐝 Suggest OpenAPI changes", "[bugfix-api-v1.2-fix]"},
			expectedBaseBranch:    "bugfix/api-v1.2_fix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			originalGithubRef := os.Getenv("GITHUB_REF")
			originalGithubHeadRef := os.Getenv("GITHUB_HEAD_REF")
			originalGithubBaseRef := os.Getenv("GITHUB_BASE_REF")
			originalGithubEventName := os.Getenv("GITHUB_EVENT_NAME")

			defer func() {
				os.Setenv("GITHUB_REF", originalGithubRef)
				os.Setenv("GITHUB_HEAD_REF", originalGithubHeadRef)
				os.Setenv("GITHUB_BASE_REF", originalGithubBaseRef)
				os.Setenv("GITHUB_EVENT_NAME", originalGithubEventName)
			}()

			// Set up test environment
			if tt.sourceBranch == "main" || tt.sourceBranch == "master" {
				os.Setenv("GITHUB_REF", "refs/heads/"+tt.sourceBranch)
				os.Setenv("GITHUB_HEAD_REF", "")
				os.Setenv("GITHUB_EVENT_NAME", "push")
			} else {
				os.Setenv("GITHUB_REF", "refs/pull/123/merge")
				os.Setenv("GITHUB_HEAD_REF", tt.sourceBranch)
				os.Setenv("GITHUB_BASE_REF", "main")
				os.Setenv("GITHUB_EVENT_NAME", "pull_request")
			}

			// Test the title generation logic directly
			title := getSuggestPRTitlePrefix()
			sourceBranch := environment.GetSourceBranch()
			isMainBranch := environment.IsMainBranch(sourceBranch)
			if !isMainBranch {
				sanitizedSourceBranch := environment.SanitizeBranchName(sourceBranch)
				title = title + " [" + sanitizedSourceBranch + "]"
			}

			targetBaseBranch := environment.GetTargetBaseBranch()
			if strings.HasPrefix(targetBaseBranch, "refs/") {
				targetBaseBranch = strings.TrimPrefix(targetBaseBranch, "refs/heads/")
			}

			// Verify title contains expected parts
			for _, expectedPart := range tt.expectedTitleContains {
				assert.Contains(t, title, expectedPart, "Title should contain: %s", expectedPart)
			}

			// Verify base branch
			assert.Equal(t, tt.expectedBaseBranch, targetBaseBranch)
		})
	}
}
