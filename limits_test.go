// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import (
	"strings"
	"testing"
)

// TestDeepBlockNestingIsRefused is a denial of service by CPU. Block quotes
// cost two bytes a level and the parse is quadratic in depth, so a few hundred
// kilobytes buy tens of seconds — 400 KB took 21.9 s on the machine this was
// written on, and the curve is ×4 for every doubling.
func TestDeepBlockNestingIsRefused(t *testing.T) {
	deep := strings.Repeat("> ", 300) + "x\n"
	_, err := Parse([]byte(deep))
	if err == nil {
		t.Fatal("300 nested quotes parsed; the default limit is 256")
	}
	if !strings.Contains(err.Error(), "256") {
		t.Errorf("the error does not name the limit it enforced: %v", err)
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Errorf("the error does not name the line: %v", err)
	}
}

// TestRealWorldNestingStillParses is the control the default is chosen
// against. mddepthprobe read the 14,651 real .md files on this machine: the
// deepest reaches FIVE leading ">" markers and the widest indentation is 73
// columns, about 36 levels. All of that has to parse.
func TestRealWorldNestingStillParses(t *testing.T) {
	cases := map[string]string{
		"five quotes, the deepest real document": strings.Repeat("> ", 5) + "x\n",
		"73 columns of indent, the widest real":  strings.Repeat(" ", 73) + "x\n",
		"a quote inside a list inside a quote":   "> - > x\n",
		"a list nested 20 levels":                nestedList(20),
		"an ordinary document":                   "# Title\n\ntext\n\n- a\n- b\n\n> quoted\n",
	}
	for name, src := range cases {
		doc, err := Parse([]byte(src))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if doc == nil || len(doc.Blocks) == 0 {
			t.Errorf("%s: no blocks", name)
		}
	}
}

func nestedList(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(strings.Repeat("  ", i) + "- x\n")
	}
	return b.String()
}

// TestFencedCodeIsNotNesting is the control that keeps the guard from refusing
// documents for their own TEXT. Inside a fence a leading ">" is a transcript
// and a deep indent is a code sample; counting either as nesting would refuse
// a README for quoting a shell session.
func TestFencedCodeIsNotNesting(t *testing.T) {
	cases := map[string]string{
		"a transcript in a backtick fence": "```\n" + strings.Repeat("> ", 400) + "x\n```\n",
		"a deeply indented code sample":    "```python\n" + strings.Repeat(" ", 900) + "pass\n```\n",
		"a tilde fence":                    "~~~\n" + strings.Repeat("> ", 400) + "x\n~~~\n",
		"a longer closing fence":           "````\n" + strings.Repeat("> ", 400) + "x\n````\n",
	}
	for name, src := range cases {
		if _, err := Parse([]byte(src)); err != nil {
			t.Errorf("%s: refused its own content: %v", name, err)
		}
	}
}

// TestAClosedFenceStopsProtecting is a WITNESS, not part of the control above.
//
// It lived in TestFencedCodeIsNotNesting until that function was run with the
// guard disabled and failed — and a control that fails without the fix is a
// witness wearing the wrong name. The distinction matters here more than
// usual: without it, a document could switch the guard off for good by opening
// a fence anywhere near the top.
func TestAClosedFenceStopsProtecting(t *testing.T) {
	closed := "```\nx\n```\n" + strings.Repeat("> ", 400) + "x\n"
	if _, err := Parse([]byte(closed)); err == nil {
		t.Error("nesting after a closed fence was not counted; a fence would switch the guard off")
	}
}

// TestFenceRulesFollowTheSpec pins the fence tracking against CommonMark 4.5,
// quoted from the spec rather than recalled, because every one of these rules
// decides whether a real document is refused for its own content:
//
//	"The closing code fence may be preceded by up to three spaces of
//	 indentation, and may be followed only by spaces or tabs, which are
//	 ignored."                                            (spec, line 1959)
//	"The closing code fence must use the same character as the opening"  (2015)
//	"The closing code fence must be at least as long as the opening"     (2042)
//
// An opener indented four columns is an indented code block, not a fence
// (4.4/4.5), and a backtick fence's info string may not contain a backtick —
// which is what stops inline code on its own line from opening one.
func TestFenceRulesFollowTheSpec(t *testing.T) {
	openers := []struct {
		line string
		ok   bool
		why  string
	}{
		{"```", true, "three backticks"},
		{"~~~~", true, "four tildes"},
		{"   ```", true, "three spaces is still a fence"},
		{"    ```", false, "four spaces is an indented code block"},
		{"``", false, "two backticks is not a fence"},
		{"```go", true, "an info string is allowed"},
		{"```a`b", false, "a backtick fence's info string may not contain a backtick"},
		{"~~~a`b", true, "a tilde fence's info string may"},
		{"   ", false, "spaces only"},
		{"", false, "empty"},
		{"text", false, "prose"},
	}
	for _, c := range openers {
		_, _, got := opensFence([]byte(c.line))
		if got != c.ok {
			t.Errorf("opensFence(%q) = %v, want %v (%s)", c.line, got, c.ok, c.why)
		}
	}

	closers := []struct {
		line string
		ok   bool
		why  string
	}{
		{"```", true, "same character, same length"},
		{"````", true, "longer than the opener"},
		{"``", false, "must be at least as long as the opening fence"},
		{"   ```", true, "up to three spaces of indentation"},
		{"    ```", false, "four spaces does not close"},
		{"``` ", true, "may be followed only by spaces or tabs"},
		{"```\t", true, "a tab counts as whitespace after it"},
		{"``` x", false, "anything else on the line and it does not close"},
		{"~~~", false, "must use the same character as the opening"},
		{"", false, "empty"},
	}
	for _, c := range closers {
		if got := closesFence([]byte(c.line), '`', 3); got != c.ok {
			t.Errorf("closesFence(%q, '`', 3) = %v, want %v (%s)", c.line, got, c.ok, c.why)
		}
	}
}

// TestAnIndentedSampleInsideAFenceSurvives is the document the indentation
// rule above protects: a fence holding a Markdown sample whose own lines are
// indented. Treating the sample's indented ``` as a closer would end tracking
// early and refuse the file for its own text.
func TestAnIndentedSampleInsideAFenceSurvives(t *testing.T) {
	src := "````\n" +
		"    ```\n" +
		strings.Repeat(" ", 600) + "deep sample line\n" +
		"    ```\n" +
		"````\n"
	if _, err := Parse([]byte(src)); err != nil {
		t.Errorf("a fenced Markdown sample was refused for its own content: %v", err)
	}
}

// TestLimitsAreCallerChosen covers the two conventions borrowed from
// go-richdoc/rst's LineLengthLimit and go-richdoc/latex's MaxDepth: zero is
// the default, and a negative value is no limit at all.
func TestLimitsAreCallerChosen(t *testing.T) {
	deep := strings.Repeat("> ", 300) + "x\n"
	if _, err := ParseWithLimits([]byte("> > > x\n"), Limits{MaxBlockNesting: 2}); err == nil {
		t.Error("a caller's own tighter limit was not enforced")
	}
	if _, err := ParseWithLimits([]byte(deep), Limits{}); err == nil {
		t.Error("zero should mean the default of 256, which 300 exceeds")
	}
	if _, err := ParseWithLimits([]byte(deep), Limits{MaxBlockNesting: -1}); err != nil {
		t.Errorf("a negative limit should mean no limit: %v", err)
	}
}

// TestBlockPrefixDepthCountsBothHalves is the unit-level planted positive: the
// metric is quote markers PLUS indentation levels, and a scan that silently
// counted only one half would still pass every case above that uses the other.
func TestBlockPrefixDepthCountsBothHalves(t *testing.T) {
	cases := []struct {
		line string
		want int
	}{
		{"x", 0},
		{"> x", 1},
		{"> > > x", 3},
		{"    x", 2},   // four columns, two levels
		{"\tx", 2},     // a tab is four columns
		{"> > \tx", 4}, // two quotes and a tab
		{strings.Repeat(" ", 73) + "x", 36},
		{"", 0},
		{">", 1},

		// List markers are part of the prefix too, which the first version of
		// this scan missed entirely: FuzzParse produced "*   *   *   ... x" in
		// six seconds, a line the scan read as depth ZERO. See
		// blockPrefixDepth and testdata/fuzz/FuzzParse.
		{"- x", 1},
		{"* x", 1},
		{"+ x", 1},
		{"1. x", 1},
		{"42) x", 1},
		{"*   *   *   x", 3},                   // the attack shape
		{strings.Repeat("*   ", 64) + "x", 64}, // and at length
		{"- > - > x", 4},                       // starters of both kinds, alternating
		{"  - - x", 3},                         // indentation plus two markers
		{"1. 2. 3. x", 3},                      // ordered nesting is cheaper still
		{"-", 1},                               // a marker alone opens an empty item
		{"1.", 1},

		// And the shapes that must NOT read as markers, or ordinary prose
		// would be counted as nesting.
		{"*emphasis* text", 0},
		{"***", 0},
		{"**strong** text", 0},
		{"1.no space", 0},
		{"1234567890. x", 0}, // more than nine digits is not an ordered marker
		{"-no space", 0},
	}
	for _, c := range cases {
		if got := blockPrefixDepth([]byte(c.line)); got != c.want {
			t.Errorf("blockPrefixDepth(%q) = %d, want %d", c.line, got, c.want)
		}
	}
}
