// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
	gast "github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

// TestParseNodes checks that Parse builds the expected richdoc nodes for each
// Markdown construct.
func TestParseNodes(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []richdoc.Block
	}{
		{
			"heading-level",
			"### Deep\n",
			[]richdoc.Block{richdoc.Heading{Level: 3, Inlines: []richdoc.Inline{richdoc.Text{Value: "Deep"}}}},
		},
		{
			"setext-heading",
			"Title\n=====\n",
			[]richdoc.Block{richdoc.Heading{Level: 1, Inlines: []richdoc.Inline{richdoc.Text{Value: "Title"}}}},
		},
		{
			"emphasis-vs-strong",
			"*i* and **b**\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Emph{Inlines: []richdoc.Inline{richdoc.Text{Value: "i"}}},
				richdoc.Text{Value: " and "},
				richdoc.Strong{Inlines: []richdoc.Inline{richdoc.Text{Value: "b"}}},
			}}},
		},
		{
			"strikethrough-and-code",
			"~~s~~ `c`\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Strikethrough{Inlines: []richdoc.Inline{richdoc.Text{Value: "s"}}},
				richdoc.Text{Value: " "},
				richdoc.Code{Value: "c"},
			}}},
		},
		{
			"hard-break",
			"a  \nb\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Text{Value: "a"}, richdoc.LineBreak{}, richdoc.Text{Value: "b"},
			}}},
		},
		{
			"soft-break",
			"a\nb\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Text{Value: "a\nb"},
			}}},
		},
		{
			"link-with-title",
			"[t](u \"ti\")\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Link{URL: "u", Title: "ti", Inlines: []richdoc.Inline{richdoc.Text{Value: "t"}}},
			}}},
		},
		{
			"image-with-title",
			"![a](u \"ti\")\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Image{URL: "u", Alt: "a", Title: "ti"},
			}}},
		},
		{
			"autolink",
			"<http://a.example>\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Link{URL: "http://a.example", Inlines: []richdoc.Inline{richdoc.Text{Value: "http://a.example"}}},
			}}},
		},
		{
			// goldmark's AutoLink.URL doesn't add "mailto:" itself for an
			// email autolink (only its own HTML renderer does, separately);
			// Parse has to add it, or the link is unreachable.
			"autolink-email",
			"<a@b.example>\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Link{URL: "mailto:a@b.example", Inlines: []richdoc.Inline{richdoc.Text{Value: "a@b.example"}}},
			}}},
		},
		{
			// goldmark's Link.Destination/.Title are raw source bytes; a
			// backslash-escaped destination must come out decoded, matching
			// what goldmark's own HTML renderer resolves it to.
			"link-destination-decoded",
			`[t](foo\(bar\))` + "\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Link{URL: "foo(bar)", Inlines: []richdoc.Inline{richdoc.Text{Value: "t"}}},
			}}},
		},
		{
			"fenced-code-lang",
			"```rust\nlet x = 1;\n```\n",
			[]richdoc.Block{richdoc.CodeBlock{Language: "rust", Text: "let x = 1;\n"}},
		},
		{
			"indented-code",
			"    plain\n",
			[]richdoc.Block{richdoc.CodeBlock{Language: "", Text: "plain\n"}},
		},
		{
			"ordered-start",
			"5. five\n6. six\n",
			[]richdoc.Block{richdoc.List{Ordered: true, Start: 5, Tight: true, Items: []richdoc.ListItem{
				{Blocks: []richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Text{Value: "five"}}}}},
				{Blocks: []richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Text{Value: "six"}}}}},
			}}},
		},
		{
			// goldmark's List.Start is 0 for an unordered list (meaningless)
			// but a genuine, valid CommonMark ordinal for an ordered one —
			// "0. ok" must not be clamped up to Start: 1.
			"ordered-start-zero",
			"0. ok\n",
			[]richdoc.Block{richdoc.List{Ordered: true, Start: 0, Tight: true, Items: []richdoc.ListItem{
				{Blocks: []richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Text{Value: "ok"}}}}},
			}}},
		},
		{
			"raw-html-block",
			"<div>x</div>\n",
			[]richdoc.Block{richdoc.RawBlock{Format: "html", Text: "<div>x</div>\n"}},
		},
		{
			"raw-html-inline",
			"a <b>c</b>\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Text{Value: "a "},
				richdoc.RawInline{Format: "html", Text: "<b>"},
				richdoc.Text{Value: "c"},
				richdoc.RawInline{Format: "html", Text: "</b>"},
			}}},
		},
		{
			"thematic-break",
			"---\n",
			[]richdoc.Block{richdoc.ThematicBreak{}},
		},
		{
			"table-alignments",
			"| a | b | c | d |\n| :- | :-: | -: | - |\n| 1 | 2 | 3 | 4 |\n",
			[]richdoc.Block{richdoc.Table{
				Align: []richdoc.Alignment{richdoc.AlignLeft, richdoc.AlignCenter, richdoc.AlignRight, richdoc.AlignDefault},
				Header: []richdoc.Cell{
					{Inlines: []richdoc.Inline{richdoc.Text{Value: "a"}}},
					{Inlines: []richdoc.Inline{richdoc.Text{Value: "b"}}},
					{Inlines: []richdoc.Inline{richdoc.Text{Value: "c"}}},
					{Inlines: []richdoc.Inline{richdoc.Text{Value: "d"}}},
				},
				Rows: [][]richdoc.Cell{{
					{Inlines: []richdoc.Inline{richdoc.Text{Value: "1"}}},
					{Inlines: []richdoc.Inline{richdoc.Text{Value: "2"}}},
					{Inlines: []richdoc.Inline{richdoc.Text{Value: "3"}}},
					{Inlines: []richdoc.Inline{richdoc.Text{Value: "4"}}},
				}},
			}},
		},
		{
			"entity-and-escape-decoded",
			"a &amp; b and c \\* d and &#42;\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Text{Value: "a & b and c * d and *"},
			}}},
		},
		{
			"blockquote",
			"> q\n",
			[]richdoc.Block{richdoc.BlockQuote{Blocks: []richdoc.Block{
				richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Text{Value: "q"}}},
			}}},
		},
		{
			"reference-definition-dropped",
			"[x][r]\n\n[r]: http://r.example\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Link{URL: "http://r.example", Inlines: []richdoc.Inline{richdoc.Text{Value: "x"}}},
			}}},
		},
		{
			// A [^id] reference maps to an inline Footnote holding the body
			// found in the definition; the trailing definition list is dropped.
			"footnote",
			"Text.[^n]\n\n[^n]: The note.\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Text{Value: "Text."},
				richdoc.Footnote{Blocks: []richdoc.Block{
					richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Text{Value: "The note."}}},
				}},
			}}},
		},
		{
			// An explicit heading attribute becomes the Heading.ID.
			"heading-anchor",
			"## Intro {#sec-intro}\n",
			[]richdoc.Block{richdoc.Heading{Level: 2, ID: "sec-intro", Inlines: []richdoc.Inline{richdoc.Text{Value: "Intro"}}}},
		},
		{
			// A clean #fragment link maps to a RefLabel cross-reference.
			"cross-reference",
			"See [the intro](#sec-intro).\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Text{Value: "See "},
				richdoc.CrossRef{Target: "sec-intro", Kind: richdoc.RefLabel, Inlines: []richdoc.Inline{richdoc.Text{Value: "the intro"}}},
				richdoc.Text{Value: "."},
			}}},
		},
		{
			// A titled #fragment link is left as a plain Link, not a cross-ref.
			"cross-reference-titled-stays-link",
			"[x](#sec \"tip\")\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Link{URL: "#sec", Title: "tip", Inlines: []richdoc.Inline{richdoc.Text{Value: "x"}}},
			}}},
		},
		{
			// An angle-bracket destination with a space is an unclean fragment
			// and stays a plain Link.
			"cross-reference-unclean-stays-link",
			"[x](<#a b>)\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Link{URL: "#a b", Inlines: []richdoc.Inline{richdoc.Text{Value: "x"}}},
			}}},
		},
		{
			// A bare "#" destination is too short to be a fragment and stays a
			// plain Link.
			"cross-reference-bare-hash-stays-link",
			"[x](#)\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Link{URL: "#", Inlines: []richdoc.Inline{richdoc.Text{Value: "x"}}},
			}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !reflect.DeepEqual(d.Blocks, tc.want) {
				t.Errorf("Parse(%q).Blocks =\n%#v\nwant\n%#v", tc.src, d.Blocks, tc.want)
			}
		})
	}
}

// TestParseEmpty checks that an empty document parses to a block-less document.
func TestParseEmpty(t *testing.T) {
	d, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(d.Blocks) != 0 {
		t.Errorf("Parse(nil) has %d blocks, want 0", len(d.Blocks))
	}
}

// TestWriteSyntax checks that Write emits the expected Markdown syntax for
// documents built directly with the richdoc model, including nodes that Parse
// never produces (math, and various degenerate shapes).
func TestWriteSyntax(t *testing.T) {
	cases := []struct {
		name    string
		doc     *richdoc.Document
		want    string
		wantSub bool // want is a substring rather than the whole output
	}{
		{
			"heading",
			richdoc.New().H(2, richdoc.Txt("Hi")).Doc(),
			"## Hi\n", false,
		},
		{
			"heading-level-clamped-low",
			richdoc.New().Add(richdoc.Heading{Level: 0, Inlines: []richdoc.Inline{richdoc.Txt("z")}}).Doc(),
			"# z\n", false,
		},
		{
			"heading-level-clamped-high",
			richdoc.New().Add(richdoc.Heading{Level: 9, Inlines: []richdoc.Inline{richdoc.Txt("z")}}).Doc(),
			"###### z\n", false,
		},
		{
			"inline-styles",
			richdoc.New().P(
				richdoc.Italic(richdoc.Txt("i")), richdoc.Txt(" "),
				richdoc.Bold(richdoc.Txt("b")), richdoc.Txt(" "),
				richdoc.Strike(richdoc.Txt("s")), richdoc.Txt(" "),
				richdoc.Mono("c"),
			).Doc(),
			"*i* **b** ~~s~~ `c`\n", false,
		},
		{
			"link-and-image-no-title",
			richdoc.New().P(
				richdoc.Href("u", "", richdoc.Txt("t")), richdoc.Txt(" "),
				richdoc.Img("v", "a", ""),
			).Doc(),
			"[t](u) ![a](v)\n", false,
		},
		{
			"link-and-image-with-title",
			richdoc.New().P(
				richdoc.Href("u", "ti", richdoc.Txt("t")), richdoc.Txt(" "),
				richdoc.Img("v", "a", "ci"),
			).Doc(),
			"[t](u \"ti\") ![a](v \"ci\")\n", false,
		},
		{
			"hard-break",
			richdoc.New().P(richdoc.Txt("a"), richdoc.Br(), richdoc.Txt("b")).Doc(),
			"a\\\nb\n", false,
		},
		{
			"fenced-code-with-lang",
			richdoc.New().CodeBlock("go", "x := 1\n").Doc(),
			"```go\nx := 1\n```\n", false,
		},
		{
			"code-no-trailing-newline",
			richdoc.New().CodeBlock("", "no-newline").Doc(),
			"```\nno-newline\n```\n", false,
		},
		{
			"code-containing-fence",
			richdoc.New().CodeBlock("", "a\n```\nb").Doc(),
			"````\na\n```\nb\n````\n", false,
		},
		{
			"inline-code-with-backtick",
			richdoc.New().P(richdoc.Mono("a`b")).Doc(),
			"``a`b``\n", false,
		},
		{
			"inline-code-leading-backtick",
			richdoc.New().P(richdoc.Mono("`x")).Doc(),
			"`` `x ``\n", false,
		},
		{
			"tight-list",
			richdoc.New().UList(true, richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("a")}}),
				richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("b")}})).Doc(),
			"- a\n- b\n", false,
		},
		{
			"loose-list",
			richdoc.New().UList(false, richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("a")}}),
				richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("b")}})).Doc(),
			"- a\n\n- b\n", false,
		},
		{
			"ordered-list-start",
			richdoc.New().OList(3, true, richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("a")}})).Doc(),
			"3. a\n", false,
		},
		{
			"ordered-list-start-clamped",
			richdoc.New().OList(0, true, richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("a")}})).Doc(),
			"1. a\n", false,
		},
		{
			// Two adjacent unordered List blocks would re-merge into one
			// list on the next Parse if both wrote the default "-"
			// marker (goldmark's own list grammar treats a same-bullet
			// run, blank line or not, as one continuing list). The
			// second must alternate to "+" to stay visibly separate.
			"adjacent-unordered-lists-alternate-marker",
			richdoc.New().
				UList(true, richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("a")}})).
				UList(true, richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("b")}})).
				Doc(),
			"- a\n\n+ b\n", false,
		},
		{
			// Same hazard for ordered lists: the delimiter alternates
			// between "." and ")" instead of the bullet character.
			"adjacent-ordered-lists-alternate-delimiter",
			richdoc.New().
				OList(1, true, richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("a")}})).
				OList(2, true, richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("b")}})).
				Doc(),
			"1. a\n\n2) b\n", false,
		},
		{
			// Three lists in a row alternate back to the default marker
			// for the third rather than colliding with the second (which
			// only needs to differ from its own immediate neighbours).
			"three-adjacent-unordered-lists-alternate-in-sequence",
			richdoc.New().
				UList(true, richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("a")}})).
				UList(true, richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("b")}})).
				UList(true, richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("c")}})).
				Doc(),
			"- a\n\n+ b\n\n- c\n", false,
		},
		{
			// A List immediately followed by a different-kind List (one
			// ordered, one not) never needs alternation: the two markers
			// already can't collide.
			"adjacent-lists-different-kind-no-alternation",
			richdoc.New().
				UList(true, richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("a")}})).
				OList(1, true, richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("b")}})).
				Doc(),
			"- a\n\n1. b\n", false,
		},
		{
			// The adjacency tracking is scoped per block sequence, not
			// global: two adjacent sub-lists nested inside a list item
			// need the same alternation as top-level siblings.
			"adjacent-nested-lists-alternate-marker",
			richdoc.New().UList(true, richdoc.Item(
				richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("outer")}},
				richdoc.List{Start: 1, Tight: true, Items: []richdoc.ListItem{
					{Blocks: []richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("a")}}}},
				}},
				richdoc.List{Start: 1, Tight: true, Items: []richdoc.ListItem{
					{Blocks: []richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("b")}}}},
				}},
			)).Doc(),
			"- outer\n  - a\n  + b\n", false,
		},
		{
			"blockquote",
			richdoc.New().Quote(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("q")}}).Doc(),
			"> q\n", false,
		},
		{
			"thematic-break",
			richdoc.New().HR().Doc(),
			"---\n", false,
		},
		{
			"math-block",
			richdoc.New().MathBlock("a^2+b^2").Doc(),
			"$$\na^2+b^2\n$$\n", false,
		},
		{
			"math-inline",
			richdoc.New().P(richdoc.InlineMath("x^2")).Doc(),
			"$x^2$\n", false,
		},
		{
			"raw-block-html",
			richdoc.New().RawBlock("html", "<hr/>").Doc(),
			"<hr/>\n", false,
		},
		{
			"raw-block-other-format",
			richdoc.New().RawBlock("latex", "\\newpage").Doc(),
			"\\newpage\n", false,
		},
		{
			"raw-inline",
			richdoc.New().P(richdoc.Txt("a"), richdoc.RawI("html", "<br>"), richdoc.Txt("b")).Doc(),
			"a<br>b\n", false,
		},
		{
			"table-with-alignment",
			richdoc.New().Table(
				[]richdoc.Alignment{richdoc.AlignLeft, richdoc.AlignCenter, richdoc.AlignRight, richdoc.AlignDefault},
				[]richdoc.Cell{richdoc.Td(richdoc.Txt("a")), richdoc.Td(richdoc.Txt("b")), richdoc.Td(richdoc.Txt("c")), richdoc.Td(richdoc.Txt("d"))},
				[][]richdoc.Cell{{richdoc.Td(richdoc.Txt("1")), richdoc.Td(richdoc.Txt("2")), richdoc.Td(richdoc.Txt("3")), richdoc.Td(richdoc.Txt("4"))}},
			).Doc(),
			"| a | b | c | d |\n| :--- | :---: | ---: | --- |\n| 1 | 2 | 3 | 4 |\n", false,
		},
		{
			"table-pipe-escaped-in-cell",
			richdoc.New().Table(nil,
				[]richdoc.Cell{richdoc.Td(richdoc.Txt("a|b"))},
				[][]richdoc.Cell{{richdoc.Td(richdoc.Txt("c"))}},
			).Doc(),
			"| a\\|b |", true,
		},
		{
			"table-align-widest",
			// Alignment declares more columns than the header or any row.
			richdoc.New().Table(
				[]richdoc.Alignment{richdoc.AlignLeft, richdoc.AlignCenter, richdoc.AlignRight},
				[]richdoc.Cell{richdoc.Td(richdoc.Txt("a"))},
				[][]richdoc.Cell{{richdoc.Td(richdoc.Txt("1"))}},
			).Doc(),
			"| a |  |  |\n| :--- | :---: | ---: |\n| 1 |  |  |\n", false,
		},
		{
			"heading-with-id",
			richdoc.New().Add(richdoc.Heading{Level: 2, ID: "sec-intro", Inlines: []richdoc.Inline{richdoc.Txt("Intro")}}).Doc(),
			"## Intro {#sec-intro}\n", false,
		},
		{
			"footnote",
			richdoc.New().P(richdoc.Txt("x"), richdoc.Note(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("the body")}})).Doc(),
			"x[^1]\n\n[^1]: the body\n", false,
		},
		{
			"footnote-multiblock",
			richdoc.New().P(richdoc.Txt("x"), richdoc.Note(
				richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("first")}},
				richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("second")}},
			)).Doc(),
			"x[^1]\n\n[^1]: first\n\n    second\n", false,
		},
		{
			"two-footnotes-numbered-in-order",
			richdoc.New().P(
				richdoc.Txt("a"), richdoc.Note(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("one")}}),
				richdoc.Txt("b"), richdoc.Note(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("two")}}),
			).Doc(),
			"a[^1]b[^2]\n\n[^1]: one\n\n[^2]: two\n", false,
		},
		{
			"cross-reference-label",
			richdoc.New().P(richdoc.Ref("sec-intro", richdoc.Txt("the intro"))).Doc(),
			"[the intro](#sec-intro)\n", false,
		},
		{
			"cross-reference-label-empty-uses-target",
			richdoc.New().P(richdoc.Ref("sec-intro")).Doc(),
			"[sec-intro](#sec-intro)\n", false,
		},
		{
			"cross-reference-citation",
			richdoc.New().P(richdoc.Cite("knuth1984", richdoc.Txt("Knuth"))).Doc(),
			"[@knuth1984]\n", false,
		},
		{
			"anchor-renders-inlines-only",
			richdoc.New().P(richdoc.Txt("before "), richdoc.Mark("here", richdoc.Txt("target")), richdoc.Txt(" after")).Doc(),
			"before target after\n", false,
		},
		{
			"table-ragged-and-wide-align",
			// Header has 1 column, a row has 3 cells, alignment has 2 entries:
			// the width is the maximum of the three.
			richdoc.New().Table(
				[]richdoc.Alignment{richdoc.AlignLeft, richdoc.AlignRight},
				[]richdoc.Cell{richdoc.Td(richdoc.Txt("h"))},
				[][]richdoc.Cell{{richdoc.Td(richdoc.Txt("1")), richdoc.Td(richdoc.Txt("2")), richdoc.Td(richdoc.Txt("3"))}},
			).Doc(),
			"| h |  |  |\n| :--- | ---: | --- |\n| 1 | 2 | 3 |\n", false,
		},
		{
			// A soft line break (Parse preserves it as a literal '\n' in
			// Text.Value) must not survive into an ATX heading line, which is
			// a single physical line: an embedded newline would split "Bar"
			// out of the heading into its own following paragraph on re-parse.
			"heading-with-embedded-soft-break",
			richdoc.New().Add(richdoc.Heading{Level: 2, Inlines: []richdoc.Inline{richdoc.Txt("Foo\nBar")}}).Doc(),
			"## Foo Bar\n", false,
		},
		{
			// Same hazard in a paragraph: an embedded newline right before a
			// "---"/"===" line re-parses as an accidental Setext heading.
			"paragraph-with-embedded-soft-break-before-dashes",
			richdoc.New().P(richdoc.Txt("Foo\n---")).Doc(),
			"Foo ---\n", false,
		},
		{
			// An empty fenced code block must stay empty: unconditionally
			// appending a trailing newline to an empty body adds a spurious
			// blank content line, which re-parses as one line of content
			// instead of zero.
			"empty-fenced-code-block",
			richdoc.New().Add(richdoc.CodeBlock{}).Doc(),
			"```\n```\n", false,
		},
		{
			// A literal "!" right before what this writer renders as
			// "[text](url)" must be escaped, or it re-parses as an image
			// marker on the next Parse.
			"bang-before-link",
			richdoc.New().P(richdoc.Txt("!"), richdoc.Href("u", "", richdoc.Txt("t"))).Doc(),
			`\![t](u)` + "\n", false,
		},
		{
			// A link destination containing a space or parenthesis can't be
			// written in the bare "(url)" form; it needs the "<url>" form.
			"link-destination-with-space",
			richdoc.New().P(richdoc.Href("my uri", "", richdoc.Txt("t"))).Doc(),
			"[t](<my uri>)\n", false,
		},
		{
			"link-destination-with-paren",
			richdoc.New().P(richdoc.Href("foo(bar)", "", richdoc.Txt("t"))).Doc(),
			"[t](<foo(bar)>)\n", false,
		},
		{
			// A trailing backslash would escape the closing ")" in the bare
			// form, so it forces the "<...>" form too — and the backslash
			// itself then needs escaping inside that form, or it would
			// escape the closing ">" instead.
			"link-destination-with-trailing-backslash",
			richdoc.New().P(richdoc.Href(`foo\`, "", richdoc.Txt("t"))).Doc(),
			`[t](<foo\\>)` + "\n", false,
		},
		{
			// A title containing a double quote must be escaped, or it
			// closes the title early.
			"link-title-with-quote",
			richdoc.New().P(richdoc.Href("u", `say "hi"`, richdoc.Txt("t"))).Doc(),
			`[t](u "say \"hi\"")` + "\n", false,
		},
		{
			// Code.Value already reflects CommonMark's own single-space
			// padding strip (goldmark's parser trims exactly one leading and
			// one trailing space/newline whenever both are present and the
			// content isn't blank). Writing it back with a bare fence would
			// have it stripped a SECOND time on re-parse; the extra padding
			// survives exactly one such strip and reproduces the value.
			"code-span-with-space-padding",
			richdoc.New().P(richdoc.Mono(" `` ")).Doc(),
			"```  ``  ```\n", false,
		},
		{
			// All-blank content is never affected by CommonMark's padding
			// strip (goldmark's own parser guards the whole rule behind
			// "not entirely blank"), so it needs no extra padding either.
			"code-span-all-blank",
			richdoc.New().P(richdoc.Mono("  ")).Doc(),
			"`  `\n", false,
		},
		{
			"code-span-empty",
			richdoc.New().P(richdoc.Mono("")).Doc(),
			"``\n", false,
		},
		{
			// Nested single emphasis using the same "*" marker this writer
			// always prefers would merge into "**" (strong) at the shared
			// boundary; the inner emphasis must fall back to "_".
			"nested-emphasis-same-marker",
			richdoc.New().P(richdoc.Italic(richdoc.Italic(richdoc.Txt("foo")))).Doc(),
			"_*foo*_\n", false,
		},
		{
			// Emphasis wrapping Strong is the opposite case: "*" next to a
			// leading "**" forms "***", CommonMark's own idiom for
			// strong-wrapping-emphasis, and must NOT fall back to "_" —
			// unlike the same-length collision above, the run lengths here
			// differ (1 vs. 2) and combine correctly on their own.
			"emphasis-wrapping-strong",
			richdoc.New().P(richdoc.Italic(richdoc.Bold(richdoc.Txt("bar")))).Doc(),
			"***bar***\n", false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Write(tc.doc)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			got := string(out)
			if tc.wantSub {
				if !strings.Contains(got, tc.want) {
					t.Errorf("Write output %q does not contain %q", got, tc.want)
				}
				return
			}
			if got != tc.want {
				t.Errorf("Write =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

// TestWriteEmpty checks that nil and empty documents render to empty output.
func TestWriteEmpty(t *testing.T) {
	for _, d := range []*richdoc.Document{nil, {}, richdoc.New().Doc()} {
		out, err := Write(d)
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
		if len(out) != 0 {
			t.Errorf("Write(%#v) = %q, want empty", d, out)
		}
	}
}

// TestConvertAlignsNil covers the nil-slice path, which goldmark never takes
// (its Table.Alignments is always allocated) but which the mapper guards.
func TestConvertAlignsNil(t *testing.T) {
	if got := convertAligns(nil); got != nil {
		t.Errorf("convertAligns(nil) = %v, want nil", got)
	}
}

// TestConvertInlineString covers the ast.String branch, which core CommonMark
// parsing does not exercise but extensions can.
func TestConvertInlineString(t *testing.T) {
	c := &converter{src: []byte("lit")}
	got := c.convertInline(gast.NewString([]byte("lit")))
	want := []richdoc.Inline{richdoc.Text{Value: "lit"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("convertInline(String) = %#v, want %#v", got, want)
	}
}

// TestConvertInlineUnknown covers the fallthrough for an inline node type the
// mapper does not represent (here a table cell node passed in inline position).
func TestConvertInlineUnknown(t *testing.T) {
	c := &converter{}
	if got := c.convertInline(extast.NewTableCell()); got != nil {
		t.Errorf("convertInline(unknown) = %#v, want nil", got)
	}
}

// TestItoa covers the integer formatter, including the zero case that ordered
// lists (whose start is clamped to >= 1) never reach.
func TestItoa(t *testing.T) {
	for n, want := range map[int]string{0: "0", 1: "1", 7: "7", 42: "42", 100: "100"} {
		if got := itoa(n); got != want {
			t.Errorf("itoa(%d) = %q, want %q", n, got, want)
		}
	}
}
