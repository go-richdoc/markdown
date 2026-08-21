// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import (
	"strings"

	"github.com/go-richdoc/richdoc"
)

// Write renders a [richdoc.Document] to clean CommonMark (with GFM tables and
// strikethrough, PHP Markdown Extra footnotes and explicit heading anchors).
// The error return is always nil; it is kept for symmetry with [Parse]. A nil
// document renders to empty output.
//
// Footnotes are collected while the body renders and their definitions are
// emitted, numbered in reference order, after every block.
func Write(d *richdoc.Document) ([]byte, error) {
	if d == nil || len(d.Blocks) == 0 {
		return []byte{}, nil
	}
	w := &writer{}
	body := w.renderBlocks(d.Blocks)
	if defs := w.renderFootnoteDefs(); defs != "" {
		body += "\n\n" + defs
	}
	return []byte(body + "\n"), nil
}

// writer holds the render-wide footnote accumulator. A footnote reference
// appends its body here and emits a `[^n]` marker; the bodies are rendered as
// `[^n]: …` definitions once the document body is complete.
type writer struct {
	footnotes []richdoc.Footnote
}

// renderFootnoteDefs renders the accumulated footnote definitions. Rendering a
// body may itself reference further footnotes, which append to the slice; the
// index-based loop picks them up so their numbers stay in reference order.
func (w *writer) renderFootnoteDefs() string {
	if len(w.footnotes) == 0 {
		return ""
	}
	var parts []string
	for i := 0; i < len(w.footnotes); i++ {
		body := w.renderBlocks(w.footnotes[i].Blocks)
		parts = append(parts, footnoteDef(i+1, body))
	}
	return strings.Join(parts, "\n\n")
}

// footnoteDef formats one `[^n]: …` definition, prefixing the first body line
// with the label and indenting every continuation line by four spaces so
// goldmark reads it back as the note's body.
func footnoteDef(n int, body string) string {
	marker := "[^" + itoa(n) + "]: "
	lines := strings.Split(body, "\n")
	var sb strings.Builder
	for i, ln := range lines {
		if i == 0 {
			sb.WriteString(marker)
			sb.WriteString(ln)
			continue
		}
		sb.WriteByte('\n')
		if ln != "" {
			sb.WriteString("    ")
			sb.WriteString(ln)
		}
	}
	return sb.String()
}

// renderBlocks renders a sequence of blocks separated by a blank line, with no
// trailing newline.
func (w *writer) renderBlocks(blocks []richdoc.Block) string {
	return w.renderBlocksSep(blocks, "\n\n")
}

// renderBlocksSep renders a sequence of blocks joined by sep. A tight list
// passes a single newline so that a nested block (for example a sub-list) does
// not introduce the blank line that would make the enclosing list loose.
func (w *writer) renderBlocksSep(blocks []richdoc.Block, sep string) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, w.renderBlock(b))
	}
	return strings.Join(parts, sep)
}

// renderBlock renders a single block to CommonMark without a trailing newline.
func (w *writer) renderBlock(b richdoc.Block) string {
	switch n := b.(type) {
	case richdoc.Heading:
		level := n.Level
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		s := strings.Repeat("#", level) + " " + w.renderInlines(n.Inlines)
		if n.ID != "" {
			s += " {#" + n.ID + "}"
		}
		return s
	case richdoc.Paragraph:
		return w.renderInlines(n.Inlines)
	case richdoc.CodeBlock:
		return renderCodeBlock(n)
	case richdoc.BlockQuote:
		return prefixLines(w.renderBlocks(n.Blocks), "> ", ">")
	case richdoc.List:
		return w.renderList(n)
	case richdoc.Table:
		return w.renderTable(n)
	case richdoc.MathBlock:
		return "$$\n" + n.TeX + "\n$$"
	case richdoc.RawBlock:
		// Raw blocks pass their verbatim text through unchanged; "html" is the
		// common case, and any other format degrades to its best-effort text.
		return n.Text
	default:
		// richdoc.Block is a closed interface, so the only remaining variant
		// is ThematicBreak, which carries no data.
		_ = n
		return "---"
	}
}

// renderCodeBlock emits a fenced code block, choosing a backtick fence long
// enough to contain any backtick run in the body.
func renderCodeBlock(n richdoc.CodeBlock) string {
	fence := strings.Repeat("`", longestBacktickRun(n.Text)+1)
	if len(fence) < 3 {
		fence = "```"
	}
	body := n.Text
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return fence + n.Language + "\n" + body + fence
}

// renderList renders an ordered or unordered list, honouring tightness.
func (w *writer) renderList(l richdoc.List) string {
	blockSep := "\n\n"
	sep := "\n\n"
	if l.Tight {
		blockSep = "\n"
		sep = "\n"
	}
	items := make([]string, 0, len(l.Items))
	for i, it := range l.Items {
		marker := "- "
		if l.Ordered {
			marker = itoa(l.Start+i) + ". "
		}
		items = append(items, indentItem(w.renderBlocksSep(it.Blocks, blockSep), marker))
	}
	return strings.Join(items, sep)
}

// indentItem prefixes the first line of content with marker and every
// following line with matching spaces, so nested blocks stay inside the item.
func indentItem(content, marker string) string {
	pad := strings.Repeat(" ", len(marker))
	lines := strings.Split(content, "\n")
	var sb strings.Builder
	for i, ln := range lines {
		if i > 0 {
			sb.WriteByte('\n')
			if ln != "" {
				sb.WriteString(pad)
			}
		} else {
			sb.WriteString(marker)
		}
		sb.WriteString(ln)
	}
	return sb.String()
}

// prefixLines prefixes each line of s: non-empty lines with prefix, empty lines
// with emptyPrefix (a bare quote marker keeps blockquotes contiguous).
func prefixLines(s, prefix, emptyPrefix string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if ln == "" {
			lines[i] = emptyPrefix
		} else {
			lines[i] = prefix + ln
		}
	}
	return strings.Join(lines, "\n")
}

// renderTable emits a GFM pipe table with an alignment delimiter row.
func (w *writer) renderTable(t richdoc.Table) string {
	cols := len(t.Header)
	for _, row := range t.Rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if len(t.Align) > cols {
		cols = len(t.Align)
	}
	var rows []string
	rows = append(rows, w.renderTableRow(t.Header, cols))
	rows = append(rows, renderDelimiterRow(t.Align, cols))
	for _, row := range t.Rows {
		rows = append(rows, w.renderTableRow(row, cols))
	}
	return strings.Join(rows, "\n")
}

// renderTableRow renders one row padded to cols cells.
func (w *writer) renderTableRow(cells []richdoc.Cell, cols int) string {
	var sb strings.Builder
	sb.WriteString("|")
	for c := 0; c < cols; c++ {
		var content string
		if c < len(cells) {
			content = w.renderTableCell(cells[c].Inlines)
		}
		sb.WriteString(" ")
		sb.WriteString(content)
		sb.WriteString(" |")
	}
	return sb.String()
}

// renderDelimiterRow renders the header delimiter row encoding column
// alignments.
func renderDelimiterRow(align []richdoc.Alignment, cols int) string {
	var sb strings.Builder
	sb.WriteString("|")
	for c := 0; c < cols; c++ {
		a := richdoc.AlignDefault
		if c < len(align) {
			a = align[c]
		}
		switch a {
		case richdoc.AlignLeft:
			sb.WriteString(" :--- |")
		case richdoc.AlignCenter:
			sb.WriteString(" :---: |")
		case richdoc.AlignRight:
			sb.WriteString(" ---: |")
		default:
			sb.WriteString(" --- |")
		}
	}
	return sb.String()
}

// renderTableCell renders inline cell content, flattening newlines and
// escaping pipes so the cell stays on one table row.
func (w *writer) renderTableCell(inlines []richdoc.Inline) string {
	s := w.renderInlines(inlines)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}

// renderInlines renders a sequence of inline nodes.
func (w *writer) renderInlines(inlines []richdoc.Inline) string {
	var sb strings.Builder
	for _, in := range inlines {
		sb.WriteString(w.renderInline(in))
	}
	return sb.String()
}

// renderInline renders a single inline node.
func (w *writer) renderInline(in richdoc.Inline) string {
	switch n := in.(type) {
	case richdoc.Text:
		return escapeText(n.Value)
	case richdoc.Emph:
		return "*" + w.renderInlines(n.Inlines) + "*"
	case richdoc.Strong:
		return "**" + w.renderInlines(n.Inlines) + "**"
	case richdoc.Strikethrough:
		return "~~" + w.renderInlines(n.Inlines) + "~~"
	case richdoc.Code:
		return renderCode(n.Value)
	case richdoc.Link:
		return "[" + w.renderInlines(n.Inlines) + "](" + n.URL + titleSuffix(n.Title) + ")"
	case richdoc.Image:
		return "![" + escapeText(n.Alt) + "](" + n.URL + titleSuffix(n.Title) + ")"
	case richdoc.Math:
		return "$" + n.TeX + "$"
	case richdoc.RawInline:
		return n.Text
	case richdoc.Footnote:
		// Emit the reference now and defer the body to the definition list;
		// numbering follows the order references are rendered.
		w.footnotes = append(w.footnotes, n)
		return "[^" + itoa(len(w.footnotes)) + "]"
	case richdoc.CrossRef:
		return w.renderCrossRef(n)
	case richdoc.Anchor:
		// CommonMark has no anchor syntax; render only the marked text. Parse
		// never produces an Anchor, so this is a write-only degradation that
		// drops the id.
		return w.renderInlines(n.Inlines)
	default:
		// richdoc.Inline is a closed interface; the only remaining variant is
		// LineBreak, a hard break rendered as backslash-newline.
		_ = n
		return "\\\n"
	}
}

// renderCrossRef renders a cross-reference. A label reference round-trips as an
// internal link `[text](#target)`; a citation, which CommonMark cannot express,
// degrades to the pandoc `[@key]` form. When a label reference carries no
// visible text the target stands in for it.
func (w *writer) renderCrossRef(n richdoc.CrossRef) string {
	if n.Kind == richdoc.RefCite {
		return "[@" + n.Target + "]"
	}
	text := w.renderInlines(n.Inlines)
	if text == "" {
		text = escapeText(n.Target)
	}
	return "[" + text + "](#" + n.Target + ")"
}

// renderCode renders an inline code span, widening the backtick fence and
// padding with spaces when the content itself contains backticks.
func renderCode(value string) string {
	fence := strings.Repeat("`", longestBacktickRun(value)+1)
	if strings.HasPrefix(value, "`") || strings.HasSuffix(value, "`") {
		return fence + " " + value + " " + fence
	}
	return fence + value + fence
}

// titleSuffix formats an optional link/image title.
func titleSuffix(title string) string {
	if title == "" {
		return ""
	}
	return " \"" + title + "\""
}

// escapeText backslash-escapes the ASCII punctuation that would otherwise be
// interpreted as inline Markdown, keeping ordinary prose clean.
func escapeText(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\', '`', '*', '_', '[', ']', '<', '~':
			sb.WriteByte('\\')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// longestBacktickRun returns the length of the longest run of consecutive
// backticks in s.
func longestBacktickRun(s string) int {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	return longest
}

// itoa formats a non-negative int without importing strconv for a single use.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
