package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
)

func suggestTestModel() Model {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "oldFunc(x)", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.layout.focus = paneDiff
	m.layout.width = 80
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 1 // the removed line
	return m
}

func TestModel_SuggestKeyEntersSuggestingMode(t *testing.T) {
	m := suggestTestModel()
	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model := result.(Model)
	assert.True(t, model.annot.annotating, "should enter input mode")
	assert.True(t, model.annot.suggesting, "should be in suggesting mode")
	assert.False(t, model.annot.fileAnnotating)
	assert.Equal(t, "oldFunc(x)", model.annot.input.Value(), "input seeded with original line content")
	assert.NotNil(t, cmd)
}

func TestModel_SaveSuggestionStoresReplacement(t *testing.T) {
	m := suggestTestModel()
	m.saveSuggestion("newFunc(x, opts)", "a.go", 2, "-")

	a, ok := m.store.At("a.go", 2, "-")
	require.True(t, ok)
	assert.Equal(t, "newFunc(x, opts)", a.Replacement)
	assert.False(t, m.annot.suggesting, "suggesting flag reset after save")
}

func TestModel_SaveSuggestionPreservesExistingComment(t *testing.T) {
	m := suggestTestModel()
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Comment: "use options form"})
	m.saveSuggestion("newFunc(x, opts)", "a.go", 2, "-")

	a, ok := m.store.At("a.go", 2, "-")
	require.True(t, ok)
	assert.Equal(t, "use options form", a.Comment, "comment retained")
	assert.Equal(t, "newFunc(x, opts)", a.Replacement)
}

func TestModel_DiscardSuggestionKeepsComment(t *testing.T) {
	m := suggestTestModel()
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Comment: "note", Replacement: "edit()"})
	m.discardSuggestion()

	a, ok := m.store.At("a.go", 2, "-")
	require.True(t, ok, "annotation kept because it still has a comment")
	assert.Equal(t, "note", a.Comment)
	assert.Empty(t, a.Replacement, "replacement rolled back")
}

func TestModel_DiscardSuggestionRemovesCommentlessAnnotation(t *testing.T) {
	m := suggestTestModel()
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Replacement: "edit()"})
	m.discardSuggestion()

	_, ok := m.store.At("a.go", 2, "-")
	assert.False(t, ok, "suggestion-only annotation removed entirely on discard")
}

func TestModel_HunkLineHeightIncludesPreviewRows(t *testing.T) {
	m := suggestTestModel()
	hunks := m.findHunks()

	base := m.hunkLineHeight(1, hunks, m.buildAnnotationSet())

	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Replacement: "one\ntwo\nthree"})
	withPreview := m.hunkLineHeight(1, hunks, m.buildAnnotationSet())

	assert.Equal(t, base+3, withPreview, "three replacement lines add three visual rows")
}

func TestModel_RenderDiffShowsSuggestionPreview(t *testing.T) {
	m := suggestTestModel()
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Replacement: "newFunc(x, opts)"})

	out := m.renderDiff()
	assert.Contains(t, out, "newFunc(x, opts)", "preview content rendered")
	assert.Contains(t, out, strings.TrimSpace(suggestionPrefix), "preview marker rendered")
}

func TestModel_SuggestionOnlyRendersNoEmptyCommentRow(t *testing.T) {
	m := suggestTestModel()
	// suggestion with no comment: the comment portion must contribute zero rows.
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Replacement: "x"})
	key := m.annotationKey(2, "-")
	assert.Equal(t, 0, m.wrappedAnnotationLineCount(key), "no blank comment row for suggestion-only record")
	assert.Equal(t, 1, m.previewVisualRowCount(key))
}

func TestModel_ApplyQuitEntersConfirmWhenApplicable(t *testing.T) {
	m := suggestTestModel()
	m.applyApplicable = true
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Replacement: "x"})

	model, _ := m.handleApplyQuit()
	assert.True(t, model.(Model).inConfirmApply, "should show confirm prompt")
}

func TestModel_ApplyQuitHintWhenNotApplicable(t *testing.T) {
	m := suggestTestModel()
	m.applyApplicable = false
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Replacement: "x"})

	model, _ := m.handleApplyQuit()
	mm := model.(Model)
	assert.False(t, mm.inConfirmApply)
	assert.Contains(t, mm.keys.hint, "not available")
}

func TestModel_ApplyQuitHintWhenNoSuggestions(t *testing.T) {
	m := suggestTestModel()
	m.applyApplicable = true
	// only a comment, no replacement
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Comment: "note"})

	model, _ := m.handleApplyQuit()
	mm := model.(Model)
	assert.False(t, mm.inConfirmApply)
	assert.Contains(t, mm.keys.hint, "no suggestions")
}

func TestModel_ConfirmApplyKeyConfirms(t *testing.T) {
	m := suggestTestModel()
	m.inConfirmApply = true

	model, _ := m.handleConfirmApplyKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	assert.True(t, model.(Model).applyRequested, "y confirms apply-on-quit")
}

func TestModel_ConfirmApplyKeyCancels(t *testing.T) {
	m := suggestTestModel()
	m.inConfirmApply = true

	model, _ := m.handleConfirmApplyKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	mm := model.(Model)
	assert.False(t, mm.inConfirmApply)
	assert.False(t, mm.applyRequested)
}

func TestModel_SuggestionStatsExcludesRemovedAndComments(t *testing.T) {
	m := suggestTestModel()
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Replacement: "edit"})    // counts
	m.store.Add(annotation.Annotation{File: "a.go", Line: 3, Type: "-", Replacement: "edit"})    // removed → excluded
	m.store.Add(annotation.Annotation{File: "a.go", Line: 4, Type: "+", Comment: "just a note"}) // no replacement → excluded

	edits, files := m.suggestionStats()
	assert.Equal(t, 1, edits)
	assert.Equal(t, 1, files)
}

func TestModel_ConfirmApplyStatusBarPrompt(t *testing.T) {
	m := suggestTestModel()
	m.layout.width = 200
	m.inConfirmApply = true
	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "+", Replacement: "edit"})

	assert.Contains(t, m.statusBarText(), "apply 1 suggested edit to 1 file and quit? [y/n]")
}

func TestModel_StatusShowsSuggestionIcon(t *testing.T) {
	m := suggestTestModel()
	m.layout.width = 200
	assert.NotContains(t, m.statusBarText(), "✎", "no pencil before any suggestion")

	m.store.Add(annotation.Annotation{File: "a.go", Line: 2, Type: "-", Replacement: "x"})
	assert.Contains(t, m.statusBarText(), "✎", "pencil shown once a suggestion exists")
}
