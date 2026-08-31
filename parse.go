// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import (
	"github.com/go-richdoc/richdoc"
	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// md is the shared goldmark instance: CommonMark plus the GFM table and
// strikethrough extensions, the PHP-Markdown-Extra footnote extension
// (`[^id]` references with `[^id]: …` definitions) and explicit heading
// attributes (`## Title {#id}`). It is safe for concurrent use.
//
// Only WithHeadingAttribute is enabled, not WithAutoHeadingID: the former reads
// back the ids an author wrote and leaves every other heading anchorless, which
// round-trips faithfully; auto ids would fabricate an anchor on every heading
// and Write would then stamp a `{#id}` the source never had.
var md = goldmark.New(
	goldmark.WithExtensions(
		extension.Table,
		extension.Strikethrough,
		extension.Footnote,
	),
	goldmark.WithParserOptions(
		parser.WithHeadingAttribute(),
	),
)

// Parse converts CommonMark source (with GFM tables and strikethrough, PHP
// Markdown Extra footnotes and explicit heading anchors) into a
// [richdoc.Document]. goldmark's parser never reports an error, so the returned
// error is always nil; it is kept in the signature for symmetry with [Write]
// and for forward compatibility.
func Parse(src []byte) (*richdoc.Document, error) {
	root := md.Parser().Parse(text.NewReader(src))
	c := &converter{src: src, notes: collectFootnotes(root)}
	return &richdoc.Document{Blocks: c.convertFlow(root)}, nil
}

// converter carries the parse-wide state (the source bytes and the resolved
// footnote definitions) through the recursive conversion.
type converter struct {
	src   []byte
	notes map[int]*extast.Footnote
}

// collectFootnotes indexes the footnote definitions goldmark gathers into the
// trailing [extast.FootnoteList] by their resolved Index, so a [^id] reference
// (an [extast.FootnoteLink], which carries the same Index) can be turned into an
// inline [richdoc.Footnote] holding the note body at the reference site. The map
// is nil when the document has no footnotes.
func collectFootnotes(root gast.Node) map[int]*extast.Footnote {
	var notes map[int]*extast.Footnote
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		list, ok := c.(*extast.FootnoteList)
		if !ok {
			continue
		}
		for f := list.FirstChild(); f != nil; f = f.NextSibling() {
			fn := f.(*extast.Footnote)
			if notes == nil {
				notes = make(map[int]*extast.Footnote)
			}
			notes[fn.Index] = fn
		}
	}
	return notes
}

// convertFlow converts every block-level child of n into richdoc blocks,
// dropping children that carry no representable content (for example link
// reference definitions).
func (c *converter) convertFlow(n gast.Node) []richdoc.Block {
	var blocks []richdoc.Block
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		if b := c.convertBlock(ch); b != nil {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// convertBlock maps a single goldmark block node to a richdoc block, or nil
// when the node has no richdoc representation (link reference definitions,
// which goldmark keeps in the tree but renders invisibly, and the footnote
// list, whose bodies are inlined at their reference sites).
func (c *converter) convertBlock(n gast.Node) richdoc.Block {
	switch b := n.(type) {
	case *gast.Heading:
		h := richdoc.Heading{Level: b.Level, Inlines: c.convertInlines(b)}
		if id, ok := b.AttributeString("id"); ok {
			h.ID = string(id.([]byte))
		}
		return h
	case *gast.Paragraph:
		return richdoc.Paragraph{Inlines: c.convertInlines(b)}
	case *gast.TextBlock:
		// TextBlock is goldmark's paragraph-without-spacing, used inside tight
		// list items and table cells. It maps onto a plain paragraph.
		return richdoc.Paragraph{Inlines: c.convertInlines(b)}
	case *gast.FencedCodeBlock:
		return richdoc.CodeBlock{Language: string(b.Language(c.src)), Text: string(b.Text(c.src))}
	case *gast.CodeBlock:
		return richdoc.CodeBlock{Text: string(b.Text(c.src))}
	case *gast.Blockquote:
		return richdoc.BlockQuote{Blocks: c.convertFlow(b)}
	case *gast.List:
		return c.convertList(b)
	case *gast.ThematicBreak:
		return richdoc.ThematicBreak{}
	case *gast.HTMLBlock:
		return richdoc.RawBlock{Format: "html", Text: string(b.Text(c.src))}
	case *extast.Table:
		return c.convertTable(b)
	case *extast.FootnoteList:
		// The definitions are surfaced inline at each reference; the list node
		// itself, which goldmark appends at document end, carries nothing extra.
		return nil
	}
	return nil
}

// convertList maps a goldmark list, preserving ordered/tight state and the
// starting number of ordered lists.
func (c *converter) convertList(n *gast.List) richdoc.List {
	// n.Start is 0 for an unordered list (meaningless there, and Write never
	// reads it unless Ordered is true) and otherwise CommonMark's own
	// ordinal, which is always >= 0 — "0. ok" is a valid ordered list
	// starting at 0, so it must not be clamped up to 1.
	l := richdoc.List{Ordered: n.IsOrdered(), Start: n.Start, Tight: n.IsTight}
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		if item, ok := ch.(*gast.ListItem); ok {
			l.Items = append(l.Items, richdoc.ListItem{Blocks: c.convertFlow(item)})
		}
	}
	return l
}

// convertTable maps a GFM table, its header row and body rows, and per-column
// alignments.
func (c *converter) convertTable(n *extast.Table) richdoc.Table {
	t := richdoc.Table{Align: convertAligns(n.Alignments)}
	for row := n.FirstChild(); row != nil; row = row.NextSibling() {
		switch row.(type) {
		case *extast.TableHeader:
			t.Header = c.convertRow(row)
		case *extast.TableRow:
			t.Rows = append(t.Rows, c.convertRow(row))
		}
	}
	return t
}

// convertRow converts the cells of a single table row.
func (c *converter) convertRow(row gast.Node) []richdoc.Cell {
	var cells []richdoc.Cell
	for ch := row.FirstChild(); ch != nil; ch = ch.NextSibling() {
		if cell, ok := ch.(*extast.TableCell); ok {
			cells = append(cells, richdoc.Cell{Inlines: c.convertInlines(cell)})
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
func (c *converter) convertInlines(n gast.Node) []richdoc.Inline {
	var out []richdoc.Inline
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		for _, in := range c.convertInline(ch) {
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
// break; an unrepresentable node (a footnote backlink, for example) yields
// nothing.
func (c *converter) convertInline(n gast.Node) []richdoc.Inline {
	switch i := n.(type) {
	case *gast.Text:
		val := decodeText(i.Segment.Value(c.src))
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
		return []richdoc.Inline{richdoc.Code{Value: string(i.Text(c.src))}}
	case *gast.Emphasis:
		if i.Level == 2 {
			return []richdoc.Inline{richdoc.Strong{Inlines: c.convertInlines(i)}}
		}
		return []richdoc.Inline{richdoc.Emph{Inlines: c.convertInlines(i)}}
	case *extast.Strikethrough:
		return []richdoc.Inline{richdoc.Strikethrough{Inlines: c.convertInlines(i)}}
	case *gast.Link:
		if target, ok := internalTarget(i); ok {
			return []richdoc.Inline{richdoc.CrossRef{
				Target:  target,
				Kind:    richdoc.RefLabel,
				Inlines: c.convertInlines(i),
			}}
		}
		return []richdoc.Inline{richdoc.Link{
			// goldmark's Destination/Title are the RAW source bytes (see
			// parseLinkDestination in its own parser/link.go): a
			// backslash-escaped or entity-encoded destination or title is
			// only resolved by its HTML renderer at output time
			// (util.URLEscape's resolveReference step, and defaultWriter.Write
			// for the title), never by the parser itself. decodeText applies
			// that same resolution so richdoc stores the semantic value, not
			// source syntax — matching every other inline conversion here.
			URL:     decodeText(i.Destination),
			Title:   decodeText(i.Title),
			Inlines: c.convertInlines(i),
		}}
	case *gast.Image:
		return []richdoc.Inline{richdoc.Image{
			URL:   decodeText(i.Destination),
			Alt:   decodeText(i.Text(c.src)),
			Title: decodeText(i.Title),
		}}
	case *gast.AutoLink:
		url := string(i.URL(c.src))
		if i.AutoLinkType == gast.AutoLinkEmail {
			// AutoLink.URL only prepends a scheme when the node carries an
			// explicit Protocol (a "<scheme:...>" autolink); an email
			// autolink ("<foo@bar.example>") has none, and goldmark's own
			// HTML renderer adds "mailto:" separately at render time — so it
			// has to be added here too, or the link is unreachable.
			url = "mailto:" + url
		}
		return []richdoc.Inline{richdoc.Link{
			URL:     url,
			Inlines: []richdoc.Inline{richdoc.Text{Value: string(i.Label(c.src))}},
		}}
	case *gast.RawHTML:
		return []richdoc.Inline{richdoc.RawInline{Format: "html", Text: string(i.Segments.Value(c.src))}}
	case *extast.FootnoteLink:
		// goldmark only emits a FootnoteLink when a matching definition exists,
		// so the lookup always resolves; the body is inlined here.
		return []richdoc.Inline{richdoc.Footnote{Blocks: c.convertFlow(c.notes[i.Index])}}
	}
	return nil
}

// internalTarget reports whether an internal Markdown link (a clean `#fragment`
// destination with no title) should map to a [richdoc.CrossRef] of kind
// [richdoc.RefLabel] rather than a plain [richdoc.Link], returning the bare
// fragment when it does. Anything else — an external URL, a bare `#`, a title,
// an unclean fragment — stays a Link so the mapping never over-reaches.
func internalTarget(l *gast.Link) (string, bool) {
	if len(l.Title) != 0 {
		return "", false
	}
	dest := l.Destination
	if len(dest) < 2 || dest[0] != '#' {
		return "", false
	}
	frag := dest[1:]
	if !cleanFragment(frag) {
		return "", false
	}
	return string(frag), true
}

// cleanFragment reports whether frag is a link fragment simple enough to write
// back verbatim as `](#frag)` and re-parse identically: no whitespace and none
// of the characters that would break the link destination or fragment.
func cleanFragment(frag []byte) bool {
	for _, b := range frag {
		switch b {
		case ' ', '\t', '\n', '\r', '(', ')', '<', '>', '#', '\\', '"':
			return false
		}
	}
	return true
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
