package actions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v63/github"
	config "github.com/speakeasy-api/sdk-gen-config"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/git"
	"github.com/speakeasy-api/sdk-generation-action/internal/telemetry"
	"golang.org/x/exp/slices"
)

const testReportCommentPrefix = "view your test report for target %s"

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
	if providedTargetName := environment.SpecifiedTarget(); providedTargetName != "" {
		testedTargets = append(testedTargets, providedTargetName)
	}

	var prNumber *int
	targetLockIDs := make(map[string]string)
	if len(testedTargets) == 0 || os.Getenv("GITHUB_EVENT_NAME") != "workflow_dispatch" {
		// We look for all files modified in the PR or Branch to see what SDK targets have been modified
		files, number, err := g.GetChangedFilesForPRorBranch()
		if err != nil {
			fmt.Printf("Failed to get commited files: %s\n", err.Error())
		}

		prNumber = number

		for _, file := range files {
			if strings.Contains(file, "gen.yaml") || strings.Contains(file, "gen.lock") {
				cfgDir := filepath.Dir(file)
				cfg, err := config.Load(filepath.Dir(file))
				if err != nil {
					return fmt.Errorf("failed to load config: %w", err)
				}

				var genLockID string
				if cfg.LockFile != nil {
					genLockID = cfg.LockFile.ID
				}

				outDir, err := filepath.Abs(filepath.Dir(cfgDir))
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
					// If there are multiple SDKs in a workflow we ensure output path is unique
					if targetOutput == outDir && !slices.Contains(testedTargets, name) {
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
	var errs []error
	for _, target := range testedTargets {
		// TODO: Once we have stable test reports we will probably want to use GH API to leave a PR comment/clean up old comments
		err := cli.Test(target)
		if err != nil {
			errs = append(errs, err)
		}

		var testReportURL string
		if genLockID, ok := targetLockIDs[target]; ok && genLockID != "" {
			testReportURL = formatTestReportURL(ctx, genLockID)
		} else {
			fmt.Println(fmt.Sprintf("No gen.lock ID found for target %s", target))
		}

		if testReportURL == "" {
			fmt.Println(fmt.Sprintf("No test report URL could be formed for target %s", target))
		} else {
			if err := writeTestReportComment(g, prNumber, testReportURL, target, err != nil); err != nil {
				fmt.Println(fmt.Sprintf("Failed to write test report comment: %s\n", err.Error()))
			}
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

func writeTestReportComment(g *git.Git, prNumber *int, testReportURL, targetName string, isError bool) error {
	if prNumber == nil {
		fmt.Println(fmt.Sprintf("No PR number found for target %s must skip test report comment", targetName))
		return nil
	}

	currentPRComments, _ := g.ListPRComments(*prNumber)
	for _, comment := range currentPRComments {
		commentBody := comment.GetBody()
		if strings.Contains(commentBody, fmt.Sprintf(testReportCommentPrefix, targetName)) {
			if err := g.DeletePRComment(comment.GetID()); err != nil {
				fmt.Println(fmt.Sprintf("Failed to delete existing test report comment: %s\n", err.Error()))
			}
		}
	}

	titleComment := "✅ Tests Passed ✅"
	if isError {
		titleComment = "❌ Tests Failed ❌"
	}

	body := titleComment + "\n\n" + fmt.Sprintf(testReportCommentPrefix, targetName) + " " + fmt.Sprintf("[here](%s)", testReportURL)

	err := g.WritePRComment(*prNumber, body, github.PullRequestComment{})

	return err
}
