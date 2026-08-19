// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import (
	"reflect"
	"testing"
)

// corpus holds representative Markdown documents exercising every block and
// inline construct the converter handles. Each must survive a round-trip:
// Parse -> Write -> Parse reproduces the same richdoc tree, up to the
// normalisation the writer performs (emphasis markers to '*', list markers to
// '-'/'1.', setext headings to ATX, autolinks to inline links).
var corpus = map[string]string{
	"headings-atx": "# One\n\n## Two\n\n### Three\n\n#### Four\n\n##### Five\n\n###### Six\n",

	"headings-setext": "Title\n=====\n\nSubtitle\n--------\n",

	"paragraphs-and-breaks": "First paragraph with a soft\nline break inside it.\n\nSecond paragraph with a hard  \nline break inside it.\n",

	"inline-styles": "Text with *emph*, **strong**, ~~strike~~ and `code` spans.\n",

	"code-span-with-backtick": "Use `` a`b `` and `` `lead `` and `trail` `` here.\n",

	"links-and-images": "A [plain link](http://a.example) and a [titled link](http://b.example \"tip\").\n\n![alt text](img.png) then ![titled](pic.png \"cap\").\n",

	"autolink": "Visit <http://auto.example> today.\n",

	"nested-tight-list": "- one\n- two\n  - two-a\n  - two-b\n- three\n",

	"loose-list": "- alpha\n\n- beta\n\n- gamma\n",

	"ordered-list": "3. third\n4. fourth\n5. fifth\n",

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
			if !reflect.DeepEqual(d1, d2) {
				t.Errorf("round-trip changed the tree\n--- rewritten source ---\n%s\n--- d1 ---\n%#v\n--- d2 ---\n%#v", out, d1, d2)
			}
		})
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
