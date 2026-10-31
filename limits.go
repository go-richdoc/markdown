// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import "fmt"

// Limits bounds what this reader will do with a document nobody wrote.
//
// ⛔ MaxBlockNesting is how deeply block constructs may nest, and it exists
// because the cost of that nesting is not the cost of bytes. Measured by
// BenchmarkListNesting, which carries its own equal-size FLAT control:
//
//	nested list, 100 levels, 10.3 KB    1.40 ms   |  the same 10.3 KB flat   288 µs
//	nested list, 300 levels, 90.9 KB   30.86 ms   |  the same 90.9 KB flat  1.05 ms
//
// Same bytes, same construct count: 4.9x at 100 levels and 29.5x at 300, so
// the ratio itself grows with depth.
//
// Block quotes are the shape that makes it a defect rather than a slow case,
// because they cost the author TWO BYTES a level. Measured by
// BenchmarkQuoteNestingDeep (2026-10-31, Apple M4 Max, Go 1.27.2):
//
//	 10 000 deep    20 KB    54.7 ms
//	 50 000 deep   100 KB     1.35 s
//	100 000 deep   200 KB     5.97 s
//	200 000 deep   400 KB    21.87 s
//
// Doubling the depth multiplies the time by about 3.7, so 400 KB of source
// occupies a core for 22 seconds and a megabyte for minutes.
//
// ⛔ The cost belongs to goldmark rather than to this package's walk, which is
// what decides where the guard goes. BenchmarkGoldmarkOnly times the engine
// alone on the same documents: at depth 100 it is 22% of the whole conversion,
// at 1000 it is 75%, and at 5000 it is 92% -- the share grows with depth, so
// at the depth that matters almost all of the time is spent before this
// package's code runs at all. A counter in our own recursion would never
// reach it.
//
// So this is checked in a PASS OF ITS OWN before goldmark is handed the
// document -- the same placement go-richdoc/fb2 uses for the same reason, and
// the lesson go-odf/odf learned from a 115 MiB allocation inside a decoder
// while its own counter stood at two.
//
// goldmark has no nesting bound of its own: a search of its whole source for a
// maximum-depth or nesting limit finds nothing. The CommonMark spec is content
// for one to exist -- "Implementations may impose limits on parentheses
// nesting to avoid performance issues, but at least three levels of nesting
// should be supported" (spec §6.3, about link destinations) -- so a bound is
// the implementation's to choose, and this one is chosen from a measurement.
//
// THE DEFAULT IS 256, AND IT IS MEASURED. mddepthprobe (beside the Markdown
// corpus) read the 14 651 real `.md` files on the machine this was written on
// and counted both halves of the metric below: the deepest document reaches
// FIVE leading ">" markers, and the widest indentation is 73 columns -- about
// 36 levels. 256 clears the widest real document seven times over.
//
// Zero means the default. A NEGATIVE value means no limit, for a caller who
// would rather spend the time than lose the document, which is the convention
// go-richdoc/rst's LineLengthLimit and go-richdoc/latex's MaxDepth already use.
type Limits struct {
	MaxBlockNesting int
}

const defaultMaxBlockNesting = 256

func (l Limits) orDefaults() Limits {
	if l.MaxBlockNesting == 0 {
		l.MaxBlockNesting = defaultMaxBlockNesting
	}
	return l
}

// checkNesting refuses a document whose block prefix goes deeper than max on
// any line. It reads lines and never parses: the point is to answer before
// goldmark starts.
//
// ⛔ It tracks FENCED CODE, and that is not a refinement -- it is what keeps
// the guard from refusing real documents. Inside a fence, a leading ">" or a
// deep indent is CONTENT: a README quoting a transcript, or a Python sample
// indented twenty levels, would otherwise be refused for its own text. An
// indented (four-space) code block is not tracked, because telling one from a
// list continuation needs the parse this scan exists to precede; the default
// is set high enough that the difference cannot matter.
func checkNesting(src []byte, max int) error {
	if max <= 0 {
		return nil
	}
	line, fence := 1, byte(0)
	fenceLen := 0
	for i := 0; i < len(src); line++ {
		end := i
		for end < len(src) && src[end] != '\n' {
			end++
		}
		raw := src[i:end]
		if fence != 0 {
			if closesFence(raw, fence, fenceLen) {
				fence, fenceLen = 0, 0
			}
		} else if ch, n, ok := opensFence(raw); ok {
			fence, fenceLen = ch, n
		} else if depth := blockPrefixDepth(raw); depth > max {
			return fmt.Errorf("markdown: line %d nests blocks %d deep, over the limit of %d", line, depth, max)
		}
		i = end + 1
	}
	return nil
}

// blockPrefixDepth is how many block levels a line's PREFIX declares. A line's
// prefix is a SEQUENCE of block starters, which is the part that matters: a
// quote marker, a list marker, or indentation, each possibly followed by
// another.
//
// ⛔ The first version counted only indentation and ">" markers and stopped at
// the first other character — so it read
//
//   - *   *   *   ... x
//
// as depth ZERO. In CommonMark a "*" and three spaces open an item whose
// content begins four columns in, so that line is nested lists at FOUR BYTES A
// LEVEL, all on one line, and measured (BenchmarkOneLineListNesting) 20 KB of
// it costs 74.6 ms, quadratically: worse per byte than the block quotes this
// guard was written for.
//
// FuzzParse found it in six seconds, through the property that measures the
// depth of the tree that CAME BACK rather than trusting the guard. No unit
// test here would have: they all asked about shapes I had thought of.
//
// It is still a prefix measure and deliberately conservative. It cannot know
// whether an indent continues a list or opens a code block, and it counts
// "* * *" — a thematic break — as three levels. Over-counting is the safe
// direction for a guard whose default is seven times the widest real document;
// under-counting is what the fuzzer just punished.
func blockPrefixDepth(line []byte) int {
	depth, i := 0, 0
	for {
		cols := 0
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			if line[i] == '\t' {
				cols += 4
			} else {
				cols++
			}
			i++
		}
		// Indentation is how a nested list's continuation lines are spelled,
		// so it counts — two columns to a level.
		depth += cols / 2
		if i >= len(line) {
			return depth
		}
		if line[i] == '>' {
			depth++
			i++
			if i < len(line) && line[i] == ' ' {
				i++
			}
			continue
		}
		n := listMarker(line[i:])
		if n == 0 {
			return depth
		}
		depth++
		i += n
	}
}

// listMarker returns how many bytes a list marker and its following spaces
// occupy at the start of s, or zero if s does not begin with one.
//
// The marker rules are the spec's (CommonMark 5.2/5.3): a bullet is "-", "+"
// or "*"; an ordered marker is one to nine digits then "." or ")"; and either
// must be followed by a space, a tab, or the end of the line. That last clause
// is what keeps "*emphasis*" and "***" from reading as markers, which would
// turn ordinary prose into nesting.
//
// Up to four following spaces belong to the MARKER rather than to the content,
// because they set where the item's content begins; counting them as
// indentation as well would double-count every list in every document.
func listMarker(s []byte) int {
	i := 0
	switch {
	case len(s) > 0 && (s[0] == '-' || s[0] == '+' || s[0] == '*'):
		i = 1
	default:
		for i < len(s) && i < 9 && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == 0 || i >= len(s) || (s[i] != '.' && s[i] != ')') {
			return 0
		}
		i++
	}
	if i >= len(s) {
		return i // a marker alone on the line opens an empty item
	}
	if s[i] != ' ' && s[i] != '\t' {
		return 0
	}
	for n := 0; i < len(s) && n < 4 && (s[i] == ' ' || s[i] == '\t'); n++ {
		i++
	}
	return i
}

// opensFence reports a fenced-code opener: three or more backticks or tildes,
// indented at most three columns (CommonMark 4.5 — four would be an indented
// code block instead).
func opensFence(line []byte) (byte, int, bool) {
	i := 0
	for i < len(line) && line[i] == ' ' && i < 4 {
		i++
	}
	if i >= len(line) || i > 3 {
		return 0, 0, false
	}
	ch := line[i]
	if ch != '`' && ch != '~' {
		return 0, 0, false
	}
	n := 0
	for i < len(line) && line[i] == ch {
		n++
		i++
	}
	if n < 3 {
		return 0, 0, false
	}
	// A backtick fence's info string may not contain a backtick (CommonMark
	// 4.5), which is what keeps inline code on one line from opening a fence.
	if ch == '`' {
		for ; i < len(line); i++ {
			if line[i] == '`' {
				return 0, 0, false
			}
		}
	}
	return ch, n, true
}

// closesFence reports a closing fence, by the spec's three rules for one
// (CommonMark 4.5, read rather than recalled): it "must use the same character
// as the opening", "must be at least as long as the opening fence", and "may
// be preceded by up to three spaces of indentation, and may be followed only
// by spaces or tabs".
//
// ⛔ The indentation rule is the one that matters to a GUARD. Skipping any
// amount of indentation, as the first version did, would end fence tracking on
// a deeply indented line that the spec says does not close anything — so a
// fenced block holding an indented Markdown sample would have its own
// remaining content counted as nesting, and the document refused for its text.
func closesFence(line []byte, ch byte, openLen int) bool {
	i := 0
	for i < len(line) && line[i] == ' ' && i < 3 {
		i++
	}
	n := 0
	for i < len(line) && line[i] == ch {
		n++
		i++
	}
	if n < openLen {
		return false
	}
	for ; i < len(line); i++ {
		if line[i] != ' ' && line[i] != '\t' {
			return false
		}
	}
	return true
}
