package actions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	config "github.com/speakeasy-api/sdk-gen-config"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/git"
	"github.com/speakeasy-api/sdk-generation-action/internal/telemetry"
	"golang.org/x/exp/slices"
)

const testReportHeader = "SDK Test Report"

type TestReport struct {
	Success bool
	URL     string
}

func Test(ctx context.Context) error {
	g, err := initAction()
	if err != nil {
		return err
	}

	if err := SetupEnvironment(); err != nil {
		return fmt.Errorf("failed to setup environment: %w", err)
	}

	if _, err = cli.Download("latest", g); err != nil {
		return err
	}

	wf, err := configuration.GetWorkflowAndValidateLanguages(false)
	if err != nil {
		return err
	}

	// Always resolve the PR number
	var prNumber *int
	_, number, err := g.GetChangedFilesForPRorBranch()
	if err != nil {
		fmt.Printf("Failed to get PR info: %s\n", err.Error())
	}
	prNumber = number

	// Resolve gen.lock IDs for all workflow targets so we can build report URLs
	targetLockIDs := make(map[string]string)
	for name, target := range wf.Targets {
		targetOutput := ""
		if target.Output != nil {
			targetOutput = *target.Output
		}
		outDir := filepath.Join(environment.GetRepoPath(), targetOutput)
		cfg, err := config.Load(outDir)
		if err != nil {
			fmt.Printf("Failed to load config for target %s: %s\n", name, err.Error())
			continue
		}
		if cfg.LockFile != nil {
			targetLockIDs[name] = cfg.LockFile.ID
		}
	}

	var testedTargets []string
	if providedTargetName := environment.SpecifiedTarget(); providedTargetName != "" {
		testedTargets = append(testedTargets, providedTargetName)
	} else {
		// No target specified — discover targets from changed files in the PR
		files, _, err := g.GetChangedFilesForPRorBranch()
		if err != nil {
			fmt.Printf("Failed to get changed files: %s\n", err.Error())
		}

		for _, file := range files {
			if strings.Contains(file, "gen.yaml") || strings.Contains(file, "gen.lock") {
				configDir := filepath.Dir(filepath.Dir(file)) // gets out of .speakeasy
				outDir, err := filepath.Abs(configDir)
				if err != nil {
					return err
				}
				for name, target := range wf.Targets {
					targetOutput := ""
					if target.Output != nil {
						targetOutput = *target.Output
					}
					targetOutput, err := filepath.Abs(filepath.Join(environment.GetWorkingDirectory(), targetOutput))
					if err != nil {
						return err
					}
					if targetOutput == outDir && !slices.Contains(testedTargets, name) {
						testedTargets = append(testedTargets, name)
					}
				}
			}
		}
	}
	if len(testedTargets) == 0 {
		fmt.Println("No target was provided ... skipping tests")
		return nil
	}

	// we will pretty much never have a test action for multiple targets
	// but if a customer manually setup their triggers in this way, we will run test sequentially for clear output

	testReports := make(map[string]TestReport)
	var errs []error
	for _, target := range testedTargets {
		err := cli.Test(target)
		if err != nil {
			errs = append(errs, err)
		}

		testReportURL := ""
		if genLockID, ok := targetLockIDs[target]; ok && genLockID != "" {
			testReportURL = formatTestReportURL(ctx, genLockID)
		} else {
			fmt.Printf("No gen.lock ID found for target %s (available targets: %v)\n", target, targetLockIDs)
		}

		if testReportURL == "" {
			fmt.Printf("No test report URL could be formed for target %s\n", target)
		}

		testReports[target] = TestReport{
			Success: err == nil,
			URL:     testReportURL,
		}
	}

	if len(testReports) > 0 && prNumber != nil {
		if err := writeTestReportComment(g, prNumber, testReports); err != nil {
			fmt.Printf("Failed to write test report comment: %s\n", err.Error())
		}
	} else if len(testReports) > 0 && prNumber == nil {
		fmt.Println("Skipping test report PR comment: could not determine PR number")
	}

	if len(errs) > 0 {
		return fmt.Errorf("test failures occurred: %w", errors.Join(errs...))
	}

	return nil
}

func formatTestReportURL(ctx context.Context, genLockID string) string {
	executionID := os.Getenv(telemetry.ExecutionKeyEnvironmentVariable)
	if executionID == "" {
		return ""
	}

	if ctx.Value(telemetry.OrgSlugKey) == nil {
		return ""
	}
	orgSlug, ok := ctx.Value(telemetry.OrgSlugKey).(string)
	if !ok {
		return ""
	}

	if ctx.Value(telemetry.WorkspaceSlugKey) == nil {
		return ""
	}
	workspaceSlug, ok := ctx.Value(telemetry.WorkspaceSlugKey).(string)
	if !ok {
		return ""
	}

	return fmt.Sprintf("https://app.speakeasy.com/org/%s/%s/generate/sdks/%s/tests/%s", orgSlug, workspaceSlug, genLockID, executionID)
}

func writeTestReportComment(g *git.Git, prNumber *int, testReports map[string]TestReport) error {
	if prNumber == nil {
		return fmt.Errorf("PR number is nil, cannot post comment")
	}

	currentPRComments, err := g.ListIssueComments(*prNumber)
	if err != nil {
		fmt.Printf("Failed to list PR comments: %s\n", err.Error())
	}

	// Each target gets its own comment to avoid race conditions when
	// multiple targets run in parallel as separate workflow jobs.
	for target, report := range testReports {
		targetHeader := fmt.Sprintf("%s: %s", testReportHeader, target)

		// Delete any existing comment for this specific target
		for _, comment := range currentPRComments {
			if strings.Contains(comment.GetBody(), targetHeader) {
				if err := g.DeleteIssueComment(comment.GetID()); err != nil {
					fmt.Printf("Failed to delete existing test report comment for %s: %s\n", target, err.Error())
				}
			}
		}

		statusEmoji := "✅"
		statusText := "passed"
		if !report.Success {
			statusEmoji = "❌"
			statusText = "failed"
		}

		var body string
		if report.URL != "" {
			body = fmt.Sprintf("%s **%s** — tests %s &nbsp; [View Report](%s)\n",
				statusEmoji, targetHeader, statusText, report.URL)
		} else {
			body = fmt.Sprintf("%s **%s** — tests %s\n",
				statusEmoji, targetHeader, statusText)
		}

		if err := g.WriteIssueComment(*prNumber, body); err != nil {
			fmt.Printf("Failed to write test report comment for %s: %s\n", target, err.Error())
		}
	}

	return nil
}
