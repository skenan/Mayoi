package main

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/locales"
)

// defHarness is a folder of Go files that call one another's functions,
// with use.go open.
func defHarness(t *testing.T) (*harness, *editor) {
	hs := newHarness(t, false)
	for name, body := range map[string]string{
		"use.go":        "package main\n\nfunc run() {\n    helper()\n    var c Closer\n    c.Close()\n    missing()\n}\n",
		"lib/helper.go": "package main\n\nfunc helper() {}\n",
		"lib/close.go":  "package main\n\nfunc (a A) Close() {}\nfunc (b B) Close() {}\n",
		"notes.md":      "func helper() is in lib\n",
		"lib/other.go":  "package main\n\nvar c = 1\n",
	} {
		os.MkdirAll(hs.path(name)[:strings.LastIndexByte(hs.path(name), '/')], 0o755)
		if err := os.WriteFile(hs.path(name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hs.a.openFile(hs.path("use.go"), true)
	hs.settle()
	return hs, hs.a.activeEditor()
}

// at puts the caret in the first of what in d's text.
func at(hs *harness, d *editor, what string) {
	pos := len([]rune(d.buf.String()[:strings.Index(d.buf.String(), what)])) + 1
	hs.place(d, pos, pos)
}

// F12 goes to the one definition of a name in another file, at once, its
// name selected; Markdown's mention of it is none.
func TestGoToDefinition(t *testing.T) {
	hs, d := defHarness(t)
	at(hs, d, "helper()")
	hs.a.run("go.definition")
	hs.settle()
	got := hs.a.activeEditor()
	if got == nil || got.path != hs.path("lib/helper.go") {
		t.Fatalf("went to %v, want lib/helper.go", got.path)
	}
	snap := got.buf.Snapshot()
	if s := snap.Slice(min(got.sel[0], got.sel[1]), max(got.sel[0], got.sel[1])); s != "helper" || snap.LineAt(got.sel[1]) != 2 {
		t.Errorf("selected %q on line %d, want helper on line 2", s, snap.LineAt(got.sel[1]))
	}
	if hs.a.defs.open {
		t.Error("the box of definitions opened for one")
	}
}

// Several definitions of a name show in a box to choose one from.
func TestGoToOneOfDefinitions(t *testing.T) {
	hs, d := defHarness(t)
	at(hs, d, "Close()")
	hs.a.run("go.definition")
	hs.settle()
	if !hs.a.defs.open || len(hs.a.defs.hits) != 2 {
		t.Fatalf("box open %v with %d definitions, want 2", hs.a.defs.open, len(hs.a.defs.hits))
	}
	if !hs.tt.HasText("func (b B) Close() {}") {
		t.Error("the box does not show the definitions")
	}
	hs.key(0, ui.KeyDown)
	hs.key(0, ui.KeyEnter)
	if got := hs.a.activeEditor(); hs.a.defs.open || got == nil || got.path != hs.path("lib/close.go") || got.buf.Snapshot().LineAt(got.sel[1]) != 3 {
		t.Fatalf("chose the second: editor %v, want lib/close.go on line 3", got.path)
	}
}

// A name with no definition says so.
func TestNoDefinition(t *testing.T) {
	hs, d := defHarness(t)
	at(hs, d, "missing()")
	hs.a.run("go.definition")
	hs.settle()
	if got := hs.a.activeEditor(); got != d || !hs.tt.HasText(locales.DefinitionNone("missing")) {
		t.Errorf("editor %v, toast shown %v", got.path, hs.tt.HasText(locales.DefinitionNone("missing")))
	}
}

// ⌘-click on a name goes to its definition.
func TestCmdClickGoesToDefinition(t *testing.T) {
	hs, d := defHarness(t)
	hs.place(d, 0, 0)
	r, ok := hs.tt.TextCaret()
	if !ok {
		t.Fatal("no caret")
	}
	f := hs.a.font
	// "    helper()" is line 3: its name from column 4.
	hs.tt.ClickAtWith(ui.Cmd, r.X+6*f.charW, r.Y+3*f.lineH+f.lineH/2)
	hs.settle()
	if got := hs.a.activeEditor(); got == nil || got.path != hs.path("lib/helper.go") {
		t.Fatalf("⌘-click went to %v, want lib/helper.go", got.path)
	}
}

// A variable defined above the caret in its file is the one meant: F12
// goes there, though other files define its name too.
func TestGoToVariableInScope(t *testing.T) {
	hs, d := defHarness(t)
	at(hs, d, "c.Close()")
	hs.a.run("go.definition")
	hs.settle()
	if got := hs.a.activeEditor(); hs.a.defs.open || got != d || got.buf.Snapshot().LineAt(got.sel[1]) != 4 {
		t.Fatalf("box open %v, editor %v, want use.go on line 4", hs.a.defs.open, got.path)
	}
}
