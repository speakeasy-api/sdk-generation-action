package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"gopkg.in/yaml.v3"
)

const defaultAPIURL = "https://api.prod.speakeasyapi.dev"

type logProxyLevel string

const (
	logProxyLevelInfo  logProxyLevel = "info"
	logProxyLevelError logProxyLevel = "error"
)

type logProxyEntry struct {
	LogLevel logProxyLevel          `json:"log_level"`
	Message  string                 `json:"message"`
	Source   string                 `json:"source"`
	Tags     map[string]interface{} `json:"tags"`
}

func LogActionResult() error {
	key := os.Getenv("SPEAKEASY_API_KEY")
	if key == "" {
		return fmt.Errorf("no speakeasy api key available.")
	}

	logLevel := logProxyLevelInfo
	logMessage := "Success in Github Action"
	if !strings.Contains(strings.ToLower(os.Getenv("GH_ACTION_RESULT")), "success") {
		logLevel = logProxyLevelError
		logMessage = "Failure in Github Action"
	}

	request := logProxyEntry{
		LogLevel: logLevel,
		Message:  logMessage,
		Source:   "gh_action",
		Tags: map[string]interface{}{
			"target":            os.Getenv("TARGET"),
			"speakeasy_version": os.Getenv("RESOLVED_SPEAKEASY_VERSION"),
			"gh_repo":           os.Getenv("GITHUB_REPOSITORY"),
			"gh_action_version": os.Getenv("GH_ACTION_VERSION"),
			"gh_action_step":    os.Getenv("GH_ACTION_STEP"),
			"gh_action_result":  os.Getenv("GH_ACTION_RESULT"),
			"gh_action_run":     fmt.Sprintf("https://github.com/%s/actions/runs/%s", os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID")),
			"run_origin":        "gh_action",
		},
	}

	languages := environment.GetLanguages()
	languages = strings.ReplaceAll(languages, "\\n", "\n")
	langs := []string{}
	if err := yaml.Unmarshal([]byte(languages), &langs); err != nil {
		fmt.Println("No language provided in github actions config.")
	}
	if len(langs) > 0 {
		request.Tags["language"] = langs[0]
	}

	if os.Getenv("GITHUB_REPOSITORY") != "" {
		request.Tags["gh_repo"] = strings.Split(os.Getenv("GITHUB_REPOSITORY"), "/")[0]
	}

	body, err := json.Marshal(&request)
	if err != nil {
		return err
	}

	baseURL := os.Getenv("SPEAKEASY_SERVER_URL")
	if baseURL == "" {
		baseURL = defaultAPIURL
	}

	req, err := http.NewRequest("POST", baseURL+"/v1/log/proxy", bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", key)

	client := &http.Client{
		Timeout: time.Second * 5,
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Print("failure sending log to speakeasy.")
		return nil
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("failure sending log to speakeasy with status %s.", resp.Status)
	}

	return nil
}
