package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func blobURL(server string, data []byte) string {
	return fmt.Sprintf("%s/sync/v3/files/%x", server, sha256.Sum256(data))
}

func TestCloudHTTPDelayedBlob(t *testing.T) {
	for _, debug := range []bool{false, true} {
		t.Run(fmt.Sprintf("debug=%t", debug), func(t *testing.T) {
			t.Parallel()
			data := []byte("complete native blob after the former fifteen-second deadline")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-time.After(16 * time.Second):
					if _, err := w.Write(data); err != nil {
						t.Error(err)
					}
				case <-r.Context().Done():
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, blobURL(server.URL, data), nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := newCloudHTTPClient(debug).Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			got, err := io.ReadAll(resp.Body)
			if err != nil || !bytes.Equal(got, data) {
				t.Fatalf("body = %q, error = %v", got, err)
			}
		})
	}
}

func TestCloudHTTPBlobRecovery(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		for _, failure := range []string{"headers", "body", "timeout", "body timeout", "status"} {
			t.Run(method+"/"+failure, func(t *testing.T) {
				var calls atomic.Int64
				data := bytes.Repeat([]byte("native bytes\x00\xff"), 4096)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := calls.Add(1)
					if r.Header.Get("Authorization") != "Bearer private-test-token" || r.Header.Get("rm-filename") != "document/page.rm" || r.Header.Get("x-goog-hash") != "crc32c=example" {
						t.Error("retry changed request headers")
					}
					if method == http.MethodPut {
						got, err := io.ReadAll(r.Body)
						if err != nil || !bytes.Equal(got, data) {
							t.Errorf("upload bytes changed: len=%d, %v", len(got), err)
						}
					}
					if n == 1 {
						switch failure {
						case "headers", "body":
							conn, _, err := w.(http.Hijacker).Hijack()
							if err != nil {
								t.Error(err)
								return
							}
							if failure == "body" {
								if _, err := fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\npartial", len(data)); err != nil {
									t.Error(err)
								}
							}
							if err := conn.Close(); err != nil {
								t.Error(err)
							}
						case "timeout":
							<-r.Context().Done()
						case "body timeout":
							w.Header().Set("Content-Length", "10000")
							if _, err := w.Write([]byte("partial")); err != nil {
								t.Error(err)
							}
							w.(http.Flusher).Flush()
							<-r.Context().Done()
						case "status":
							w.Header().Set("Retry-After", "0")
							w.WriteHeader(http.StatusServiceUnavailable)
						}
						return
					}
					if _, err := w.Write(data); err != nil {
						t.Error(err)
					}
				}))
				defer server.Close()
				client := newCloudHTTPClient(false)
				transport := client.Transport.(*blobTransport)
				transport.attemptTimeout = 100 * time.Millisecond
				transport.retryDelay = time.Millisecond
				var body io.Reader
				if method == http.MethodPut {
					body = bytes.NewReader(data)
				}
				req, err := http.NewRequestWithContext(t.Context(), method, blobURL(server.URL, data), body)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", "Bearer private-test-token")
				req.Header.Set("rm-filename", "document/page.rm")
				req.Header.Set("x-goog-hash", "crc32c=example")
				resp, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = resp.Body.Close() }()
				got, err := io.ReadAll(resp.Body)
				if err != nil || !bytes.Equal(got, data) {
					t.Fatalf("partial/changed response: len=%d, %v", len(got), err)
				}
				if calls.Load() != 2 {
					t.Fatalf("requests = %d, want 2", calls.Load())
				}
			})
		}
	}
}

func TestCloudHTTPRetryBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name, method, path string
		status             int
		want               int64
		stream             bool
	}{
		{"blob exhausted", "GET", "/sync/v3/files/" + strings.Repeat("a", 64), 503, 3, false},
		{"blob not found", "GET", "/sync/v3/files/" + strings.Repeat("a", 64), 404, 1, false},
		{"root commit", "PUT", "/sync/v3/root", 503, 1, false},
		{"auth creation", "POST", "/token/json/2/device/new", 503, 1, false},
		{"unhashed put", "PUT", "/sync/v3/files/not-a-hash", 503, 1, false},
		{"invalid hash", "GET", "/sync/v3/files/" + strings.Repeat("z", 64), 503, 1, false},
		{"blob rejected", "PUT", "/sync/v3/files/" + strings.Repeat("a", 64), 400, 1, false},
		{"streaming put", "PUT", "/sync/v3/files/" + strings.Repeat("a", 64), 503, 1, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tt.status)
			}))
			defer server.Close()
			client := newCloudHTTPClient(false)
			client.Transport.(*blobTransport).retryDelay = time.Millisecond
			var body io.Reader = bytes.NewReader([]byte("unchanged content"))
			if tt.stream {
				body = io.NopCloser(body)
			}
			req, err := http.NewRequestWithContext(t.Context(), tt.method, server.URL+tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if calls.Load() != tt.want || resp.StatusCode != tt.status {
				t.Fatalf("calls=%d, status=%d", calls.Load(), resp.StatusCode)
			}
		})
	}
}

func TestCloudHTTPExhaustedTransport(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := newCloudHTTPClient(false)
	client.Transport.(*blobTransport).retryDelay = time.Millisecond
	_, err := client.Get(blobURL(server.URL, []byte("blob")))
	if err == nil || calls.Load() != 3 {
		t.Fatalf("calls=%d,error=%v", calls.Load(), err)
	}
}

func TestCloudHTTPBodyCancellation(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Length", "10000")
		if _, err := w.Write([]byte("partial")); err != nil {
			t.Error(err)
		}
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, blobURL(server.URL, []byte("blob")), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = newCloudHTTPClient(false).Do(req)
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 1 {
		t.Fatalf("calls=%d,error=%v", calls.Load(), err)
	}
}

func TestCloudHTTPReopeningFailedBody(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client := newCloudHTTPClient(false)
	client.Transport.(*blobTransport).retryDelay = time.Millisecond
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, blobURL(server.URL, []byte("body")), bytes.NewReader([]byte("body")))
	if err != nil {
		t.Fatal(err)
	}
	failed := errors.New("original upload cannot be reopened")
	req.GetBody = func() (io.ReadCloser, error) { return nil, failed }
	_, err = client.Do(req)
	if !errors.Is(err, failed) || calls.Load() != 1 {
		t.Fatalf("calls=%d,error=%v", calls.Load(), err)
	}
}

func TestCloudHTTPDateRetryAfter(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", time.Now().Add(time.Minute).UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, blobURL(server.URL, []byte("blob")), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = newCloudHTTPClient(false).Do(req)
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 1 {
		t.Fatalf("calls=%d,error=%v", calls.Load(), err)
	}
}

func TestCloudHTTPCancellationAndLostCommit(t *testing.T) {
	for _, tt := range []struct {
		name, method, path string
		canceled, lost     bool
	}{
		{"canceled", "GET", "/sync/v3/files/" + strings.Repeat("a", 64), true, false},
		{"deadline", "GET", "/sync/v3/files/" + strings.Repeat("a", 64), false, false},
		{"lost root response", "PUT", "/sync/v3/root", false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if tt.lost {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					if err := conn.Close(); err != nil {
						t.Error(err)
					}
					return
				}
				w.Header().Set("Retry-After", "10")
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
			defer cancel()
			if tt.canceled {
				cancel()
			}
			req, err := http.NewRequestWithContext(ctx, tt.method, server.URL+tt.path, bytes.NewReader([]byte("root")))
			if err != nil {
				t.Fatal(err)
			}
			_, err = newCloudHTTPClient(false).Do(req)
			if err == nil {
				t.Fatal("expected interrupted operation")
			}
			want := int64(1)
			if tt.canceled {
				want = 0
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			}
			if !tt.lost && !tt.canceled && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
			if calls.Load() != want {
				t.Fatalf("requests = %d, want %d", calls.Load(), want)
			}
		})
	}
}

func TestCloudClientBlobRecoveryWiring(t *testing.T) {
	for _, debug := range []bool{false, true} {
		t.Run(fmt.Sprintf("debug=%t", debug), func(t *testing.T) {
			var calls atomic.Int64
			data := []byte("sdk must receive only complete native bytes")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(http.StatusBadGateway)
					return
				}
				if _, err := w.Write(data); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			configPath := filepath.Join(t.TempDir(), "credentials")
			if err := os.WriteFile(configPath, []byte("usertoken: test-user-token\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("REMARKABLE_HOST", server.URL)
			t.Setenv("REMARKABLE_CONFIG", configPath)
			// Root construction resets the flags to their documented defaults.
			newRootCmd()
			debugFlag, noCacheFlag = debug, true
			t.Cleanup(func() { newRootCmd() })
			client, err := newCloudClient(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.GetBlobFresh(t.Context(), fmt.Sprintf("%x", sha256.Sum256(data)), "page.rm")
			if err != nil || !bytes.Equal(got, data) || calls.Load() != 2 {
				t.Fatalf("body=%q, calls=%d, error=%v", got, calls.Load(), err)
			}
		})
	}
}

func TestCloudHTTPRedirectSafety(t *testing.T) {
	for _, tt := range []struct {
		name, method, path string
		redirected         bool
	}{
		{"blob read", "GET", "/sync/v3/files/" + strings.Repeat("a", 64), true},
		{"blob put", "PUT", "/sync/v3/files/" + strings.Repeat("a", 64), true},
		{"root read", "GET", "/sync/v3/root", true},
		{"root commit", "PUT", "/sync/v3/root", false},
		{"auth creation", "POST", "/token/json/2/device/new", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var original, redirected atomic.Int64
			data := []byte("identical redirected upload")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/redirected" {
					original.Add(1)
					http.Redirect(w, r, "/redirected", http.StatusTemporaryRedirect)
					return
				}
				redirected.Add(1)
				if r.Method != tt.method || r.Header.Get("rm-filename") != "page.rm" {
					t.Error("redirect changed method/headers")
				}
				if tt.method == http.MethodPut {
					got, err := io.ReadAll(r.Body)
					if err != nil || !bytes.Equal(got, data) {
						t.Errorf("redirect changed bytes: %q, %v", got, err)
					}
				}
				if _, err := w.Write(data); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			var body io.Reader
			if tt.method != http.MethodGet {
				body = bytes.NewReader(data)
			}
			req, err := http.NewRequestWithContext(t.Context(), tt.method, server.URL+tt.path, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("rm-filename", "page.rm")
			resp, err := newCloudHTTPClient(false).Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			wantStatus, wantRedirected := http.StatusTemporaryRedirect, int64(0)
			if tt.redirected {
				wantStatus, wantRedirected = http.StatusOK, 1
			}
			if original.Load() != 1 || redirected.Load() != wantRedirected || resp.StatusCode != wantStatus {
				t.Fatalf("original=%d redirected=%d status=%d", original.Load(), redirected.Load(), resp.StatusCode)
			}
		})
	}
}
