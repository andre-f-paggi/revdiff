package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/style"
)

// suggestionPrefix marks each proposed replacement line in the preview. A fixed
// glyph (distinct from the configurable annotation marker) so a suggested edit
// reads as "proposed new content" rather than commentary.
const suggestionPrefix = "✎ "

// startSuggestEdit enters input mode to propose a literal replacement for the
// current cursor line. The input is seeded with the line's original content (or
// the existing suggestion when re-editing) so the user edits real text. The
// replacement is stored on the same annotation record as any comment.
func (m *Model) startSuggestEdit() tea.Cmd {
	m.clearPendingInputState()
	dl, ok := m.cursorDiffLine()
	if !ok || dl.ChangeType == diff.ChangeDivider {
		return nil
	}
	hunks := m.findHunks()
	if m.isCollapsedHidden(m.nav.diffCursor, hunks) || m.isDeleteOnlyPlaceholder(m.nav.diffCursor, hunks) {
		return nil
	}

	editorKey := m.editorKeyDisplay()
	placeholder := "suggested edit..."
	if editorKey != "" {
		placeholder = fmt.Sprintf("suggested edit... (%s for editor)", editorKey)
	}

	// seed: existing suggestion when re-editing, else the original line content.
	// multi-line existing replacements bypass SetValue (textinput flattens \n) and
	// are stashed in existingMultiline, mirroring the comment-editing path.
	lineNum := m.diffLineNum(dl)
	preFill := dl.Content
	var existingMultiline string
	if a, found := m.store.At(m.file.name, lineNum, string(dl.ChangeType)); found && a.Replacement != "" {
		if strings.Contains(a.Replacement, "\n") {
			existingMultiline = a.Replacement
			placeholder = m.multiLinePlaceholder()
			preFill = ""
		} else {
			preFill = a.Replacement
		}
	}

	ti, cmd := m.newAnnotationInput(placeholder, 3+lipgloss.Width(m.annotPrefix()))
	if preFill != "" {
		ti.SetValue(preFill)
	}

	m.annot.input = ti
	m.annot.annotating = true
	m.annot.fileAnnotating = false
	m.annot.suggesting = true
	m.annot.existingMultiline = existingMultiline
	m.ensureLineAnnotationInputVisible()
	return cmd
}

// saveSuggestion persists text as the replacement on the (line, changeType)
// record, preserving any existing comment. Target fields are passed explicitly
// so the Enter-key and external-editor paths share it without temporal coupling.
func (m *Model) saveSuggestion(text, fileName string, line int, changeType string) {
	if text == "" {
		m.cancelAnnotation()
		return
	}
	a := annotation.Annotation{File: fileName, Line: line, Type: changeType}
	if existing, ok := m.store.At(fileName, line, changeType); ok {
		a = existing // keep comment + range
	}
	a.Replacement = text
	m.store.Add(a)
	m.annot.annotating = false
	m.annot.fileAnnotating = false
	m.annot.suggesting = false
	m.annot.existingMultiline = ""
	m.tree.RefreshFilter(m.annotatedFiles())
	m.syncViewportToCursor()
}

// discardSuggestion removes the suggested replacement on the current cursor line
// (rolling back to the original content), keeping any comment. When the record
// holds only a suggestion, the whole annotation is removed.
func (m *Model) discardSuggestion() tea.Cmd {
	dl, ok := m.cursorDiffLine()
	if !ok || dl.ChangeType == diff.ChangeDivider {
		return nil
	}
	lineNum := m.diffLineNum(dl)
	a, found := m.store.At(m.file.name, lineNum, string(dl.ChangeType))
	if !found || a.Replacement == "" {
		return nil
	}
	if a.Comment == "" {
		m.store.Delete(m.file.name, lineNum, string(dl.ChangeType))
		m.annot.cursorOnAnnotation = false
		m.tree.RefreshFilter(m.annotatedFiles())
		if newFile := m.tree.SelectedFile(); newFile != "" && newFile != m.file.name {
			m.file.loadSeq++
			return m.loadFileDiff(newFile)
		}
	} else {
		a.Replacement = ""
		m.store.Add(a)
	}
	m.syncViewportToCursor()
	return nil
}

// hasSuggestions reports whether the current file has any suggested edit
// (drives the status-bar indicator).
func (m Model) hasSuggestions() bool {
	for _, a := range m.store.Get(m.file.name) {
		if a.Replacement != "" {
			return true
		}
	}
	return false
}

// previewCacheKey keys previewCache: the comparable inputs to the wrap+style
// pipeline. width self-invalidates a stale entry on resize.
type previewCacheKey struct {
	replacement string
	width       int
}

// replacementForKey returns the suggested replacement for the line-level
// annotation identified by key, or "" when none exists.
func (m Model) replacementForKey(key string) string {
	if key == annotKeyFile {
		return ""
	}
	for _, a := range m.store.Get(m.file.name) {
		if m.annotationKey(a.Line, a.Type) == key {
			return a.Replacement
		}
	}
	return ""
}

// previewVisualRowCount returns the number of visual rows the suggested-edit
// preview occupies for key (0 when there is no replacement).
func (m *Model) previewVisualRowCount(key string) int {
	replacement := m.replacementForKey(key)
	if replacement == "" {
		return 0
	}
	return len(m.previewVisualRows(replacement))
}

// previewVisualRows is the single source of truth for how a suggested edit is
// painted: the fully-styled visual rows for replacement at the current pane
// width. previewVisualRowCount uses len() of this; the painter iterates these
// rows directly. Memoized on previewCache; invalidated by invalidatePreviewRows.
func (m *Model) previewVisualRows(replacement string) []string {
	wrapW := max(m.diffContentWidth()-1, wrapMinContent)
	key := previewCacheKey{replacement: replacement, width: wrapW}
	if rows, ok := m.annot.previewCache[key]; ok {
		return rows
	}
	rows := m.composePreviewRows(replacement, wrapW)
	m.annot.previewCache[key] = rows
	return rows
}

// composePreviewRows builds the styled preview rows for a replacement: each
// logical line is prefixed with the suggestion marker; long lines wrap at wrapW
// with marker-width indentation on continuation rows. Each row is colored with
// the resolver's suggestion foreground (the suggestion background is applied by
// the painter via extendLineBg). In no-color mode the marker alone distinguishes
// the rows.
func (m Model) composePreviewRows(replacement string, wrapW int) []string {
	indent := strings.Repeat(" ", lipgloss.Width(suggestionPrefix))
	fg := style.Color("")
	if !m.cfg.noColors {
		fg = m.resolver.SuggestionFg()
	}

	var rows []string
	for _, logical := range strings.Split(replacement, "\n") {
		segment := suggestionPrefix + logical
		var lines []string
		if wrapW > wrapMinContent && lipgloss.Width(segment) > wrapW {
			lines = m.wrapContent(segment, wrapW)
		} else {
			lines = []string{segment}
		}
		for i, line := range lines {
			if i > 0 {
				line = indent + strings.TrimPrefix(line, indent)
			}
			rows = append(rows, m.styleSuggestionRow(line, fg))
		}
	}
	return rows
}

// styleSuggestionRow wraps a single preview row's text in the suggestion
// background and foreground, resetting both at the end so the row is self
// contained (extendLineBg later pads the trailing gap with the same background).
func (m Model) styleSuggestionRow(text string, fg style.Color) string {
	if m.cfg.noColors {
		return text
	}
	bg := m.resolver.SuggestionBg()
	return string(bg) + string(fg) + text + "\033[39m"
}

// invalidatePreviewRows clears the cached preview-row slices. Called on the same
// events as invalidateAnnotationRows (file load, theme apply, theme cancel).
func (m *Model) invalidatePreviewRows() {
	clear(m.annot.previewCache)
}

// renderSuggestionPreview writes the suggested-edit preview rows below the diff
// line at idx, on the suggestion highlight background. No-op when the line has no
// replacement.
func (m Model) renderSuggestionPreview(b *strings.Builder, key string) {
	replacement := m.replacementForKey(key)
	if replacement == "" {
		return
	}
	bg := style.Color("")
	if !m.cfg.noColors {
		bg = m.resolver.SuggestionBg()
	}
	for _, row := range m.previewVisualRows(replacement) {
		b.WriteString(m.extendLineBg(" "+row, bg) + "\n")
	}
}
