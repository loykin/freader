package metrics

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// getFreeAddr reserves a free address by binding to :0, returns the addr string, and closes it.
func getFreeAddr() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr, nil
}

func TestServerStartStop(t *testing.T) {
	addr, err := getFreeAddr()
	if err != nil {
		t.Fatalf("failed to get free addr: %v", err)
	}

	s, err := Start(addr)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() { _ = s.Stop() })

	// Register default metrics and bump one value to ensure /metrics has content
	if err := Register(prometheus.DefaultRegisterer); err != nil {
		t.Fatalf("register default failed: %v", err)
	}
	IncLines(1)

	// Poll the endpoint until it responds OK or timeout
	url := fmt.Sprintf("http://%s/metrics", addr)
	deadline := time.Now().Add(3 * time.Second)
	var resp *http.Response
	for time.Now().Before(deadline) {
		resp, err = http.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if resp == nil {
		t.Fatalf("no response from %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	// Check the body contains a known metric name
	r := bufio.NewReader(resp.Body)
	found := false
	for {
		line, err := r.ReadString('\n')
		if strings.Contains(line, "freader_lines_total") {
			found = true
			break
		}
		if err != nil {
			break
		}
	}
	if !found {
		t.Fatalf("metrics output does not contain freader_lines_total")
	}
}

func TestStopNilServer(t *testing.T) {
	var s *Server
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop on nil server returned error: %v", err)
	}
}
