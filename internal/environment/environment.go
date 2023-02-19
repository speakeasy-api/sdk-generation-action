package environment

import (
	"os"
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
	return os.Getenv("INPUT_CREATE_RELEASE") == "true" || IsPHPPublished()
}

func GetAccessToken() string {
	return os.Getenv("INPUT_GITHUB_ACCESS_TOKEN")
}

func GetInvokeTime() time.Time {
	return invokeTime
}

func IsPythonPublished() bool {
	return os.Getenv("INPUT_PUBLISH_PYTHON") == "true"
}

func IsTypescriptPublished() bool {
	return os.Getenv("INPUT_PUBLISH_TYPESCRIPT") == "true"
}

func IsPHPPublished() bool {
	return os.Getenv("INPUT_PUBLISH_PHP") == "true"
}

func GetOpenAPIDocAuthHeader() string {
	return os.Getenv("INPUT_OPENAPI_DOC_AUTH_HEADER")
}

func GetOpenAPIDocAuthToken() string {
	return os.Getenv("INPUT_OPENAPI_DOC_AUTH_TOKEN")
}
