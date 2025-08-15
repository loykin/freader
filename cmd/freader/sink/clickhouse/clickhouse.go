package clickhouse

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/loykin/freader/cmd/freader/sink/common"
)

type Sink struct {
	batcher  common.Batcher
	conn     ch.Conn
	database string
	table    string
	host     string
	labels   map[string]string
}

func New(addr, database, table, user, pass, host string, labels map[string]string, batchSize int, batchInterval time.Duration, includes, excludes []string) (common.Sink, error) {
	if addr == "" || table == "" {
		return nil, fmt.Errorf("clickhouse addr and table are required")
	}
	// Build options: support HTTP and native
	var opts ch.Options
	if strings.Contains(addr, "://") {
		u, err := url.Parse(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid ch addr: %w", err)
		}
		hostport := u.Host
		secure := u.Scheme == "https"
		opts = ch.Options{Addr: []string{hostport}, Protocol: ch.HTTP, Auth: ch.Auth{Username: user, Password: pass, Database: database}}
		if secure {
			opts.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
		}
	} else {
		opts = ch.Options{Addr: []string{addr}, Auth: ch.Auth{Username: user, Password: pass, Database: database}}
	}
	// Run embedded migrations to ensure table exists
	if err := runMigrations(&opts, database, table); err != nil {
		return nil, err
	}
	// Open insert connection
	conn, err := ch.Open(&opts)
	if err != nil {
		return nil, err
	}
	s := &Sink{
		batcher:  common.NewBatcher(batchSize, batchInterval, includes, excludes),
		conn:     conn,
		database: database,
		table:    table,
		host:     host,
		labels:   labels,
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
				slog.Error("clickhouse flush failed", "error", err)
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tbl := s.table
	if s.database != "" && !strings.Contains(tbl, ".") {
		tbl = s.database + "." + s.table
	}
	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO "+tbl+" (ts, host, labels, message)")
	if err != nil {
		return err
	}
	for _, ln := range lines {
		if err := batch.Append(time.Now(), s.host, s.labels, ln); err != nil {
			return err
		}
	}
	return batch.Send()
}
