// Command dizee shrinks JSON, logs, terminal output and plain text so AI
// agents spend fewer context tokens on formatting noise.
//
// Compression stats go to stderr; the compressed bytes go to stdout, so
// dizee can be dropped into a pipeline without changing its plumbing.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/saromanov/dizee"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	var (
		forcedKind  = flag.String("k", "", "force content kind: json, log, terminal or text (default: auto-detect)")
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

	res, err := dizee.Shrink(b, dizee.Kind(strings.TrimSpace(*forcedKind)))
	if err != nil {
		_, _ = os.Stdout.Write(b) // pass the original through so pipelines survive
		fmt.Fprintf(os.Stderr, "dizee: %v\n", err)
		return 1
	}

	out := b
	if res.Stats.Compressed {
		out = res.Output
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fmt.Fprintf(os.Stderr, "dizee: %v\n", err)
		return 1
	}

	if *showStats {
		printStats(res)
	}
	return 0
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
