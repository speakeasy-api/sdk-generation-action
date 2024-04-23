package git

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/require"
)

func newTestRepo(t *testing.T) (*git.Repository, billy.Filesystem) {
	t.Helper()

	mfs := memfs.New()

	err := filepath.WalkDir("./fixtures", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		fixture, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fixture.Close()

		f, err := mfs.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(f, fixture)
		if err != nil {
			return err
		}

		return nil
	})
	require.NoError(t, err, "expected to walk the fixture directory")

	storage := memory.NewStorage()
	repo, err := git.Init(storage, mfs)
	require.NoError(t, err, "expected empty repo to be initialized")

	wt, err := repo.Worktree()
	require.NoError(t, err, "expected to get worktree")

	_, err = wt.Add(".")
	require.NoError(t, err, "expected to add all files")

	_, err = wt.Commit("initial commit", &git.CommitOptions{})
	require.NoError(t, err, "expected to commit all files")

	return repo, mfs
}

func TestGit_CheckDirDirty(t *testing.T) {
	repo, mfs := newTestRepo(t)

	f, err := mfs.Create("dirty-file")
	require.NoError(t, err, "expected to create a dirty file")
	defer f.Close()
	fmt.Fprintln(f, "sample content")

	g := Git{repo: repo}
	dirty, str, err := g.CheckDirDirty(".", map[string]string{})
	require.NoError(t, err, "expected to check the directory")

	require.Equal(t, `new file found: []string{"dirty-file"}`, str)
	require.True(t, dirty, "expected the directory to be dirty")
}

func TestGit_CheckDirDirty_IgnoredFiles(t *testing.T) {
	repo, mfs := newTestRepo(t)

	f, err := mfs.Create("workflow.lock")
	require.NoError(t, err, "expected to create a dirty file")
	defer f.Close()
	fmt.Fprintln(f, "sample content")

	g := Git{repo: repo}
	dirty, str, err := g.CheckDirDirty(".", map[string]string{})
	require.NoError(t, err, "expected to check the directory")

	require.Equal(t, "", str, "expected no dirty files reported")
	require.False(t, dirty, "expected the directory to be clean")
}
