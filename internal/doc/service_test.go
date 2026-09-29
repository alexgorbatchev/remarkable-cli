package doc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	cloud "github.com/alexgorbatchev/go-remarkable-cloud"
)

func TestDocListAndInspect_MockServer(t *testing.T) {
	// Setup mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token/v2/user":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("fake-token"))
		case "/sync/v3/root":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"hash":"root-hash","generation":1,"schemaVersion":3}`))
		case "/sync/v3/files/root-hash":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("doc-hash:doc-123:0:100\n"))
		case "/sync/v3/files/doc-hash":
			w.WriteHeader(http.StatusOK)
			// Return document metadata schema
			w.Write([]byte("meta-hash:doc-123.metadata:0:50\n"))
		case "/sync/v3/files/meta-hash":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"visibleName":"Test Note","type":"DocumentType","lastModified":"2026-09-29T10:00:00Z"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	client, err := cloud.NewClient(
		cloud.WithConfig(&cloud.Config{
			DeviceToken: "mock-dev-token",
			UserToken:   "mock-user-token",
		}),
		cloud.WithEndpoints(&cloud.Endpoints{
			WebappHost:  ts.URL,
			StorageHost: ts.URL,
		}),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()
	items, err := List(ctx, client, "", "", 10)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
	if items[0].ID != "doc-123" {
		t.Errorf("expected item doc-123, got %s", items[0].ID)
	}
}
