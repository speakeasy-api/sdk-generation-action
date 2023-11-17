package git

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

var (
	fileBoundaryRegex  = regexp.MustCompile(`(?m)^diff --git a\/.*? b\/.*?$`)
	fileNameRegex      = regexp.MustCompile(`(?m)^--- a\/(.*?)$`)
	versionChangeRegex = regexp.MustCompile(`_?(sdk|gen|Gen|SDK)_?[vV]ersion`)
	userAgentRegex     = regexp.MustCompile(`speakeasy-sdk/`)
)

func IsGitDiffSignificant(diff string, ignoreChangePatterns map[string]string) bool {
	if environment.ForceGeneration() {
		return true
	}

	diffs := fileBoundaryRegex.Split(diff, -1)

	significantChanges := false

outer:
	for _, diff := range diffs {
		if strings.TrimSpace(diff) == "" {
			continue
		}

		matches := fileNameRegex.FindStringSubmatch(diff)
		if len(matches) != 2 {
			continue
		}

		filename := fileNameRegex.FindStringSubmatch(diff)[1]
		if strings.Contains(filename, "gen.yaml") {
			continue
		}

		lines := strings.Split(diff, "\n")
		for i, line := range lines {
			isAddition := strings.HasPrefix(line, "+ ") || strings.HasPrefix(line, "+\t")
			isSpecificPatternIgnored := false
			if i > 1 && isAddition && strings.HasPrefix(lines[i-1], "- ") {
				priorLine := lines[i-1]
				for fromPattern, toPattern := range ignoreChangePatterns {
					if strings.Contains(priorLine, fromPattern) && strings.Contains(line, toPattern) {
						isSpecificPatternIgnored = true
						break
					}
				}

			}
			isNotVersionChange := !versionChangeRegex.MatchString(line)
			isNotUAChange := !userAgentRegex.MatchString(line)

			significantChanges = isAddition && isNotVersionChange && isNotUAChange && !isSpecificPatternIgnored

			if significantChanges {
				fmt.Println(line)
				break outer
			}
		}
	}

	return significantChanges
}
