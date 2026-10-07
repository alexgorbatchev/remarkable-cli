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
// way to it, is failing: any server error, or one of the transient 408 and 429
// statuses the HTTP policy retries.
func cloudFailureStatus(code int) bool {
	return code >= http.StatusInternalServerError || code == http.StatusRequestTimeout || code == http.StatusTooManyRequests
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
func cloudUnavailable(err error) bool {
	var status *cloud.StatusError
	var proxy *proxyConnectError
	if errors.As(err, &status) && cloudFailureStatus(status.StatusCode) || errors.As(err, &proxy) {
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
		if operation.Op == proxyConnectOp {
			return cloudUnavailable(operation.Err)
		}
		return true
	}
	var request *url.Error
	return errors.As(err, &request) && (errors.Is(request.Err, io.EOF) || errors.Is(request.Err, io.ErrUnexpectedEOF))
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
