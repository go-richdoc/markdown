// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package markdown_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-richdoc/markdown"
	"github.com/go-richdoc/richdoc"
)

// FuzzParse is the seventh and last of this family's readers to get a fuzz
// target, and the only one whose tokenising is somebody else's: goldmark is
// fuzzed upstream, so what this exercises is the CONVERSION of its AST and the
// nesting guard in front of it.
//
// Four properties:
//
//   - Parse does not panic. The conversion walks goldmark's tree recursively
//     and reads the source by byte offsets taken from it, which is where an
//     off-by-one lives.
//   - its two results are exclusive: a document OR an error, never neither.
//     The error is the guard's; goldmark reports none of its own.
//   - Parse is DETERMINISTIC. Footnote definitions are collected into a map
//     keyed by index and read back at the reference sites, and Go randomises
//     map iteration on purpose.
//   - the NESTING GUARD holds, measured on what came back rather than
//     trusted: with a small limit, a document that parses must not carry a
//     block tree deeper than the limit allows. A guard counted in the wrong
//     place passes its own unit test and still lets the document through.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"# Title\n\ntext with *emphasis*, **strong**, `code` and a [link](http://x/).\n",
		"- a\n- b\n  - nested\n\n1. one\n2. two\n",
		"> quoted\n>\n> > deeper\n",
		"| a | b |\n|---|---|\n| 1 | 2 |\n",
		"~~struck~~ and a footnote[^1]\n\n[^1]: the note\n",
		"## Heading {#explicit-id}\n\n```go\nfunc main() {}\n```\n",
		"    indented code\n\n***\n\n![alt](img.png \"title\")\n",
		// The guard's own shapes: nesting that must be refused, and the fenced
		// content that must NOT be.
		strings.Repeat("> ", 300) + "x\n",
		"```\n" + strings.Repeat("> ", 300) + "x\n```\n",
		"````\n    ```\n" + strings.Repeat(" ", 400) + "sample\n    ```\n````\n",
		// Malformed and adversarial inline shapes.
		strings.Repeat("[", 60) + "x" + strings.Repeat("]", 60) + "\n",
		strings.Repeat("*", 60) + "x" + strings.Repeat("*", 60) + "\n",
		"[ref]\n\n[ref]: http://x/\n",
		"<div>raw html</div>\n\n<!-- comment -->\n",
		"",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		lim := markdown.Limits{MaxBlockNesting: 16}
		doc, err := markdown.ParseWithLimits(src, lim)
		if err != nil {
			if doc != nil {
				t.Fatalf("both a document and an error: %v", err)
			}
			return
		}
		if doc == nil {
			t.Fatal("neither a document nor an error")
		}

		again, err2 := markdown.ParseWithLimits(src, lim)
		if err2 != nil || again == nil {
			t.Fatalf("the second Parse disagreed with the first: doc=%v err=%v", again != nil, err2)
		}
		if first, second := fmt.Sprintf("%#v", doc), fmt.Sprintf("%#v", again); first != second {
			t.Fatalf("two parses of the same bytes differ\nfirst:\n%.400s\nsecond:\n%.400s", first, second)
		}

		// The guard holds, measured on the tree. The slack is generous
		// because one block level can legitimately add more than one richdoc
		// level (a list adds List -> Item -> Paragraph); what is under test is
		// that the limit is counted where the nesting is, not an exact ratio.
		if d, ceiling := fuzzTreeDepth(doc.Blocks), 4*lim.MaxBlockNesting+16; d > ceiling {
			t.Errorf("the tree nests %d deep with MaxBlockNesting %d, over the %d allowed",
				d, lim.MaxBlockNesting, ceiling)
		}
	})
}

func fuzzTreeDepth(bs []richdoc.Block) int {
	if len(bs) == 0 {
		return 0
	}
	deepest := 0
	for _, b := range bs {
		for _, child := range fuzzChildBlocks(b) {
			if d := fuzzTreeDepth(child); d > deepest {
				deepest = d
			}
		}
	}
	return deepest + 1
}

func fuzzChildBlocks(b richdoc.Block) [][]richdoc.Block {
	switch v := b.(type) {
	case richdoc.BlockQuote:
		return [][]richdoc.Block{v.Blocks}
	case richdoc.List:
		out := make([][]richdoc.Block, 0, len(v.Items))
		for _, it := range v.Items {
			out = append(out, it.Blocks)
		}
		return out
	case richdoc.Table:
		var out [][]richdoc.Block
		for _, row := range append([][]richdoc.Cell{v.Header}, v.Rows...) {
			for _, c := range row {
				if len(c.Blocks) > 0 {
					out = append(out, c.Blocks)
				}
			}
		}
		return out
	}
	return nil
}
