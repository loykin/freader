# Multiline examples

This example demonstrates how to collect multiline logs and group them into logical records using freader’s multiline aggregator.

Two sample datasets are provided:
- Generic logs: continuation lines are indented (leading whitespace)
- Java logs: stack traces with lines starting with whitespace, "at ", or "Caused by:"

## Layout

```
examples/multiline/
  README.md
  main.go
  logs/
    generic/
      generic.log
    java/
      java.log
```

## Run

By default, the example auto-selects which directory to read based on the `-java` flag when `-path` is not provided.

- Generic (non-Java) mode:

```bash
# Reads ./examples/multiline/logs/generic/*.log
# Uses StartPattern ^(INFO|WARN|ERROR) and continuation ^\s
FREADER_DEMO_SECONDS=3 go run ./examples/multiline
```

- Java mode:

```bash
# Reads ./examples/multiline/logs/java/*.log
# Uses StartPattern ^(ERROR|WARN|INFO|Exception) and continuation ^(\s|at\s|Caused by:)
FREADER_DEMO_SECONDS=3 go run ./examples/multiline -java
```

- Custom path and separator:

```bash
# Override the input path or glob
FREADER_DEMO_SECONDS=3 go run ./examples/multiline -path "./my/logs/*.log" -sep "\n" -timeout 500ms
```

Notes:
- Set FREADER_DEMO_SECONDS to auto-stop after a few seconds for demo purposes; otherwise it runs until interrupted (Ctrl+C).
- For details about multiline timeout semantics and offsets, see the top-level README section: "Offset semantics and restart caveats".
