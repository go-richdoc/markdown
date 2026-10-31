// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package markdown_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/go-richdoc/markdown"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// The cost of BLOCK nesting is not the cost of bytes, and these benchmarks are
// what says so. They exist because the same measurement was written three
// times as a throwaway program under /tmp and lost three times to a reboot: a
// benchmark is the durable, reproducible form of it, and `go test -bench` is
// the command anybody can repeat.
//
// Each shape comes with a FLAT control of the SAME byte size. Without the
// control the numbers only say "a big document is slow", which is not a
// finding; with it they say the cost is the nesting, which is.
//
// Measured 2026-10-31 on an Apple M4 Max, Go 1.27.2, `-benchtime 1x`, no
// load. BenchmarkQuoteNestingDeep carries the sizes that are too slow to
// commit to `go test -bench=.`:
//
//	block quotes  10 000 deep    20 KB    54.7 ms
//	              50 000 deep   100 KB     1.35 s
//	             100 000 deep   200 KB     5.97 s
//	             200 000 deep   400 KB    21.87 s     <- x2 depth, x3.7 time
//
// Quadratic, in other words, and the asymmetry is what makes it a defect
// rather than a slow case: a block quote costs the author TWO BYTES a level,
// so 400 KB of source buys 22 seconds of CPU and a megabyte buys minutes.
//
// A nested list is the same cost with a different price tag — indentation
// makes the document itself O(n^2) bytes — which is why each case here is run
// against a FLAT control of the same size:
//
//	nested list  100 levels   10.3 KB    1.40 ms  |  flat, same size   288 µs
//	nested list  300 levels   90.9 KB   30.86 ms  |  flat, same size  1.05 ms
//
// 4.9x at 100 levels and 29.5x at 300: the ratio itself grows with depth.
//
// And the cost is GOLDMARK'S rather than this package's own walk, which is
// what decides where a bound belongs. BenchmarkGoldmarkOnly times the engine
// alone on the same documents:
//
//	depth    full Parse    goldmark alone    the engine's share
//	  100       153 µs            34 µs            22%
//	 1000       886 µs           664 µs            75%
//	 5000     14.67 ms         13.48 ms            92%
//
// The share grows with depth, so at the depth that matters the guard has to
// run BEFORE goldmark is handed the document. A counter in our own recursion
// would never reach it.

// quoteDoc is n nested block quotes, which cost two bytes per level -- the
// cheapest nesting Markdown has, and therefore the dangerous one.
func quoteDoc(n int) []byte {
	return []byte(strings.Repeat("> ", n) + "deep\n")
}

// nestedListDoc is n list levels. Indentation is how CommonMark spells list
// nesting, so this document is inherently O(n^2) bytes -- which is exactly why
// flatListDoc exists.
func nestedListDoc(n int) []byte {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(strings.Repeat("  ", i) + "- x\n")
	}
	return []byte(b.String())
}

// flatListDoc is n list items at ONE level, padded so the document is the same
// size as nestedListDoc(n). It is the control: same bytes, same construct
// count, no nesting.
func flatListDoc(n int) []byte {
	nested := len(nestedListDoc(n))
	var b strings.Builder
	for b.Len() < nested {
		b.WriteString("- " + strings.Repeat("x", 64) + "\n")
	}
	return []byte(b.String()[:nested])
}

func benchParse(b *testing.B, src []byte) {
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := markdown.Parse(src); err != nil {
			b.Fatalf("Parse: %v", err)
		}
	}
}

func BenchmarkQuoteNesting(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("depth=%d", n), func(b *testing.B) { benchParse(b, quoteDoc(n)) })
	}
}

// BenchmarkQuoteNestingDeep is where the curve becomes visible, and it is
// gated because one iteration of the last case takes the better part of a
// minute: `go test -bench=.` has to stay a command somebody runs.
//
//	NESTCOST_DEEP=1 go test -bench=QuoteNestingDeep -benchtime=1x ./...
func BenchmarkQuoteNestingDeep(b *testing.B) {
	if os.Getenv("NESTCOST_DEEP") == "" {
		b.Skip("set NESTCOST_DEEP=1; the last case takes tens of seconds per iteration")
	}
	for _, n := range []int{10000, 50000, 100000, 200000} {
		b.Run(fmt.Sprintf("depth=%d", n), func(b *testing.B) { benchParse(b, quoteDoc(n)) })
	}
}

// BenchmarkGoldmarkOnly times the ENGINE alone on the same documents, because
// "the parse is slow" and "our walk is slow" are different defects with
// different fixes — and the difference decides where the bound belongs. Read
// against BenchmarkQuoteNesting at the same depth: what is left is this
// package's own conversion.
func BenchmarkGoldmarkOnly(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		src := quoteDoc(n)
		b.Run(fmt.Sprintf("depth=%d", n), func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			gm := goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Footnote))
			for b.Loop() {
				gm.Parser().Parse(text.NewReader(src))
			}
		})
	}
}

// oneLineListDoc is n nested bullet lists on a SINGLE line: in CommonMark a
// "*" followed by three spaces opens an item whose content starts four columns
// in, so "*   *   *   x" is three nested lists and each level costs four
// bytes.
//
// ⛔ This shape was found by FuzzParse, not by thinking about the format. The
// first version of the nesting guard scanned a line's prefix for indentation
// and ">" markers and stopped at the first other character, so it read a line
// like this as depth ZERO and never saw it. The fuzzer produced one in six
// seconds — through the property that measures the DEPTH OF THE TREE THAT CAME
// BACK rather than trusting the guard, which is the only assertion that could
// have caught it.
func oneLineListDoc(n int) []byte {
	return []byte(strings.Repeat("*   ", n) + "x\n")
}

func BenchmarkOneLineListNesting(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		src := oneLineListDoc(n)
		b.Run(fmt.Sprintf("depth=%d", n), func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := markdown.ParseWithLimits(src, markdown.Limits{MaxBlockNesting: -1}); err != nil {
					b.Fatalf("Parse: %v", err)
				}
			}
		})
	}
}

func BenchmarkListNesting(b *testing.B) {
	for _, n := range []int{100, 300} {
		src := nestedListDoc(n)
		flat := flatListDoc(n)
		b.Run(fmt.Sprintf("nested=%d", n), func(b *testing.B) { benchParse(b, src) })
		// The control, at the same size, in the same run: reading one without
		// the other is how "big is slow" gets mistaken for a finding.
		b.Run(fmt.Sprintf("flat_same_size=%d", len(flat)), func(b *testing.B) { benchParse(b, flat) })
	}
}

// TestTheControlIsTheSameSize is the planted positive for the benchmark pair
// above: the comparison only attributes cost to NESTING if both documents are
// the same size, and a generator that quietly made the flat one smaller would
// turn the whole measurement into a tautology.
func TestTheControlIsTheSameSize(t *testing.T) {
	for _, n := range []int{10, 100, 300} {
		nested, flat := nestedListDoc(n), flatListDoc(n)
		if len(nested) != len(flat) {
			t.Errorf("n=%d: nested is %d bytes and the flat control %d; the comparison would measure size, not nesting",
				n, len(nested), len(flat))
		}
		if got := strings.Count(string(flat), "\n- "); got == 0 {
			t.Errorf("n=%d: the flat control holds no list items at all", n)
		}
	}
}

// TestTheShapesAreWhatTheyClaim is the other planted positive: a generator
// that produced the wrong DEPTH would make every number above meaningless, and
// nothing else in the suite looks at these documents.
func TestTheShapesAreWhatTheyClaim(t *testing.T) {
	for _, n := range []int{1, 5, 64} {
		if got := strings.Count(string(quoteDoc(n)), ">"); got != n {
			t.Errorf("quoteDoc(%d) holds %d markers", n, got)
		}
		lines := strings.Split(strings.TrimRight(string(nestedListDoc(n)), "\n"), "\n")
		if len(lines) != n {
			t.Fatalf("nestedListDoc(%d) has %d lines", n, len(lines))
		}
		deepest := len(lines[len(lines)-1]) - len(strings.TrimLeft(lines[len(lines)-1], " "))
		if want := 2 * (n - 1); deepest != want {
			t.Errorf("nestedListDoc(%d) indents its last line %d spaces, want %d", n, deepest, want)
		}
	}
}
