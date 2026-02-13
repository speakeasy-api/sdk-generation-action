//go:build integration

package integration_test

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-github/v63/github"
	"github.com/speakeasy-api/sdk-generation-action/internal/download"
	internalgit "github.com/speakeasy-api/sdk-generation-action/internal/git"
	"golang.org/x/oauth2"
)

const testRepo = "speakeasy-api/sdk-generation-action-test-repo"
const testRepoOwner = "speakeasy-api"
const testRepoName = "sdk-generation-action-test-repo"

// requireAcceptanceTest skips the test unless SPEAKEASY_ACCEPTANCE=1 is set,
// since these tests rely on third-party APIs (GitHub, Speakeasy).
func requireAcceptanceTest(t *testing.T) {
	t.Helper()
	if os.Getenv("SPEAKEASY_ACCEPTANCE") != "1" {
		t.Skip("skipping: set SPEAKEASY_ACCEPTANCE=1 to run acceptance tests that hit third-party APIs")
	}
}

// requireContainerEnvironment skips the test unless running inside the
// Docker container (SPEAKEASY_ACTION_CONTAINER=true).
func requireContainerEnvironment(t *testing.T) {
	t.Helper()
	if os.Getenv("SPEAKEASY_ACTION_CONTAINER") != "true" {
		t.Skip("skipping: must run inside container (SPEAKEASY_ACTION_CONTAINER != true)")
	}
}

// getTestToken reads GITHUB_TOKEN or INPUT_GITHUB_ACCESS_TOKEN from env and
// skips the test if neither is set.
func getTestToken(t *testing.T) string {
	t.Helper()
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("INPUT_GITHUB_ACCESS_TOKEN")
	}
	if token == "" {
		t.Skip("skipping: no GITHUB_TOKEN or INPUT_GITHUB_ACCESS_TOKEN set")
	}
	return token
}

// getAPIKey reads SPEAKEASY_API_KEY from env and skips the test if not set.
func getAPIKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("SPEAKEASY_API_KEY")
	if key == "" {
		t.Skip("skipping: SPEAKEASY_API_KEY not set")
	}
	return key
}

// setupTestEnvironment sets all the GitHub Actions env vars needed by the
// action code via t.Setenv (automatically restored after test).
func setupTestEnvironment(t *testing.T, workspace, token, branchName string) {
	t.Helper()
	t.Setenv("GITHUB_WORKSPACE", workspace)
	t.Setenv("INPUT_GITHUB_ACCESS_TOKEN", token)
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_REPOSITORY", testRepo)
	t.Setenv("GITHUB_REPOSITORY_OWNER", testRepoOwner)
	t.Setenv("GITHUB_REF", "refs/heads/"+branchName)
	t.Setenv("GITHUB_OUTPUT", filepath.Join(workspace, "output.txt"))
	t.Setenv("GITHUB_WORKFLOW", "integration-test")
	t.Setenv("GITHUB_RUN_ID", "1")
	t.Setenv("GITHUB_RUN_ATTEMPT", "1")
	t.Setenv("INPUT_WORKING_DIRECTORY", "")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	t.Setenv("INPUT_DEBUG", "true")
	// Clear env vars that could interfere
	t.Setenv("GITHUB_HEAD_REF", "")
	t.Setenv("GITHUB_BASE_REF", "")
	t.Setenv("GITHUB_EVENT_NAME", "push")
	t.Setenv("GITHUB_EVENT_PATH", "")
	t.Setenv("INPUT_FEATURE_BRANCH", "")
	t.Setenv("INPUT_OPENAPI_DOC_LOCATION", "")
	t.Setenv("INPUT_SPEAKEASY_VERSION", "")
	t.Setenv("INPUT_DOCS_GENERATION", "")
	t.Setenv("INPUT_TARGET", "")
	t.Setenv("INPUT_SIGNED_COMMITS", "")
	t.Setenv("INPUT_ENABLE_SDK_CHANGELOG", "")
	t.Setenv("INPUT_SKIP_COMPILE", "")
	t.Setenv("INPUT_SKIP_RELEASE", "")
	t.Setenv("INPUT_PUSH_CODE_SAMPLES_ONLY", "")
	t.Setenv("PR_CREATION_PAT", "")
	t.Setenv("INPUT_NPM_TAG", "")
}

// pushOrphanBranch creates a temp directory with minimal speakeasy project
// files, initialises a git repo, and pushes it as an orphan branch to the
// test repo.
func pushOrphanBranch(t *testing.T, token, branchName string) {
	t.Helper()

	dir := t.TempDir()

	// Write minimal speakeasy project files
	writeSpeakeasyProjectFiles(t, dir)

	// Init git repo and push orphan branch
	runGitCLI(t, dir, "init")
	runGitCLI(t, dir, "config", "user.name", "Integration Test")
	runGitCLI(t, dir, "config", "user.email", "test@speakeasy.com")
	runGitCLI(t, dir, "checkout", "--orphan", branchName)
	runGitCLI(t, dir, "add", "-A")
	runGitCLI(t, dir, "commit", "-m", "ci: integration test setup")

	remoteURL := fmt.Sprintf("https://gen:%s@github.com/%s.git", token, testRepo)
	runGitCLI(t, dir, "remote", "add", "origin", remoteURL)
	runGitCLI(t, dir, "push", "--force", "origin", branchName)
}

// writeSpeakeasyProjectFiles writes the minimal files that the action/CLI
// expect for a speakeasy run.
func writeSpeakeasyProjectFiles(t *testing.T, dir string) {
	t.Helper()

	// openapi.yaml — a trivial valid spec
	specContent := `openapi: "3.0.0"
info:
  title: Test API
  version: "1.0.0"
paths:
  /health:
    get:
      operationId: getHealth
      responses:
        "200":
          description: OK
`
	writeFile(t, filepath.Join(dir, "openapi.yaml"), specContent)

	// .speakeasy/workflow.yaml
	speakeasyDir := filepath.Join(dir, ".speakeasy")
	if err := os.MkdirAll(speakeasyDir, 0o755); err != nil {
		t.Fatalf("mkdir .speakeasy: %v", err)
	}

	workflowContent := `workflowVersion: 1.0.0
speakeasyVersion: latest
sources:
  test-source:
    inputs:
      - location: openapi.yaml
targets:
  go:
    target: go
    source: test-source
`
	writeFile(t, filepath.Join(speakeasyDir, "workflow.yaml"), workflowContent)

	// gen.yaml — minimal Go config
	genContent := `configVersion: 2.0.0
generation:
  sdkClassName: testsdk
go:
  version: 0.0.1
  packageName: github.com/speakeasy-api/sdk-generation-action-test-repo
`
	writeFile(t, filepath.Join(speakeasyDir, "gen.yaml"), genContent)
}

// cleanupTestBranches closes any PRs and deletes remote branches created
// during the test. Registered via t.Cleanup so it always runs.
func cleanupTestBranches(t *testing.T, token, branchName string) {
	t.Helper()
	ctx := context.Background()
	client := newGitHubClient(token)

	// Close any PRs whose head branch contains our branch name
	prs, _, err := client.PullRequests.List(ctx, testRepoOwner, testRepoName, &github.PullRequestListOptions{
		State: "open",
	})
	if err != nil {
		t.Logf("cleanup: failed to list PRs: %v", err)
	} else {
		for _, pr := range prs {
			headRef := pr.GetHead().GetRef()
			if strings.Contains(headRef, branchName) || headRef == branchName {
				t.Logf("cleanup: closing PR #%d (%s)", pr.GetNumber(), headRef)
				_, _, err := client.PullRequests.Edit(ctx, testRepoOwner, testRepoName, pr.GetNumber(), &github.PullRequest{
					State: github.String("closed"),
				})
				if err != nil {
					t.Logf("cleanup: failed to close PR #%d: %v", pr.GetNumber(), err)
				}

				// Also delete the PR's head branch
				_, err = client.Git.DeleteRef(ctx, testRepoOwner, testRepoName, "heads/"+headRef)
				if err != nil {
					t.Logf("cleanup: failed to delete PR branch %s: %v", headRef, err)
				}
			}
		}
	}

	// Delete the test branch itself
	_, err = client.Git.DeleteRef(ctx, testRepoOwner, testRepoName, "heads/"+branchName)
	if err != nil {
		t.Logf("cleanup: failed to delete branch %s: %v", branchName, err)
	}
}

// findPRForBranch searches open PRs whose head branch contains our test
// branch name (the UUID portion). Returns nil if no matching PR is found.
func findPRForBranch(t *testing.T, client *github.Client, branchName string) *github.PullRequest {
	t.Helper()
	ctx := context.Background()

	prs, _, err := client.PullRequests.List(ctx, testRepoOwner, testRepoName, &github.PullRequestListOptions{
		State: "open",
	})
	if err != nil {
		t.Logf("findPRForBranch: failed to list PRs: %v", err)
		return nil
	}

	for _, pr := range prs {
		// The generated branch name will contain the test branch name
		if strings.Contains(pr.GetHead().GetRef(), branchName) {
			return pr
		}
	}

	return nil
}

// newGitHubClient creates an authenticated GitHub API client.
func newGitHubClient(token string) *github.Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

// runGitCLI executes a git command in the given directory and fails the test
// if it returns a non-zero exit code.
func runGitCLI(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if len(args) > 0 && args[0] == "commit" {
		args = append([]string{"-c", "commit.gpgsign=false"}, args...)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Integration Test",
		"GIT_AUTHOR_EMAIL=test@speakeasy.com",
		"GIT_COMMITTER_NAME=Integration Test",
		"GIT_COMMITTER_EMAIL=test@speakeasy.com",
		"GIT_TERMINAL_PROMPT=0",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %s\n%s", args, err, string(output))
	}
	return string(output)
}

// writeFile is a test helper that writes content to a file, failing the test
// on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// ensureSpeakeasyCLI downloads the speakeasy CLI to /bin/speakeasy if it is
// not already present.
func ensureSpeakeasyCLI(t *testing.T, token string) {
	t.Helper()

	const cliPath = "/bin/speakeasy"
	if _, err := os.Stat(cliPath); err == nil {
		t.Logf("speakeasy CLI already present at %s", cliPath)
		return
	}

	t.Logf("downloading speakeasy CLI...")
	client := newGitHubClient(token)
	ctx := context.Background()

	releases, _, err := client.Repositories.ListReleases(ctx, "speakeasy-api", "speakeasy", &github.ListOptions{PerPage: 20})
	if err != nil {
		t.Fatalf("list speakeasy releases: %v", err)
	}

	goos := strings.ToLower(runtime.GOOS)
	goarch := strings.ToLower(runtime.GOARCH)

	var downloadURL string
	for _, release := range releases {
		if release.GetDraft() || release.GetPrerelease() {
			continue
		}
		for _, asset := range release.Assets {
			if internalgit.ArtifactMatchesRelease(asset.GetName(), goos, goarch) {
				downloadURL = asset.GetBrowserDownloadURL()
				break
			}
		}
		if downloadURL != "" {
			break
		}
		// Fallback to linux amd64
		for _, asset := range release.Assets {
			if asset.GetName() == "speakeasy_linux_amd64.zip" {
				downloadURL = asset.GetBrowserDownloadURL()
				break
			}
		}
		if downloadURL != "" {
			break
		}
	}
	if downloadURL == "" {
		t.Fatal("could not find speakeasy CLI download URL")
	}

	zipPath := filepath.Join(t.TempDir(), "speakeasy.zip")
	if err := download.DownloadFile(downloadURL, zipPath, "", ""); err != nil {
		t.Fatalf("download speakeasy CLI: %v", err)
	}

	// Extract the zip to /bin/
	extractZip(t, zipPath, "/bin")

	if err := os.Chmod(cliPath, 0o755); err != nil {
		t.Fatalf("chmod speakeasy CLI: %v", err)
	}
	t.Logf("speakeasy CLI installed at %s", cliPath)
}

// extractZip extracts a zip archive to the given destination directory.
func extractZip(t *testing.T, zipPath, destDir string) {
	t.Helper()

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip %s: %v", zipPath, err)
	}
	defer r.Close()

	for _, f := range r.File {
		destPath := filepath.Join(destDir, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(destPath, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			t.Fatalf("mkdir for zip entry: %v", err)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", f.Name, err)
		}
		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			t.Fatalf("create %s: %v", destPath, err)
		}
		if _, err := io.Copy(outFile, rc); err != nil {
			outFile.Close()
			rc.Close()
			t.Fatalf("extract %s: %v", f.Name, err)
		}
		outFile.Close()
		rc.Close()
	}
}

// runSpeakeasyLocal runs `speakeasy run` in the given directory.
func runSpeakeasyLocal(t *testing.T, dir, apiKey string) {
	t.Helper()

	cmd := exec.Command("/bin/speakeasy", "run")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"SPEAKEASY_API_KEY="+apiKey,
		"SPEAKEASY_RUN_LOCATION=action",
		"SPEAKEASY_ENVIRONMENT=github",
		"GIT_TERMINAL_PROMPT=0",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("speakeasy run failed: %v\n%s", err, string(output))
	}
	t.Logf("speakeasy run completed successfully")
}

// pushOrphanBranchWithSDK creates a temp directory with the default single-op
// speakeasy project files, runs `speakeasy run` to generate a real SDK
// baseline, and pushes everything as an orphan branch.
func pushOrphanBranchWithSDK(t *testing.T, token, branchName, apiKey string) string {
	return pushOrphanBranchWithCustomSDK(t, token, branchName, apiKey, writeSpeakeasyProjectFiles)
}

// pushOrphanBranchWithCustomSDK creates a temp directory, calls writeFiles to
// populate it, runs `speakeasy run` to generate a real SDK baseline, and pushes
// everything as an orphan branch. Returns the directory path so the caller
// can modify files (e.g. update the spec) before the workflow runs.
func pushOrphanBranchWithCustomSDK(t *testing.T, token, branchName, apiKey string, writeFiles func(*testing.T, string)) string {
	t.Helper()

	dir := t.TempDir()

	// Write project files using the provided function
	writeFiles(t, dir)

	// Init git repo and push orphan branch with initial spec
	runGitCLI(t, dir, "init")
	runGitCLI(t, dir, "config", "user.name", "Integration Test")
	runGitCLI(t, dir, "config", "user.email", "test@speakeasy.com")
	runGitCLI(t, dir, "checkout", "--orphan", branchName)
	runGitCLI(t, dir, "add", "-A")
	runGitCLI(t, dir, "commit", "-m", "ci: initial spec")

	remoteURL := fmt.Sprintf("https://gen:%s@github.com/%s.git", token, testRepo)
	runGitCLI(t, dir, "remote", "add", "origin", remoteURL)

	// Configure url.*.insteadOf for git auth (needed by speakeasy run)
	authenticatedPrefix := fmt.Sprintf("https://gen:%s@github.com/", token)
	runGitCLI(t, dir, "config", "--local",
		fmt.Sprintf("url.%s.insteadOf", authenticatedPrefix),
		"https://github.com/",
	)

	runGitCLI(t, dir, "push", "--force", "origin", branchName)

	// Download CLI and run speakeasy to generate SDK baseline
	ensureSpeakeasyCLI(t, token)
	runSpeakeasyLocal(t, dir, apiKey)

	// Commit generated SDK files and push
	runGitCLI(t, dir, "add", "-A")
	runGitCLI(t, dir, "commit", "-m", "ci: generated SDK baseline")
	runGitCLI(t, dir, "push", "--force", "origin", branchName)

	return dir
}

// enablePersistentEditsInGenYaml modifies the generated gen.yaml to enable
// persistent edits. After the first `speakeasy run`, gen.yaml contains
// `persistentEdits: {}` — this replaces it with an enabled config.
func enablePersistentEditsInGenYaml(t *testing.T, dir string) {
	t.Helper()
	genYamlPath := filepath.Join(dir, ".speakeasy", "gen.yaml")
	content, err := os.ReadFile(genYamlPath)
	if err != nil {
		t.Fatalf("read gen.yaml: %v", err)
	}
	s := string(content)
	if strings.Contains(s, "persistentEdits: {}") {
		s = strings.Replace(s, "  persistentEdits: {}", "  persistentEdits:\n    enabled: true", 1)
	} else if !strings.Contains(s, "persistentEdits:") {
		s = strings.Replace(s, "generation:\n", "generation:\n  persistentEdits:\n    enabled: true\n", 1)
	} else {
		t.Logf("gen.yaml already has a non-empty persistentEdits section, skipping modification")
		return
	}
	if err := os.WriteFile(genYamlPath, []byte(s), 0o644); err != nil {
		t.Fatalf("write gen.yaml: %v", err)
	}
	t.Logf("enabled persistentEdits in gen.yaml")
}

// findGeneratedGoFile finds a generated Go file suitable for editing.
// Prefers files in models/operations/ that should be stable across spec changes.
func findGeneratedGoFile(t *testing.T, dir string) string {
	t.Helper()
	// Try models/operations/ first — these don't change when new endpoints are added
	patterns := []string{
		filepath.Join(dir, "models", "operations", "*.go"),
		filepath.Join(dir, "*", "*.go"),
		filepath.Join(dir, "*", "*", "*.go"),
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			return matches[0]
		}
	}
	t.Fatal("no generated .go files found")
	return ""
}

// addCommentToGoFile adds a unique comment after the package line in a Go file.
// Returns the comment string for later verification.
func addCommentToGoFile(t *testing.T, filePath string) string {
	t.Helper()
	comment := "// PERSISTENT-EDIT-TEST: this comment should survive SDK regeneration"

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read %s: %v", filePath, err)
	}

	lines := strings.Split(string(content), "\n")
	var result []string
	inserted := false
	for _, line := range lines {
		result = append(result, line)
		if !inserted && strings.HasPrefix(line, "package ") {
			result = append(result, "", comment)
			inserted = true
		}
	}
	if !inserted {
		t.Fatalf("could not find package line in %s", filePath)
	}

	if err := os.WriteFile(filePath, []byte(strings.Join(result, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", filePath, err)
	}
	return comment
}

// getFileContentFromRef retrieves file content from a specific git ref via
// the GitHub API.
func getFileContentFromRef(t *testing.T, client *github.Client, ref, path string) string {
	t.Helper()
	ctx := context.Background()
	fileContent, _, _, err := client.Repositories.GetContents(ctx, testRepoOwner, testRepoName, path, &github.RepositoryContentGetOptions{
		Ref: ref,
	})
	if err != nil {
		t.Fatalf("get file content from %s:%s: %v", ref, path, err)
	}
	content, err := fileContent.GetContent()
	if err != nil {
		t.Fatalf("decode file content: %v", err)
	}
	return content
}

// writeSpeakeasyProjectFilesWithBothOps writes project files with a spec
// containing both /health and /status operations. Used as the starting point
// for conflict tests where /status will later be removed.
func writeSpeakeasyProjectFilesWithBothOps(t *testing.T, dir string) {
	t.Helper()

	specContent := `openapi: "3.0.0"
info:
  title: Test API
  version: "1.0.0"
paths:
  /health:
    get:
      operationId: getHealth
      responses:
        "200":
          description: OK
  /status:
    get:
      operationId: getStatus
      summary: Get service status
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema:
                type: object
                properties:
                  status:
                    type: string
`
	writeFile(t, filepath.Join(dir, "openapi.yaml"), specContent)

	speakeasyDir := filepath.Join(dir, ".speakeasy")
	if err := os.MkdirAll(speakeasyDir, 0o755); err != nil {
		t.Fatalf("mkdir .speakeasy: %v", err)
	}

	workflowContent := `workflowVersion: 1.0.0
speakeasyVersion: latest
sources:
  test-source:
    inputs:
      - location: openapi.yaml
targets:
  go:
    target: go
    source: test-source
`
	writeFile(t, filepath.Join(speakeasyDir, "workflow.yaml"), workflowContent)

	genContent := `configVersion: 2.0.0
generation:
  sdkClassName: testsdk
go:
  version: 0.0.1
  packageName: github.com/speakeasy-api/sdk-generation-action-test-repo
`
	writeFile(t, filepath.Join(speakeasyDir, "gen.yaml"), genContent)
}

// writeSpecHealthOnly overwrites openapi.yaml with only the /health endpoint,
// effectively removing the /status operation.
func writeSpecHealthOnly(t *testing.T, dir string) {
	t.Helper()
	specContent := `openapi: "3.0.0"
info:
  title: Test API
  version: "1.0.0"
paths:
  /health:
    get:
      operationId: getHealth
      responses:
        "200":
          description: OK
`
	writeFile(t, filepath.Join(dir, "openapi.yaml"), specContent)
}

// addInlineEditToStatusField finds the generated Go file containing the
// `Status` response field (from the /status operation) and appends an inline
// comment to that exact line. Returns the file path and the edit marker.
// This creates a same-line conflict when the spec renames the property.
func addInlineEditToStatusField(t *testing.T, dir string) (filePath string, editMarker string) {
	t.Helper()
	editMarker = "// user-edit: persistent edit test marker"

	// Search for the generated file containing the Status field with json tag
	patterns := []string{
		filepath.Join(dir, "models", "operations", "*.go"),
		filepath.Join(dir, "models", "components", "*.go"),
		filepath.Join(dir, "models", "*.go"),
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			content, err := os.ReadFile(match)
			if err != nil {
				continue
			}
			if !strings.Contains(string(content), `json:"status`) {
				continue
			}
			// Found the file — edit the line with the Status field
			lines := strings.Split(string(content), "\n")
			edited := false
			for i, line := range lines {
				if strings.Contains(line, `json:"status`) {
					lines[i] = line + " " + editMarker
					edited = true
					break
				}
			}
			if !edited {
				continue
			}
			if err := os.WriteFile(match, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
				t.Fatalf("write %s: %v", match, err)
			}
			t.Logf("edited line with Status field in %s", match)
			return match, editMarker
		}
	}
	t.Fatal("could not find generated file with Status json field")
	return "", ""
}

// writeSpecWithRenamedProperty overwrites openapi.yaml, renaming the `status`
// response property to `serviceStatus`. The /status operation is kept — only
// the response schema property name changes.
func writeSpecWithRenamedProperty(t *testing.T, dir string) {
	t.Helper()
	specContent := `openapi: "3.0.0"
info:
  title: Test API
  version: "1.0.0"
paths:
  /health:
    get:
      operationId: getHealth
      responses:
        "200":
          description: OK
  /status:
    get:
      operationId: getStatus
      summary: Get service status
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema:
                type: object
                properties:
                  serviceStatus:
                    type: string
`
	writeFile(t, filepath.Join(dir, "openapi.yaml"), specContent)
}

// writeUpdatedSpecWithNewOperation overwrites openapi.yaml in dir to add a
// /status GET endpoint alongside the existing /health endpoint.
func writeUpdatedSpecWithNewOperation(t *testing.T, dir string) {
	t.Helper()

	specContent := `openapi: "3.0.0"
info:
  title: Test API
  version: "1.0.0"
paths:
  /health:
    get:
      operationId: getHealth
      responses:
        "200":
          description: OK
  /status:
    get:
      operationId: getStatus
      summary: Get service status
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema:
                type: object
                properties:
                  status:
                    type: string
`
	writeFile(t, filepath.Join(dir, "openapi.yaml"), specContent)
}
