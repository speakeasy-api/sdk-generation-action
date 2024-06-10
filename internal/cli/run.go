package cli

import (
	"encoding/json"
	"fmt"
	"github.com/speakeasy-api/sdk-generation-action/internal/actions"
	"os"
	"regexp"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

type RunResults struct {
	LintingReportURL     string
	ChangesReportURL     string
	OpenAPIChangeSummary string
}

func Run(sourcesOnly bool, installationURLs map[string]string, repoURL string, repoSubdirectories map[string]string) (*RunResults, error) {
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

	tags := actions.ProcessRegistryTags()
	if len(tags) > 0 {
		tagString := strings.Join(tags, ",")
		args = append(args, "--registry-tags", tagString)
	}

	if environment.ForceGeneration() {
		fmt.Println("force input enabled - setting SPEAKEASY_FORCE_GENERATION=true")
		os.Setenv("SPEAKEASY_FORCE_GENERATION", "true")
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

	out, err := runSpeakeasyCommand(args...)
	if err != nil {
		return nil, fmt.Errorf("error running workflow: %w - %s", err, out)
	}

	lintingReportURL := getLintingReportURL(out)
	changesReportURL := getChangesReportURL(out)
	// read from file
	// ignore errors: the change summary is optional
	// and won't be available first run
	changeSummary, _ := os.ReadFile(file.Name())

	fmt.Println(out)
	return &RunResults{
		LintingReportURL:     lintingReportURL,
		ChangesReportURL:     changesReportURL,
		OpenAPIChangeSummary: string(changeSummary),
	}, nil
}

var (
	lintingReportRegex = regexp.MustCompile(`(?m).*?(https:\/\/app.speakeasyapi.dev\/org\/.*?\/.*?\/linting-report\/.*?)\s`)
	changesReportRegex = regexp.MustCompile(`(?m).*?(https:\/\/app.speakeasyapi.dev\/org\/.*?\/.*?\/changes-report\/.*?)\s`)
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
