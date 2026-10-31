// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package markdown_test

import (
	"strings"
	"testing"

	"github.com/go-richdoc/markdown"
	"github.com/go-richdoc/richdoc"
)

// TestFootnoteCycleDoesNotCrash covers a DENIAL OF SERVICE, and fifty-five
// bytes are enough for it:
//
//	a footnote[^1]
//
//	[^1]: the note[^1]
//
// A footnote's body is inlined at its reference, so resolving one is
// recursive; a note whose body cites itself recursed forever and Go answered
// "fatal error: stack overflow". That is not a panic — no recover() catches it
// — so a service converting a document it did not write could be killed by it
// and could not defend itself.
//
// ⛔ It is the same shape go-richdoc/rst was fixed for two rounds earlier, and
// the audit that followed MISSED THIS ONE: it searched the family for a
// `map[string]` of definitions, where this map is keyed by goldmark's int
// Index. Searching for the shape of the code already seen instead of for the
// behaviour — a definition inlined by reference — is how the second instance
// survived the sweep that was looking for it. FuzzParse found it in two
// minutes.
func TestFootnoteCycleDoesNotCrash(t *testing.T) {
	cases := map[string]string{
		"a note citing itself":       "a footnote[^1]\n\n[^1]: the note[^1]\n",
		"a mutual pair":              "a[^a] b[^b]\n\n[^a]: see[^b]\n\n[^b]: see[^a]\n",
		"a three-step ring":          "x[^a]\n\n[^a]: 1[^b]\n\n[^b]: 2[^c]\n\n[^c]: 3[^a]\n",
		"a note citing itself twice": "x[^1]\n\n[^1]: a[^1] and b[^1]\n",
	}
	for name, src := range cases {
		doc, err := markdown.Parse([]byte(src))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if doc == nil {
			t.Errorf("%s: no document", name)
			continue
		}
		// Bounded: the point of the guard is that the cycle stops, so a few
		// dozen bytes in must not become thousands out.
		if n := len(richdoc.PlainText(doc)); n > 400 {
			t.Errorf("%s: %d bytes of text from %d of source", name, n, len(src))
		}
		// And it must still be writable, which is what a consumer does next.
		if _, err := markdown.Write(doc); err != nil {
			t.Errorf("%s: Write: %v", name, err)
		}
	}
}

// TestAnOrdinaryFootnoteStillInlines is the control: the guard must stop a
// CYCLE and nothing else. A note that does not cite itself still arrives as a
// richdoc.Footnote carrying its body, which is this package's whole reason for
// resolving them.
func TestAnOrdinaryFootnoteStillInlines(t *testing.T) {
	doc, err := markdown.Parse([]byte("text[^1]\n\n[^1]: the body\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var found bool
	for _, b := range doc.Blocks {
		p, ok := b.(richdoc.Paragraph)
		if !ok {
			continue
		}
		for _, in := range p.Inlines {
			note, ok := in.(richdoc.Footnote)
			if !ok {
				continue
			}
			found = true
			if got := richdoc.PlainText(&richdoc.Document{Blocks: note.Blocks}); !strings.Contains(got, "the body") {
				t.Errorf("the note came back without its body: %q", got)
			}
		}
	}
	if !found {
		t.Error("no footnote in the document at all")
	}
}

// TestTheSameFootnoteTwiceStillExpandsTwice is the other half of the control.
// The guard is keyed on a note being expanded ON THE CURRENT PATH, not on
// having been expanded before — so two separate references to one note must
// each carry its body, and a guard that remembered instead of tracking would
// quietly empty the second one.
func TestTheSameFootnoteTwiceStillExpandsTwice(t *testing.T) {
	doc, err := markdown.Parse([]byte("one[^1] and two[^1]\n\n[^1]: shared\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	bodies := 0
	for _, b := range doc.Blocks {
		p, ok := b.(richdoc.Paragraph)
		if !ok {
			continue
		}
		for _, in := range p.Inlines {
			if note, ok := in.(richdoc.Footnote); ok {
				if got := richdoc.PlainText(&richdoc.Document{Blocks: note.Blocks}); strings.Contains(got, "shared") {
					bodies++
				}
			}
		}
	}
	if bodies != 2 {
		t.Errorf("%d of the two references carry the body", bodies)
	}
}
