package apply

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

func TestApply_SingleLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "one\ntwo\nthree\n")

	res := Apply(dir, []Edit{{Path: "a.go", Line: 2, Replacement: "TWO", Key: "k"}})
	applied, oob, errs := res.Counts()
	assert.Equal(t, 1, applied)
	assert.Equal(t, 0, oob)
	assert.Equal(t, 0, errs)

	got, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	assert.Equal(t, "one\nTWO\nthree\n", string(got))
}

func TestApply_MultiLineReplacement(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "one\ntwo\nthree\n")

	res := Apply(dir, []Edit{{Path: "a.go", Line: 2, Replacement: "2a\n2b", Key: "k"}})
	assert.Equal(t, []string{"k"}, res.AppliedKeys())

	got, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	assert.Equal(t, "one\n2a\n2b\nthree\n", string(got))
}

func TestApply_DescendingOrderNoShift(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "one\ntwo\nthree\nfour\n")

	// two edits in the same file: a multi-line replacement on line 2 must not
	// shift the line-4 edit (applied highest-first).
	res := Apply(dir, []Edit{
		{Path: "a.go", Line: 2, Replacement: "2a\n2b\n2c", Key: "k2"},
		{Path: "a.go", Line: 4, Replacement: "FOUR", Key: "k4"},
	})
	applied, _, _ := res.Counts()
	assert.Equal(t, 2, applied)

	got, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	assert.Equal(t, "one\n2a\n2b\n2c\nthree\nFOUR\n", string(got))
}

func TestApply_PreservesCRLF(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "one\r\ntwo\r\nthree\r\n")

	Apply(dir, []Edit{{Path: "a.go", Line: 2, Replacement: "TWO", Key: "k"}})

	got, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	assert.Equal(t, "one\r\nTWO\r\nthree\r\n", string(got), "CRLF endings preserved")
}

func TestApply_NoTrailingNewlinePreserved(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "one\ntwo")

	Apply(dir, []Edit{{Path: "a.go", Line: 2, Replacement: "TWO", Key: "k"}})

	got, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	assert.Equal(t, "one\nTWO", string(got), "absent trailing newline stays absent")
}

func TestApply_OutOfBounds(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "one\ntwo\n")

	res := Apply(dir, []Edit{{Path: "a.go", Line: 9, Replacement: "x", Key: "k"}})
	applied, oob, _ := res.Counts()
	assert.Equal(t, 0, applied)
	assert.Equal(t, 1, oob)

	got, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	assert.Equal(t, "one\ntwo\n", string(got), "file untouched when no edit applies")
}

func TestApply_MissingFileErrors(t *testing.T) {
	dir := t.TempDir()
	res := Apply(dir, []Edit{{Path: "nope.go", Line: 1, Replacement: "x", Key: "k"}})
	_, _, errs := res.Counts()
	assert.Equal(t, 1, errs)
	assert.NotEmpty(t, res.Errors())
}

func TestApply_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(filepath.Dir(dir), "victim.txt")
	require.NoError(t, os.WriteFile(outside, []byte("safe\n"), 0o644))

	res := Apply(dir, []Edit{{Path: "../victim.txt", Line: 1, Replacement: "HACKED", Key: "k"}})
	_, _, errs := res.Counts()
	assert.Equal(t, 1, errs)

	got, _ := os.ReadFile(outside)
	assert.Equal(t, "safe\n", string(got), "file outside root must not be written")
}

func TestApply_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	writeFile(t, dir, "a.go", "a1\na2\n")
	writeFile(t, dir, "sub/b.go", "b1\nb2\n")

	res := Apply(dir, []Edit{
		{Path: "a.go", Line: 1, Replacement: "A1", Key: "ka"},
		{Path: "sub/b.go", Line: 2, Replacement: "B2", Key: "kb"},
	})
	applied, _, _ := res.Counts()
	assert.Equal(t, 2, applied)

	a, _ := os.ReadFile(filepath.Join(dir, "a.go"))
	b, _ := os.ReadFile(filepath.Join(dir, "sub", "b.go"))
	assert.Equal(t, "A1\na2\n", string(a))
	assert.Equal(t, "b1\nB2\n", string(b))
}
