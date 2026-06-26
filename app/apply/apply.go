// Package apply writes suggested-edit replacements into working-tree files.
// It is pure file I/O with errors-as-values: callers (the composition root)
// translate annotations into Edits and act on the returned Result. It does not
// import the UI or VCS layers.
package apply

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/umputun/revdiff/app/fsutil"
)

// Edit is a single whole-line replacement in a working-tree file.
type Edit struct {
	Path        string // repo-root-relative path (forward slashes)
	Line        int    // 1-based working-tree line number to replace
	Replacement string // new content (may contain "\n" for multiple lines)
	Key         string // opaque caller key, echoed back so the caller can mark the source applied
}

// Status is the outcome of a single Edit.
type Status int

const (
	// StatusApplied means the line was replaced and the file written.
	StatusApplied Status = iota
	// StatusOutOfBounds means Line did not exist in the file (skipped).
	StatusOutOfBounds
	// StatusError means the file could not be read or written (skipped).
	StatusError
)

// Outcome records what happened to one Edit, tagged with its caller Key.
type Outcome struct {
	Key    string
	Status Status
	Err    error
}

// Result aggregates the per-Edit outcomes of an Apply call.
type Result struct {
	Outcomes []Outcome
}

// Counts returns the number of applied, out-of-bounds, and errored edits.
func (r Result) Counts() (applied, outOfBounds, errs int) {
	for _, o := range r.Outcomes {
		switch o.Status {
		case StatusApplied:
			applied++
		case StatusOutOfBounds:
			outOfBounds++
		case StatusError:
			errs++
		}
	}
	return applied, outOfBounds, errs
}

// AppliedKeys returns the caller keys of edits that were written to disk.
func (r Result) AppliedKeys() []string {
	var keys []string
	for _, o := range r.Outcomes {
		if o.Status == StatusApplied {
			keys = append(keys, o.Key)
		}
	}
	return keys
}

// Errors returns the non-nil errors across all outcomes.
func (r Result) Errors() []error {
	var errs []error
	for _, o := range r.Outcomes {
		if o.Err != nil {
			errs = append(errs, o.Err)
		}
	}
	return errs
}

// Apply writes the edits to files under root. Edits are grouped by file; within
// a file they are applied highest-line-first so a multi-line replacement never
// shifts a not-yet-applied lower line. Each file's dominant line ending and
// trailing-newline state are preserved. Every Edit yields exactly one Outcome.
func Apply(root string, edits []Edit) Result {
	byFile := map[string][]Edit{}
	var order []string
	for _, e := range edits {
		if _, ok := byFile[e.Path]; !ok {
			order = append(order, e.Path)
		}
		byFile[e.Path] = append(byFile[e.Path], e)
	}

	var res Result
	for _, path := range order {
		res.Outcomes = append(res.Outcomes, applyFile(root, path, byFile[path])...)
	}
	return res
}

// applyFile applies all edits targeting a single file.
func applyFile(root, rel string, edits []Edit) []Outcome {
	abs, err := safeJoin(root, rel)
	if err != nil {
		return failAll(edits, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return failAll(edits, fmt.Errorf("read %s: %w", rel, err))
	}

	lines, eol, trailingNL := splitLines(data)

	// highest line first so earlier replacements don't shift later indices.
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Line > edits[j].Line })

	outcomes := make([]Outcome, 0, len(edits))
	applied := 0
	for _, e := range edits {
		if e.Line < 1 || e.Line > len(lines) {
			outcomes = append(outcomes, Outcome{Key: e.Key, Status: StatusOutOfBounds,
				Err: fmt.Errorf("%s:%d out of range (file has %d lines)", rel, e.Line, len(lines))})
			continue
		}
		repl := strings.Split(strings.ReplaceAll(e.Replacement, "\r\n", "\n"), "\n")
		idx := e.Line - 1
		next := make([]string, 0, len(lines)-1+len(repl))
		next = append(next, lines[:idx]...)
		next = append(next, repl...)
		next = append(next, lines[idx+1:]...)
		lines = next
		outcomes = append(outcomes, Outcome{Key: e.Key, Status: StatusApplied})
		applied++
	}

	if applied == 0 {
		return outcomes
	}

	out := strings.Join(lines, eol)
	if trailingNL {
		out += eol
	}
	if err := fsutil.AtomicWriteFile(abs, []byte(out)); err != nil {
		werr := fmt.Errorf("write %s: %w", rel, err)
		for i := range outcomes {
			if outcomes[i].Status == StatusApplied {
				outcomes[i] = Outcome{Key: outcomes[i].Key, Status: StatusError, Err: werr}
			}
		}
	}
	return outcomes
}

// splitLines splits file bytes into logical lines without their terminators,
// reporting the dominant EOL ("\r\n" if any CRLF is present, else "\n") and
// whether the file ended with a newline. CR is stripped per line so CRLF files
// round-trip when rejoined with the reported EOL.
func splitLines(data []byte) (lines []string, eol string, trailingNL bool) {
	s := string(data)
	eol = "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		eol = "\r\n"
	}
	trailingNL = strings.HasSuffix(s, "\n")
	if trailingNL {
		s = s[:len(s)-1] // drop the final terminator so Split yields no trailing ""
	}
	if s == "" {
		return nil, eol, trailingNL
	}
	parts := strings.Split(s, "\n")
	for i := range parts {
		parts[i] = strings.TrimSuffix(parts[i], "\r")
	}
	return parts, eol, trailingNL
}

// safeJoin resolves a repo-root-relative path against root, rejecting absolute
// paths and any path that escapes root after cleaning (defense against ".." and
// crafted listings). Best-effort under a trusted working tree, not TOCTOU-proof.
func safeJoin(root, rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute path not allowed: %s", rel)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	target, err := filepath.Abs(filepath.Join(rootAbs, clean))
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	within, err := filepath.Rel(rootAbs, target)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	if within == ".." || strings.HasPrefix(within, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root: %s", rel)
	}
	return target, nil
}

// failAll marks every edit with the same error (file could not be opened/located).
func failAll(edits []Edit, err error) []Outcome {
	outcomes := make([]Outcome, len(edits))
	for i, e := range edits {
		outcomes[i] = Outcome{Key: e.Key, Status: StatusError, Err: err}
	}
	return outcomes
}
