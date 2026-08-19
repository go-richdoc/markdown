// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import (
	"strings"

	"github.com/go-richdoc/richdoc"
)

// Write renders a [richdoc.Document] to clean CommonMark (with GFM tables and
// strikethrough). The error return is always nil; it is kept for symmetry with
// [Parse]. A nil document renders to empty output.
func Write(d *richdoc.Document) ([]byte, error) {
	if d == nil || len(d.Blocks) == 0 {
		return []byte{}, nil
	}
	out := renderBlocks(d.Blocks)
	return []byte(out + "\n"), nil
}

// renderBlocks renders a sequence of blocks separated by a blank line, with no
// trailing newline.
func renderBlocks(blocks []richdoc.Block) string {
	return renderBlocksSep(blocks, "\n\n")
}

// renderBlocksSep renders a sequence of blocks joined by sep. A tight list
// passes a single newline so that a nested block (for example a sub-list) does
// not introduce the blank line that would make the enclosing list loose.
func renderBlocksSep(blocks []richdoc.Block, sep string) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, renderBlock(b))
	}
	return strings.Join(parts, sep)
}

// renderBlock renders a single block to CommonMark without a trailing newline.
func renderBlock(b richdoc.Block) string {
	switch n := b.(type) {
	case richdoc.Heading:
		level := n.Level
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		return strings.Repeat("#", level) + " " + renderInlines(n.Inlines)
	case richdoc.Paragraph:
		return renderInlines(n.Inlines)
	case richdoc.CodeBlock:
		return renderCodeBlock(n)
	case richdoc.BlockQuote:
		return prefixLines(renderBlocks(n.Blocks), "> ", ">")
	case richdoc.List:
		return renderList(n)
	case richdoc.Table:
		return renderTable(n)
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
func renderList(l richdoc.List) string {
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
		items = append(items, indentItem(renderBlocksSep(it.Blocks, blockSep), marker))
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
func renderTable(t richdoc.Table) string {
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
	rows = append(rows, renderTableRow(t.Header, cols))
	rows = append(rows, renderDelimiterRow(t.Align, cols))
	for _, row := range t.Rows {
		rows = append(rows, renderTableRow(row, cols))
	}
	return strings.Join(rows, "\n")
}

// renderTableRow renders one row padded to cols cells.
func renderTableRow(cells []richdoc.Cell, cols int) string {
	var sb strings.Builder
	sb.WriteString("|")
	for c := 0; c < cols; c++ {
		var content string
		if c < len(cells) {
			content = renderTableCell(cells[c].Inlines)
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
func renderTableCell(inlines []richdoc.Inline) string {
	s := renderInlines(inlines)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}

// renderInlines renders a sequence of inline nodes.
func renderInlines(inlines []richdoc.Inline) string {
	var sb strings.Builder
	for _, in := range inlines {
		sb.WriteString(renderInline(in))
	}
	return sb.String()
}

// renderInline renders a single inline node.
func renderInline(in richdoc.Inline) string {
	switch n := in.(type) {
	case richdoc.Text:
		return escapeText(n.Value)
	case richdoc.Emph:
		return "*" + renderInlines(n.Inlines) + "*"
	case richdoc.Strong:
		return "**" + renderInlines(n.Inlines) + "**"
	case richdoc.Strikethrough:
		return "~~" + renderInlines(n.Inlines) + "~~"
	case richdoc.Code:
		return renderCode(n.Value)
	case richdoc.Link:
		return "[" + renderInlines(n.Inlines) + "](" + n.URL + titleSuffix(n.Title) + ")"
	case richdoc.Image:
		return "![" + escapeText(n.Alt) + "](" + n.URL + titleSuffix(n.Title) + ")"
	case richdoc.Math:
		return "$" + n.TeX + "$"
	case richdoc.RawInline:
		return n.Text
	default:
		// richdoc.Inline is a closed interface, so the only remaining variant
		// is LineBreak, a hard break rendered as backslash-newline.
		_ = n
		return "\\\n"
	}
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
