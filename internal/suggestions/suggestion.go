package suggestions

import (
	"fmt"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"strconv"
	"strings"
)

type GithubAnnotation struct {
	Error       string
	LineNumber  int
	Suggestion  []string
	Explanation []string
}

func Suggest() error {
	if _, err := cli.Suggest(environment.GetOpenAPIDocs(), environment.GetOpenAPIDocOutput()); err != nil {
		return err
	}
	return nil
}

func ParseOutput(out string) []GithubAnnotation {
	lines := strings.Split(out, "\n")
	var fix, explanation []string
	var errorMessage string
	lineNumber := -1
	isFix, isExplanation := false, false
	outAnnotations := make([]GithubAnnotation, 0)

	for _, line := range lines {
		if strings.Contains(line, "[line") { // Grab line number
			isFix, isExplanation = false, false
			lineNumber, _ = getLineNumber(line)
			errorMessage = line
		} else if strings.Contains(line, "Suggested Fix:") { // Start tracking fix
			isFix = true
			fix = append(fix, strings.TrimPrefix(line, "Suggested Fix:"))
			continue
		} else if strings.Contains(line, "Explanation:") { // Start tracking explanation
			isFix = false
			isExplanation = true
			explanation = append(explanation, strings.TrimPrefix(line, "Explanation:"))
			continue
		}

		if line == "" {
			isFix, isExplanation = false, false
		}

		if isFix { // add to fix
			fix = append(fix, line)
		}
		if isExplanation { // add to explanation
			explanation = append(explanation, line)
		}

		if !isFix && !isExplanation && len(fix) != 0 && len(explanation) != 0 && lineNumber != -1 {
			outAnnotations = append(outAnnotations, GithubAnnotation{
				Error:       errorMessage,
				LineNumber:  lineNumber,
				Suggestion:  fix,
				Explanation: explanation,
			})
			fix, explanation, lineNumber, errorMessage = nil, nil, -1, ""
		}
	}

	return outAnnotations
}

func getLineNumber(errStr string) (int, error) {
	lineStr := strings.Split(errStr, "[line ")
	if len(lineStr) < 2 {
		return -1, fmt.Errorf("line number cannot be found in err %s", errStr)
	}

	lineNumStr := strings.Split(lineStr[1], "]")[0]
	lineNum, err := strconv.Atoi(lineNumStr)
	if err != nil {
		return -1, err
	}

	return lineNum, nil
}
