package main

import (
	"fmt"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	"os"
)

func main() {
	fs := memfs.New()
	r, err := git.Clone(memory.NewStorage(), fs, &git.CloneOptions{
		URL: "https://github.com/speakeasy-api/speakeasy-client-sdk-java",
	})
	if err != nil {
		fmt.Printf("error getting worktree: %s", err)
		os.Exit(0)
	}

	w, err := r.Worktree()
	if err != nil {
		fmt.Printf("error getting worktree: %s", err)
		os.Exit(0)
	}

	status, err := w.Status()
	if err != nil {
		fmt.Printf("error getting status: %s", err)
		os.Exit(0)
	}
	fmt.Printf("status clean before rename: %t\n", status.IsClean())

	err = fs.Rename("build/pom.xml", "build/pom.xml.old")
	if err != nil {
		fmt.Printf("error renaming: %s", err)
		os.Exit(0)
	}

	for _, e := range w.Excludes {
		fmt.Printf("exclude: %s\n", e)
	}

	err = w.AddGlob("*")
	if err != nil {
		fmt.Printf("error renaming: %s", err)
		os.Exit(0)
	}

	status, err = w.Status()
	if err != nil {
		fmt.Printf("error getting status: %s", err)
		os.Exit(0)
	}

	fmt.Printf("status clean after rename: %t", status.IsClean())
}
