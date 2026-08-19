// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import (
	"github.com/go-richdoc/richdoc"
	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// md is the shared goldmark instance: CommonMark plus the GFM table and
// strikethrough extensions. It is safe for concurrent use.
var md = goldmark.New(
	goldmark.WithExtensions(
		extension.Table,
		extension.Strikethrough,
	),
)

// Parse converts CommonMark source (with GFM tables and strikethrough) into a
// [richdoc.Document]. goldmark's parser never reports an error, so the
// returned error is always nil; it is kept in the signature for symmetry with
// [Write] and for forward compatibility.
func Parse(src []byte) (*richdoc.Document, error) {
	root := md.Parser().Parse(text.NewReader(src))
	return &richdoc.Document{Blocks: convertFlow(root, src)}, nil
}

// convertFlow converts every block-level child of n into richdoc blocks,
// dropping children that carry no representable content (for example link
// reference definitions).
func convertFlow(n gast.Node, src []byte) []richdoc.Block {
	var blocks []richdoc.Block
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if b := convertBlock(c, src); b != nil {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// convertBlock maps a single goldmark block node to a richdoc block, or nil
// when the node has no richdoc representation (link reference definitions,
// which goldmark keeps in the tree but renders invisibly).
func convertBlock(n gast.Node, src []byte) richdoc.Block {
	switch b := n.(type) {
	case *gast.Heading:
		return richdoc.Heading{Level: b.Level, Inlines: convertInlines(b, src)}
	case *gast.Paragraph:
		return richdoc.Paragraph{Inlines: convertInlines(b, src)}
	case *gast.TextBlock:
		// TextBlock is goldmark's paragraph-without-spacing, used inside tight
		// list items and table cells. It maps onto a plain paragraph.
		return richdoc.Paragraph{Inlines: convertInlines(b, src)}
	case *gast.FencedCodeBlock:
		return richdoc.CodeBlock{Language: string(b.Language(src)), Text: string(b.Text(src))}
	case *gast.CodeBlock:
		return richdoc.CodeBlock{Text: string(b.Text(src))}
	case *gast.Blockquote:
		return richdoc.BlockQuote{Blocks: convertFlow(b, src)}
	case *gast.List:
		return convertList(b, src)
	case *gast.ThematicBreak:
		return richdoc.ThematicBreak{}
	case *gast.HTMLBlock:
		return richdoc.RawBlock{Format: "html", Text: string(b.Text(src))}
	case *extast.Table:
		return convertTable(b, src)
	}
	return nil
}

// convertList maps a goldmark list, preserving ordered/tight state and the
// starting number of ordered lists.
func convertList(n *gast.List, src []byte) richdoc.List {
	l := richdoc.List{Ordered: n.IsOrdered(), Start: n.Start, Tight: n.IsTight}
	if l.Start < 1 {
		l.Start = 1
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if item, ok := c.(*gast.ListItem); ok {
			l.Items = append(l.Items, richdoc.ListItem{Blocks: convertFlow(item, src)})
		}
	}
	return l
}

// convertTable maps a GFM table, its header row and body rows, and per-column
// alignments.
func convertTable(n *extast.Table, src []byte) richdoc.Table {
	t := richdoc.Table{Align: convertAligns(n.Alignments)}
	for row := n.FirstChild(); row != nil; row = row.NextSibling() {
		switch row.(type) {
		case *extast.TableHeader:
			t.Header = convertRow(row, src)
		case *extast.TableRow:
			t.Rows = append(t.Rows, convertRow(row, src))
		}
	}
	return t
}

// convertRow converts the cells of a single table row.
func convertRow(row gast.Node, src []byte) []richdoc.Cell {
	var cells []richdoc.Cell
	for c := row.FirstChild(); c != nil; c = c.NextSibling() {
		if cell, ok := c.(*extast.TableCell); ok {
			cells = append(cells, richdoc.Cell{Inlines: convertInlines(cell, src)})
		}
	}
	return cells
}

// convertAligns maps goldmark's column alignments onto richdoc's.
func convertAligns(in []extast.Alignment) []richdoc.Alignment {
	if in == nil {
		return nil
	}
	out := make([]richdoc.Alignment, len(in))
	for i, a := range in {
		switch a {
		case extast.AlignLeft:
			out[i] = richdoc.AlignLeft
		case extast.AlignRight:
			out[i] = richdoc.AlignRight
		case extast.AlignCenter:
			out[i] = richdoc.AlignCenter
		default:
			out[i] = richdoc.AlignDefault
		}
	}
	return out
}

// convertInlines converts every inline-level child of n, coalescing adjacent
// text runs so that goldmark's tokenisation (which may split literal text at,
// for example, an unmatched backtick) does not leak into the model and unsettle
// the round-trip.
func convertInlines(n gast.Node, src []byte) []richdoc.Inline {
	var out []richdoc.Inline
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		for _, in := range convertInline(c, src) {
			if t, ok := in.(richdoc.Text); ok && len(out) > 0 {
				if prev, ok := out[len(out)-1].(richdoc.Text); ok {
					out[len(out)-1] = richdoc.Text{Value: prev.Value + t.Value}
					continue
				}
			}
			out = append(out, in)
		}
	}
	return out
}

// convertInline maps a single goldmark inline node to zero or more richdoc
// inlines. A text node may yield a trailing [richdoc.LineBreak] for a hard
// break; an unrepresentable node yields nothing.
func convertInline(n gast.Node, src []byte) []richdoc.Inline {
	switch i := n.(type) {
	case *gast.Text:
		val := decodeText(i.Segment.Value(src))
		if i.SoftLineBreak() {
			val += "\n"
		}
		var out []richdoc.Inline
		if val != "" {
			out = append(out, richdoc.Text{Value: val})
		}
		if i.HardLineBreak() {
			out = append(out, richdoc.LineBreak{})
		}
		return out
	case *gast.String:
		return []richdoc.Inline{richdoc.Text{Value: string(i.Value)}}
	case *gast.CodeSpan:
		return []richdoc.Inline{richdoc.Code{Value: string(i.Text(src))}}
	case *gast.Emphasis:
		if i.Level == 2 {
			return []richdoc.Inline{richdoc.Strong{Inlines: convertInlines(i, src)}}
		}
		return []richdoc.Inline{richdoc.Emph{Inlines: convertInlines(i, src)}}
	case *extast.Strikethrough:
		return []richdoc.Inline{richdoc.Strikethrough{Inlines: convertInlines(i, src)}}
	case *gast.Link:
		return []richdoc.Inline{richdoc.Link{
			URL:     string(i.Destination),
			Title:   string(i.Title),
			Inlines: convertInlines(i, src),
		}}
	case *gast.Image:
		return []richdoc.Inline{richdoc.Image{
			URL:   string(i.Destination),
			Alt:   decodeText(i.Text(src)),
			Title: string(i.Title),
		}}
	case *gast.AutoLink:
		url := string(i.URL(src))
		return []richdoc.Inline{richdoc.Link{
			URL:     url,
			Inlines: []richdoc.Inline{richdoc.Text{Value: string(i.Label(src))}},
		}}
	case *gast.RawHTML:
		return []richdoc.Inline{richdoc.RawInline{Format: "html", Text: string(i.Segments.Value(src))}}
	}
	return nil
}

// decodeText turns a raw Markdown text segment into its literal string,
// resolving character references and removing backslash escapes, matching what
// goldmark's HTML renderer would emit (minus HTML escaping). richdoc holds
// neutral literal text, so [Write] re-adds the escapes when it emits Markdown.
func decodeText(raw []byte) string {
	decoded := util.ResolveNumericReferences(raw)
	decoded = util.ResolveEntityNames(decoded)
	decoded = util.UnescapePunctuations(decoded)
	return string(decoded)
}
