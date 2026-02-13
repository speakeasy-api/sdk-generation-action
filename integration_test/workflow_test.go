//go:build integration

package integration_test

import (
	"os"
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

// TestRunWorkflow_PRMode_PersistentEdits verifies that manual edits to generated
// files (e.g. a comment added to a model file) are preserved across SDK
// regeneration when persistentEdits is enabled in gen.yaml.
//
// Flow:
//  1. Generate initial SDK baseline via speakeasy run
//  2. Enable persistentEdits in gen.yaml + add a comment to a generated file
//  3. Run speakeasy locally again (verifies the comment survives locally)
//  4. Push everything to the remote
//  5. Push a spec change (add /status endpoint)
//  6. Run actions.RunWorkflow() to regenerate the SDK via the action
//  7. Verify the comment was retained in the PR
func TestRunWorkflow_PRMode_PersistentEdits(t *testing.T) {
	requireAcceptanceTest(t)
	requireContainerEnvironment(t)
	token := getTestToken(t)
	apiKey := getAPIKey(t)

	testID := uuid.New().String()[:8]
	branchName := "test-integration-" + testID
	workspace := t.TempDir()

	t.Cleanup(func() { cleanupTestBranches(t, token, branchName) })

	// Step 1: Generate initial SDK baseline
	dir := pushOrphanBranchWithSDK(t, token, branchName, apiKey)

	// Step 2: Enable persistent edits and add a comment to a generated file
	enablePersistentEditsInGenYaml(t, dir)
	editedFile := findGeneratedGoFile(t, dir)
	comment := addCommentToGoFile(t, editedFile)
	editedRelPath, _ := filepath.Rel(dir, editedFile)
	t.Logf("edited file: %s (added comment: %q)", editedRelPath, comment)

	// Step 3: Run speakeasy locally again (persistent edits should preserve the comment)
	runSpeakeasyLocal(t, dir, apiKey)

	// Verify the comment survived the local run
	contentAfterLocal, err := os.ReadFile(editedFile)
	require.NoError(t, err)
	require.Contains(t, string(contentAfterLocal), comment,
		"comment should survive local speakeasy run with persistent edits")

	// Step 4: Commit everything and push
	runGitCLI(t, dir, "add", "-A")
	runGitCLI(t, dir, "commit", "-m", "ci: enable persistent edits + manual edit")
	runGitCLI(t, dir, "push", "--force", "origin", branchName)

	// Step 5: Push a change to openapi.yaml (add /status endpoint)
	writeUpdatedSpecWithNewOperation(t, dir)
	runGitCLI(t, dir, "add", "-A")
	runGitCLI(t, dir, "commit", "-m", "feat: add status endpoint")
	runGitCLI(t, dir, "push", "--force", "origin", branchName)

	// Step 6: Run the full workflow via the action
	setupTestEnvironment(t, workspace, token, branchName)
	t.Setenv("SPEAKEASY_API_KEY", apiKey)
	t.Setenv("INPUT_MODE", "pr")
	t.Setenv("INPUT_FORCE", "true")

	err = actions.RunWorkflow()
	require.NoError(t, err, "RunWorkflow should succeed")

	// Step 7: Verify the comment was retained in the PR
	client := newGitHubClient(token)
	pr := findPRForBranch(t, client, branchName)
	require.NotNil(t, pr, "expected a PR to be created")

	headRef := pr.GetHead().GetRef()
	prFileContent := getFileContentFromRef(t, client, headRef, editedRelPath)
	assert.Contains(t, prFileContent, comment,
		"persistent edit comment should survive regeneration via RunWorkflow")
	t.Logf("verified: comment preserved in PR branch %s, file %s", headRef, editedRelPath)
}

// TestRunWorkflow_PRMode_PersistentEditsConflict verifies the behavior when a
// user has edited a line in a generated file that the generator also changes
// (a property rename in the spec). This creates a same-line conflict in the
// persistent edits 3-way merge.
//
// Flow:
//  1. Generate SDK with /status having a `status` response property
//  2. Enable persistentEdits + edit the line containing the Status field
//  3. Run speakeasy locally (registers the edit)
//  4. Push everything
//  5. Rename `status` → `serviceStatus` in the spec (same-line change)
//  6. Push the spec change
//  7. Run actions.RunWorkflow() — observe conflict behavior
func TestRunWorkflow_PRMode_PersistentEditsConflict(t *testing.T) {
	requireAcceptanceTest(t)
	requireContainerEnvironment(t)
	token := getTestToken(t)
	apiKey := getAPIKey(t)

	testID := uuid.New().String()[:8]
	branchName := "test-integration-" + testID
	workspace := t.TempDir()

	t.Cleanup(func() { cleanupTestBranches(t, token, branchName) })

	// Step 1: Generate SDK baseline with both /health and /status
	dir := pushOrphanBranchWithCustomSDK(t, token, branchName, apiKey, writeSpeakeasyProjectFilesWithBothOps)

	// Step 2: Enable persistent edits and edit the Status field line
	enablePersistentEditsInGenYaml(t, dir)
	editedFile, editMarker := addInlineEditToStatusField(t, dir)
	editedRelPath, _ := filepath.Rel(dir, editedFile)
	t.Logf("edited field line in %s (marker: %q)", editedRelPath, editMarker)

	// Step 3: Run speakeasy locally (registers the edit via persistent edits)
	runSpeakeasyLocal(t, dir, apiKey)

	// Verify the edit survived locally
	contentAfterLocal, err := os.ReadFile(editedFile)
	require.NoError(t, err)
	require.Contains(t, string(contentAfterLocal), editMarker,
		"inline edit should survive local speakeasy run")

	// Step 4: Commit everything and push
	runGitCLI(t, dir, "add", "-A")
	runGitCLI(t, dir, "commit", "-m", "ci: enable persistent edits + edit status field")
	runGitCLI(t, dir, "push", "--force", "origin", branchName)

	// Step 5: Rename `status` → `serviceStatus` in the spec
	writeSpecWithRenamedProperty(t, dir)
	runGitCLI(t, dir, "add", "-A")
	runGitCLI(t, dir, "commit", "-m", "feat: rename status property to serviceStatus")
	runGitCLI(t, dir, "push", "--force", "origin", branchName)

	// Step 6: Run the full workflow via the action
	setupTestEnvironment(t, workspace, token, branchName)
	t.Setenv("SPEAKEASY_API_KEY", apiKey)
	t.Setenv("INPUT_MODE", "pr")
	t.Setenv("INPUT_FORCE", "true")

	err = actions.RunWorkflow()

	// Step 7: Verify conflict behavior.
	//
	// The user edited the line with `Status *string ...` and the generator
	// renamed it to `ServiceStatus *string ...`. Both sides modify the same
	// line — the 3-way merge produces a conflict. RunWorkflow fails and no
	// PR is created.
	require.Error(t, err, "RunWorkflow should fail when persistent edits conflict with spec changes")

	errStr := err.Error()
	assert.Contains(t, errStr, "conflict",
		"error should mention conflict")
	assert.Contains(t, errStr, "models/operations/getstatus.go",
		"error should identify the conflicting file")

	// No PR should be created — the conflict aborts before committing.
	client := newGitHubClient(token)
	pr := findPRForBranch(t, client, branchName)
	assert.Nil(t, pr, "no PR should be created when there is an unresolved conflict")
}
