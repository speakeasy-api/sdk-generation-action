package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

type RunResults struct {
	LintingReport string
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

	if environment.ForceGeneration() {
		fmt.Println("force input enabled - setting SPEAKEASY_FORCE_GENERATION=true")
		os.Setenv("SPEAKEASY_FORCE_GENERATION", "true")
	}

	//if environment.ShouldOutputTests() {
	// TODO: Add CLI flag for outputting tests
	//}

	out, err := runSpeakeasyCommand(args...)
	if err != nil {
		return nil, fmt.Errorf("error running workflow: %w - %s", err, out)
	}

	lintingReportURL := getLintingReportURL(out)

	fmt.Println(out)
	return &RunResults{
		LintingReport: lintingReportURL,
	}, nil
}

var lintingReportRegex = regexp.MustCompile(`(?m).*?(https:\/\/app.speakeasyapi.dev\/org\/.*?\/.*?\/linting-report\/.*?)\s`)

func getLintingReportURL(out string) string {
	matches := lintingReportRegex.FindStringSubmatch(out)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}
