// Command dizee shrinks JSON, logs, test reports, terminal output and plain
// text so AI agents spend fewer context tokens on formatting noise.
//
// Compression stats go to stderr; the compressed bytes go to stdout, so
// dizee can be dropped into a pipeline without changing its plumbing.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/saromanov/dizee"
	"github.com/saromanov/dizee/internal/store"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) > 1 && os.Args[1] == "retrieve" {
		return runRetrieve(os.Args[2:])
	}
	var (
		forcedKind  = flag.String("k", "", "force content kind: json, log, diff, code, test, terminal or text (default: auto-detect)")
		diffContext = flag.Int("diff-context", 1, "context lines to keep at each diff run boundary (1 or 2)")
		showStats   = flag.Bool("stats", true, "print compression stats to stderr")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("dizee " + version)
		return 0
	}

	in := io.Reader(os.Stdin)
	if flag.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "dizee: too many arguments, pass at most one file")
		return 1
	}
	if flag.NArg() == 1 {
		f, err := os.Open(flag.Arg(0))
		if err != nil {
			fmt.Fprintf(os.Stderr, "dizee: %v\n", err)
			return 1
		}
		defer func() { _ = f.Close() }()
		in = f
	}

	b, err := io.ReadAll(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dizee: %v\n", err)
		return 1
	}

	kind := dizee.Kind(strings.TrimSpace(*forcedKind))
	detectedKind := kind
	if detectedKind == "" {
		detectedKind = dizee.Detect(b)
	}
	var recovery *store.Store
	var recoveryErr error
	if isLossy(detectedKind) {
		recovery, recoveryErr = store.OpenDefault()
		if recovery != nil {
			defer func() { _ = recovery.Close() }()
		}
	}
	var saver recoveryStore
	if recovery != nil {
		saver = recovery
	}
	outcome := shrinkWithRecovery(b, kind, *diffContext, saver)
	if recoveryErr != nil {
		outcome.warning = recoveryErr
	}
	if outcome.err != nil {
		_, _ = os.Stdout.Write(b) // pass the original through so pipelines survive
		fmt.Fprintf(os.Stderr, "dizee: %v\n", outcome.err)
		return 1
	}
	if outcome.warning != nil {
		fmt.Fprintf(os.Stderr, "dizee: recovery unavailable; kept original: %v\n", outcome.warning)
	}
	if _, err := os.Stdout.Write(outcome.output); err != nil {
		fmt.Fprintf(os.Stderr, "dizee: %v\n", err)
		return 1
	}

	if *showStats {
		printStats(outcome.result)
	}
	return 0
}

func runRetrieve(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "dizee: usage: dizee retrieve <handle>")
		return 1
	}
	s, err := store.OpenDefault()
	if err != nil {
		fmt.Fprintf(os.Stderr, "dizee: %v\n", err)
		return 1
	}
	defer func() { _ = s.Close() }()
	content, err := s.Get(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "dizee: %v\n", err)
		return 1
	}
	if _, err := os.Stdout.Write(content); err != nil {
		fmt.Fprintf(os.Stderr, "dizee: %v\n", err)
		return 1
	}
	return 0
}

type recoveryStore interface {
	Put([]byte) (string, error)
}

type shrinkOutcome struct {
	result  dizee.Result
	output  []byte
	warning error
	err     error
}

func shrinkWithRecovery(b []byte, force dizee.Kind, diffContext int, recovery recoveryStore) shrinkOutcome {
	kind := force
	if kind == "" {
		kind = dizee.Detect(b)
	}
	unchanged := func() shrinkOutcome {
		return shrinkOutcome{
			result: dizee.Result{Stats: dizee.Stats{
				Kind: kind, Before: len(b), After: len(b), Tokens: (len(b) + 3) / 4,
			}},
			output: b,
		}
	}
	if isLossy(kind) && recovery == nil {
		outcome := unchanged()
		outcome.warning = errors.New("store unavailable")
		return outcome
	}
	var res dizee.Result
	var err error
	if kind == dizee.KindDiff {
		res, err = dizee.ShrinkDiff(b, diffContext)
	} else {
		res, err = dizee.Shrink(b, force)
	}
	if err != nil {
		return shrinkOutcome{err: err}
	}
	if !res.Stats.Compressed || !isLossy(kind) {
		out := b
		if res.Stats.Compressed {
			out = res.Output
		}
		return shrinkOutcome{result: res, output: out}
	}

	handle := store.Handle(b)
	if len(res.Output)+len(handle)+len("[dizee:]\n") >= len(b) {
		return unchanged()
	}
	savedHandle, err := recovery.Put(b)
	if err != nil {
		outcome := unchanged()
		outcome.warning = err
		return outcome
	}
	out := make([]byte, 0, len(res.Output)+len(savedHandle)+len("[dizee:]\n"))
	out = append(out, "[dizee:"...)
	out = append(out, savedHandle...)
	out = append(out, "]\n"...)
	out = append(out, res.Output...)
	res.Output = out
	res.Stats.After = len(out)
	res.Stats.Tokens = (len(out) + 3) / 4
	return shrinkOutcome{result: res, output: out}
}

func isLossy(kind dizee.Kind) bool {
	switch kind {
	case dizee.KindLog, dizee.KindDiff, dizee.KindCode, dizee.KindTest, dizee.KindTerminal:
		return true
	default:
		return false
	}
}

func printStats(res dizee.Result) {
	fmt.Fprint(os.Stderr, formatStats(res))
}

func formatStats(res dizee.Result) string {
	name := string(res.Stats.Kind)
	if !res.Stats.Compressed {
		return fmt.Sprintf("dizee: %s: kept original (%d bytes, ~%d tokens, nothing to save)\n", name, res.Stats.Before, res.Stats.Tokens)
	}
	pct := 0.0
	if res.Stats.Before > 0 {
		pct = 100 * float64(res.Saved()) / float64(res.Stats.Before)
	}
	return fmt.Sprintf("dizee: %s: %d -> %d bytes (~%d tokens, -%.1f%%)\n", name, res.Stats.Before, res.Stats.After, res.Stats.Tokens, pct)
}
