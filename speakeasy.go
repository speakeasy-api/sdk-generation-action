package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func runSpeakeasyCommand(args ...string) (string, error) {
	cmdPath := strings.Join([]string{baseDir, "speakeasy"}, string(os.PathSeparator))

	output, err := exec.Command(cmdPath, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error running speakeasy command: speakeasy %s - %w", strings.Join(args, " "), err)
	}

	return string(output), nil
}

func getSpeakeasyVersion() (string, error) {
	out, err := runSpeakeasyCommand("--version")
	if err != nil {
		return "", err
	}

	r := regexp.MustCompile(`.*?([0-9]+\.[0-9]+\.[0-9])$`)

	return r.FindStringSubmatch(strings.TrimSpace(out))[1], nil
}
