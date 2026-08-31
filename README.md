# go-richdoc/markdown

Convert between Markdown and the format-agnostic
[`richdoc`](https://github.com/go-richdoc/richdoc) document model.

- **`Parse`** turns CommonMark (with the GFM table and strikethrough
  extensions, PHP-Markdown-Extra footnotes and explicit heading anchors) into a
  `*richdoc.Document`. Parsing delegates to the reference Go Markdown library,
  [goldmark](https://github.com/yuin/goldmark); this package never hand-rolls a
  CommonMark parser. It walks goldmark's AST and builds the richdoc tree.
- **`Write`** renders a `*richdoc.Document` back to clean CommonMark.

### Enabled goldmark extensions and options

`Parse` configures goldmark with:

- `extension.Table` and `extension.Strikethrough` — GFM tables and `~~strike~~`.
- `extension.Footnote` — PHP Markdown Extra footnotes (`text[^id]` references
  with `[^id]: …` definitions). This is goldmark's own footnote extension; the
  package does **not** hand-roll footnote parsing.
- `parser.WithHeadingAttribute()` — explicit heading anchors written as
  `## Title {#id}`. `WithAutoHeadingID()` is deliberately **not** enabled:
  auto ids would fabricate an anchor on every heading, and `Write` would then
  stamp a `{#id}` the source never had. Only ids an author wrote are captured,
  which keeps the round-trip faithful.

The package is pure Go and builds with `CGO_ENABLED=0`.

```go
import "github.com/go-richdoc/markdown"

doc, err := markdown.Parse([]byte("# Title\n\nHello **world**.\n"))
// ... inspect or edit doc ...
out, err := markdown.Write(doc)
```

## Round-trip

The pair aims for a stable round-trip: `Write(Parse(src))` is semantically
equivalent to `src`, and `Parse(Write(Parse(src)))` reproduces the same richdoc
tree, up to harmless normalisation:

- emphasis markers normalised to `*` (`_emph_` → `*emph*`);
- list markers normalised to `-` / `1.`;
- setext headings rewritten as ATX (`#`);
- CommonMark autolinks (`<http://x>`) rewritten as inline links;
- character references and backslash escapes decoded into literal text on
  parse, and re-escaped on write;
- a soft line break inside a block flattened to a single space on write — it
  survives `Parse` as a literal newline in `Text.Value`, but writing it
  through unchanged would corrupt the block it lands in: an ATX heading is a
  single physical line, and a paragraph line ending right before a `---`/`===`
  line re-parses as an accidental Setext heading. Flattening it matches the
  soft break's own HTML rendering (whitespace).

## Node mapping

### Parse (goldmark AST → richdoc)

| goldmark node                         | richdoc node                                  |
| ------------------------------------- | --------------------------------------------- |
| `Heading` (ATX and setext)            | `Heading{Level, ID, Inlines}` (`ID` from a `{#id}` attribute) |
| `Paragraph`, `TextBlock`              | `Paragraph{Inlines}`                          |
| `FencedCodeBlock`                     | `CodeBlock{Language, Text}`                   |
| `CodeBlock` (indented)                | `CodeBlock{Text}` (no language)              |
| `Blockquote`                          | `BlockQuote{Blocks}`                          |
| `List`                                | `List{Ordered, Start, Tight, Items}`         |
| `ListItem`                            | `ListItem{Blocks}`                            |
| `ThematicBreak`                       | `ThematicBreak{}`                             |
| `extension/ast.Table`                 | `Table{Align, Header, Rows}`                  |
| `extension/ast.TableCell`             | `Cell{Inlines}`                              |
| `HTMLBlock`                           | `RawBlock{Format:"html", Text}`              |
| `LinkReferenceDefinition`             | *dropped* (resolved into the referring link)  |
| `Text`                                | `Text{Value}` (+ `LineBreak` on hard break)  |
| `String`                              | `Text{Value}`                                |
| `CodeSpan`                            | `Code{Value}`                                |
| `Emphasis` level 1                    | `Emph{Inlines}`                              |
| `Emphasis` level 2                    | `Strong{Inlines}`                            |
| `extension/ast.Strikethrough`         | `Strikethrough{Inlines}`                     |
| `Link`                                | `Link{URL, Title, Inlines}`                  |
| `Image`                               | `Image{URL, Alt, Title}`                     |
| `AutoLink`                            | `Link{URL, Inlines:[Text(label)]}`           |
| `RawHTML` (inline)                    | `RawInline{Format:"html", Text}`             |
| `Link` with a clean `#fragment` dest  | `CrossRef{Target, Kind:RefLabel, Inlines}`   |
| `extension/ast.FootnoteLink`          | `Footnote{Blocks}` (body from its definition) |
| `extension/ast.FootnoteList`          | *dropped* (bodies inlined at each reference)  |
| `extension/ast.FootnoteBacklink`      | *dropped* (goldmark render artifact)          |

Adjacent text runs are coalesced so goldmark's tokenisation does not leak into
the model.

### Write (richdoc → CommonMark)

| richdoc node        | Markdown output                                             |
| ------------------- | ---------------------------------------------------------- |
| `Heading`           | `#`…`######` ATX heading (level clamped to 1–6), `{#id}` suffix when `ID` set |
| `Paragraph`         | inline text                                                |
| `CodeBlock`         | fenced ``` ``` ``` block with language (fence widened past inner backticks) |
| `BlockQuote`        | `>`-prefixed lines                                         |
| `List`              | `-` / `N.` items, tight or loose                          |
| `Table`             | GFM pipe table with an alignment delimiter row            |
| `ThematicBreak`     | `---`                                                      |
| `MathBlock`         | `$$` … `$$`                                                |
| `RawBlock`          | verbatim text (`html` and any other format)               |
| `Text`              | escaped literal text                                       |
| `Emph`              | `*emph*`                                                   |
| `Strong`            | `**strong**`                                              |
| `Strikethrough`     | `~~strike~~`                                              |
| `Code`              | `` `code` `` (backticks widened / padded as needed)       |
| `Link`              | `[text](url "title")`                                     |
| `Image`             | `![alt](url "title")`                                     |
| `Math`              | `$…$`                                                      |
| `LineBreak`         | backslash + newline hard break                            |
| `RawInline`         | verbatim text                                              |
| `Footnote`          | `[^n]` reference, body emitted as a `[^n]: …` definition (numbered in reference order, continuation lines indented 4 spaces) |
| `CrossRef` (RefLabel) | `[text](#target)` internal link (target stands in when the reference has no text) |
| `CrossRef` (RefCite)  | pandoc `[@key]` citation (CommonMark has no native citation) |
| `Anchor`            | its inline content only — CommonMark has no anchor syntax, so the `ID` is dropped (write-only; `Parse` never produces an `Anchor`) |

### Constructs routed through raw passthrough

goldmark core has no math extension, so `$…$` / `$$…$$` are **not** recognised
on parse: they remain literal `Text`. Writing a `MathBlock` / `Math` node
(produced by another source) emits `$$…$$` / `$…$`.

Any HTML goldmark exposes — block or inline — is preserved as
`RawBlock`/`RawInline` with `Format:"html"`. No other CommonMark or GFM
construct in scope currently needs a raw fallback: every one maps onto a
first-class richdoc node.

### Cross-references from internal links

A Markdown link whose destination is a **clean** `#fragment` — no title, a
non-empty fragment with no whitespace and none of `()<>#\"` — maps to a
`CrossRef{Kind:RefLabel}`. Anything else stays a plain `Link`: an external URL,
a bare `#`, a titled link, or a fragment that could not be written back and
re-parsed identically (for example `[x](<#a b>)`, whose fragment contains a
space). The mapping is intentionally conservative so it never turns an ordinary
link into a cross-reference by accident, and it round-trips: a mapped
`CrossRef` writes back as `[text](#target)` and re-parses to the same node.

## License

BSD-3-Clause. See [LICENSE](LICENSE).
