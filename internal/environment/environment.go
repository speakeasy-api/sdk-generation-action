package environment

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Mode string

const (
	ModeDirect Mode = "direct"
	ModePR     Mode = "pr"
)

type Action string

const (
	ActionValidate           Action = "validate"
	ActionGenerate           Action = "generate"
	ActionSuggest            Action = "suggest"
	ActionFinalize           Action = "finalize"
	ActionFinalizeSuggestion Action = "finalize-suggestion"
	ActionRelease            Action = "release"
)

var (
	baseDir    = "/"
	invokeTime = time.Now()
)

func init() {
	// Allows us to run this locally
	if os.Getenv("SPEAKEASY_ENVIRONMENT") == "local" {
		baseDir = "./"
	}
}

func GetBaseDir() string {
	return baseDir
}

func IsDebugMode() bool {
	return os.Getenv("INPUT_DEBUG") == "true" || os.Getenv("RUNNER_DEBUG") == "1"
}

func ForceGeneration() bool {
	return os.Getenv("INPUT_FORCE") == "true"
}

func GetMode() Mode {
	mode := os.Getenv("INPUT_MODE")
	if mode == "" {
		return ModeDirect
	}

	return Mode(mode)
}

func GetAction() Action {
	action := os.Getenv("INPUT_ACTION")
	if action == "" {
		return ActionGenerate
	}

	return Action(action)
}

func GetPinnedSpeakeasyVersion() string {
	return os.Getenv("INPUT_SPEAKEASY_VERSION")
}

func GetOpenAPIDocLocation() string {
	return os.Getenv("INPUT_OPENAPI_DOC_LOCATION")
}

func GetOpenAPIDocs() string {
	return os.Getenv("INPUT_OPENAPI_DOCS")
}

func GetOpenAPIDocOutput() string {
	return os.Getenv("INPUT_OPENAPI_DOC_OUTPUT")
}

func GetLanguages() string {
	return os.Getenv("INPUT_LANGUAGES")
}

func CreateGitRelease() bool {
	return os.Getenv("INPUT_CREATE_RELEASE") == "true" || IsLanguagePublished("php") || IsLanguagePublished("terraform")
}

func GetAccessToken() string {
	return os.Getenv("INPUT_GITHUB_ACCESS_TOKEN")
}

func GetGPGFingerprint() string {
	return os.Getenv("INPUT_GPG_FINGERPRINT")
}

func GetInvokeTime() time.Time {
	return invokeTime
}

func IsLanguagePublished(lang string) bool {
	if lang == "go" {
		return os.Getenv("INPUT_CREATE_RELEASE") == "true"
	}

	return os.Getenv(fmt.Sprintf("INPUT_PUBLISH_%s", strings.ToUpper(lang))) == "true"
}

func IsJavaPublished() bool {
	return os.Getenv("INPUT_PUBLISH_JAVA") == "true"
}

func GetOpenAPIDocAuthHeader() string {
	return os.Getenv("INPUT_OPENAPI_DOC_AUTH_HEADER")
}

func GetOpenAPIDocAuthToken() string {
	return os.Getenv("INPUT_OPENAPI_DOC_AUTH_TOKEN")
}

func GetWorkflowName() string {
	return os.Getenv("GITHUB_WORKFLOW")
}

func GetWorkflowEventPayloadPath() string {
	return os.Getenv("GITHUB_EVENT_PATH")
}

func GetBranchName() string {
	return os.Getenv("INPUT_BRANCH_NAME")
}

func GetRef() string {
	return os.Getenv("GITHUB_REF")
}

func GetPreviousGenVersion() string {
	return os.Getenv("INPUT_PREVIOUS_GEN_VERSION")
}

func GetWorkingDirectory() string {
	return os.Getenv("INPUT_WORKING_DIRECTORY")
}

func GetRepo() string {
	return os.Getenv("GITHUB_REPOSITORY")
}

func GetGithubServerURL() string {
	return os.Getenv("GITHUB_SERVER_URL")
}

func GetWorkspace() string {
	return os.Getenv("GITHUB_WORKSPACE")
}

func ShouldOutputTests() bool {
	return os.Getenv("INPUT_OUTPUT_TESTS") == "true"
}
