package markdown

import (
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestATableCaptionBecomesAnEmphasisedParagraph pins the degradation richdoc
// v0.5.0's Table.Caption gets here. A GFM pipe table has no caption -- the
// extension parses a cell's content as inlines and has no syntax for a table title
// -- so the choice is between dropping the author's words and putting them where a
// reader sees them. This writes them as an emphasised paragraph after the table.
//
// It does not come back as a caption on the next Parse, which is why the asymmetry
// is written down here and in the README rather than hidden: GFM gives this writer
// no way to mark one. No table this package PARSES ever has a caption, so the round
// trip is unaffected -- a caption can only arrive from another converter.
func TestATableCaptionBecomesAnEmphasisedParagraph(t *testing.T) {
	out, err := Write(richdoc.New().Add(richdoc.Table{
		Caption: []richdoc.Inline{richdoc.Txt("Should be Table 1")},
		Header:  []richdoc.Cell{richdoc.Td(richdoc.Txt("a")), richdoc.Td(richdoc.Txt("b"))},
		Rows:    [][]richdoc.Cell{{richdoc.Td(richdoc.Txt("1")), richdoc.Td(richdoc.Txt("2"))}},
	}).Doc())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "*Should be Table 1*") {
		t.Errorf("the caption is gone:\n%s", got)
	}
	// After the table, not before it: a caption that precedes its table reads as
	// a heading for whatever came earlier.
	if strings.Index(got, "*Should be Table 1*") < strings.Index(got, "| a | b |") {
		t.Errorf("the caption precedes the table:\n%s", got)
	}
}

// TestATableWithoutACaptionIsUnchanged is the CONTROL: every existing table must
// come out exactly as before, with no trailing paragraph. It passes either way.
func TestATableWithoutACaptionIsUnchanged(t *testing.T) {
	out, err := Write(richdoc.New().Add(richdoc.Table{
		Header: []richdoc.Cell{richdoc.Td(richdoc.Txt("a"))},
		Rows:   [][]richdoc.Cell{{richdoc.Td(richdoc.Txt("1"))}},
	}).Doc())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if strings.Contains(string(out), "*") {
		t.Errorf("a table with no caption gained emphasis:\n%s", out)
	}
}

// TestACellsBlocksAreNotLost pins what this converter does with the OTHER field
// v0.5.0 added. A GFM cell cannot hold blocks, so Cell.Blocks has nowhere to go --
// but richdoc's contract says a producer that fills Blocks also fills Inlines with
// the flattened view, and that is what this writer reads. The words survive; the
// structure is the format's limit, not a loss this package introduces.
func TestACellsBlocksAreNotLost(t *testing.T) {
	out, err := Write(richdoc.New().Add(richdoc.Table{
		Header: []richdoc.Cell{richdoc.Td(richdoc.Txt("name"))},
		Rows: [][]richdoc.Cell{{{
			Inlines: []richdoc.Inline{richdoc.Txt("one, two")},
			Blocks: []richdoc.Block{richdoc.List{Items: []richdoc.ListItem{
				richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("one")}}),
				richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("two")}}),
			}}},
		}}},
	}).Doc())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(string(out), "one, two") {
		t.Errorf("the cell's words are gone:\n%s", out)
	}
}
