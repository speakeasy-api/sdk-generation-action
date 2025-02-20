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

const testReportHeader = "SDK Tests Report"

type TestReport struct {
	Success bool
	URL     string
}

func Test(ctx context.Context) error {
	g, err := initAction()
	if err != nil {
		return err
	}

	if _, err = cli.Download("latest", g); err != nil {
		return err
	}

	wf, err := configuration.GetWorkflowAndValidateLanguages(false)
	if err != nil {
		return err
	}

	// This will only come in via workflow dispatch, we do accept 'all' as a special case
	var testedTargets []string
	if providedTargetName := environment.SpecifiedTarget(); providedTargetName != "" && os.Getenv("GITHUB_EVENT_NAME") == "workflow_dispatch" {
		testedTargets = append(testedTargets, providedTargetName)
	}

	var prNumber *int
	targetLockIDs := make(map[string]string)
	fmt.Println("TESTED TARGETS")
	fmt.Println(testedTargets)
	if len(testedTargets) == 0 {
		// We look for all files modified in the PR or Branch to see what SDK targets have been modified
		files, number, err := g.GetChangedFilesForPRorBranch()
		if err != nil {
			fmt.Printf("Failed to get commited files: %s\n", err.Error())
		}

		prNumber = number

		for _, file := range files {
			if strings.Contains(file, "gen.yaml") || strings.Contains(file, "gen.lock") {
				relativeCfgDir := filepath.Dir(file)
				cfg, err := config.Load(filepath.Join(environment.GetWorkspace(), "repo", relativeCfgDir))
				if err != nil {
					return fmt.Errorf("failed to load config: %w", err)
				}

				file, _ := os.ReadFile(file)

				fmt.Println("file")
				fmt.Println(string(file))

				var genLockID string
				fmt.Println("LOOKING FOR GEN LOCK ID")
				if cfg.LockFile != nil {
					genLockID = cfg.LockFile.ID
					fmt.Println("GEN LOCK ID FOUND: ", genLockID)
				}

				relativeOutDir, err := filepath.Abs(filepath.Dir(relativeCfgDir))
				if err != nil {
					return err
				}
				fmt.Println("THIS IS THE OUT DIR")
				fmt.Println(relativeOutDir)
				for name, target := range wf.Targets {
					targetOutput := ""
					if target.Output != nil {
						targetOutput = *target.Output
					}
					targetOutput, err := filepath.Abs(filepath.Join(environment.GetWorkingDirectory(), targetOutput))
					if err != nil {
						return err
					}
					// If there are multiple SDKs in a workflow we ensure output path is unique
					if targetOutput == relativeCfgDir && !slices.Contains(testedTargets, name) {
						fmt.Println("TARGET FOUND: ", name)
						fmt.Println(genLockID)
						targetLockIDs[name] = genLockID
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
		// TODO: Once we have stable test reports we will probably want to use GH API to leave a PR comment/clean up old comments
		err := cli.Test(target)
		if err != nil {
			errs = append(errs, err)
		}

		testReportURL := "placeholder"
		if genLockID, ok := targetLockIDs[target]; ok && genLockID != "" {
			testReportURL = formatTestReportURL(ctx, genLockID)
		} else {
			fmt.Println(fmt.Sprintf("No gen.lock ID found for target %s", target))
		}

		if testReportURL == "" {
			fmt.Println(fmt.Sprintf("No test report URL could be formed for target %s", target))
		} else {
			testReports[target] = TestReport{
				Success: err == nil,
				URL:     testReportURL,
			}
		}
	}

	if len(testReports) > 0 {
		if err := writeTestReportComment(g, prNumber, testReports); err != nil {
			fmt.Println(fmt.Sprintf("Failed to write test report comment: %s\n", err.Error()))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("test failures occured: %w", errors.Join(errs...))
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

	return fmt.Sprintf("https://app.speakeasy.com/org/%s/%s/targets/%s/tests/%s", orgSlug, workspaceSlug, genLockID, executionID)
}

func writeTestReportComment(g *git.Git, prNumber *int, testReports map[string]TestReport) error {
	if prNumber == nil {
		return fmt.Errorf("PR number is nil, cannot post comment")
	}

	currentPRComments, _ := g.ListIssueComments(*prNumber)
	for _, comment := range currentPRComments {
		commentBody := comment.GetBody()
		if strings.Contains(commentBody, testReportHeader) {
			if err := g.DeleteIssueComment(comment.GetID()); err != nil {
				fmt.Println(fmt.Sprintf("Failed to delete existing test report comment: %s\n", err.Error()))
			}
		}
	}

	titleComment := fmt.Sprintf("## **%s**\n\n", testReportHeader)

	tableHeader := "| Target | Status | Report |\n|--------|--------|--------|\n"

	var tableRows strings.Builder
	for target, report := range testReports {
		statusEmoji := "✅"
		if !report.Success {
			statusEmoji = "❌"
		}
		tableRows.WriteString(fmt.Sprintf("| %s | %s | [view report](%s) |\n", target, statusEmoji, report.URL))
	}

	// Combine everything
	body := titleComment + tableHeader + tableRows.String()

	err := g.WriteIssueComment(*prNumber, body)

	return err
}
