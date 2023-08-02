package actions

import (
	"encoding/base64"
	"fmt"
	"golang.org/x/exp/rand"
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
		if k == "cli_output" {
			delimiter := randomDelimiter()
			err = printAndWriteString(f, fmt.Sprintf("%s<<%s\n%s\n%s", k, delimiter, v, delimiter))
			if err != nil {
				return err
			}
			continue
		}
		err = printAndWriteString(f, fmt.Sprintf("%s=%s\n", k, v))
		if err != nil {
			return err
		}
	}

	return nil
}

func printAndWriteString(f *os.File, out string) error {
	fmt.Print(out)
	if _, err := f.WriteString(out); err != nil {
		return fmt.Errorf("error writing output: %w", err)
	}
}

func randomDelimiter() string {
	b := make([]byte, 15)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}
