package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/apply"
)

// applyApplicable reports whether apply-on-quit (Ctrl+S) can write suggestions
// to files. Only git working-tree review qualifies: the diff's "new" side must
// be the working tree (so added/context lines map to working-tree line numbers)
// and a repo root must exist to resolve the repo-root-relative annotation paths.
// Disabled for --staged (new side is the index), two-ref diffs, --stdin,
// --compare, --all-files, and --only / non-git repos.
func applyApplicable(opts options, gitRoot string) bool {
	if gitRoot == "" {
		return false
	}
	if opts.Stdin || opts.Staged || opts.AllFiles {
		return false
	}
	if opts.CompareOld != "" || opts.CompareNew != "" {
		return false
	}
	if len(opts.Only) > 0 {
		return false
	}
	if opts.Refs.Against != "" || strings.Contains(opts.Refs.Base, "..") {
		return false // two-ref diff has no working-tree "new" side
	}
	return true
}

// applySuggestions writes every applicable suggested replacement in the store to
// its working-tree file under root, marks the applied annotations so FormatOutput
// tags them [applied], and prints a human summary to w. Removed-line ("-")
// suggestions have no working-tree counterpart and are skipped.
func applySuggestions(store *annotation.Store, root string, w io.Writer) {
	var edits []apply.Edit
	skippedRemoved := 0
	for file, anns := range store.All() {
		for _, a := range anns {
			if a.Replacement == "" || a.Line == 0 {
				continue
			}
			if a.Type == "-" {
				skippedRemoved++
				continue
			}
			edits = append(edits, apply.Edit{
				Path:        file,
				Line:        a.Line,
				Replacement: a.Replacement,
				Key:         applyKey(file, a.Line, a.Type),
			})
		}
	}

	res := apply.Apply(root, edits)

	appliedKeys := map[string]bool{}
	for _, k := range res.AppliedKeys() {
		appliedKeys[k] = true
	}
	appliedFiles := map[string]bool{}
	for file, anns := range store.All() {
		for _, a := range anns {
			if appliedKeys[applyKey(file, a.Line, a.Type)] {
				store.MarkApplied(file, a.Line, a.Type)
				appliedFiles[file] = true
			}
		}
	}

	applied, oob, errs := res.Counts()
	var b strings.Builder
	fmt.Fprintf(&b, "revdiff: applied %d suggested %s to %d %s",
		applied, plural(applied, "edit", "edits"), len(appliedFiles), plural(len(appliedFiles), "file", "files"))
	if skippedRemoved > 0 {
		fmt.Fprintf(&b, "; skipped %d on removed %s", skippedRemoved, plural(skippedRemoved, "line", "lines"))
	}
	if oob > 0 {
		fmt.Fprintf(&b, "; %d out of range", oob)
	}
	if errs > 0 {
		fmt.Fprintf(&b, "; %d %s", errs, plural(errs, "error", "errors"))
	}
	fmt.Fprintln(w, b.String())
	for _, e := range res.Errors() {
		fmt.Fprintf(w, "  %v\n", e)
	}
}

// applyKey is the stable identity for a line-level annotation, used to correlate
// apply outcomes back to store records.
func applyKey(file string, line int, changeType string) string {
	return fmt.Sprintf("%s\x00%d\x00%s", file, line, changeType)
}

// plural returns one or many depending on n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
