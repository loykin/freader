# freader

[![Coverage](./coverage-badge.svg)](./coverage-badge.svg)

A lightweight and reliable file log collector written in Go. It can be used both as a library (embedded in your app) and as a simple CLI tool.

Features:
- Easy to embed in Go applications
- Simple standalone binary (`cmd/freader`)
- Multi-platform (Linux, macOS, Windows; amd64/arm64)
- Multi-byte/string record separators ("\n", "\r\n", or tokens like "<END>")
- Flexible fingerprint strategies: deviceAndInode, checksum, and checksumSeperator (hash until Nth separator)
- Multiple sinks: console, file, ClickHouse, OpenSearch (with per-sink validation)
- Prometheus metrics support

## 🚀 Installation

Prebuilt binaries are available on the Releases page:
- https://github.com/loykin/freader/releases

Or build from source (Go 1.21+ required):

```bash
# macOS/Linux
go build -o freader ./cmd/freader

# install globally (ensure GOPATH/bin or GOBIN is in PATH)
go install ./cmd/freader
```

## 1) Quick Start (CLI)

Run the simplest form:

```bash
./freader
```

Common examples:
- Include multiple paths and change polling interval:
  ```bash
  ./freader --include ./log,/var/log --poll-interval 5s
  ```
- Change fingerprint strategy:
  ```bash
  ./freader --fingerprint-strategy deviceAndInode
  ```
- Use a config file (or env var):
  ```bash
  ./freader --config ./config/config.toml
  FREADER_CONFIG=./config/config.toml ./freader
  ```

Sinks:
- Default: console (stdout)
- Other backends: file, ClickHouse, OpenSearch (configured via config/env vars)

Prometheus metrics:
- Enable in config to expose `/metrics` (default `:2112`) including collector and sink metrics:
  ```toml
  [prometheus]
  enable = true
  addr = ":2112"
  ```

## 2) Configuration

An example configuration is provided at `config/config.toml`.

Minimal example (nested sections):

```toml
[collector]
# Target paths
include = ["./examples/embeded/log", "./examples/embeded/log/*.log"]
exclude = ["*.tmp"]

# Reader options
poll-interval = "2s"
separator = "\n"                  # supports multi-byte like "\r\n" or tokens like "<END>"
fingerprint-strategy = "checksum"   # or "deviceAndInode" or "checksumSeperator"
fingerprint-size = 64                # for checksum; for checksumSeperator it means N separators

[sink]
# Forwarding/output sink (optional)
type = "console"
labels = { env = "dev", app = "freader" }

# [sink.console]
# stream = "stdout"  # or "stderr"
```

Validation:
- Strategy-specific checks are enforced (e.g., checksum requires fingerprint-size > 0; checksumSeperator requires non-empty collector.separator).
- Each sink has its own validation (e.g., file.path must be set when sink.type="file").

Environment variables are also supported (uppercase; nested keys use `__`). Examples:
- `FREADER_COLLECTOR__INCLUDE="./log,./log/*.log"`
- `FREADER_COLLECTOR__SEPARATOR="<END>"`
- `FREADER_COLLECTOR__FINGERPRINT_STRATEGY=checksumSeperator`
- `FREADER_COLLECTOR__FINGERPRINT_SIZE=2`
- `FREADER_PROMETHEUS__ENABLE=true`
- `FREADER_SINK__TYPE=clickhouse`
- `FREADER_SINK__CLICKHOUSE__ADDR=http://localhost:8123`

## 3) Embed in your Go application (Library)

Example:

```
package main

import (
    "fmt"
    "log/slog"
    "os"
    "time"

    "github.com/loykin/freader"
)

func main() {
    slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})))

    cfg := freader.Config{}
    cfg.Default()
    cfg.Include = []string{"./log/*.log"} // files to read

    cfg.OnLineFunc = func(line string) {
        fmt.Println(line)
    }

    c, _ := freader.NewCollector(cfg)
    c.Start()
    time.Sleep(10 * time.Second)
    c.Stop()
}
```

See examples/ for:
- `examples/embeded` — embed directly into an app
- `examples/log_reader` — use TailReader only
- `examples/lumberjack_rotation` — log rotation behavior (device+inode and checksum)
- `examples/checksum_reader` — simple checksum strategy reader with bundled sample logs
- `examples/checksum_seperator` — checksumSeperator strategy with a custom token separator and bundled sample logs

## Notes & Tips
- Default sink: console (stdout)
- Changing `sink.type` disables console to avoid duplicate output
- Include/exclude filters apply at the sink stage
- Separator is a string and can be multi-byte; lines are emitted only when a full separator is seen (no partial records)
- Checksum-based strategies are cross-platform and friendly for Windows; device+inode may be OS-specific
- Offsets can be persisted (`collector.store-offsets=true`) to resume without loss after restarts
- Enable Prometheus for monitoring in production



