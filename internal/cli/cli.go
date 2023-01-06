package cli

import (
	"fmt"
	"regexp"
	"strings"
)

func GetSupportedLanguages() ([]string, error) {
	out, err := runSpeakeasyCommand("generate", "sdk", "--help")
	if err != nil {
		return nil, err
	}

	r := regexp.MustCompile(`available options: \[(.*?)\]`)

	langs := r.FindStringSubmatch(strings.TrimSpace(out))[1]

	return strings.Split(langs, ", "), nil
}

func GetSpeakeasyVersion() (string, error) {
	out, err := runSpeakeasyCommand("--version")
	if err != nil {
		return "", err
	}

	r := regexp.MustCompile(`.*?([0-9]+\.[0-9]+\.[0-9]+)$`)

	return r.FindStringSubmatch(strings.TrimSpace(out))[1], nil
}

func Generate(docPath, lang, outputDir string) error {
	out, err := runSpeakeasyCommand("generate", "sdk", "-s", docPath, "-l", lang, "-o", outputDir, "-y")
	if err != nil {
		return fmt.Errorf("error generating sdk: %w - %s", err, out)
	}
	fmt.Println(out)
	return nil
}
