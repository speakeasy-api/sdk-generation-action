package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("speakeasy-version: ", os.Getenv("INPUT_SPEAKEASY-VERSION"))
	fmt.Println("openapi-doc-location: ", os.Getenv("INPUT_OPENAPI-DOC-LOCATION"))
	fmt.Println("github-access-token: ", os.Getenv("INPUT_GITHUB-ACCESS-TOKEN"))
	fmt.Println("languages: ", os.Getenv("INPUT_LANGUAGES"))

	fmt.Println("Docker Container ENV")
	for _, env := range os.Environ() {
		fmt.Println(env)
	}
}
