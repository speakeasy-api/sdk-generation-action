package git

import (
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/require"
)

func TestLoggingStorer_RecordsHashOnMiss(t *testing.T) {
	wrapped := newLoggingStorer(memory.NewStorage())

	missing := plumbing.NewHash("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	_, err := wrapped.EncodedObject(plumbing.AnyObject, missing)
	require.ErrorIs(t, err, plumbing.ErrObjectNotFound)

	h, stack, ok := wrapped.lastMissingObject()
	require.True(t, ok, "expected a miss to be recorded")
	require.Equal(t, missing, h)
	require.NotEmpty(t, stack, "expected a non-empty stack trace")
}

func TestLoggingStorer_DoesNotRecordOnSuccess(t *testing.T) {
	store := memory.NewStorage()
	obj := store.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	require.NoError(t, err)
	_, err = w.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	hash, err := store.SetEncodedObject(obj)
	require.NoError(t, err)

	wrapped := newLoggingStorer(store)
	got, err := wrapped.EncodedObject(plumbing.AnyObject, hash)
	require.NoError(t, err)
	require.Equal(t, hash, got.Hash())

	_, _, ok := wrapped.lastMissingObject()
	require.False(t, ok, "successful lookup should not record a miss")
}

func TestLoggingStorer_ResetClearsRecord(t *testing.T) {
	wrapped := newLoggingStorer(memory.NewStorage())

	_, err := wrapped.EncodedObject(plumbing.AnyObject, plumbing.NewHash("deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"))
	require.ErrorIs(t, err, plumbing.ErrObjectNotFound)
	_, _, ok := wrapped.lastMissingObject()
	require.True(t, ok)

	wrapped.reset()

	_, _, ok = wrapped.lastMissingObject()
	require.False(t, ok, "reset should clear the recorded miss")
}

func TestPushErr_EnrichesErrObjectNotFoundWithMissingHash(t *testing.T) {
	g := &Git{storerLog: newLoggingStorer(memory.NewStorage())}

	missing := plumbing.NewHash("cafebabecafebabecafebabecafebabecafebabe")
	_, err := g.storerLog.EncodedObject(plumbing.AnyObject, missing)
	require.ErrorIs(t, err, plumbing.ErrObjectNotFound)

	wrapped := g.pushErr(plumbing.ErrObjectNotFound)
	require.Error(t, wrapped)

	msg := wrapped.Error()
	require.Contains(t, msg, "error pushing changes")
	require.Contains(t, msg, "missing object "+missing.String())
	require.Contains(t, msg, "storer lookup stack")
}

func TestPushErr_UnchangedForNonObjectNotFoundErrors(t *testing.T) {
	g := &Git{storerLog: newLoggingStorer(memory.NewStorage())}

	wrapped := g.pushErr(errFake("something else went wrong"))
	require.Error(t, wrapped)
	require.False(t, strings.Contains(wrapped.Error(), "missing object"),
		"non-ObjectNotFound errors should not be enriched")
}

type errFake string

func (e errFake) Error() string { return string(e) }
