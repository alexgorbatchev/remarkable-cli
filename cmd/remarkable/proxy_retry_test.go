package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProxyConnectBlobRetry(t *testing.T) {
	for _, status := range []int{408, 429, 503, 501} {
		for _, operation := range []string{"GET blob", "PUT blob", "PUT root"} {
			t.Run(fmt.Sprintf("%d/%s", status, operation), func(t *testing.T) {
				var connects atomic.Int32
				var handlers sync.WaitGroup
				origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodPut {
						data, err := io.ReadAll(r.Body)
						if err != nil || string(data) != "payload" || r.Header.Get("rm-filename") != "test.pdf" {
							t.Errorf("replayed request changed: %q, %v", data, err)
						}
					}
					_, _ = io.WriteString(w, "complete")
				}))
				defer origin.Close()
				proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					handlers.Add(1)
					defer handlers.Done()
					if r.Method != http.MethodConnect {
						t.Errorf("proxy method = %s", r.Method)
						return
					}
					if connects.Add(1) == 1 {
						w.Header().Set("Retry-After", "0")
						w.WriteHeader(status)
						return
					}
					upstream, err := net.DialTimeout("tcp", strings.TrimPrefix(origin.URL, "https://"), time.Second)
					if err != nil {
						t.Error(err)
						return
					}
					defer func() {
						if err := upstream.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
							t.Error(err)
						}
					}()
					downstream, buffered, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					defer func() {
						if err := downstream.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
							t.Error(err)
						}
					}()
					if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
						t.Error(err)
						return
					}
					if err := buffered.Flush(); err != nil {
						t.Error(err)
						return
					}
					done := make(chan struct{})
					go func() { _, _ = io.Copy(upstream, buffered); _ = upstream.Close(); close(done) }()
					_, _ = io.Copy(downstream, upstream)
					_ = downstream.Close()
					<-done
				}))
				defer func() {
					proxy.Close()
					handlers.Wait()
				}()
				client := newCloudHTTPClient(false)
				transport := client.Transport.(*blobTransport)
				transport.retryDelay = time.Millisecond
				base := transport.base.(*http.Transport)
				proxyURL, err := url.Parse(proxy.URL)
				if err != nil {
					t.Fatal(err)
				}
				base.Proxy = http.ProxyURL(proxyURL)
				base.TLSClientConfig = origin.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
				defer base.CloseIdleConnections()
				target := blobURL(origin.URL, []byte("payload"))
				if strings.HasSuffix(operation, "root") {
					target = origin.URL + "/sync/v3/root"
				}
				req, err := http.NewRequestWithContext(t.Context(), strings.Fields(operation)[0], target, strings.NewReader("payload"))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("rm-filename", "test.pdf")
				response, err := client.Do(req)
				shouldRetry := status != 501 && !strings.HasSuffix(operation, "root")
				if shouldRetry {
					if err != nil {
						t.Fatal(err)
					}
					defer func() {
						if err := response.Body.Close(); err != nil {
							t.Error(err)
						}
					}()
					data, err := io.ReadAll(response.Body)
					if err != nil || string(data) != "complete" || connects.Load() != 2 {
						t.Fatalf("response=%q err=%v connects=%d", data, err, connects.Load())
					}
				} else if err == nil || connects.Load() != 1 {
					if response != nil {
						_ = response.Body.Close()
					}
					t.Fatalf("unsafe replay: err=%v connects=%d", err, connects.Load())
				}
			})
		}
	}
}

func TestProxyConnectRetryAfterCancellation(t *testing.T) {
	var calls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(429)
	}))
	defer proxy.Close()
	client := newCloudHTTPClient(false)
	base := client.Transport.(*blobTransport).base.(*http.Transport)
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	base.Proxy = http.ProxyURL(proxyURL)
	defer base.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, blobURL("https://cloud.invalid", []byte("x")), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Do(req)
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}
