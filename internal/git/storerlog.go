package git

import (
	"errors"
	"runtime/debug"
	"sync"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage"
)

// loggingStorer wraps a storage.Storer and records the hash of the most
// recent object lookup that returned plumbing.ErrObjectNotFound, along
// with the Go stack trace captured at that call site.
//
// This exists purely so pushErr can attach diagnostic context when go-git
// returns its otherwise-bare "object not found" error during push. The
// wrapper delegates every storer method to the underlying storer via
// field embedding; only EncodedObject is intercepted.
type loggingStorer struct {
	storage.Storer
	mu          sync.Mutex
	lastMissing plumbing.Hash
	lastStack   []byte
}

func newLoggingStorer(underlying storage.Storer) *loggingStorer {
	return &loggingStorer{Storer: underlying}
}

func (s *loggingStorer) EncodedObject(t plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, error) {
	obj, err := s.Storer.EncodedObject(t, h)
	if err != nil && errors.Is(err, plumbing.ErrObjectNotFound) {
		stack := debug.Stack()
		s.mu.Lock()
		s.lastMissing = h
		s.lastStack = stack
		s.mu.Unlock()
	}
	return obj, err
}

// lastMissingObject returns the hash and stack of the most recent lookup
// that returned ErrObjectNotFound. The bool is false if nothing has been
// recorded yet.
func (s *loggingStorer) lastMissingObject() (plumbing.Hash, []byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastMissing.IsZero() {
		return plumbing.ZeroHash, nil, false
	}
	return s.lastMissing, s.lastStack, true
}

// reset clears any previously recorded miss. Call this before a push so
// the captured record reflects only that push's lookups, not stale state
// from earlier fetches or pushes.
func (s *loggingStorer) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastMissing = plumbing.ZeroHash
	s.lastStack = nil
}
