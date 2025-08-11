package logging

import (
	"fmt"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

func Info(msg string, args ...interface{}) {
	fmt.Println("INFO: ", fmt.Sprintf(msg, args...))
}

func Error(msg string, args ...interface{}) {
	fmt.Println("ERROR: ", fmt.Sprintf(msg, args...))
}

func Debug(msg string, args ...interface{}) {
	if environment.IsDebugMode() {
		fmt.Println("::debug::", fmt.Sprintf(msg, args...))
	}
}
