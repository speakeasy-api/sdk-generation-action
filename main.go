package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println(os.Getenv("INPUT_SPEAKEASY-VERSION"))
	fmt.Println(os.Getenv("INPUT_OPENAPI-DOC-LOCATION"))
	fmt.Println(os.Getenv("INPUT_GITHUB-ACCESS-TOKEN"))
	fmt.Println(os.Getenv("INPUT_LANGUAGES"))
}
