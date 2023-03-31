package git

import (
	"fmt"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"regexp"
	"strings"
)

var (
	fileBoundaryRegex  = regexp.MustCompile(`(?m)^diff --git a\/.*? b\/.*?$`)
	fileNameRegex      = regexp.MustCompile(`(?m)^--- a\/(.*?)$`)
	versionChangeRegex = regexp.MustCompile(`_(sdk|gen)_?[vV]ersion`)
)

func IsGitDiffSignificant(diff string) bool {
	if environment.ForceGeneration() {
		return true
	}

	diffs := fileBoundaryRegex.Split(diff, -1)

	significantChanges := false

	for _, diff := range diffs {
		if strings.TrimSpace(diff) == "" {
			continue
		}

		filename := fileNameRegex.FindStringSubmatch(diff)[1]
		if !strings.Contains(filename, "gen.yaml") {
			lines := strings.Split(diff, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "+ ") && !versionChangeRegex.MatchString(line) {
					fmt.Println(line)
					significantChanges = true
					break
				}
			}
		}

		if significantChanges {
			break
		}
	}

	return significantChanges
}
