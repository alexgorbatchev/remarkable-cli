package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	cloudAttemptTimeout = 90 * time.Second
	cloudBlobAttempts   = 3
	cloudRetryDelay     = 250 * time.Millisecond
	blobPathPrefix      = "/sync/v3/files/"
)

// The command context bounds the entire operation; each attempt gets its own
// deadline, including reading its body. Client.Timeout would instead bound all
// attempts together and reintroduce premature migration failures.
func newCloudHTTPClient(debug bool) *http.Client {
	var base http.RoundTripper = http.DefaultTransport
	if debug {
		base = &debugTransport{base: base}
	}
	return &http.Client{
		Transport: &blobTransport{base: base, attemptTimeout: cloudAttemptTimeout, retryDelay: cloudRetryDelay},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// A lost root commit cannot safely be repeated at a redirect target.
			if via[0].Method != http.MethodGet && via[0].Method != http.MethodHead && !replayableBlob(via[0]) {
				return http.ErrUseLastResponse
			}
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}
}

type blobTransport struct {
	base           http.RoundTripper
	attemptTimeout time.Duration
	retryDelay     time.Duration
}

func (t *blobTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	attempts := 1
	if replayableBlob(req) {
		attempts = cloudBlobAttempts
	}
	for i := 0; ; i++ {
		if err := req.Context().Err(); err != nil {
			if i == 0 && req.Body != nil {
				_ = req.Body.Close()
			} // no base transport took ownership
			return nil, err
		}
		attempt, cancel, err := t.newAttempt(req, i)
		if err != nil {
			return nil, err
		}
		resp, err := t.base.RoundTrip(attempt)
		if err == nil && attempts > 1 && resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			// The SDK consumes blobs with ReadAll. Complete the read here so a
			// truncated body can be retried before exposing any partial bytes.
			resp, err = bufferBlobResponse(resp)
			cancel()
		}
		if req.Context().Err() != nil {
			closeResponse(resp)
			cancel()
			return nil, req.Context().Err()
		}
		if i+1 == attempts || !transientResponse(resp, err) {
			if err != nil {
				closeResponse(resp)
				cancel()
				return nil, err
			}
			resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: cancel}
			return resp, nil
		}
		delay := retryAfter(resp, t.retryDelay*time.Duration(1<<i))
		closeResponse(resp)
		cancel()
		if err := waitRetry(req.Context(), delay); err != nil {
			return nil, err
		}
	}
}

func (t *blobTransport) newAttempt(req *http.Request, n int) (*http.Request, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(req.Context(), t.attemptTimeout)
	attempt := req.Clone(ctx)
	if n > 0 && req.Body != nil && req.Body != http.NoBody {
		body, err := req.GetBody()
		if err != nil {
			cancel()
			return nil, nil, fmt.Errorf("reopening immutable blob body: %w", err)
		}
		attempt.Body = body
	}
	return attempt, cancel, nil
}

func replayableBlob(req *http.Request) bool {
	if !strings.HasPrefix(req.URL.Path, blobPathPrefix) {
		return false
	}
	hash := strings.TrimPrefix(req.URL.Path, blobPathPrefix)
	if len(hash) != 64 {
		return false
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return false
	}
	return req.Method == http.MethodGet || req.Method == http.MethodPut && req.GetBody != nil
}

func bufferBlobResponse(resp *http.Response) (*http.Response, error) {
	data, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))
	return resp, nil
}

func transientResponse(resp *http.Response, err error) bool {
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false
		}
		var networkError net.Error
		return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
			errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EPIPE) ||
			errors.As(err, &networkError) && networkError.Timeout()
	}
	switch resp.StatusCode {
	case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

func retryAfter(resp *http.Response, fallback time.Duration) time.Duration {
	if resp == nil {
		return fallback
	}
	value := resp.Header.Get("Retry-After")
	if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(0, time.Until(date))
	}
	return fallback
}

func waitRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func closeResponse(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	} // best effort; the request failure is returned
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error {
	defer b.cancel()
	return b.ReadCloser.Close()
}
