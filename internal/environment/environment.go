package environment

import (
	"fmt"
	"os"
	"strings"
	"time"
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
	return os.Getenv("INPUT_DEBUG") == "true"
}

func ForceGeneration() bool {
	return os.Getenv("INPUT_FORCE") == "true"
}

func GetMode() string {
	return os.Getenv("INPUT_MODE")
}

func GetPinnedSpeakeasyVersion() string {
	return os.Getenv("INPUT_SPEAKEASY_VERSION")
}

func GetOpenAPIDocLocation() string {
	return os.Getenv("INPUT_OPENAPI_DOC_LOCATION")
}

func GetLanguages() string {
	return os.Getenv("INPUT_LANGUAGES")
}

func CreateGitRelease() bool {
	return os.Getenv("INPUT_CREATE_RELEASE") == "true" || IsLanguagePublished("php")
}

func GetAccessToken() string {
	return os.Getenv("INPUT_GITHUB_ACCESS_TOKEN")
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
