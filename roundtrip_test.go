// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// corpus holds representative Markdown documents exercising every block and
// inline construct the converter handles. Each must survive a round-trip:
// Parse -> Write -> Parse reproduces the same richdoc tree, up to the
// normalisation the writer performs (emphasis markers to '*', list markers to
// '-'/'1.', setext headings to ATX, autolinks to inline links, and a soft
// line break inside a block flattened to a space — see escapeText).
var corpus = map[string]string{
	"headings-atx": "# One\n\n## Two\n\n### Three\n\n#### Four\n\n##### Five\n\n###### Six\n",

	"headings-setext": "Title\n=====\n\nSubtitle\n--------\n",

	"headings-setext-multiline": "Foo\nBar\n---\n",

	"paragraph-lazy-continuation-not-thematic-break": "Foo\n    ---\n",

	"paragraphs-and-breaks": "First paragraph with a soft\nline break inside it.\n\nSecond paragraph with a hard  \nline break inside it.\n",

	"inline-styles": "Text with *emph*, **strong**, ~~strike~~ and `code` spans.\n",

	"code-span-with-backtick": "Use `` a`b `` and `` `lead `` and `trail` `` here.\n",

	"links-and-images": "A [plain link](http://a.example) and a [titled link](http://b.example \"tip\").\n\n![alt text](img.png) then ![titled](pic.png \"cap\").\n",

	"autolink": "Visit <http://auto.example> today.\n",

	"nested-tight-list": "- one\n- two\n  - two-a\n  - two-b\n- three\n",

	"loose-list": "- alpha\n\n- beta\n\n- gamma\n",

	"ordered-list": "3. third\n4. fourth\n5. fifth\n",

	// A bullet-character change (goldmark's own list grammar, mirroring
	// CommonMark) starts a genuinely new list even with no blank line
	// between; Parse must keep them as two List blocks, and Write must
	// alternate markers so the rewritten source still parses as two lists
	// (not one re-merged, wrongly loosened list) on the next Parse.
	"adjacent-unordered-lists-marker-change": "- foo\n- bar\n+ baz\n",

	// Same hazard for an ordered-list delimiter change ("." to ")").
	"adjacent-ordered-lists-delimiter-change": "1. foo\n2. bar\n3) baz\n",

	// The same marker-change split nested two levels deep inside a list
	// item, exercising the adjacency tracking scoped to that item's own
	// block sequence rather than the top-level document.
	"adjacent-nested-lists-marker-change": "- outer\n  - a\n  - b\n  + c\n",

	"list-item-multiblock": "- first paragraph\n\n  second paragraph in the same item\n\n- next item\n",

	"blockquote": "> quoted line one\n> quoted line two\n>\n> quoted paragraph two\n",

	"nested-blockquote-list": "> outer\n>\n> - a\n> - b\n",

	"fenced-code": "```go\nfmt.Println(\"hi\")\nx := 1\n```\n",

	"indented-code": "    indented line one\n    indented line two\n",

	"table-alignments": "| left | center | right | plain |\n| :--- | :---: | ---: | --- |\n| l | c | r | p |\n| 1 | 2 | 3 | 4 |\n",

	"thematic-break": "before\n\n---\n\nafter\n",

	"raw-html": "A paragraph with <b>bold</b> and <i>italic</i> inline HTML.\n\n<div class=\"box\">\n  raw block\n</div>\n",

	"reference-definitions": "See [the ref][r] and [another][s].\n\n[r]: http://ref.example\n[s]: http://two.example \"titled\"\n",

	"escapes": "Literal chars: a\\*b, c\\_d, e\\`f, g\\[h\\], i\\~j and k\\<l.\n",

	"footnotes": "A claim needing support.[^1] And a second one.[^2]\n\n[^1]: The first note body.\n\n[^2]: The second note body.\n",

	"footnote-multiblock": "See the note.[^1]\n\n[^1]: First paragraph of the note.\n\n    Second paragraph of the note.\n",

	"heading-anchors": "# Plain Heading\n\n## Anchored Heading {#sec-intro}\n\nBody text under the section.\n",

	"cross-references": "See the [introduction](#sec-intro) and also the [summary](#sec-end).\n",

	"empty": "",
}

func TestRoundTrip(t *testing.T) {
	for name, src := range corpus {
		t.Run(name, func(t *testing.T) {
			d1, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			out, err := Write(d1)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			d2, err := Parse(out)
			if err != nil {
				t.Fatalf("re-Parse: %v", err)
			}
			want := normalizeSoftBreaks(d1)
			if !reflect.DeepEqual(want, d2) {
				t.Errorf("round-trip changed the tree\n--- rewritten source ---\n%s\n--- d1 (soft-break normalized) ---\n%#v\n--- d2 ---\n%#v", out, want, d2)
			}
		})
	}
}

// normalizeSoftBreaks returns a copy of d with every soft-line-break newline
// Parse preserves in a richdoc.Text.Value flattened to a single space — the
// same normalisation Write's escapeText applies, so d1 compares equal to the
// tree Write -> Parse actually produces (see escapeText's own doc comment for
// why the newline can't survive unchanged).
func normalizeSoftBreaks(d *richdoc.Document) *richdoc.Document {
	out := richdoc.Clone(d)
	out.Blocks = normalizeBlocks(out.Blocks)
	return out
}

func normalizeBlocks(blocks []richdoc.Block) []richdoc.Block {
	for i, b := range blocks {
		blocks[i] = normalizeBlock(b)
	}
	return blocks
}

func normalizeBlock(b richdoc.Block) richdoc.Block {
	switch n := b.(type) {
	case richdoc.Heading:
		n.Inlines = normalizeInlines(n.Inlines)
		return n
	case richdoc.Paragraph:
		n.Inlines = normalizeInlines(n.Inlines)
		return n
	case richdoc.List:
		for i, it := range n.Items {
			it.Blocks = normalizeBlocks(it.Blocks)
			n.Items[i] = it
		}
		return n
	case richdoc.BlockQuote:
		n.Blocks = normalizeBlocks(n.Blocks)
		return n
	case richdoc.Table:
		normalizeCells(n.Header)
		for _, row := range n.Rows {
			normalizeCells(row)
		}
		return n
	default:
		return b
	}
}

func normalizeCells(cells []richdoc.Cell) {
	for i, c := range cells {
		c.Inlines = normalizeInlines(c.Inlines)
		cells[i] = c
	}
}

func normalizeInlines(inlines []richdoc.Inline) []richdoc.Inline {
	for i, in := range inlines {
		inlines[i] = normalizeInline(in)
	}
	return inlines
}

func normalizeInline(in richdoc.Inline) richdoc.Inline {
	switch n := in.(type) {
	case richdoc.Text:
		n.Value = strings.ReplaceAll(n.Value, "\n", " ")
		return n
	case richdoc.Emph:
		n.Inlines = normalizeInlines(n.Inlines)
		return n
	case richdoc.Strong:
		n.Inlines = normalizeInlines(n.Inlines)
		return n
	case richdoc.Strikethrough:
		n.Inlines = normalizeInlines(n.Inlines)
		return n
	case richdoc.Link:
		n.Inlines = normalizeInlines(n.Inlines)
		return n
	case richdoc.Footnote:
		n.Blocks = normalizeBlocks(n.Blocks)
		return n
	case richdoc.Anchor:
		n.Inlines = normalizeInlines(n.Inlines)
		return n
	case richdoc.CrossRef:
		n.Inlines = normalizeInlines(n.Inlines)
		return n
	default:
		return in
	}
}

// TestRoundTripStableOutput checks that a second Write of the re-parsed tree
// produces byte-identical output, i.e. the writer reaches a fixed point.
func TestRoundTripStableOutput(t *testing.T) {
	for name, src := range corpus {
		t.Run(name, func(t *testing.T) {
			d1, _ := Parse([]byte(src))
			out1, _ := Write(d1)
			d2, _ := Parse(out1)
			out2, _ := Write(d2)
			if string(out1) != string(out2) {
				t.Errorf("writer not idempotent\n--- out1 ---\n%s\n--- out2 ---\n%s", out1, out2)
			}
		})
	}
}
