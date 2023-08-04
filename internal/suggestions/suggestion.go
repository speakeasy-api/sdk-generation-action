package suggestions

import (
	"fmt"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/git"
	"strings"
)

type prBodyInfo struct {
	suggestions  []string
	explanations []string
}

func Suggest(docPath, maxSuggestions string) (string, error) {
	out, err := cli.Suggest(docPath, maxSuggestions, environment.GetOpenAPIDocOutput())
	if err != nil {
		return "", err
	}
	return out, nil
}

func WriteSuggestions(g *git.Git, prNumber *int, out string) error {
	body := parseOutput(out)
	output := formatSuggestionsAndExplanations(body)
	if len(output) > 0 {
		// Writes suggestions and explanations in PR body
		if err := g.WritePRBody(prNumber, output); err != nil {
			return fmt.Errorf("error writing PR body: %w", err)
		}
	}

	return nil
}

func formatSuggestionsAndExplanations(body prBodyInfo) string {
	var output string
	for i := 0; i < len(body.suggestions); i++ {
		output += fmt.Sprintf("**Suggestion %d**: %s\n\n", i+1, body.suggestions[i])
		output += fmt.Sprintf("**Explanation %d**: %s\n\n", i+1, body.explanations[i])
	}
	return output
}

func parseOutput(out string) prBodyInfo {
	var info prBodyInfo
	lines := strings.Split(out, "\n")
	suggestion, explanation := "", ""
	isSuggestion, isExplanation := false, false

	for _, line := range lines {
		if strings.Contains(line, "Suggestion:") {
			isSuggestion = true
			if strings.TrimSpace(suggestion) != "" {
				info.suggestions = append(info.suggestions, suggestion)
			}
			suggestion = ""
			continue
		} else if strings.Contains(line, "Explanation:") {
			isSuggestion = false
			isExplanation = true
			if strings.TrimSpace(explanation) != "" {
				info.explanations = append(info.explanations, explanation)
			}
			explanation = ""
			continue
		}

		if strings.TrimSpace(line) == "" {
			isSuggestion, isExplanation = false, false
		}

		if isSuggestion {
			suggestion += line
		}
		if isExplanation {
			explanation += line
		}
	}

	if strings.TrimSpace(suggestion) != "" {
		info.suggestions = append(info.suggestions, suggestion)
	}
	if strings.TrimSpace(explanation) != "" {
		info.explanations = append(info.explanations, explanation)
	}

	return info
}
