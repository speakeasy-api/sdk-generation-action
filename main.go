package main

import (
	"log"
	"net/url"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

func main() {
	/*fmt.Println("speakeasy-version: ", os.Getenv("INPUT_SPEAKEASY-VERSION"))
	fmt.Println("openapi-doc-location: ", os.Getenv("INPUT_OPENAPI-DOC-LOCATION"))
	fmt.Println("github-access-token: ", os.Getenv("INPUT_GITHUB-ACCESS-TOKEN"))
	fmt.Println("languages: ", os.Getenv("INPUT_LANGUAGES"))

	fmt.Println("Docker Container ENV")
	for _, env := range os.Environ() {
		fmt.Println(env)
	}*/

	accessToken := os.Getenv("INPUT_GITHUB-ACCESS-TOKEN")
	if accessToken == "" {
		log.Fatal("No access token provided")
	}

	githubURL := os.Getenv("GITHUB_SERVER_URL")
	githubRepoLocation := os.Getenv("GITHUB_REPOSITORY")

	repoPath, err := url.JoinPath(githubURL, githubRepoLocation)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Cloning repo: ", repoPath)

	_, err = git.PlainClone("/repo", false, &git.CloneOptions{
		URL:      repoPath,
		Progress: os.Stdout,
		Auth: &http.BasicAuth{
			Username: "gen", // yes, this can be anything except an empty string
			Password: accessToken,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
