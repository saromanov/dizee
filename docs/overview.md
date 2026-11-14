# dizee — service overview

`dizee` shrinks noisy machine output (JSON, logs, diffs, Go code, test reports,
terminal output, plain text) before it reaches an AI agent's context window, so
the model spends tokens on content instead of formatting noise. It ships as a
Go library (`github.com/saromanov/dizee`) and a CLI (`cmd/dizee`). Compression
is deterministic, single-pass and cheap; every transform carries a guardrail
that the output is never larger than the input.

## Architecture

Three layers:

1. **Library** (repository root, package `dizee`): detection + transforms.
   Pure in-memory, no filesystem access.
2. **CLI** (`cmd/dizee`): stdin/file → shrink → stdout pipeline. Stats go to
   stderr, compressed bytes to stdout, so the binary is safe inside pipes.
   The CLI adds the only stateful piece: the recovery store for lossy
   transforms.
3. **Recovery store** (`internal/store`): SQLite database at
   `~/.dizee/store.db` that keeps the exact original bytes of every lossy
   shrink, keyed by a SHA-256 handle.

Data flow for the CLI:

```
stdin/file → dizee.Detect (or forced kind) → transform → guardrails → stdout
                                                              ↓
                                              lossy kind → store.Put → handle
```

For lossy kinds the CLI prefixes the compressed output with a deterministic
handle and stores the original:

```
[dizee:c5de4150a0848963f2eb6afea5f349ce7e5422f686fdcb0f20c8af8a7f1b54de]
2026-01-02 03:04:05 INFO retrying [x3]
```

`dizee retrieve <handle>` prints the original bytes back to stdout.

## Library API

```go
kind := dizee.Detect(b)              // Kind: json|log|diff|code|test|terminal|text; never fails
res, err := dizee.Shrink(b, "")      // "" = auto-detect; forced Kind also accepted
res, err := dizee.ShrinkDiff(b, 1)   // diff with explicit context (1 or 2 only)
res.Stats.Kind         // what ran
res.Stats.Compressed   // false ⇒ res.Output is nil, caller must use the input bytes
res.Stats.Before / After / Tokens
res.Saved()            // bytes removed, 0 when the original was kept
```

`Stats.Tokens` is the `output bytes / 4` heuristic, rounded up. It is for
comparing transforms, not for billing.

## Detection pipeline

`Detect` runs in a fixed order, first match wins:

1. **JSON** — input starts with `{`/`[` (after whitespace) and `json.Valid`;
   or every non-empty line is valid JSON with ≥ 2 lines (NDJSON).
2. **Terminal** — ≥ 3 ESC (0x1b) bytes. Fewer escapes are treated as noise in
   ordinary text.
3. **Diff** — a `diff --git ` header or an `@@ -l,c +l,c @@` hunk header.
4. **Test report** — pytest (`=== FAILURES ===` or `==== N passed ... ====`),
   Jest (`PASS|FAIL <file>` headers), JUnit XML (root element
   `testsuite`/`testsuites`) or `go test` output (package summary line
   `ok|FAIL|? pkg` plus at least one `=== RUN`/`--- PASS|FAIL|SKIP` line).
5. **Log** — ≥ 3 non-empty lines matching a timestamp/level regex
   (ISO 8601, syslog, `[hh:mm:ss]`, `[LEVEL]`, bare `INFO/ERROR/...`), and at
   least half of the non-empty lines match.
6. **Text** — everything else.

Go source is **never** auto-detected: `kind = "code"` must be forced. The
thresholds above are deliberately conservative so ordinary prose mentioning
tests or levels is not rewritten.

## Transforms

| kind | transform | lossy | recovery |
|---|---|---|---|
| `json` | `json.Compact`; NDJSON compacted per line, blank lines kept | no | — |
| `log` | runs of identical lines collapse to `line [xN]`; blank-line runs collapse to one | yes | yes |
| `diff` | keep headers/hunks/changed lines; context runs collapse to `[+N context lines]` keeping 1–2 boundary lines each side (`ShrinkDiff` context arg) | yes | yes |
| `code` | Go only: `go/parser` (with `parser.SkipObjectResolution`), `FuncDecl.Body = nil`, re-print via `go/printer` (`TabIndent | UseSpaces`) — imports and signatures survive, bodies do not | yes | yes |
| `test` | pytest/Jest/JUnit/`go test`: counters and complete failures kept, passes collapse to `[xN passed]` | yes | yes |
| `terminal` | strip ANSI (CSI, OSC, two-/three-byte escapes) and carriage-return redraws, then **re-detect** the cleaned text and apply `json`/`log`/`text` on top | yes | yes |
| `text` | trim trailing whitespace, collapse blank-line runs | near-lossless (line endings may differ) | no |

Details worth knowing:

- **pytest**: everything up to `=== FAILURES ===` collapses; `file.py .F.. [ 42%]`
  progress lines count dots as passed (`[xN passed]`); the failures section and
  the final `=====` summary are kept verbatim.
- **Jest**: `PASS`/`FAIL <file>` headers and `Test Suites:/Tests:/Snapshots:/Time:/Ran
  all test suites` lines are kept; `✓` lines collapse while inside a `FAIL`
  file every line is kept.
- **JUnit**: passed `<testcase>` elements are dropped and replaced by an
  `<!-- [xN passed] -->` XML comment; failed/errored/skipped cases keep all
  attributes and their `<failure>/<error>/<skipped>` children.
- **go test**: package summary lines (`ok pkg 0.5s`, `FAIL pkg 0.1s`,
  `? pkg [no test files]`) are always kept. `--- PASS` collapses together with
  its `=== RUN/PAUSE/CONT` lines and any log lines emitted while it ran
  (a pending-line buffer tracks open `=== RUN` positions by test name). A
  `--- FAIL` (or bare `FAIL`) flushes, switches to keep mode, and every
  following line — panics, stack traces, `t.Log` output — is kept verbatim
  until the next package summary line. The ancestor `=== RUN` lines of a
  failing subtest are flushed with it for context. A bare `PASS` line is
  dropped: the `ok` package line carries the verdict.
- **terminal → ANSI stripping**: CSI sequences (`ESC [ params intermediates
  final`), OSC sequences (`ESC ] ... BEL|ESC \`), and remaining short escapes.
  An OSC payload that hits `\n` stops the scan there instead of swallowing the
  rest of the input (cap: 4096 bytes); malformed sequences skip to the first
  grammar-breaking byte, so scanning always makes progress. `\r`-redraws keep
  only the text after the last `\r` of the line; a `\r` immediately before
  `\n` is a CRLF line break, not a redraw.
- **Trailing newline** is preserved on every transform: the input's trailing
  `\n` is stripped, remembered (`hadTrailing`), and re-appended only if it was
  there.

## Guardrails

- If a transform does not make the input smaller, `newResult` returns
  `Compressed: false` with the original stats — **output is never larger than
  the input**.
- The CLI counts the `[dizee:<handle>]\n` prefix against the savings; if the
  prefixed output would not be smaller, the original passes through.
- If the recovery store is unavailable or `Put` fails, lossy output is not
  emitted: the CLI warns on stderr (`dizee: recovery unavailable; kept
  original: ...`) and passes the original bytes through.
- Errors (invalid JSON under forced `-k json`, unparsable Go under `-k code`,
  unrecognized report under `-k test`): original bytes reach stdout so
  pipelines survive, the error goes to stderr, exit code 1.
- The library never touches the filesystem; recovery is a CLI-only concern.

## CLI

```
go install github.com/saromanov/dizee/cmd/dizee@latest
```

```
dizee < payload.json            # stdin → stdout, stats → stderr
dizee -k log app.log            # force kind (json|log|diff|code|test|terminal|text)
dizee -diff-context=2 < p.patch # context lines kept per diff run boundary (1 or 2, default 1)
dizee -stats=false < f.log      # silence stderr stats (on by default)
dizee -version
dizee retrieve <handle>         # print original bytes for a [dizee:...] handle
```

At most one file argument; more than one is an error. `retrieve` must be the
first argument (`os.Args[1]`) — it is checked before flag parsing.

## Code structure

| path | contents |
|---|---|
| `dizee.go` | package doc, `Kind`/`Stats`/`Result`, `Detect`, `Shrink`, JSON/log/text/terminal transforms, ANSI stripping, log-line regex |
| `diff.go` | diff detection, `ShrinkDiff` (public, context 1–2), context-run collapse |
| `code.go` | Go-source skeleton via `go/parser` + `go/printer` |
| `test.go` | test-report detection and pytest/Jest shrinks, `finishTestLines` helper |
| `gotest.go` | `go test` detection and collapse (pending-buffer state machine) |
| `junit.go` | JUnit XML detection and streaming re-encode with passed cases dropped |
| `cmd/dizee/main.go` | CLI: flags, stdin/file, `shrinkWithRecovery`, stats formatting, `retrieve` |
| `internal/store/store.go` | SQLite recovery store: `OpenDefault` (`~/.dizee/store.db`), `Handle` (SHA-256 hex), `Put`, `Get`, handle validation |
| `*_test.go` | table/fixture-style tests per source file, plus `cmd/dizee/main_test.go` |

## Stack

- Go 1.25 (`go.mod` `go 1.25.0`; CI pins Go 1.27).
- Single runtime dependency: `modernc.org/sqlite` — pure-Go SQLite, no cgo.
  This keeps `CGO_ENABLED=0` builds viable for the cross-platform release.
- Stdlib only otherwise: `encoding/json`, `encoding/xml`, `go/ast`,
  `go/parser`, `go/printer`, `crypto/sha256`, `database/sql`, `regexp`.

## Tests and CI

```
go test ./...            # from repository root
go test -race ./...      # what CI runs
go vet ./...             # CI gate
gofmt -l .               # CI gate: must be empty
```

Tests live next to the code (`dizee_test.go`, `diff_test.go`, `gotest_test.go`,
`junit_test.go`, `code_test.go`, `test_test.go`, `terminal_test.go`,
`cmd/dizee/main_test.go`, `internal/store/store_test.go`) and cover detection
thresholds, transform output, guardrails and CLI end-to-end behaviour.

CI (`.github/workflows/ci.yml`): on push to `master` and all PRs —
`gofmt` check → `go vet` → `go test -race`. Release workflow builds
cross-platform binaries and creates a GitHub release.

## Conventions

- All exported API surface lives in the root package; only `Shrink`,
  `ShrinkDiff`, `Detect` and the `Kind`/`Result`/`Stats` types are public.
- Every transform keeps the trailing-newline contract described above; reuse
  the `hadTrailing`/`finishTestLines` pattern for new transforms.
- New kinds must be added in four places: the `Kind` const block, the
  `Detect` order, the `Shrink` switch, and `isLossy` in `cmd/dizee/main.go`.
- Error messages follow `shrink <kind>: <cause>` in the library and
  `dizee: <message>` on the CLI, with original-bytes passthrough on failure.
- Commit messages are lowercase Conventional-Commits-style prefixes
  (`dizee: ...`, `ci: ...`, `docs: ...`); all code, comments, README and
  issues are in English.

## Pitfalls

- **`-k code` on non-Go input fails by design**: `go/parser` rejects it, the
  original passes through, exit 1. Go code is never auto-detected.
- **Detection order matters**: ≥ 3 ESC bytes win over everything except JSON;
  a colorized log is detected as `terminal`, and only after stripping does the
  log transform run (this chaining is intentional).
- **NDJSON needs ≥ 2 non-empty valid-JSON lines**; a single JSON line is
  plain `text` unless it starts with `{`/`[`.
- **`isLog` is conservative**: fewer than 3 matching lines, or less than half
  the non-empty lines matching, falls back to `text`.
- **`-diff-context` only accepts 1 or 2**; `ShrinkDiff` returns an error
  otherwise, and the CLI passes the value through unvalidated to the library.
- **The recovery store is keyed by content hash**: identical inputs
  deduplicate (`ON CONFLICT DO NOTHING`); `dizee retrieve` accepts only a
  64-char lowercase hex handle.
- **Store is single-connection**: `db.SetMaxOpenConns(1)` +
  `PRAGMA busy_timeout = 1000` — concurrent CLI invocations may briefly wait
  on the lock but never corrupt the DB.
- **`retrieve` bypasses flags**: `os.Args[1] == "retrieve"` is matched before
  `flag.Parse`, so `dizee -stats=false retrieve <h>` parses `<h>` as a flag
  argument and fails — the handle must follow `retrieve` directly.
- **Token estimates are bytes/4**: good for A/B-comparing transforms, wrong
  for billing or exact model tokenization.
