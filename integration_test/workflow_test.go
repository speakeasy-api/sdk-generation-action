//go:build integration

package integration_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/speakeasy-api/sdk-generation-action/internal/actions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunWorkflow_PRMode_E2E runs the full action workflow in PR mode against
// a real GitHub repository and verifies that a PR is created.
//
// Each test run pushes an orphan commit to a unique branch
// "test-integration-<uuid>" on speakeasy-api/sdk-generation-action-test-repo.
// The UUID-scoped branch ensures parallel runs can't collide.
func TestRunWorkflow_PRMode_E2E(t *testing.T) {
	requireAcceptanceTest(t)
	requireContainerEnvironment(t)
	token := getTestToken(t)
	apiKey := getAPIKey(t)

	testID := uuid.New().String()[:8]
	branchName := "test-integration-" + testID
	workspace := t.TempDir()

	// Setup: push orphan branch with speakeasy config
	pushOrphanBranch(t, token, branchName)

	// Cleanup: always close PRs + delete branches (runs in LIFO order)
	t.Cleanup(func() { cleanupTestBranches(t, token, branchName) })

	// Configure environment to simulate GitHub Actions context
	setupTestEnvironment(t, workspace, token, branchName)
	t.Setenv("SPEAKEASY_API_KEY", apiKey)
	t.Setenv("INPUT_MODE", "pr")
	t.Setenv("INPUT_FORCE", "true")

	// Run the full workflow
	err := actions.RunWorkflow()
	require.NoError(t, err, "RunWorkflow should succeed")

	// Verify: PR was created targeting our branch
	client := newGitHubClient(token)
	pr := findPRForBranch(t, client, branchName)
	require.NotNil(t, pr, "expected a PR to be created")
	assert.Equal(t, branchName, pr.GetBase().GetRef(),
		"PR base branch should be our test branch")

	// Verify: git credentials were configured in cloned repo
	repoDir := filepath.Join(workspace, "repo")
	cmd := exec.Command("git", "config", "--local", "--get-regexp", `url\..*\.insteadOf`)
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git config should find insteadOf entries")
	assert.Contains(t, string(output), "https://github.com/",
		"insteadOf should rewrite the GitHub host")
}

// TestRunWorkflow_PRMode_WithChangelog generates a real SDK baseline, pushes a
// spec change that adds a new operation (/status), then runs the workflow and
// verifies the resulting PR body contains a meaningful changelog describing the
// added operation.
func TestRunWorkflow_PRMode_WithChangelog(t *testing.T) {
	requireAcceptanceTest(t)
	requireContainerEnvironment(t)
	token := getTestToken(t)
	apiKey := getAPIKey(t)

	testID := uuid.New().String()[:8]
	branchName := "test-integration-" + testID
	workspace := t.TempDir()

	// Cleanup: always close PRs + delete branches (runs in LIFO order)
	t.Cleanup(func() { cleanupTestBranches(t, token, branchName) })

	// Setup: push orphan branch with a real generated SDK baseline
	dir := pushOrphanBranchWithSDK(t, token, branchName, apiKey)

	// Modify the spec to add a new /status operation
	writeUpdatedSpecWithNewOperation(t, dir)
	runGitCLI(t, dir, "add", "-A")
	runGitCLI(t, dir, "commit", "-m", "feat: add status endpoint")
	runGitCLI(t, dir, "push", "--force", "origin", branchName)

	// Configure environment to simulate GitHub Actions context
	setupTestEnvironment(t, workspace, token, branchName)
	t.Setenv("SPEAKEASY_API_KEY", apiKey)
	t.Setenv("INPUT_MODE", "pr")
	t.Setenv("INPUT_FORCE", "true")
	// Enable SDK changelog so the CLI computes a local spec diff and includes
	// operation-level changes (e.g. "GetStatus(): Added") in the PR body.
	t.Setenv("INPUT_ENABLE_SDK_CHANGELOG", "true")

	// Run the full workflow
	err := actions.RunWorkflow()
	require.NoError(t, err, "RunWorkflow should succeed")

	// Verify: PR was created targeting our branch
	client := newGitHubClient(token)
	pr := findPRForBranch(t, client, branchName)
	require.NotNil(t, pr, "expected a PR to be created")
	assert.Equal(t, branchName, pr.GetBase().GetRef(),
		"PR base branch should be our test branch")

	// Verify: PR body contains the SDK changelog with operation-level changes
	body := pr.GetBody()
	require.NotEmpty(t, body, "PR body should not be empty")
	t.Logf("PR body:\n%s", body)

	bodyLower := strings.ToLower(body)
	hasOperationRef := strings.Contains(bodyLower, "getstatus") || strings.Contains(bodyLower, "get_status")
	assert.True(t, hasOperationRef,
		"PR body should reference the new operation (GetStatus or get_status), got:\n%s", body)

	assert.Contains(t, body, "Added",
		"PR body should indicate the operation was added")
}
