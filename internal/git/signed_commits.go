package git

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/google/go-github/v63/github"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"golang.org/x/sync/errgroup"
)

type signedTreeBlobCreator func(ctx context.Context, path string, content []byte) (string, error)

const (
	// maxConcurrentBlobUploads bounds parallel Git.CreateBlob calls so large SDK
	// repositories upload quickly without tripping GitHub's secondary rate limits.
	maxConcurrentBlobUploads = 8

	// signedTreeChunkEntryLimit keeps individual Git.CreateTree calls small enough
	// to avoid GitHub's timeout-prone large tree payload path while preserving the
	// Git Database API flow required for Verified commits.
	signedTreeChunkEntryLimit = 100

	// signedTreeChunkByteLimit is a conservative cap for one CreateTree JSON body.
	// GitHub's practical request limit is around 40 MiB; keep chunks well below it.
	signedTreeChunkByteLimit = 30 * 1024 * 1024

	// signedBlobByteLimit is a conservative preflight cap for one CreateBlob JSON
	// body, which includes base64 content and request JSON overhead.
	signedBlobByteLimit = 30 * 1024 * 1024
)

// Generates the tree to commit based on the commit reference and source files. If doesn't exist on the remote
// host, it will create and push it.
func (g *Git) createAndPushTree(ref *github.Reference, sourceFiles git.Status) (tree *github.Tree, err error) {
	_, githubRepoLocation := g.getRepoMetadata()
	owner, repo := g.getOwnerAndRepo(githubRepoLocation)
	w, _ := g.repo.Worktree()
	ctx := context.Background()

	idx, err := signedCommitIndex(g.repo)
	if err != nil {
		return nil, err
	}

	entries, stats, err := buildSignedCommitTreeEntries(ctx, sourceFiles, idx, w.Filesystem.Root(), w.Filesystem.Join, func(ctx context.Context, path string, content []byte) (string, error) {
		return g.createBlobWithRetry(ctx, owner, repo, path, content)
	})
	if err != nil {
		return nil, err
	}

	logging.Info("Creating signed commit tree with %d changed entries (%d deletes), uploaded %d bytes across %d blobs", len(entries), stats.Deletes, stats.BytesUploaded, stats.BlobsUploaded)

	return g.createTreeChain(ctx, owner, repo, *ref.Object.SHA, entries, signedTreeChunkEntryLimit, signedTreeChunkByteLimit)
}

// createTreeChain builds a final tree incrementally by applying sorted chunks to
// the previous tree SHA. This avoids one oversized CreateTree request while
// keeping the signed-commit Git Database API path intact.
func (g *Git) createTreeChain(ctx context.Context, owner, repo, baseTree string, entries []*github.TreeEntry, entryLimit, byteLimit int) (*github.Tree, error) {
	if len(entries) == 0 {
		logging.Info("No signed commit tree entries to create; reusing base tree %s", baseTree)
		return &github.Tree{SHA: github.String(baseTree)}, nil
	}

	chunks, err := chunkSignedCommitTreeEntries(baseTree, entries, entryLimit, byteLimit)
	if err != nil {
		return nil, err
	}

	currentBase := baseTree
	var tree *github.Tree
	for i, chunk := range chunks {
		chunkBytes, err := estimateCreateTreePayloadBytes(currentBase, chunk)
		if err != nil {
			return nil, err
		}
		logging.Info("Creating signed commit tree chunk %d/%d with %d entries (%d estimated payload bytes, base_tree=%s)", i+1, len(chunks), len(chunk), chunkBytes, currentBase)

		tree, err = g.createTreeWithRetry(ctx, owner, repo, currentBase, chunk)
		if err != nil {
			return nil, fmt.Errorf("error creating signed commit tree chunk %d/%d: %w", i+1, len(chunks), err)
		}
		currentBase = tree.GetSHA()
	}

	return tree, nil
}

func chunkSignedCommitTreeEntries(baseTree string, entries []*github.TreeEntry, entryLimit, byteLimit int) ([][]*github.TreeEntry, error) {
	if entryLimit <= 0 {
		entryLimit = signedTreeChunkEntryLimit
	}
	if byteLimit <= 0 {
		byteLimit = signedTreeChunkByteLimit
	}

	chunks := [][]*github.TreeEntry{}
	chunk := []*github.TreeEntry{}
	for _, entry := range entries {
		singleEntryBytes, err := estimateCreateTreePayloadBytes(baseTree, []*github.TreeEntry{entry})
		if err != nil {
			return nil, err
		}
		if singleEntryBytes > byteLimit {
			return nil, fmt.Errorf("tree entry for %s is too large for signed commit tree API payload: estimated %d bytes exceeds chunk limit %d bytes", entry.GetPath(), singleEntryBytes, byteLimit)
		}

		candidate := append(append([]*github.TreeEntry{}, chunk...), entry)
		candidateBytes, err := estimateCreateTreePayloadBytes(baseTree, candidate)
		if err != nil {
			return nil, err
		}
		if len(chunk) > 0 && (len(chunk) >= entryLimit || candidateBytes > byteLimit) {
			chunks = append(chunks, chunk)
			chunk = []*github.TreeEntry{entry}
			continue
		}

		chunk = candidate
	}
	if len(chunk) > 0 {
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

func estimateCreateTreePayloadBytes(baseTree string, entries []*github.TreeEntry) (int, error) {
	payload := struct {
		BaseTree string              `json:"base_tree"`
		Tree     []*github.TreeEntry `json:"tree"`
	}{BaseTree: baseTree, Tree: entries}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	return len(encoded), nil
}

// createTreeWithRetry creates a git tree, retrying transient GitHub failures.
// Large generated SDK commits can produce thousands of tree entries; GitHub's
// tree API may occasionally return transient 5xx responses after all blobs have
// already been uploaded, so retrying avoids wasting the whole generation run.
func (g *Git) createTreeWithRetry(ctx context.Context, owner, repo, baseTree string, entries []*github.TreeEntry) (*github.Tree, error) {
	var tree *github.Tree
	op := func() error {
		createdTree, resp, err := g.client.Git.CreateTree(ctx, owner, repo, baseTree, entries)
		if err != nil {
			if !isRetryableGitHubError(resp, err) {
				return backoff.Permanent(err)
			}
			if wait, ok := rateLimitRetryAfter(err); ok {
				if sleepErr := sleepWithContext(ctx, wait); sleepErr != nil {
					return backoff.Permanent(sleepErr)
				}
			}
			return err
		}
		if createdTree.GetSHA() == "" {
			return backoff.Permanent(errors.New("empty tree SHA returned"))
		}
		tree = createdTree
		return nil
	}

	bo := backoff.WithContext(backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 3), ctx)
	if err := backoff.Retry(op, bo); err != nil {
		return nil, formatCreateTreeError(err, baseTree, len(entries))
	}
	return tree, nil
}

func formatCreateTreeError(err error, baseTree string, entryCount int) error {
	message := fmt.Sprintf("create tree failed after retries (base_tree=%s, entries=%d): %v", baseTree, entryCount, err)

	var githubErr *github.ErrorResponse
	if errors.As(err, &githubErr) {
		details := []string{}
		if githubErr.Response != nil {
			details = append(details, fmt.Sprintf("status=%d", githubErr.Response.StatusCode))
		}
		if githubErr.Message != "" {
			details = append(details, fmt.Sprintf("message=%q", githubErr.Message))
		}
		if len(githubErr.Errors) > 0 {
			details = append(details, fmt.Sprintf("errors=%+v", githubErr.Errors))
		}
		if githubErr.DocumentationURL != "" {
			details = append(details, fmt.Sprintf("documentation_url=%s", githubErr.DocumentationURL))
		}
		if len(details) > 0 {
			message = fmt.Sprintf("%s (%s)", message, strings.Join(details, ", "))
		}
	}

	return fmt.Errorf("%s", message)
}

// createBlobWithRetry uploads a single base64-encoded blob, retrying transient
// GitHub failures (timeouts, 5xx, rate limiting) with exponential backoff.
// Permanent failures (e.g. auth, 4xx) abort immediately.
func (g *Git) createBlobWithRetry(ctx context.Context, owner, repo, path string, content []byte) (string, error) {
	encoded := base64.StdEncoding.EncodeToString(content)

	var sha string
	op := func() error {
		blob, resp, err := g.client.Git.CreateBlob(ctx, owner, repo, &github.Blob{
			Content:  github.String(encoded),
			Encoding: github.String("base64"),
		})
		if err != nil {
			if !isRetryableBlobError(resp, err) {
				return backoff.Permanent(err)
			}
			// When GitHub tells us how long to wait (rate limiting/abuse
			// detection), honor that window before letting backoff retry.
			// A short generic backoff would just burn the remaining retries
			// against a limit that has not reset yet.
			if wait, ok := rateLimitRetryAfter(err); ok {
				if sleepErr := sleepWithContext(ctx, wait); sleepErr != nil {
					return backoff.Permanent(sleepErr)
				}
			}
			return err
		}
		if blob.GetSHA() == "" {
			return backoff.Permanent(fmt.Errorf("empty blob SHA returned for %s", path))
		}
		sha = blob.GetSHA()
		return nil
	}

	bo := backoff.WithContext(backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 3), ctx)
	if err := backoff.Retry(op, bo); err != nil {
		return "", err
	}
	return sha, nil
}

// isRetryableBlobError reports whether a Git.CreateBlob failure is worth
// retrying. Transient conditions are network errors, GitHub rate limiting, and
// 5xx/408/429 responses; everything else is treated as permanent.
func isRetryableBlobError(resp *github.Response, err error) bool {
	return isRetryableGitHubError(resp, err)
}

func isRetryableGitHubError(resp *github.Response, err error) bool {
	var rateErr *github.RateLimitError
	var abuseErr *github.AbuseRateLimitError
	if errors.As(err, &rateErr) || errors.As(err, &abuseErr) {
		return true
	}

	if resp == nil || resp.Response == nil {
		// No HTTP response usually means a network/transport error.
		return true
	}

	switch {
	case resp.StatusCode >= 500:
		return true
	case resp.StatusCode == http.StatusRequestTimeout, resp.StatusCode == http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}

// rateLimitRetryAfter returns the server-indicated wait duration for a GitHub
// rate-limit or abuse-detection error, if one is available and in the future.
func rateLimitRetryAfter(err error) (time.Duration, bool) {
	var abuseErr *github.AbuseRateLimitError
	if errors.As(err, &abuseErr) && abuseErr.RetryAfter != nil && *abuseErr.RetryAfter > 0 {
		return *abuseErr.RetryAfter, true
	}

	var rateErr *github.RateLimitError
	if errors.As(err, &rateErr) {
		if wait := time.Until(rateErr.Rate.Reset.Time); wait > 0 {
			return wait, true
		}
	}

	return 0, false
}

// sleepWithContext waits for d or until ctx is done, whichever comes first.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type signedTreeStats struct {
	Deletes       int
	BlobsUploaded int
	BytesUploaded int
}

func signedCommitIndex(repo *git.Repository) (*index.Index, error) {
	indexStorer, ok := repo.Storer.(storer.IndexStorer)
	if !ok {
		return nil, errors.New("repository storage does not expose git index")
	}
	idx, err := indexStorer.Index()
	if err != nil {
		return nil, fmt.Errorf("error getting staged index for signed commit: %w", err)
	}
	return idx, nil
}

func buildSignedCommitTreeEntries(ctx context.Context, sourceFiles git.Status, idx *index.Index, worktreeRoot string, join func(elem ...string) string, createBlob signedTreeBlobCreator) ([]*github.TreeEntry, signedTreeStats, error) {
	type uploadJob struct {
		file    string
		deleted bool
		mode    filemode.FileMode
	}

	jobs := make([]uploadJob, 0, len(sourceFiles))
	for file, fileStatus := range sourceFiles {
		file = filepath.ToSlash(file)
		switch fileStatus.Staging {
		case git.Unmodified, git.Untracked:
			continue
		case git.Deleted:
			jobs = append(jobs, uploadJob{file: file, deleted: true, mode: filemode.Regular})
		default:
			entry, err := signedCommitIndexEntry(idx, file)
			if err != nil {
				return nil, signedTreeStats{}, err
			}
			jobs = append(jobs, uploadJob{file: file, mode: entry.Mode})
		}
	}

	slices.SortFunc(jobs, func(a, b uploadJob) int {
		return strings.Compare(a.file, b.file)
	})

	entries := make([]*github.TreeEntry, len(jobs))
	stats := signedTreeStats{}
	var mu sync.Mutex

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(maxConcurrentBlobUploads)

	for i, job := range jobs {
		i, job := i, job

		if job.deleted {
			entries[i] = &github.TreeEntry{
				Path: github.String(job.file),
				Mode: github.String(githubTreeMode(job.mode)),
			}
			mu.Lock()
			stats.Deletes++
			mu.Unlock()
			continue
		}

		if job.mode == filemode.Submodule {
			entry, err := signedCommitIndexEntry(idx, job.file)
			if err != nil {
				return nil, stats, err
			}
			entries[i] = &github.TreeEntry{
				Path: github.String(job.file),
				Type: github.String("commit"),
				SHA:  github.String(entry.Hash.String()),
				Mode: github.String(githubTreeMode(job.mode)),
			}
			continue
		}

		eg.Go(func() error {
			filePath := join(worktreeRoot, job.file)
			content, err := signedCommitFileContent(filePath, job.mode)
			if err != nil {
				logging.Info("Error getting file content: %v %s", err, filePath)
				return err
			}
			if payloadBytes := estimateCreateBlobPayloadBytes(content); payloadBytes > signedBlobByteLimit {
				return fmt.Errorf("file %s is too large for signed commit blob API payload: estimated %d bytes exceeds limit %d bytes", job.file, payloadBytes, signedBlobByteLimit)
			}

			sha, err := createBlob(ctx, job.file, content)
			if err != nil {
				return fmt.Errorf("error creating blob for %s: %w", job.file, err)
			}

			entries[i] = &github.TreeEntry{
				Path: github.String(job.file),
				Type: github.String("blob"),
				SHA:  github.String(sha),
				Mode: github.String(githubTreeMode(job.mode)),
			}
			mu.Lock()
			stats.BlobsUploaded++
			stats.BytesUploaded += len(content)
			mu.Unlock()
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, stats, err
	}

	return entries, stats, nil
}

func signedCommitIndexEntry(idx *index.Index, file string) (*index.Entry, error) {
	if idx == nil {
		return nil, errors.New("staged index is nil")
	}
	entry, err := idx.Entry(filepath.ToSlash(file))
	if err != nil {
		return nil, fmt.Errorf("error getting staged index entry for %s: %w", file, err)
	}
	if entry.Mode.IsMalformed() || entry.Mode == filemode.Empty || entry.Mode == filemode.Dir {
		return nil, fmt.Errorf("unsupported git mode %s for %s", entry.Mode, file)
	}
	return entry, nil
}

func signedCommitFileContent(filePath string, mode filemode.FileMode) ([]byte, error) {
	if mode == filemode.Symlink {
		target, err := os.Readlink(filePath)
		if err != nil {
			return nil, err
		}
		return []byte(target), nil
	}
	return os.ReadFile(filePath)
}

func githubTreeMode(mode filemode.FileMode) string {
	switch mode {
	case filemode.Executable:
		return "100755"
	case filemode.Symlink:
		return "120000"
	case filemode.Submodule:
		return "160000"
	default:
		return "100644"
	}
}

func estimateCreateBlobPayloadBytes(content []byte) int {
	return len(`{"content":"","encoding":"base64"}`) + base64.StdEncoding.EncodedLen(len(content))
}
