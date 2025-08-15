package opensearch

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenSearchSink_StartStopWithServer(t *testing.T) {
	// Fake _bulk endpoint that always returns success
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"took":1,"errors":false,"items":[{"index":{"status":201}}]}`))
	}))
	defer ts.Close()

	s, err := New(ts.URL, "logs-freader", "", "", "h1", map[string]string{"k": "v"}, 2, 10*time.Millisecond, nil, nil)
	if err != nil {
		t.Fatalf("failed to create sink: %v", err)
	}
	defer func() { _ = s.Stop() }()

	s.Enqueue("hello")
	time.Sleep(30 * time.Millisecond)
}

func TestOpenSearchSink_MissingConfig(t *testing.T) {
	if _, err := New("", "", "", "", "h1", nil, 1, 1, nil, nil); err == nil {
		t.Fatal("expected error when url or index missing")
	}
}
