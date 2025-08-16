# freader

A lightweight and reliable file log collector written in Go. It can be used both as a library (embedded in your app) and as a simple CLI tool.

Features:
- Easy to embed in Go applications
- Simple standalone binary (`cmd/freader`)
- Multi-platform (Linux, macOS, Windows; amd64/arm64)
- Multiple sinks: console, file, ClickHouse, OpenSearch
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

Minimal example:

```toml
# Target paths
include = ["./examples/embeded/log/*.log"]
exclude = ["*.tmp"]

poll-interval = "2s"
fingerprint-strategy = "checksum"

[sink]
type = "console"
labels = { env = "dev", app = "freader" }
```

Environment variables are also supported (uppercase; nested keys use `__`). Examples:
- `FREADER_INCLUDE="./log,/var/log"`
- `FREADER_PROMETHEUS_ENABLE=true`
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
- `examples/lumberjack_rotation` — log rotation behavior

## Notes & Tips
- Default sink: console (stdout)
- Changing `sink.type` disables console to avoid duplicate output
- Include/exclude filters apply at the sink stage
- If buffers are full, lines may be dropped
- Enable Prometheus for monitoring in production


