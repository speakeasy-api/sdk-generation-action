package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v63/github"
	"github.com/pkg/errors"
	"github.com/speakeasy-api/sdk-generation-action/internal/utils"
	"github.com/speakeasy-api/sdk-generation-action/internal/versionbumps"

	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/git"
	"github.com/speakeasy-api/sdk-generation-action/internal/run"

	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
)

func RunWorkflow() error {
	g, err := initAction()
	if err != nil {
		return err
	}

	if err := SetupEnvironment(); err != nil {
		return fmt.Errorf("failed to setup environment: %w", err)
	}

	// The top-level CLI can always use latest. The CLI itself manages pinned versions.
	resolvedVersion, err := cli.Download("latest", g)
	if err != nil {
		return err
	}

	// This flag is generally deprecated, it will not be provided on new action instances
	pinnedVersion := cli.GetVersion(environment.GetPinnedSpeakeasyVersion())
	if pinnedVersion != "latest" {
		resolvedVersion = pinnedVersion
		// This environment variable is read by the CLI to determine which version should be used to execute `run`
		if err := environment.SetCLIVersionToUse(pinnedVersion); err != nil {
			return fmt.Errorf("failed to set pinned speakeasy version: %w", err)
		}
	}

	mode := environment.GetMode()

	wf, err := configuration.GetWorkflowAndValidateLanguages(true)
	if err != nil {
		return err
	}

	sourcesOnly := wf.Targets == nil || len(wf.Targets) == 0

	branchName := ""
	var pr *github.PullRequest
	if mode == environment.ModePR {
		var err error
		branchName, pr, err = g.FindExistingPR("", environment.ActionRunWorkflow, sourcesOnly)
		if err != nil {
			return err
		}

		if pr != nil {
			os.Setenv("GH_PULL_REQUEST", *pr.URL)
		}
	}

	// We want to stay on main if we're pushing code samples because we want to tag the code samples with `main`
	if !environment.PushCodeSamplesOnly() && !environment.IsTestMode() {
		branchName, err = g.FindOrCreateBranch(branchName, environment.ActionRunWorkflow)
		if err != nil {
			return err
		}
	}

	success := false
	defer func() {
		if shouldDeleteBranch(success) {
			if err := g.DeleteBranch(branchName); err != nil {
				logging.Debug("failed to delete branch %s: %v", branchName, err)
			}
		}
	}()

	if branchName != "" {
		os.Setenv("SPEAKEASY_ACTIVE_BRANCH", branchName)
	}

	runRes, outputs, err := run.Run(g, pr, wf)
	if err != nil {
		fmt.Println("error received: %v", err)
		// Check if this is a custom code clean apply failure
		if strings.HasPrefix(err.Error(), "Generation failed as a result of custom code application conflict") {
			fmt.Println("Inside")
			if conflictErr := handleCustomCodeConflict(g, err.Error()); conflictErr != nil {
				logging.Error("Failed to handle custom code conflict: %v", conflictErr)
				// Fall through to original error handling
			}
		}
		if err := setOutputs(outputs); err != nil {
			logging.Debug("failed to set outputs: %v", err)
		}
		return err
	}

	anythingRegenerated := false

	var releaseInfo releases.ReleasesInfo
	runResultInfo, err := json.MarshalIndent(runRes, "", "  ")
	if err != nil {
		logging.Debug("failed to marshal runRes : %s\n", err)
	} else {
		logging.Debug("Result of running the command is: %s\n", runResultInfo)
	}
	if runRes.GenInfo != nil {
		docVersion := runRes.GenInfo.OpenAPIDocVersion
		resolvedVersion = runRes.GenInfo.SpeakeasyVersion

		releaseInfo = releases.ReleasesInfo{
			ReleaseTitle:       environment.GetInvokeTime().Format("2006-01-02 15:04:05"),
			DocVersion:         docVersion,
			SpeakeasyVersion:   resolvedVersion,
			GenerationVersion:  runRes.GenInfo.GenerationVersion,
			DocLocation:        environment.GetOpenAPIDocLocation(),
			Languages:          map[string]releases.LanguageReleaseInfo{},
			LanguagesGenerated: map[string]releases.GenerationInfo{},
		}

		for _, supportedTargetName := range cli.GetSupportedTargetNames() {
			langGenInfo, ok := runRes.GenInfo.Languages[supportedTargetName]
			if ok && outputs[utils.OutputTargetRegenerated(supportedTargetName)] == "true" {
				anythingRegenerated = true

				path := outputs[utils.OutputTargetDirectory(supportedTargetName)]
				path = strings.TrimPrefix(path, "./")

				releaseInfo.LanguagesGenerated[supportedTargetName] = releases.GenerationInfo{
					Version: langGenInfo.Version,
					Path:    path,
				}

				if published, ok := outputs[utils.OutputTargetPublish(supportedTargetName)]; ok && published == "true" {
					releaseInfo.Languages[supportedTargetName] = releases.LanguageReleaseInfo{
						PackageName: langGenInfo.PackageName,
						Version:     langGenInfo.Version,
						Path:        path,
					}
				}
			}
		}

		if environment.PushCodeSamplesOnly() {
			// If we're just pushing code samples we don't want to raise a PR
			return nil
		}

		releasesDir, err := getReleasesDir()
		if err != nil {
			return err
		}

		if err := releases.UpdateReleasesFile(releaseInfo, releasesDir); err != nil {
			logging.Error("error while updating releases file: %v", err.Error())
			return err
		}

		if _, err := g.CommitAndPush(docVersion, resolvedVersion, "", environment.ActionRunWorkflow, false, runRes.VersioningInfo.VersionReport); err != nil {
			return err
		}
	}

	outputs["resolved_speakeasy_version"] = resolvedVersion
	if sourcesOnly {
		if _, err := g.CommitAndPush("", resolvedVersion, "", environment.ActionRunWorkflow, sourcesOnly, nil); err != nil {
			return err
		}
	}

	// If test mode is successful to this point, exit here
	if environment.IsTestMode() {
		success = true
		return nil
	}

	if err := finalize(finalizeInputs{
		Outputs:              outputs,
		BranchName:           branchName,
		AnythingRegenerated:  anythingRegenerated,
		SourcesOnly:          sourcesOnly,
		Git:                  g,
		VersioningInfo:       runRes.VersioningInfo,
		LintingReportURL:     runRes.LintingReportURL,
		ChangesReportURL:     runRes.ChangesReportURL,
		OpenAPIChangeSummary: runRes.OpenAPIChangeSummary,
		GenInfo:              runRes.GenInfo,
		currentRelease:       &releaseInfo,
		releaseNotes:         runRes.ReleaseNotes,
	}); err != nil {
		return err
	}

	success = true

	return nil
}

func handleCustomCodeConflict(g *git.Git, errorMsg string) error {
	logging.Info("Handling custom code conflict: %s", errorMsg)
	
	workspaceDir := filepath.Join(environment.GetWorkspace(), "repo", environment.GetWorkingDirectory())
	
	// 1. Capture the current diff into a patchfile
	timestamp := time.Now().Unix()
	patchFile := fmt.Sprintf("conflict-patch-%d.patch", timestamp)
	patchPath := filepath.Join(workspaceDir, patchFile)
	
	logging.Info("Capturing diff to patchfile: %s", patchPath)
	diffCmd := exec.Command("git", "diff", "--binary")
	diffCmd.Dir = workspaceDir
	diffOutput, err := diffCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to capture diff: %w", err)
	}
	
	if err := os.WriteFile(patchPath, diffOutput, 0644); err != nil {
		return fmt.Errorf("failed to write patch file: %w", err)
	}
	
	// 2. Reset the github worktree
	logging.Info("Resetting worktree")
	if err := g.Reset("--hard", "HEAD"); err != nil {
		return fmt.Errorf("failed to reset worktree: %w", err)
	}
	
	// 3. Create a new branch: speakeasy/resolve-{ts}
	branchName := fmt.Sprintf("speakeasy/resolve-%d", timestamp)
	logging.Info("Creating conflict resolution branch: %s", branchName)
	
	checkoutCmd := exec.Command("git", "checkout", "-b", branchName)
	checkoutCmd.Dir = workspaceDir
	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("failed to create branch %s: %w", branchName, err)
	}
	
	// 4. Apply the patchfile using --3way
	logging.Info("Applying patch with 3-way merge")
	applyCmd := exec.Command("git", "apply", "--3way", patchFile)
	applyCmd.Dir = workspaceDir
	if err := applyCmd.Run(); err != nil {
		// This is expected to fail with conflicts - we continue
		logging.Info("Patch application failed as expected (conflicts): %v", err)
	}
	
	// 5. Stage all changes
	logging.Info("Staging all changes")
	if err := g.Add("."); err != nil {
		return fmt.Errorf("failed to stage changes: %w", err)
	}
	
	// 6. Commit with the specified message
	commitMsg := `Apply patch with conflicts - requires manual resolution

- Patch could not be applied cleanly
- Conflict markers indicate areas needing attention
- Review and resolve conflicts, then merge this PR`
	
	logging.Info("Committing changes")
	commitCmd := exec.Command("git", "commit", "-m", commitMsg)
	commitCmd.Dir = workspaceDir
	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}
	
	// Push the branch
	logging.Info("Pushing branch %s", branchName)
	pushCmd := exec.Command("git", "push", "origin", branchName)
	pushCmd.Dir = workspaceDir
	if err := pushCmd.Run(); err != nil {
		return fmt.Errorf("failed to push branch: %w", err)
	}
	
	// 7. Create the conflict resolution PR
	logging.Info("Creating conflict resolution PR")
	
	// Get current user for assignee
	currentUser := os.Getenv("GITHUB_ACTOR")
	if currentUser == "" {
		currentUser = "self"
	}
	
	prTitle := fmt.Sprintf("🔧 Resolve conflicts: %s", extractPatchDescription(errorMsg))
	prBody := `This patch could not be applied cleanly. Please resolve the conflicts and merge.`
	
	// Use gh CLI to create PR
	ghCmd := exec.Command("gh", "pr", "create",
		"--title", prTitle,
		"--body", prBody,
		"--assignee", currentUser)
	ghCmd.Dir = workspaceDir
	ghCmd.Env = append(os.Environ(), "GH_TOKEN="+os.Getenv("GITHUB_TOKEN"))
	
	if output, err := ghCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create PR: %w, output: %s", err, string(output))
	}
	
	// Clean up patch file
	os.Remove(patchPath)
	
	logging.Info("Successfully created conflict resolution workflow")
	return nil
}

func extractPatchDescription(errorMsg string) string {
	// Try to extract meaningful description from error message
	// This is a simple implementation - could be enhanced based on actual error format
	if len(errorMsg) > 50 {
		return errorMsg[:47] + "..."
	}
	return errorMsg
}

func shouldDeleteBranch(isSuccess bool) bool {
	isDirectMode := environment.GetMode() == environment.ModeDirect
	return !environment.IsDebugMode() && !environment.IsTestMode() && (isDirectMode || !isSuccess)
}

type finalizeInputs struct {
	Outputs              map[string]string
	BranchName           string
	AnythingRegenerated  bool
	SourcesOnly          bool
	Git                  *git.Git
	LintingReportURL     string
	ChangesReportURL     string
	OpenAPIChangeSummary string
	VersioningInfo       versionbumps.VersioningInfo
	GenInfo              *run.GenerationInfo
	currentRelease       *releases.ReleasesInfo
	// key is language target name, value is release notes
	releaseNotes map[string]string
}

// Sets outputs and creates or adds releases info
func finalize(inputs finalizeInputs) error {
	// If nothing was regenerated, we don't need to do anything
	if !inputs.AnythingRegenerated && !inputs.SourcesOnly {
		return nil
	}

	branchName, err := inputs.Git.FindAndCheckoutBranch(inputs.BranchName)
	if err != nil {
		return err
	}

	defer func() {
		inputs.Outputs["branch_name"] = branchName

		if err := setOutputs(inputs.Outputs); err != nil {
			logging.Debug("failed to set outputs: %v", err)
		}
	}()

	logging.Info("getMode from the environment: %s\n", environment.GetMode())
	logging.Info("INPUT_ENABLE_SDK_CHANGELOG: %s", environment.GetSDKChangelog())
	switch environment.GetMode() {
	case environment.ModePR:
		branchName, pr, err := inputs.Git.FindExistingPR(branchName, environment.ActionFinalize, inputs.SourcesOnly)
		if err != nil {
			return err
		}
		pr, err = inputs.Git.CreateOrUpdatePR(git.PRInfo{
			BranchName:           branchName,
			ReleaseInfo:          inputs.currentRelease,
			PreviousGenVersion:   inputs.Outputs["previous_gen_version"],
			PR:                   pr,
			SourceGeneration:     inputs.SourcesOnly,
			LintingReportURL:     inputs.LintingReportURL,
			ChangesReportURL:     inputs.ChangesReportURL,
			VersioningInfo:       inputs.VersioningInfo,
			OpenAPIChangeSummary: inputs.OpenAPIChangeSummary,
		})

		if err != nil {
			return err
		}

		if pr != nil {
			os.Setenv("GH_PULL_REQUEST", *pr.URL)
		}

		// If we are in PR mode and testing should be triggered by this PR we will attempt to fire an empty commit from our app so trigger github actions checks
		// for more info on why this is necessary see https://github.com/peter-evans/create-pull-request/blob/main/docs/concepts-guidelines.md#workarounds-to-trigger-further-workflow-runs
		// If the customer has manually set up a PR_CREATION_PAT we will not do this
		if inputs.GenInfo != nil && inputs.GenInfo.HasTestingEnabled && os.Getenv("PR_CREATION_PAT") == "" {
			sanitizedBranchName := strings.TrimPrefix(branchName, "refs/heads/")
			if err := cli.FireEmptyCommit(os.Getenv("GITHUB_REPOSITORY_OWNER"), git.GetRepo(), sanitizedBranchName); err != nil {
				fmt.Println("Failed to create empty commit to trigger testing workflow", err)
			}
		}

	case environment.ModeDirect:
		var releaseInfo *releases.ReleasesInfo
		var oldReleaseInfo string
		var languages map[string]releases.LanguageReleaseInfo
		var targetSpecificReleaseNotes releases.TargetReleaseNotes = nil
		if !inputs.SourcesOnly {
			releaseInfo = inputs.currentRelease
			languages = releaseInfo.Languages
			oldReleaseInfo = releaseInfo.String()
			logging.Info("release Notes: %+v", inputs.releaseNotes)
			if environment.GetSDKChangelog() == "true" && inputs.releaseNotes != nil {
				targetSpecificReleaseNotes = inputs.releaseNotes
			}

			// We still read from releases info for terraform generations since they use the goreleaser
			// Read from Releases.md for terraform generations
			if inputs.Outputs[utils.OutputTargetRegenerated("terraform")] == "true" {
				releaseInfo, err = getReleasesInfo()
				oldReleaseInfo = releaseInfo.String()
				targetSpecificReleaseNotes = nil
				languages = releaseInfo.Languages
				if err != nil {
					return err
				}
			}
		}

		commitHash, err := inputs.Git.MergeBranch(branchName)
		if err != nil {
			return err
		}

		if !inputs.SourcesOnly {
			if err := inputs.Git.CreateRelease(oldReleaseInfo, languages, inputs.Outputs, targetSpecificReleaseNotes); err != nil {
				return err
			}
		}

		inputs.Outputs["commit_hash"] = commitHash

		// add merging branch registry tag
		if err = addDirectModeBranchTagging(); err != nil {
			return errors.Wrap(err, "failed to tag registry images")
		}

	}

	return nil
}

func addDirectModeBranchTagging() error {
	wf, err := configuration.GetWorkflowAndValidateLanguages(true)
	if err != nil {
		return err
	}

	branch := strings.TrimPrefix(os.Getenv("GITHUB_REF"), "refs/heads/")

	var sources, targets []string
	// a tag that is applied if the target contributing is published
	var isPublished bool
	// the tagging library treats targets synonymously with code samples
	if specificTarget := environment.SpecifiedTarget(); specificTarget != "" {
		if target, ok := wf.Targets[specificTarget]; ok {
			isPublished = target.IsPublished()
			if source, ok := wf.Sources[target.Source]; ok && source.Registry != nil {
				sources = append(sources, target.Source)
			}

			if target.CodeSamples != nil && target.CodeSamples.Registry != nil {
				targets = append(targets, specificTarget)
			}
		}
	} else {
		for name, target := range wf.Targets {
			isPublished = isPublished || target.IsPublished()
			if source, ok := wf.Sources[target.Source]; ok && source.Registry != nil {
				sources = append(sources, target.Source)
			}

			if target.CodeSamples != nil && target.CodeSamples.Registry != nil {
				targets = append(targets, name)
			}
		}
	}
	if (len(sources) > 0 || len(targets) > 0) && branch != "" {
		tags := []string{branch}
		if isPublished {
			tags = append(tags, "published")
		}
		return cli.Tag(tags, sources, targets)
	}

	return nil
}

func getReleasesInfo() (*releases.ReleasesInfo, error) {
	releasesDir, err := getReleasesDir()
	if err != nil {
		return nil, err
	}

	releasesInfo, err := releases.GetLastReleaseInfo(releasesDir)
	if err != nil {
		return nil, err
	}

	return releasesInfo, nil
}
