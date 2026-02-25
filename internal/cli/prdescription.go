package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/versioning-reports/versioning"
)

// PRDescriptionInput contains all data needed to generate a PR description via CLI.
// New fields can be added without breaking compatibility - older CLIs ignore unknown fields.
type PRDescriptionInput struct {
	// Report URLs
	LintingReportURL string `json:"linting_report_url,omitempty"`
	ChangesReportURL string `json:"changes_report_url,omitempty"`

	// Workflow context
	WorkflowName    string `json:"workflow_name,omitempty"`
	SourceBranch    string `json:"source_branch,omitempty"`
	FeatureBranch   string `json:"feature_branch,omitempty"`
	Target          string `json:"target,omitempty"`
	SpecifiedTarget string `json:"specified_target,omitempty"`

	// Generation type flags
	SourceGeneration bool `json:"source_generation,omitempty"`
	DocsGeneration   bool `json:"docs_generation,omitempty"`

	// Version information
	SpeakeasyVersion string `json:"speakeasy_version,omitempty"`
	ManualBump       bool   `json:"manual_bump,omitempty"`

	// Version report data
	VersionReport *versioning.MergedVersionReport `json:"version_report,omitempty"`
}

// PRDescriptionOutput contains the generated PR title and body.
type PRDescriptionOutput struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// supportsPRDescriptionCommand checks if the CLI supports the `ci pr-description` command.
// Returns false if the command is not available (older CLI version).
var supportsPRDescriptionCommand *bool

func SupportsPRDescriptionCommand() bool {
	if supportsPRDescriptionCommand != nil {
		return *supportsPRDescriptionCommand
	}

	baseDir := environment.GetBaseDir()
	cmdPath := filepath.Join(baseDir, "bin", "speakeasy")

	cmd := exec.Command(cmdPath, "ci", "pr-description", "--help")
	err := cmd.Run()

	result := err == nil
	supportsPRDescriptionCommand = &result

	if result {
		logging.Debug("CLI supports ci pr-description command")
	} else {
		logging.Debug("CLI does not support ci pr-description command, will use legacy fallback")
	}

	return result
}

// GeneratePRDescription calls the CLI to generate a PR title and body.
// Returns nil, nil if the CLI doesn't support this command (caller should use fallback).
func GeneratePRDescription(input PRDescriptionInput) (*PRDescriptionOutput, error) {
	if !SupportsPRDescriptionCommand() {
		return nil, nil
	}

	// Marshal input to JSON
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal PR description input: %w", err)
	}

	baseDir := environment.GetBaseDir()
	cmdPath := filepath.Join(baseDir, "bin", "speakeasy")

	logging.Info("Generating PR description via CLI")
	logging.Debug("PR description input: %s", string(inputJSON))

	cmd := exec.Command(cmdPath, "ci", "pr-description", "--input", "-")
	cmd.Dir = environment.GetRepoPath()
	cmd.Stdin = bytes.NewReader(inputJSON)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "SPEAKEASY_RUN_LOCATION=action")
	cmd.Env = append(cmd.Env, "SPEAKEASY_ENVIRONMENT=github")

	output, err := cmd.Output()
	if err != nil {
		// If the command fails, log and return nil so caller uses fallback
		logging.Info("CLI pr-description command failed: %v, using legacy fallback", err)
		return nil, nil
	}

	var result PRDescriptionOutput
	if err := json.Unmarshal(output, &result); err != nil {
		logging.Info("Failed to parse CLI pr-description output: %v, using legacy fallback", err)
		return nil, nil
	}

	logging.Debug("CLI generated PR title: %s", result.Title)
	return &result, nil
}
