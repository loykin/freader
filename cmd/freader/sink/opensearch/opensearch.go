package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/loykin/freader/cmd/freader/sink/common"
	osclient "github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchutil"
)

type Sink struct {
	batcher common.Batcher
	client  *osclient.Client
	index   string
	host    string
	labels  map[string]string
}

func New(baseURL, index, user, pass, host string, labels map[string]string, batchSize int, batchInterval time.Duration, includes, excludes []string) (common.Sink, error) {
	if baseURL == "" || index == "" {
		return nil, fmt.Errorf("opensearch url and index are required")
	}
	cfg := osclient.Config{Addresses: []string{baseURL}}
	if user != "" {
		cfg.Username = user
		cfg.Password = pass
	}
	cli, err := osclient.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	s := &Sink{
		batcher: common.NewBatcher(batchSize, batchInterval, includes, excludes),
		client:  cli,
		index:   index,
		host:    host,
		labels:  labels,
	}
	s.start()
	return s, nil
}

func (s *Sink) start() {
	s.batcher.Wg.Add(1)
	go func() {
		defer s.batcher.Wg.Done()
		buf := make([]string, 0, s.batcher.BatchSize)
		ticker := time.NewTicker(s.batcher.BatchInterval)
		defer ticker.Stop()
		flush := func() {
			if len(buf) == 0 {
				return
			}
			if err := s.flush(buf); err != nil {
				slog.Error("opensearch flush failed", "error", err)
			}
			buf = buf[:0]
		}
		for {
			select {
			case <-s.batcher.StopCh:
				flush()
				return
			case <-ticker.C:
				flush()
			case line := <-s.batcher.Ch:
				buf = append(buf, line)
				if len(buf) >= s.batcher.BatchSize {
					flush()
				}
			}
		}
	}()
}

func (s *Sink) Stop() error {
	s.batcher.StopOnce.Do(func() { close(s.batcher.StopCh) })
	s.batcher.Wg.Wait()
	return nil
}

func (s *Sink) Enqueue(line string) { s.batcher.Enqueue(line) }

func (s *Sink) flush(lines []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	bi, err := opensearchutil.NewBulkIndexer(opensearchutil.BulkIndexerConfig{
		Client: s.client,
		Index:  s.index,
	})
	if err != nil {
		return err
	}
	for _, ln := range lines {
		doc := map[string]any{
			"@timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"message":    ln,
			"host":       s.host,
			"labels":     s.labels,
		}
		b, _ := json.Marshal(doc)
		err = bi.Add(ctx, opensearchutil.BulkIndexerItem{
			Action:     "index",
			DocumentID: "",
			Body:       bytes.NewReader(b),
			OnFailure: func(ctx context.Context, item opensearchutil.BulkIndexerItem, resp opensearchutil.BulkIndexerResponseItem, err error) {
				if err != nil {
					slog.Error("opensearch bulk item error", "error", err)
					return
				}
				slog.Error("opensearch bulk item failed", "status", resp.Status, "error", resp.Error)
			},
		})
		if err != nil {
			return err
		}
	}
	if err := bi.Close(ctx); err != nil {
		return err
	}
	stats := bi.Stats()
	if stats.NumFailed > 0 {
		return fmt.Errorf("opensearch bulk failed items: %d", stats.NumFailed)
	}
	return nil
}
