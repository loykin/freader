package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/loykin/freader/cmd/freader/sink/clickhouse"
	"github.com/loykin/freader/cmd/freader/sink/common"
	"github.com/loykin/freader/cmd/freader/sink/console"
	"github.com/loykin/freader/cmd/freader/sink/opensearch"
)

// Sink is the common sink interface from subpackages.
type Sink = common.Sink

// buildSink constructs and starts a sink based on Config. Returns nil when Sink is disabled.
func buildSink(cfg *Config) (Sink, error) {
	switch cfg.Sink.Type {
	case "":
		return nil, nil
	case "console":
		stream := strings.ToLower(cfg.Sink.Console.Stream)
		return console.New(stream, cfg.Sink.BatchSize, cfg.Sink.BatchInterval, cfg.Sink.Include, cfg.Sink.Exclude), nil
	case "file":
		s, err := console.NewFile(
			cfg.Sink.File.Path,
			cfg.Sink.BatchSize,
			cfg.Sink.BatchInterval,
			cfg.Sink.Include,
			cfg.Sink.Exclude,
		)
		if err != nil {
			return nil, err
		}
		return s, nil
	case "clickhouse":
		host := cfg.Sink.Host
		if host == "" {
			if h, err := os.Hostname(); err == nil {
				host = h
			}
		}
		s, err := clickhouse.New(
			cfg.Sink.ClickHouse.Addr,
			cfg.Sink.ClickHouse.Database,
			cfg.Sink.ClickHouse.Table,
			cfg.Sink.ClickHouse.User,
			cfg.Sink.ClickHouse.Password,
			host,
			cfg.Sink.Labels,
			cfg.Sink.BatchSize,
			cfg.Sink.BatchInterval,
			cfg.Sink.Include,
			cfg.Sink.Exclude,
		)
		if err != nil {
			return nil, err
		}
		return s, nil
	case "opensearch":
		host := cfg.Sink.Host
		if host == "" {
			if h, err := os.Hostname(); err == nil {
				host = h
			}
		}
		s, err := opensearch.New(
			cfg.Sink.OpenSearch.URL,
			cfg.Sink.OpenSearch.Index,
			cfg.Sink.OpenSearch.User,
			cfg.Sink.OpenSearch.Password,
			host,
			cfg.Sink.Labels,
			cfg.Sink.BatchSize,
			cfg.Sink.BatchInterval,
			cfg.Sink.Include,
			cfg.Sink.Exclude,
		)
		if err != nil {
			return nil, err
		}
		return s, nil
	default:
		return nil, fmt.Errorf("unsupported sink: %s", cfg.Sink.Type)
	}
}
