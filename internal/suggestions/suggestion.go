package suggestions

import (
	"fmt"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/git"
	"regexp"
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
		output += fmt.Sprintf("Suggestion %d: %s\n", i+1, body.suggestions[i])
		output += fmt.Sprintf("\nExplanation %d: %s", i+1, body.explanations[i])
	}
	return output
}

func parseOutput(output string) prBodyInfo {
	var info prBodyInfo

	// Define regular expressions to extract suggestions and explanations
	suggestionRegexp := regexp.MustCompile(`(?m)^Suggestion:\s*(.*)`)
	explanationRegexp := regexp.MustCompile(`(?m)^Explanation:\s*(.*)`)

	// Find all matches for suggestions and explanations
	suggestions := suggestionRegexp.FindAllStringSubmatch(output, -1)
	explanations := explanationRegexp.FindAllStringSubmatch(output, -1)

	fmt.Println("suggestions all matches: ", suggestions)
	fmt.Println("explanations all matches: ", explanations)

	// Process suggestions and explanations
	for _, suggestion := range suggestions {
		if len(suggestion) >= 2 {
			info.suggestions = append(info.suggestions, strings.TrimSpace(suggestion[1]))
		}
	}

	for _, explanation := range explanations {
		if len(explanation) >= 2 {
			info.explanations = append(info.explanations, strings.TrimSpace(explanation[1]))
		}
	}

	return info
}

//func parseOutput(out string) prBodyInfo {
//	var suggestions, explanations []string
//
//	// Split the stdout into lines
//	scanner := bufio.NewScanner(strings.NewReader(out))
//	var suggestion, explanation string
//
//	for scanner.Scan() {
//		line := scanner.Text()
//
//		if strings.HasPrefix(line, "Suggestion:") {
//			// Save the previous annotation (if any) before starting a new one
//			if suggestion != "" || explanation != "" {
//				suggestions = append(suggestions, suggestion)
//				explanations = append(explanations, explanation)
//			}
//
//			// Reset the suggestion and explanation for the new block
//			suggestion = strings.TrimSpace(strings.TrimPrefix(line, "Suggestion:"))
//			explanation = ""
//		} else if strings.HasPrefix(line, "Explanation:") {
//			// Capture the Explanation block
//			explanation = strings.TrimSpace(strings.TrimPrefix(line, "Explanation:"))
//		} else {
//			// If there are empty lines or other text in between, add them to the current explanation
//			if explanation != "" {
//				explanation += "\n" + line
//			} else if suggestion != "" {
//				suggestion += "\n" + line
//			}
//		}
//	}
//
//	// Save the last annotation, if any
//	if suggestion != "" || explanation != "" {
//		suggestions = append(suggestions, suggestion)
//		explanations = append(explanations, explanation)
//	}
//
//	return prBodyInfo{
//		suggestions:  suggestions,
//		explanations: explanations,
//	}
//}
