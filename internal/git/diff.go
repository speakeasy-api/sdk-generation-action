package git

import (
	"fmt"
	"strings"

	diffParser "github.com/speakeasy-api/git-diff-parser"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

func IsGitDiffSignificant(diff string, ignoreChangePatterns map[string]string) (bool, string, error) {
	if environment.ForceGeneration() {
		return true, "", nil
	}

	isSignificant, signifanceMsg, err := diffParser.SignificantChange(diff, func(diff *diffParser.FileDiff, change *diffParser.ContentChange) (bool, string) {
		if diff.ToFile == "gen.yaml" || diff.ToFile == "RELEASES.md" {
			return false, ""
		}
		if change.Type == diffParser.ContentChangeTypeNOOP {
			return false, ""
		}
		for pattern, replacement := range ignoreChangePatterns {
			if strings.Contains(change.From, pattern) && strings.Contains(change.To, replacement) {
				return false, ""
			}
		}
		if diff.Type == diffParser.FileDiffTypeModified {
			return true, fmt.Sprintf("significant diff %#v", diff)
		}

		return true, fmt.Sprintf("significant change %#v in %s", change, diff.ToFile)
	})
	if err != nil {
		return true, "", fmt.Errorf("failed to parse diff: %w", err)
	}
	return isSignificant, signifanceMsg, nil
}
