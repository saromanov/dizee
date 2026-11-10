# dizee

Shrink noisy JSON, logs, terminal output and plain text before they hit your AI agent's context window.

Agents read a lot of machine output: pretty-printed JSON, walls of repeated log
lines, ANSI-colored terminal dumps, trailing whitespace. `dizee` is a tiny CLI
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

Auto-detection picks the transform; force one with `-k json|log|diff|terminal|text`
when the guess is wrong:

```
dizee -k log app.log
dizee -diff-context=2 < changes.patch
```

## What it does

| kind | transform | reversible |
|---|---|---|
| `json` | compact whitespace, also works on NDJSON (one document per line) | lossless |
| `log` | collapse runs of identical lines into `line [xN]` | adds a counter marker |
| `diff` | keep headers and changed lines; collapse context runs with `-diff-context=1|2` boundary lines | omitted context is recoverable only from the original diff |
| `terminal` | strip ANSI escapes (colors, cursor moves, titles) and carriage-return redraws; cleaned text is re-detected, so a colorized log then collapses like a log | stripped control bytes are lost |
| `text` | trim trailing whitespace, collapse blank-line runs | line endings may differ |

Guardrails:

- if a transform does not make the input smaller, the original bytes are passed
  through untouched — output is never larger than input;
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
