package annotation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatOutput_SuggestionWithComment(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "a.go", Line: 42, Type: "-", Comment: "use the options form", Replacement: "newFunc(x, opts)"})

	want := "## a.go:42 (-)\nuse the options form\n```suggestion\nnewFunc(x, opts)\n```\n"
	assert.Equal(t, want, s.FormatOutput())
}

func TestFormatOutput_SuggestionOnly(t *testing.T) {
	s := NewStore()
	s.Add(Annotation{File: "a.go", Line: 7, Type: "+", Replacement: "x := 1"})

	want := "## a.go:7 (+)\n```suggestion\nx := 1\n```\n"
	assert.Equal(t, want, s.FormatOutput())
}

func TestParse_SuggestionRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		ann  Annotation
	}{
		{"comment only", Annotation{File: "a.go", Line: 1, Type: "+", Comment: "just a note"}},
		{"suggestion only", Annotation{File: "a.go", Line: 2, Type: "-", Replacement: "replaced()"}},
		{"comment and suggestion", Annotation{File: "a.go", Line: 3, Type: " ", Comment: "why\nmulti", Replacement: "line one\nline two"}},
		{"file-level suggestion", Annotation{File: "a.go", Line: 0, Type: "", Comment: "top", Replacement: "header\ncontent"}},
		{"range suggestion", Annotation{File: "a.go", Line: 10, EndLine: 12, Type: "-", Comment: "shrink", Replacement: "single line"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore()
			s.Add(tc.ann)
			parsed, err := Parse(strings.NewReader(s.FormatOutput()))
			require.NoError(t, err)
			require.Len(t, parsed, 1)
			assert.Equal(t, tc.ann.Comment, parsed[0].Comment)
			assert.Equal(t, tc.ann.Replacement, parsed[0].Replacement)
		})
	}
}

func TestParse_SuggestionContainingCodeFence(t *testing.T) {
	// Replacement content that itself contains a ``` fence must round-trip:
	// fenceFor widens the wrapping fence past the longest backtick run inside.
	repl := "```go\nfmt.Println(\"x\")\n```"
	s := NewStore()
	s.Add(Annotation{File: "doc.md", Line: 5, Type: "+", Comment: "embed code", Replacement: repl})

	out := s.FormatOutput()
	require.Contains(t, out, "````suggestion") // 4 backticks, one past the inner 3

	parsed, err := Parse(strings.NewReader(out))
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	assert.Equal(t, repl, parsed[0].Replacement)
}

func TestParse_CommentLineLooksLikeFenceOpener(t *testing.T) {
	// A comment whose line is exactly "```suggestion" must be escaped on output
	// and recovered on parse rather than starting a replacement block.
	s := NewStore()
	s.Add(Annotation{File: "a.go", Line: 1, Type: "+", Comment: "```suggestion", Replacement: "real()"})

	parsed, err := Parse(strings.NewReader(s.FormatOutput()))
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	assert.Equal(t, "```suggestion", parsed[0].Comment)
	assert.Equal(t, "real()", parsed[0].Replacement)
}

func TestParse_SuggestionWithHashLinesInReplacement(t *testing.T) {
	// "## " lines inside a replacement are literal content, not record headers.
	repl := "## not a header\nstill replacement"
	s := NewStore()
	s.Add(Annotation{File: "a.go", Line: 1, Type: "+", Replacement: repl})
	s.Add(Annotation{File: "a.go", Line: 2, Type: "+", Comment: "second"})

	parsed, err := Parse(strings.NewReader(s.FormatOutput()))
	require.NoError(t, err)
	require.Len(t, parsed, 2)
	assert.Equal(t, repl, parsed[0].Replacement)
	assert.Equal(t, "second", parsed[1].Comment)
}

func TestStore_AddClearsReplacement(t *testing.T) {
	// Re-adding with an empty Replacement clears a prior suggestion (the
	// discard-suggestion path keeps the comment but drops the edit).
	s := NewStore()
	s.Add(Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note", Replacement: "edit()"})
	s.Add(Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})

	got, ok := s.At("a.go", 1, "+")
	require.True(t, ok)
	assert.Equal(t, "note", got.Comment)
	assert.Empty(t, got.Replacement)
}
