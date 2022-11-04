package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println(os.Getenv("INPUT_SPEAKEASY_VERSION"))
	fmt.Println(os.Getenv("INPUT_OPENAPI_DOC_LOCATION"))
	fmt.Println(os.Getenv("INPUT_GITHUB_ACCESS_TOKEN"))
	fmt.Println(os.Getenv("INPUT_LANGUAGES"))
}
