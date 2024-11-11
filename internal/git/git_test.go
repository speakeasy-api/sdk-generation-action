package git

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
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

	_, err = wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Unix(0, 0),
		},
	})
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

func TestArtifactMatchesRelease(t *testing.T) {
	tests := []struct {
		name      string
		assetName string
		goos      string
		goarch    string
		want      bool
	}{
		{
			name:      "Linux amd64",
			assetName: "speakeasy_linux_amd64.zip",
			goos:      "linux",
			goarch:    "amd64",
			want:      true,
		},
		{
			name:      "Linux 386",
			assetName: "speakeasy_linux_386.zip",
			goos:      "linux",
			goarch:    "386",
			want:      true,
		},
		{
			name:      "Linux arm64",
			assetName: "speakeasy_linux_arm64.zip",
			goos:      "linux",
			goarch:    "arm64",
			want:      true,
		},
		{
			name:      "macOS amd64",
			assetName: "speakeasy_darwin_amd64.zip",
			goos:      "darwin",
			goarch:    "amd64",
			want:      true,
		},
		{
			name:      "Linux arm64/v8",
			assetName: "speakeasy_linux_arm64.zip",
			goos:      "linux",
			goarch:    "arm64/v8",
			want:      true,
		},
		{
			name:      "macOS arm64",
			assetName: "speakeasy_darwin_arm64.zip",
			goos:      "darwin",
			goarch:    "arm64",
			want:      true,
		},
		{
			name:      "Windows amd64",
			assetName: "speakeasy_windows_amd64.zip",
			goos:      "windows",
			goarch:    "amd64",
			want:      true,
		},
		{
			name:      "Windows 386",
			assetName: "speakeasy_windows_386.zip",
			goos:      "windows",
			goarch:    "386",
			want:      true,
		},
		{
			name:      "Windows arm64",
			assetName: "speakeasy_windows_arm64.zip",
			goos:      "windows",
			goarch:    "arm64",
			want:      true,
		},
		{
			name:      "Mismatched OS",
			assetName: "speakeasy_linux_amd64.zip",
			goos:      "darwin",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Mismatched arch",
			assetName: "speakeasy_linux_amd64.zip",
			goos:      "linux",
			goarch:    "arm64",
			want:      false,
		},
		{
			name:      "Checksums file",
			assetName: "checksums.txt",
			goos:      "linux",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Source code zip",
			assetName: "Source code (zip)",
			goos:      "linux",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Source code tar.gz",
			assetName: "Source code (tar.gz)",
			goos:      "linux",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Incorrect file extension",
			assetName: "speakeasy_linux_amd64.tar.gz",
			goos:      "linux",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Missing architecture",
			assetName: "speakeasy_linux.zip",
			goos:      "linux",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Wrong order of segments",
			assetName: "speakeasy_amd64_linux.zip",
			goos:      "linux",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Partial match in OS",
			assetName: "speakeasy_darwin_amd64.zip",
			goos:      "dar",
			goarch:    "amd64",
			want:      false,
		},
		{
			name:      "Partial match in arch",
			assetName: "speakeasy_linux_amd64.zip",
			goos:      "linux",
			goarch:    "amd",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ArtifactMatchesRelease(tt.assetName, tt.goos, tt.goarch); got != tt.want {
				t.Errorf("ArtifactMatchesRelease() = %v, want %v", got, tt.want)
			}
		})
	}
}
