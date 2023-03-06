package actions

import (
	"fmt"
	"os"

	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
)

func setOutputs(outputs map[string]string) error {
	logging.Info("Setting outputs:")

	outputFile := os.Getenv("GITHUB_OUTPUT")

	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("error opening output file: %w", err)
	}
	defer f.Close()

	for k, v := range outputs {
		out := fmt.Sprintf("%s=%s\n", k, v)
		fmt.Print(out)

		if _, err := f.WriteString(out); err != nil {
			return fmt.Errorf("error writing output: %w", err)
		}
	}

	return nil
}
