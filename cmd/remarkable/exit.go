package main

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"slices"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
	render "github.com/alexgorbatchev/go-remarkable-render"
	"github.com/alexgorbatchev/remarkable-cli/internal/doc"
)

// Exit statuses main reports for a failed command. SKILL.md and README.md
// document this table; keep them in sync. Status 2 is never assigned: the Go
// runtime exits with 2 on an unrecovered panic, so it must not carry a domain
// meaning.
const (
	// exitFailure covers invocation errors and every failure no other status
	// classifies, including a name that matches several items.
	exitFailure = 1
	// exitMissing reports a document or item, page, or local file or directory
	// that does not exist.
	exitMissing = 3
	// exitUnauthorized reports missing or rejected credentials.
	exitUnauthorized = 4
	// exitCloudUnavailable reports a cloud that could not be reached, timed out,
	// or failed with a server error.
	exitCloudUnavailable = 5
	// exitCommitAttempted reports a cloud write that failed after sending its
	// root commit, so the cloud may already hold the change.
	exitCommitAttempted = 6
)

// statusPrecedence orders the failure statuses from the one that wins when a
// failure has several causes to the one that loses.
var statusPrecedence = []int{exitCommitAttempted, exitUnauthorized, exitCloudUnavailable, exitMissing, exitFailure}

// exitStatus classifies err, which a command returned. When err has several
// causes, the status that comes first in statusPrecedence wins: a sent root
// commit, credentials, an unavailable cloud, a missing input, then any other
// failure. Joined errors, such as one failure per imported page, are classified
// branch by branch, so the order in which they were joined does not matter.
// Wrappers above a join only add context, apart from the root commit marker.
func exitStatus(err error) int {
	var attempted *commitAttemptedError
	if errors.As(err, &attempted) {
		return exitCommitAttempted
	}
	if branches := joinedBranches(err); branches != nil {
		status := exitFailure
		for _, branch := range branches {
			status = higherPrecedence(status, exitStatus(branch))
		}
		return status
	}
	switch {
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

// joinedBranches returns the errors joined by the first error in err's chain
// that joins several, such as one made by errors.Join, or nil when the chain
// never branches.
func joinedBranches(err error) []error {
	for ; err != nil; err = errors.Unwrap(err) {
		if joined, ok := err.(interface{ Unwrap() []error }); ok {
			return joined.Unwrap()
		}
	}
	return nil
}

// higherPrecedence returns whichever of the statuses a and b wins.
func higherPrecedence(a, b int) int {
	if slices.Index(statusPrecedence, b) < slices.Index(statusPrecedence, a) {
		return b
	}
	return a
}

// Operations crypto/tls names in the *net.OpError it returns for a TLS alert:
// one the server sent, or one this client sent when it rejected the handshake.
// The alert itself has an unexported type, so the operation identifies it;
// tls.AlertError only wraps failures of QUIC connections.
const (
	tlsRemoteAlertOp = "remote error"
	tlsLocalAlertOp  = "local error"
)

// proxyConnectOp is the operation net/http names in the *net.OpError that wraps
// a failure to connect to a proxy, including its TLS handshake with an HTTPS
// proxy.
const proxyConnectOp = "proxyconnect"

// cloudFailureStatus reports a status that means the cloud, or a proxy on the
// way to it, is failing: any server error, or a status the HTTP policy retries.
// Only some server errors are retried, but every one means the cloud is
// failing.
func cloudFailureStatus(code int) bool {
	return code >= http.StatusInternalServerError || transientStatus(code)
}

// connectionClosed reports a connection that the server or a proxy closed
// before the response, or the handshake that precedes it, was complete.
func connectionClosed(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// cloudUnavailable reports whether err shows that a cloud request could not
// reach the server or complete: a failing status from the cloud or from a proxy
// answering CONNECT, a response body that failed partway, an operation or
// attempt deadline or other network timeout, a failed network operation such as
// a refused connection or failed DNS lookup, or a connection that closed before
// its response began. Every DNS failure counts: macOS reports a lookup without
// a network as a missing host. An invalid cloud address or a TLS handshake
// refused by either side, directly or through an HTTPS proxy, is a
// configuration problem rather than an outage, so it stays a general failure.
// err must not join several errors; exitStatus classifies each branch.
func cloudUnavailable(err error) bool {
	var status *cloud.StatusError
	if errors.As(err, &status) && cloudFailureStatus(status.StatusCode) {
		return true
	}
	var proxy *proxyConnectError
	if errors.As(err, &proxy) {
		return true
	}
	// A body fails only after its handshake and headers succeeded, so even a
	// TLS alert there is a failed transfer rather than a refused handshake.
	var body *bodyReadError
	if errors.As(err, &body) {
		return true
	}
	if tlsRefused(err) {
		return false
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
		return true
	}
	var operation *net.OpError
	if errors.As(err, &operation) {
		// A proxy failure is classified as the same failure would be directly,
		// such as a proxy that does not speak TLS behind an https:// proxy URL.
		// net/http reports a direct connection that closed early inside a
		// *url.Error, which a proxyconnect error replaces.
		if operation.Op == proxyConnectOp {
			return connectionClosed(operation.Err) || cloudUnavailable(operation.Err)
		}
		return true
	}
	var request *url.Error
	return errors.As(err, &request) && connectionClosed(request.Err)
}

// tlsRefused reports a TLS handshake that failed certificate verification or
// that the server or this client refused with an alert, such as a server that
// requires a client certificate. net/http wraps a failed handshake with an
// HTTPS proxy in a *net.OpError with Op "proxyconnect", so every *net.OpError
// in the chain is inspected, not only the outermost one.
func tlsRefused(err error) bool {
	var verification *tls.CertificateVerificationError
	if errors.As(err, &verification) {
		return true
	}
	var operation *net.OpError
	for current := err; errors.As(current, &operation); current = operation.Err {
		if operation.Op == tlsRemoteAlertOp || operation.Op == tlsLocalAlertOp {
			return true
		}
	}
	return false
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
