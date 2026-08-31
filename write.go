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
//
// Two ADJACENT List blocks that are both ordered or both unordered only exist
// side by side in the model because Parse's source split them into separate
// lists (a bullet-character or delimiter change, per CommonMark's own list
// grammar) — richdoc.List otherwise has no way to record that split, since a
// single loose list with a blank line between items stays one List node.
// Writing both with this writer's one default marker ("-", or "N. ") would
// undo exactly that split: goldmark re-parses adjacent same-marker lists as
// one merged list on the next Parse, corrupting both list count and
// tightness. alternateListMarker tracks, across this one sibling sequence,
// whether the immediately preceding block was a List of the same Ordered
// kind and which marker variant it used, so this List can be written with
// the OTHER variant ("+", or "N) ") and stay visibly separate — mirroring
// wrapDelimited's marker-alternation precedent for the analogous emphasis-
// merge problem. Recursing into each container's own block sequence (list
// items, block quotes) via a fresh call keeps the tracking scoped to true
// siblings, so nested lists at any depth get the same protection.
func (w *writer) renderBlocksSep(blocks []richdoc.Block, sep string) string {
	parts := make([]string, 0, len(blocks))
	prevWasList := false
	var prevOrdered, prevAlt bool
	for _, b := range blocks {
		l, isList := b.(richdoc.List)
		alt := isList && prevWasList && prevOrdered == l.Ordered && !prevAlt
		parts = append(parts, w.renderBlock(b, alt))
		prevWasList = isList
		if isList {
			prevOrdered, prevAlt = l.Ordered, alt
		}
	}
	return strings.Join(parts, sep)
}

// renderBlock renders a single block to CommonMark without a trailing
// newline. altListMarker is only consulted when b is a richdoc.List; see
// renderBlocksSep for why an adjacent same-kind list needs it.
func (w *writer) renderBlock(b richdoc.Block, altListMarker bool) string {
	switch n := b.(type) {
	case richdoc.Heading:
		level := n.Level
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		s := strings.Repeat("#", level) + " " + headingTrailingHashEscape(w.renderInlines(n.Inlines))
		if n.ID != "" {
			s += " {#" + n.ID + "}"
		}
		return s
	case richdoc.Paragraph:
		return escapeLeadingMarker(w.renderInlines(n.Inlines))
	case richdoc.CodeBlock:
		return renderCodeBlock(n)
	case richdoc.BlockQuote:
		return prefixLines(w.renderBlocks(n.Blocks), "> ", ">")
	case richdoc.List:
		return w.renderList(n, altListMarker)
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
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return fence + n.Language + "\n" + body + fence
}

// renderList renders an ordered or unordered list, honouring tightness. alt
// selects the alternate marker glyph ("+" for unordered, ")" as the ordered
// delimiter) instead of this writer's default ("-", "."), used when the
// immediately preceding sibling is a List of the same Ordered kind that
// would otherwise re-merge with this one on the next Parse — see
// renderBlocksSep.
func (w *writer) renderList(l richdoc.List, alt bool) string {
	blockSep := "\n\n"
	sep := "\n\n"
	if l.Tight {
		blockSep = "\n"
		sep = "\n"
	}
	bullet, delim := "-", "."
	if alt {
		bullet, delim = "+", ")"
	}
	items := make([]string, 0, len(l.Items))
	for i, it := range l.Items {
		marker := bullet + " "
		if l.Ordered {
			marker = itoa(l.Start+i) + delim + " "
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
		return wrapDelimited(w.renderInlines(n.Inlines), "*", "_")
	case richdoc.Strong:
		return wrapDelimited(w.renderInlines(n.Inlines), "**", "__")
	case richdoc.Strikethrough:
		return "~~" + w.renderInlines(n.Inlines) + "~~"
	case richdoc.Code:
		return renderCode(n.Value)
	case richdoc.Link:
		return "[" + w.renderInlines(n.Inlines) + "](" + linkDestination(n.URL) + titleSuffix(n.Title) + ")"
	case richdoc.Image:
		return "![" + escapeText(n.Alt) + "](" + linkDestination(n.URL) + titleSuffix(n.Title) + ")"
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

// wrapDelimited wraps inner in marker, falling back to altMarker when
// wrapping in marker would let it merge with the SAME marker character
// already at inner's own boundary and change the total run length —
// CommonMark determines strong vs. emphasis purely by consecutive
// delimiter-run length, so "*" wrapped around content starting or ending in
// exactly one unpaired "*" (nested single emphasis, the same marker this
// writer always prefers) silently becomes a run of two, re-parsing as
// strong instead of nested emphasis. It must NOT trigger when the adjacent
// run is already a DIFFERENT length, though — "*" next to a leading "**"
// (an inner Strong) forms "***", CommonMark's own idiom for strong-wrapping-
// emphasis and already correct as-is; forcing "_" there would be wrong, not
// merely unnecessary (verified against the CommonMark spec corpus: an
// unconditional same-character check regressed two strong/emphasis cases
// this exact refinement fixes). Real source avoids the collision by
// alternating "*"/"_" between nesting levels; this writer always emits "*"
// (see the README's own normalisation note) and falls back to "_" only for
// the specific run-length collision this function detects.
func wrapDelimited(inner, marker, altMarker string) string {
	m := marker
	if collidesAtBoundary(inner, marker) {
		m = altMarker
	}
	return m + inner + m
}

// collidesAtBoundary reports whether inner starts or ends with a run of
// marker's character whose length is EXACTLY len(marker) — neither shorter
// (too little to matter) nor longer (already forms a bigger run that
// CommonMark decomposes on its own, e.g. "*" next to a leading "**" from a
// nested Strong makes "***", its own idiom for strong-wrapping-emphasis and
// already correct unmodified). An exact-length match is the one case where
// concatenating marker doubles the run to exactly 2*len(marker), changing
// what it means.
func collidesAtBoundary(inner, marker string) bool {
	n := len(marker)
	hasExactRun := func(fromEnd bool) bool {
		if len(inner) < n {
			return false
		}
		var run string
		if fromEnd {
			run = inner[len(inner)-n:]
		} else {
			run = inner[:n]
		}
		if run != strings.Repeat(marker[:1], n) {
			return false
		}
		if fromEnd {
			return len(inner) == n || inner[len(inner)-n-1] != marker[0]
		}
		return len(inner) == n || inner[n] != marker[0]
	}
	return hasExactRun(false) || hasExactRun(true)
}

// renderCode renders an inline code span, widening the backtick fence and
// padding with spaces when the content itself contains backticks, or would
// otherwise trigger CommonMark's own space-padding-strip rule on re-parse
// (goldmark's parser/code_span.go trims exactly one leading and one trailing
// byte whenever both are a space or newline and the content isn't entirely
// blank) — value already reflects that stripping, having come from richdoc's
// Code.Value, so writing it back bare would have it stripped a SECOND time.
// Padding with one extra space on each side survives exactly one such strip
// and reproduces value unchanged.
func renderCode(value string) string {
	fence := strings.Repeat("`", longestBacktickRun(value)+1)
	if needsCodeSpanPadding(value) {
		return fence + " " + value + " " + fence
	}
	return fence + value + fence
}

func needsCodeSpanPadding(value string) bool {
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "`") || strings.HasSuffix(value, "`") {
		return true
	}
	spaceOrNL := func(b byte) bool { return b == ' ' || b == '\n' }
	if !spaceOrNL(value[0]) || !spaceOrNL(value[len(value)-1]) {
		return false
	}
	return strings.Trim(value, " \n") != ""
}

// titleSuffix formats an optional link/image title, backslash-escaping any
// double quote in it — the title text itself may legitimately contain one
// (`'title "and" title'`), and writing it through unescaped would close the
// title early and break the surrounding link.
func titleSuffix(title string) string {
	if title == "" {
		return ""
	}
	return " \"" + strings.ReplaceAll(title, "\"", "\\\"") + "\""
}

// linkDestination formats a link/image destination, wrapping it in `<...>`
// whenever the bare parenthesized form (`(url)`) can't represent it safely:
// a space or control character ends the destination early, a parenthesis is
// ambiguous with the closing `)`, and a trailing backslash would escape that
// closing `)` in the bare form. Escaping is scoped to what the `<...>` form
// itself requires (`\`, `<`, `>`) so a URL that needs no wrapping round-trips
// unmodified.
func linkDestination(url string) string {
	if !needsAngleBrackets(url) {
		return url
	}
	var sb strings.Builder
	sb.WriteByte('<')
	for _, r := range url {
		switch r {
		case '\\', '<', '>':
			sb.WriteByte('\\')
		}
		sb.WriteRune(r)
	}
	sb.WriteByte('>')
	return sb.String()
}

func needsAngleBrackets(url string) bool {
	for _, r := range url {
		switch r {
		// A backslash anywhere — not just trailing — needs the bracket form:
		// the bare form's own destination scanner (goldmark's
		// parseLinkDestination) treats ANY "\" + ASCII-punct pair as an
		// escape sequence too, so a literal backslash followed by
		// punctuation elsewhere in the URL gets silently stripped by the
		// renderer's resolve step on re-parse just the same as a trailing one.
		case ' ', '\t', '\n', '(', ')', '<', '>', '\\':
			return true
		}
	}
	return false
}

// escapeText backslash-escapes the ASCII punctuation that would otherwise be
// interpreted as inline Markdown, keeping ordinary prose clean.
func escapeText(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\n':
			// A soft line break survives in richdoc.Text.Value as a literal
			// newline (Parse appends it verbatim). Writing it through
			// unchanged corrupts whatever block it lands in — an ATX heading
			// is one physical line, and a paragraph line ending right before
			// it re-parses as an accidental Setext heading whenever what
			// follows is a bare "---"/"===" line. Flattening to a space
			// matches the soft break's own HTML rendering and is the same
			// fix renderTableCell already applies for cells.
			sb.WriteByte(' ')
			continue
		case '\\', '`', '*', '_', '[', ']', '<', '~', '!', '&':
			// '!' needs escaping too: a literal "!" immediately followed by
			// a Link/CrossRef this writer renders as "[text](url)" would
			// otherwise re-parse as an image marker on the next Parse.
			// '&' likewise: richdoc.Text.Value holds the DECODED value (a
			// literal "&ouml;" from a source entity reference decodes to
			// "&ouml;" as plain text, not "ö" — see decodeText), so a
			// literal '&' immediately followed by a run that happens to
			// look like a valid entity/numeric reference re-resolves into
			// that reference's character on the next Parse if written bare.
			sb.WriteByte('\\')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// escapeLeadingMarker backslash-escapes (or, for leading indentation,
// numeric-character-references) exactly the FIRST character of a paragraph's
// fully-rendered content, when — and only when — that character would be
// reinterpreted as different block-level syntax on re-parse.
//
// escapeText is character-class-driven and position-blind: it can express
// "always escape a literal backtick" but not "escape '#' only when it opens
// the line". Block parsing runs on raw bytes, column 0 of a line, BEFORE any
// inline backslash escape is resolved (goldmark's own atx_heading.go,
// list_item.go, blockquote.go and code_block.go all key off the raw byte at
// BlockOffset) — so a decoded entity or a plain Text value that happens to
// start with '#', '-', '+', a digit run, '>', or 4+ columns of leading
// whitespace silently becomes a heading, list item, blockquote or indented
// code block one Parse->Write round trip later. '*', '_', '`' and '~' need
// no extra handling here: escapeText already escapes every occurrence of
// those unconditionally (for inline emphasis/code-span/strikethrough
// safety), which as a side effect already breaks any leading run of them
// that fenced-code or thematic-break syntax would otherwise read.
//
// This same "column 0 of the line" rule reapplies, independently, wherever
// the paragraph ends up nested — inside a blockquote (right after its own
// "> "), inside a list item (right after its own marker) — because both
// recursively re-parse their content as their own mini block-parsing
// context starting at ITS column 0. Rather than thread that context through
// the renderer, this check runs unconditionally on every paragraph; escaping
// a leading character that turns out not to be at true column 0 (because a
// list/blockquote prefix landed in front of it after all) is simply
// unnecessary, never wrong — it still decodes back to the same text.
func escapeLeadingMarker(s string) string {
	if s == "" {
		return s
	}
	if esc, ok := escapeLeadingIndent(s); ok {
		return esc
	}
	switch {
	case s[0] == '#':
		if leadingHashRunOpensHeading(s) {
			return "\\" + s
		}
	case s[0] == '>':
		// A blockquote marker needs nothing after it — even ">foo" opens one.
		return "\\" + s
	case s[0] == '-':
		// Dangerous as a bullet-list marker ("- foo", or bare "-") and,
		// independently, as an all-dashes thematic break ("---").
		if len(s) == 1 || isSpaceOrTab(s[1]) || isThematicBreakDashRun(s) {
			return "\\" + s
		}
	case s[0] == '+':
		if len(s) == 1 || isSpaceOrTab(s[1]) {
			return "\\" + s
		}
	case s[0] >= '0' && s[0] <= '9':
		if esc, ok := escapeOrderedMarker(s); ok {
			return esc
		}
	}
	return s
}

// isSpaceOrTab reports whether b is an ASCII space or tab.
func isSpaceOrTab(b byte) bool {
	return b == ' ' || b == '\t'
}

// leadingHashRunOpensHeading reports whether s starts with a run of 1-6 '#'
// characters immediately followed by a space, a tab, or nothing else —
// exactly the condition goldmark's atx_heading.go uses to open an ATX
// heading (a run of more than 6, or not followed by whitespace/EOL, is left
// alone: it can never be misread as a heading).
func leadingHashRunOpensHeading(s string) bool {
	i := 0
	for i < len(s) && s[i] == '#' {
		i++
	}
	if i > 6 {
		return false
	}
	return i == len(s) || isSpaceOrTab(s[i])
}

// isThematicBreakDashRun reports whether s, taken as a whole line, is a
// thematic break made of dashes and spaces: nothing but '-' and space/tab,
// with more than two dashes — goldmark's thematic_break.go isThematicBreak.
func isThematicBreakDashRun(s string) bool {
	count := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t':
		case '-':
			count++
		default:
			return false
		}
	}
	return count > 2
}

// escapeOrderedMarker reports whether s opens with an ordered-list marker —
// 1-9 digits followed by '.' or ')' and then a space, a tab, or nothing else
// (goldmark's list.go parseListItem; a run of 10+ digits, or a delimiter not
// immediately followed by whitespace/EOL, is never a list marker) — and if
// so returns s with a backslash inserted before that '.'/')' delimiter,
// mirroring the CommonMark spec's own idiom for this exact case ("1\. not a
// list"). Digits themselves cannot be backslash-escaped: CommonMark only
// recognises the escape before ASCII punctuation.
func escapeOrderedMarker(s string) (string, bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i > 9 || i >= len(s) {
		return s, false
	}
	d := s[i]
	if d != '.' && d != ')' {
		return s, false
	}
	rest := s[i+1:]
	if rest == "" || isSpaceOrTab(rest[0]) {
		return s[:i] + "\\" + string(d) + rest, true
	}
	return s, false
}

// escapeLeadingIndent reports whether s opens with 4 or more columns of
// space/tab indentation — goldmark's code_block.go reads that as an indented
// code block, tabs expanding to the next 4-column stop the same way
// util.TabWidth does — and if so returns s with its first whitespace
// character replaced by the equivalent numeric character reference. A
// backslash cannot help here (CommonMark backslash-escapes only ASCII
// punctuation, and space/tab are neither); a character reference decodes
// back to the exact same byte on re-parse while no longer being literal
// column-0 whitespace, which is all the indented-code-block check looks at.
func escapeLeadingIndent(s string) (string, bool) {
	if s[0] != ' ' && s[0] != '\t' {
		return s, false
	}
	width := 0
scan:
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ':
			width++
		case '\t':
			width += 4 - width%4
		default:
			break scan
		}
		if width >= 4 {
			break scan
		}
	}
	if width < 4 {
		return s, false
	}
	if s[0] == '\t' {
		return "&#9;" + s[1:], true
	}
	return "&#32;" + s[1:], true
}

// headingTrailingHashEscape backslash-escapes a trailing run of '#' in a
// heading's rendered content that goldmark's atx_heading.go would otherwise
// strip as a "closing sequence" on re-parse: a run of 1+ '#' at the very end
// of the line, preceded by whitespace, or making up the WHOLE line (which
// empties the heading entirely — "### ###" reads back with no text at all).
// Block-level parsing scans these raw bytes before backslash escapes are
// resolved, so escaping just the first '#' of that run — anywhere within it,
// since the scan walks in from the end and stops at the first non-'#' byte —
// breaks the raw-byte scan while still decoding back to the same literal
// text.
func headingTrailingHashEscape(s string) string {
	n := len(s)
	i := n
	for i > 0 && s[i-1] == '#' {
		i--
	}
	if i == n {
		return s // no trailing '#' run
	}
	if i == 0 {
		return "\\" + s // the entire content is '#' characters
	}
	if isSpaceOrTab(s[i-1]) {
		return s[:i] + "\\" + s[i:]
	}
	return s
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
