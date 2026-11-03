package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type countedBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *countedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}
func (b *countedBody) Close() error { b.closed = true; return nil }

func TestBufferedBlobLimit(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		declared   int64
		wantRead   int
		fails      bool
	}{
		{"exact", "1234", 4, 4, false},
		{"unknown exact", "1234", -1, 4, false},
		{"declared oversized", "123456789", 9, 0, true},
		{"unknown oversized", "123456789", -1, 5, true},
		{"false declaration", "123456789", 3, 5, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &countedBody{Reader: strings.NewReader(tc.data)}
			response, err := bufferBlobResponse(&http.Response{Body: body, ContentLength: tc.declared}, 4)
			if body.read != tc.wantRead || !body.closed {
				t.Fatalf("read %d bytes, closed=%v", body.read, body.closed)
			}
			if tc.fails {
				if response != nil || err == nil || !strings.Contains(err.Error(), "exceeds 4 bytes") || transientResponse(response, err) {
					t.Fatalf("oversized response = %v, %v", response, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(response.Body)
			if err != nil || string(data) != tc.data {
				t.Fatalf("buffered data %q, %v", data, err)
			}
			if err := response.Body.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
