# dizee

[![GitHub release](https://img.shields.io/github/v/release/saromanov/dizee)](https://github.com/saromanov/dizee/releases/latest)

Shrink noisy JSON, logs, diffs, Go code, test reports, terminal output and plain text before they hit your AI agent's context window.

Agents read a lot of machine output: pretty-printed JSON, walls of repeated log
lines, failed test traces, ANSI-colored terminal dumps, trailing whitespace. `dizee` is a tiny CLI
(and Go package) that compacts that output deterministically, so the model
spends tokens on content instead of formatting.

## Install

```
go install github.com/saromanov/dizee/cmd/dizee@latest
```

## Usage

Read from stdin, write to stdout — safe for pipelines. Compression stats go to
stderr, so stdout stays clean:

```
$ dizee < payload.json
{"user":"ada","roles":["admin","dev"]}
dizee: json: 128 -> 42 bytes (~11 tokens, -67.2%)

$ dizee build.log > build.compact.log
dizee: log: 45210 -> 8112 bytes (~2028 tokens, -82.1%)
```

The token count uses the `output bytes / 4` heuristic, rounded up. It is useful
for comparing transforms, not for billing or exact model tokenization.

Auto-detection picks the transform; force one with `-k json|log|diff|code|test|terminal|text`
when the guess is wrong. Go code is never auto-detected:

```
dizee -k log app.log
dizee -diff-context=2 < changes.patch
dizee -k code < main.go
dizee -k test < results.xml
```

## What it does

| kind | transform | reversible |
|---|---|---|
| `json` | compact whitespace, also works on NDJSON (one document per line) | lossless |
| `log` | collapse runs of identical lines into `line [xN]` | original saved by the CLI |
| `diff` | keep headers and changed lines; collapse context runs with `-diff-context=1|2` boundary lines | original saved by the CLI |
| `code` | Go only: keep imports, declarations and signatures, remove function bodies | original saved by the CLI |
| `test` | pytest/Jest/JUnit/`go test`: keep counters and complete failures, collapse passes into `[xN passed]` | original saved by the CLI |
| `terminal` | strip ANSI escapes (colors, cursor moves, titles) and carriage-return redraws; cleaned text is re-detected, so a colorized log then collapses like a log | original saved by the CLI |
| `text` | trim trailing whitespace, collapse blank-line runs | line endings may differ |

For lossy `log`, `diff`, `code`, `test` and `terminal` output, the CLI stores the exact input in
`~/.dizee/store.db` and prefixes the compressed output with a deterministic
SHA-256 handle:

```
[dizee:c5de4150a0848963f2eb6afea5f349ce7e5422f686fdcb0f20c8af8a7f1b54de]
2026-01-02 03:04:05 INFO retrying [x3]
```

Restore the exact original bytes with `dizee retrieve <handle>`. The Go `Shrink`
API does not access the filesystem; recovery storage is a CLI concern.

Guardrails:

- if a transform does not make the input smaller, the original bytes are passed
  through untouched — output is never larger than input;
- if the recovery store is unavailable or full, lossy output is not emitted:
  the CLI warns on stderr and passes the original bytes through;
- invalid JSON under forced `-k json` is reported on stderr, the original bytes
  still reach stdout, and the exit code is non-zero.

## Library

```go
package main

import (
	"fmt"

	"github.com/saromanov/dizee"
)

func main() {
	res, err := dizee.Shrink(input, "") // "" = auto-detect
	if err != nil {
		panic(err)
	}
	if res.Stats.Compressed {
		fmt.Printf("%d -> %d bytes\n", res.Stats.Before, res.Stats.After)
		use(res.Output)
	} else {
		use(input) // original kept
	}
}
```

## License

[MIT](LICENSE)
