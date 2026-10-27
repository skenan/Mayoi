package main

import (
	"bytes"
	"cmp"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/bg"
	"mayoi/internal/remote"
	"mayoi/internal/symbols"
	"mayoi/internal/workspace"
	"mayoi/locales"
)

// maxDefinitions is how many definitions of a name go to definition
// lists at most.
const maxDefinitions = 100

// langFamilies are the extensions of files whose names one another's
// code calls: go to definition looks in those of the file's family.
var langFamilies = [][]string{
	{".go"},
	{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts", ".vue", ".svelte"},
	{".py", ".pyi"},
	{".rb"},
	{".rs"},
	{".java", ".kt", ".kts", ".scala"},
	{".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".hh", ".m", ".mm"},
	{".cs"},
	{".swift"},
	{".php"},
	{".lua"},
	{".sh", ".bash", ".zsh"},
}

// assignDefines are the extensions of the languages where an assignment
// defines a name.
var assignDefines = []string{".py", ".pyi", ".rb", ".sh", ".bash", ".zsh", ".lua"}

// langFamily is the extensions of the family of a file's, its own alone
// for one of none.
func langFamily(name string) []string {
	ext := strings.ToLower(path.Ext(name))
	for _, f := range langFamilies {
		if slices.Contains(f, ext) {
			return f
		}
	}
	return []string{ext}
}

// defHit is a definition of the name looked for: its file, and where.
type defHit struct {
	path, rel string
	symbols.Definition
}

// defPicker is the box of the definitions of a name, when there are
// several to choose from.
type defPicker struct {
	open    bool
	name    string
	hits    []defHit
	query   string
	results []workspace.Match
	shown   string
	sel     int
	moved   bool
	// gen tells the definitions of the last look from those of an older.
	gen int
}

// goToDefinition goes to the definition of the name at the caret, or
// selected: at once when there is one, else from a box of them all.
func (a *app) goToDefinition() { a.editActive(a.definitionAtCaret)() }

// definitionAtCaret looks for the definition of the name at d's caret,
// now that the selection is known.
func (a *app) definitionAtCaret(d *editor) {
	snap := d.buf.Snapshot()
	name := wordAt(snap, d.sel[0], d.sel[1])
	if name == "" {
		return
	}
	caretLine := snap.LineAt(min(d.sel[1], snap.Len()))
	family := langFamily(d.path)
	assign := slices.Contains(assignDefines, strings.ToLower(path.Ext(d.path)))
	// The document as it is, edits and all, then the folder's files of its
	// family, those open as they are.
	var hits []defHit
	for _, def := range symbols.Definitions(d.buf.String(), name, assign) {
		hits = append(hits, defHit{path: d.path, Definition: def})
	}
	if rel, ok := a.relOf(d.path); ok {
		for i := range hits {
			hits[i].rel = rel
		}
	}
	a.defs.gen++
	gen := a.defs.gen
	a.indexStale = a.indexStale || a.index == nil
	a.refreshIndex()
	texts := map[string]string{}
	for _, o := range a.docs {
		if rel, ok := a.relOf(o.path); ok && o.dirty() {
			texts[rel] = o.buf.String()
		}
	}
	root, here, index := a.root, d.path, a.index
	own, _ := a.relOf(here)
	bg.Go(func() {
		defer recoverCrash()
		// The files, listed here when the index is not yet.
		if index == nil && root != "" {
			index = workspace.Index(root)
		}
		var rels []string
		for _, rel := range index {
			if rel != own && slices.Contains(family, strings.ToLower(path.Ext(rel))) {
				rels = append(rels, rel)
			}
		}
		found := findDefinitions(root, rels, texts, name, assign)
		a.post(func() {
			if gen != a.defs.gen {
				return
			}
			all := append(hits, found...)
			sortDefinitions(all, here, caretLine)
			a.showDefinitions(name, all, here, caretLine)
		})
	})
}

// wordAt is the name at the caret, or the one selected on a line: "" for
// none.
func wordAt(snap ui.TextSnapshot, anchor, caret int) string {
	n := snap.Len()
	a0, b0 := min(anchor, caret, n), min(max(anchor, caret), n)
	line := snap.LineAt(a0)
	s, e := snap.LineRange(line)
	if a0 != b0 {
		if snap.LineAt(b0) != line {
			return ""
		}
		return strings.TrimSpace(snap.Slice(a0, b0))
	}
	runes := []rune(snap.Slice(s, e))
	i, j := a0-s, a0-s
	for i > 0 && (isWordRune(runes[i-1]) || runes[i-1] == '$') {
		i--
	}
	for j < len(runes) && (isWordRune(runes[j]) || runes[j] == '$') {
		j++
	}
	if i == j {
		return ""
	}
	return string(runes[i:j])
}

// findDefinitions finds the definitions of name in the files rels of
// root, of the text in texts for those open. On a host, rg or grep there
// first finds the files that hold the name. It may run on any goroutine.
func findDefinitions(root string, rels []string, texts map[string]string, name string, assign bool) []defHit {
	var (
		mu   sync.Mutex
		out  []defHit
		next atomic.Int64
		wg   sync.WaitGroup
	)
	look := func(rel, text string) {
		defs := symbols.Definitions(text, name, assign)
		if len(defs) == 0 {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, def := range defs {
			out = append(out, defHit{path: inDir(root, rel), rel: rel, Definition: def})
		}
	}
	if c, rroot, ok := remote.Lookup(root); ok {
		if hits, ok := grepFiles(c, rroot, &grepQuery{query: name, matchCase: true}); ok {
			rels = slices.DeleteFunc(rels, func(rel string) bool { _, open := texts[rel]; return !open && !hits[rel] })
		}
		var toRead []string
		for _, rel := range rels {
			if text, ok := texts[rel]; ok {
				look(rel, text)
			} else {
				toRead = append(toRead, rel)
			}
		}
		for i := 0; i < len(toRead); i += remoteReadBatch {
			part := toRead[i:min(i+remoteReadBatch, len(toRead))]
			if c.ReadFiles(rroot, part, maxSearchBytes, func(rel string, data []byte) {
				if !isBinary(data) {
					look(rel, string(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))))
				}
			}) != nil {
				break
			}
		}
		return out
	}
	for range max(1, min(8, len(rels))) {
		wg.Go(func() {
			defer recoverCrash()
			for {
				i := int(next.Add(1) - 1)
				if i >= len(rels) {
					return
				}
				text, ok := texts[rels[i]]
				if !ok {
					text, ok = readSearchText(filepath.Join(root, filepath.FromSlash(rels[i])))
				}
				if ok {
					look(rels[i], text)
				}
			}
		})
	}
	wg.Wait()
	return out
}

// sortDefinitions puts the definitions nearest the caret first: those of
// its file, the nearest above it first, then those of its folder, then
// the rest by path.
func sortDefinitions(hits []defHit, here string, caretLine int) {
	dir := filepath.Dir(here)
	rank := func(h defHit) int {
		switch {
		case h.path == here && h.Line <= caretLine:
			return 0
		case h.path == here:
			return 1
		case filepath.Dir(h.path) == dir:
			return 2
		}
		return 3
	}
	slices.SortStableFunc(hits, func(x, y defHit) int {
		if c := cmp.Compare(rank(x), rank(y)); c != 0 {
			return c
		}
		if x.path == here && rank(x) == 0 {
			return cmp.Compare(y.Line, x.Line)
		}
		return cmp.Or(cmp.Compare(x.rel, y.rel), cmp.Compare(x.Line, y.Line))
	})
}

// showDefinitions goes to the one definition of name, or shows the box
// of them, but the one at the caret, which is where it is already. A
// variable defined above the caret in its file is the one it means, as
// its scope holds it, rather than the others of its name.
func (a *app) showDefinitions(name string, hits []defHit, here string, caretLine int) {
	if len(hits) > 1 {
		hits = slices.DeleteFunc(hits, func(h defHit) bool { return h.path == here && h.Line == caretLine })
	}
	switch {
	case len(hits) == 0:
		a.tell(note{message: locales.DefinitionNone(name)})
	case len(hits) == 1, hits[0].path == here && hits[0].Line < caretLine && hits[0].Kind == "variable":
		a.openDefinition(name, hits[0])
	default:
		p := &a.defs
		p.open, p.name, p.hits, p.query, p.shown, p.sel = true, name, hits[:min(len(hits), maxDefinitions)], "", "\x00", 0
	}
}

// openDefinition shows a definition in the editor, its name selected.
func (a *app) openDefinition(name string, h defHit) {
	a.openText(func() { a.openFile(h.path, true) })
	a.selectMatch(h.path, searchMatch{line: h.Line, col: h.Col, text: name})
}

// definitionsView is the box of the definitions of a name, by their
// files, which the query matches.
func (a *app) definitionsView(c *ui.Context) {
	q := &a.defs
	if !q.open {
		return
	}
	p := a.pal
	if q.shown != q.query {
		labels := make([]string, len(q.hits))
		for i, h := range q.hits {
			labels[i] = cmp.Or(h.rel, filepath.Base(h.path))
		}
		q.results = workspace.Fuzzy(labels, q.query, len(labels))
		if q.query == "" {
			q.results = make([]workspace.Match, len(q.hits))
			for i := range q.hits {
				q.results[i] = workspace.Match{Index: i}
			}
		}
		q.shown, q.sel = q.query, 0
	}
	choose := func(i int) {
		if i < 0 || i >= len(q.results) {
			return
		}
		q.open = false
		a.openDefinition(q.name, q.hits[q.results[i].Index])
	}
	ui.Overlay(c, func() {
		back := a.dialogBack(c, "definitions")
		back.Children(func() {
			box := a.floating(c, ui.Column(c).Width(680).MaxWidthPercent(92).Margin(titleBarH+8, 0, 0, 0).Radius(10), true).Clip()
			box.Role(ui.RoleDialog).Label(locales.DefinitionLabel(q.name))
			box.Children(func() {
				ui.Row(c).Padding(10, 14).Gap(10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
					ui.Icon(c, iconFunction).FontSize(15).TextColor(p.muted)
					in := a.textKeys(ui.TextInputBase(c.Key("definitions-input"), &q.query)).Grow(1).FontSize(14).AutoFocus().
						Placeholder(locales.DefinitionLabel(q.name)).Label(locales.DefinitionLabel(q.name))
					if in.Shortcut(0, ui.KeyDown) {
						q.sel, q.moved = min(q.sel+1, len(q.results)-1), true
					}
					if in.Shortcut(0, ui.KeyUp) {
						q.sel, q.moved = max(q.sel-1, 0), true
					}
					if in.Submitted() {
						choose(q.sel)
					}
				})
				ui.Scroll(c.Key("definitions")).MaxHeight(380).Padding(6).Children(func() {
					for i, m := range q.results {
						h := q.hits[m.Index]
						row := ui.ButtonBase(c.Key(fmt.Sprint("def:", h.path, ":", h.Line))).Padding(5, 10).Gap(10).Radius(6).
							Justify(ui.Start).AlignItems(ui.Center).Label(h.Text)
						if i != q.sel && row.Hovered() {
							row.Background(p.hover)
						}
						if i == q.sel && q.moved {
							row.ScrollIntoView()
						}
						if row.Clicked() {
							choose(i)
						}
						row.Transition(hoverFade)
						row.Children(func() {
							a.pickMark(c, i == q.sel, "definitions")
							icon, col := a.symbolIcon(h.Kind)
							ui.Icon(c, icon).FontSize(14).TextColor(col)
							ui.Text(c, h.Text).FontSize(12.5).Font(a.font.family).TextColor(p.text).SingleLine().Shrink(1).Grow(1).MinWidth(0)
							ui.Text(c, fmt.Sprint(cmp.Or(h.rel, filepath.Base(h.path)), ":", h.Line+1)).FontSize(11.5).TextColor(p.faint).SingleLine().Shrink(0)
						})
					}
				})
				q.moved = false
				a.quickFooter(c, false, locales.QuickGoToAction(), "", false)
			})
			if box.PressedOutside() || back.OverlayShortcut(0, ui.KeyEscape) {
				q.open = false
				a.focusColumn()
			}
		})
	})
}
