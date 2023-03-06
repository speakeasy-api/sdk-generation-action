package logging

import (
	"fmt"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

func Info(msg string, args ...interface{}) {
	fmt.Println("INFO: ", fmt.Sprintf(msg, args...))
}

func Debug(msg string, args ...interface{}) {
	if environment.IsDebugMode() {
		fmt.Println("DEBUG: ", fmt.Sprintf(msg, args...))
	}
}
