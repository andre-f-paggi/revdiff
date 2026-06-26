package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
)

func TestApplyApplicable(t *testing.T) {
	tests := []struct {
		name    string
		opts    options
		gitRoot string
		want    bool
	}{
		{"working tree", options{}, "/repo", true},
		{"no git root", options{}, "", false},
		{"staged", options{Staged: true}, "/repo", false},
		{"stdin", options{Stdin: true}, "/repo", false},
		{"all-files", options{AllFiles: true}, "/repo", false},
		{"compare", options{CompareOld: "a", CompareNew: "b"}, "/repo", false},
		{"only", options{Only: []string{"x.go"}}, "/repo", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, applyApplicable(tt.opts, tt.gitRoot))
		})
	}
}

func TestApplyApplicable_TwoRefForms(t *testing.T) {
	mk := func(baseRef, against string) options {
		var o options
		o.Refs.Base = baseRef
		o.Refs.Against = against
		return o
	}
	assert.False(t, applyApplicable(mk("a", "b"), "/repo"), "space form a b")
	assert.False(t, applyApplicable(mk("a..b", ""), "/repo"), "range form a..b")
	assert.True(t, applyApplicable(mk("main", ""), "/repo"), "single ref")
}

func TestApplySuggestions_WritesAndMarks(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.go"), []byte("one\ntwo\nthree\n"), 0o644))

	store := annotation.NewStore()
	store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Replacement: "TWO"})     // applies
	store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "-", Replacement: "gone"})    // removed → skipped
	store.Add(annotation.Annotation{File: "a.go", Line: 3, Type: "+", Comment: "just a note"}) // no replacement

	var out bytes.Buffer
	applySuggestions(store, root, &out)

	got, _ := os.ReadFile(filepath.Join(root, "a.go"))
	assert.Equal(t, "one\nTWO\nthree\n", string(got))

	applied, ok := store.At("a.go", 2, "+")
	require.True(t, ok)
	assert.True(t, applied.Applied, "applied suggestion tagged in store")

	removed, ok := store.At("a.go", 1, "-")
	require.True(t, ok)
	assert.False(t, removed.Applied, "removed-line suggestion not applied")

	assert.Contains(t, out.String(), "applied 1 suggested edit to 1 file")
	assert.Contains(t, out.String(), "skipped 1 on removed line")
}
