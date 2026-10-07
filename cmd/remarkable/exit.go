package main

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	render "github.com/alexgorbatchev/go-remarkable-render"
	"github.com/alexgorbatchev/remarkable-cli/internal/doc"
)

// Exit statuses main reports for a failed command. SKILL.md and README.md
// document this table; keep them in sync.
const (
	// exitFailure covers invocation errors and every failure no other status
	// classifies, including a name that matches several items.
	exitFailure = 1
	// exitMissing reports a document or item, page, or local file or directory
	// that does not exist.
	exitMissing = 2
	// exitUnauthorized reports missing or rejected credentials.
	exitUnauthorized = 3
	// exitCloudUnavailable reports a cloud that could not be reached, timed out,
	// or failed with a server error.
	exitCloudUnavailable = 4
	// exitCommitAttempted reports a cloud write that failed after sending its
	// root commit, so the cloud may already hold the change.
	exitCommitAttempted = 5
)

// exitStatus classifies err, which a command returned. When err has several
// causes, the first matching status in this order wins: a sent root commit,
// credentials, an unavailable cloud, then a missing input.
func exitStatus(err error) int {
	var attempted *commitAttemptedError
	switch {
	case errors.As(err, &attempted):
		return exitCommitAttempted
	case errors.Is(err, cloud.ErrUnauthorized), errors.Is(err, errCredentialsNotFound):
		return exitUnauthorized
	case cloudUnavailable(err):
		return exitCloudUnavailable
	case errors.Is(err, cloud.ErrItemNotFound), errors.Is(err, doc.ErrPageOutOfBounds),
		errors.Is(err, render.ErrPageOutOfBounds), errors.Is(err, fs.ErrNotExist):
		return exitMissing
	}
	return exitFailure
}

// cloudUnavailable reports whether err shows that a cloud request could not
// reach the server or complete: a server error status or one of the transient
// 408 and 429 statuses the HTTP policy retries, an operation or attempt
// deadline or other network timeout, a failed network operation such as a
// refused connection or failed DNS lookup, or a connection that closed before
// its response ended. An invalid cloud address or a refused TLS certificate is
// not an outage, so it stays a general failure.
func cloudUnavailable(err error) bool {
	var status *cloud.StatusError
	if errors.As(err, &status) && (status.StatusCode >= http.StatusInternalServerError ||
		status.StatusCode == http.StatusRequestTimeout || status.StatusCode == http.StatusTooManyRequests) {
		return true
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
		return true
	}
	var operation *net.OpError
	if errors.As(err, &operation) {
		return true
	}
	var request *url.Error
	return errors.As(err, &request) && (errors.Is(request.Err, io.EOF) || errors.Is(request.Err, io.ErrUnexpectedEOF))
}

// commitAttemptedError marks a failed cloud write that had sent its root
// commit. It reports the error it wraps unchanged.
type commitAttemptedError struct{ err error }

func (e *commitAttemptedError) Error() string { return e.err.Error() }
func (e *commitAttemptedError) Unwrap() error { return e.err }

// afterCommit marks err, which a cloud write returned at progress state, as a
// failure after its root commit was sent: commit-unknown or committed. It
// returns any other error unchanged, so a staged write exits by its cause.
func afterCommit(state cloud.UpdateState, err error) error {
	if err == nil || state != cloud.UpdateCommitUnknown && state != cloud.UpdateCommitted {
		return err
	}
	return &commitAttemptedError{err: err}
}
