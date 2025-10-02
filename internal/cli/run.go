package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/registry"
	"github.com/speakeasy-api/versioning-reports/versioning"
)

const BumpOverrideEnvVar = "SPEAKEASY_BUMP_OVERRIDE"

type RunResults struct {
	LintingReportURL      string
	ChangesReportURL      string
	OpenAPIChangeSummary  string
	CustomCodeApplied bool
}

func Run(sourcesOnly bool, installationURLs map[string]string, repoURL string, repoSubdirectories map[string]string, manualVersionBump *versioning.BumpType) (*RunResults, error) {
	args := []string{
		"run",
	}

	if sourcesOnly {
		args = append(args, "-s", "all")
	} else {
		specifiedTarget := environment.SpecifiedTarget()
		if specifiedTarget != "" {
			args = append(args, "-t", specifiedTarget)
		} else {
			args = append(args, "-t", "all")
		}
		urls, err := json.Marshal(installationURLs)
		if err != nil {
			return nil, fmt.Errorf("error marshalling installation urls: %w", err)
		}
		args = append(args, "--installationURLs", string(urls))

		subdirs, err := json.Marshal(repoSubdirectories)
		if err != nil {
			return nil, fmt.Errorf("error marshalling repo subdirectories: %w", err)
		}
		args = append(args, "--repo-subdirs", string(subdirs))
	}

	if repoURL != "" {
		args = append(args, "-r", repoURL)
	}

	tags := registry.ProcessRegistryTags()
	if len(tags) > 0 {
		tagString := strings.Join(tags, ",")
		args = append(args, "--registry-tags", tagString)
	}

	if environment.SetVersion() != "" {
		args = append(args, "--set-version", environment.SetVersion())
	}

	// If we are in PR mode we skip testing on generation, this should run as a PR check
	if environment.SkipTesting() || (environment.GetMode() == environment.ModePR && !sourcesOnly) {
		args = append(args, "--skip-testing")
	}

	if environment.ForceGeneration() {
		fmt.Println("\nforce input enabled - setting SPEAKEASY_FORCE_GENERATION=true")
		os.Setenv("SPEAKEASY_FORCE_GENERATION", "true")
	}

	if manualVersionBump != nil {
		os.Setenv(BumpOverrideEnvVar, string(*manualVersionBump))
	}

	//if environment.ShouldOutputTests() {
	// TODO: Add CLI flag for outputting tests
	//}
	file, err := os.CreateTemp(os.TempDir(), "speakeasy-change-summary")
	if err != nil {
		return nil, fmt.Errorf("error creating change summary file: %w", err)
	}
	os.Setenv("SPEAKEASY_OPENAPI_CHANGE_SUMMARY", file.Name())
	err = file.Close()
	if err != nil {
		return nil, fmt.Errorf("error closing change summary file: %w", err)
	}

	fmt.Println("Before=====================================================================")
	out, err := runSpeakeasyCommand(args...)
	fmt.Println("After===================================================================== %v", out)
	if err != nil {
		return nil, fmt.Errorf("error running workflow: %w - %s", err, out)
	}

	lintingReportURL := getLintingReportURL(out)
	changesReportURL := getChangesReportURL(out)
	customCodeApplied := !strings.Contains(out, "failed to apply custom code cleanly")
	// read from file
	// ignore errors: the change summary is optional
	// and won't be available first run
	changeSummary, _ := os.ReadFile(file.Name())

	fmt.Println(out)
	return &RunResults{
		LintingReportURL:     lintingReportURL,
		ChangesReportURL:     changesReportURL,
		OpenAPIChangeSummary: string(changeSummary),
		CustomCodeApplied: customCodeApplied,
	}, nil
}

var (
	lintingReportRegex = regexp.MustCompile(`(?m).*?(https:\/\/app.speakeasy.com\/org\/.*?\/.*?\/linting-report\/.*?)\s`)
	changesReportRegex = regexp.MustCompile(`(?m).*?(https:\/\/app.speakeasy.com\/org\/.*?\/.*?\/changes-report\/.*?)\s`)
)

func getLintingReportURL(out string) string {
	matches := lintingReportRegex.FindStringSubmatch(out)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

func getChangesReportURL(out string) string {
	matches := changesReportRegex.FindStringSubmatch(out)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}
