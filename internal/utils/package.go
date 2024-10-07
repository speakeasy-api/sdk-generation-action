package utils

import (
	"fmt"

	config "github.com/speakeasy-api/sdk-gen-config"
)

func GetPackageName(lang string, cfg *config.LanguageConfig) string {
	var packageName string
	switch lang {
	case "java":
		packageName = fmt.Sprintf("%s.%s", cfg.Cfg["groupID"], cfg.Cfg["artifactID"])
	case "terraform":
		packageName = fmt.Sprintf("%s/%s", cfg.Cfg["author"], cfg.Cfg["packageName"])
	default:
		packageName = fmt.Sprintf("%s", cfg.Cfg["packageName"])
	}

	return packageName
}
