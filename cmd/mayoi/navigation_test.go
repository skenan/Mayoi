package main

import "testing"

// caretSpot is where the caret of the editor shown is.
func caretSpot(t *testing.T, hs *harness) spot {
	t.Helper()
	sp, ok := hs.a.here()
	if !ok {
		t.Fatal("no editor shown")
	}
	return sp
}

// Back returns to where going to a definition left, and forward to the
// definition again.
func TestBackAndForwardOverADefinition(t *testing.T) {
	hs, d := defHarness(t)
	at(hs, d, "helper()")
	from := caretSpot(t, hs)
	hs.a.run("go.definition")
	hs.settle()
	to := caretSpot(t, hs)
	if to.path != hs.path("lib/helper.go") {
		t.Fatalf("went to %s", to.path)
	}
	hs.a.run("go.back")
	hs.settle()
	if got := caretSpot(t, hs); got != from {
		t.Fatalf("back at %+v, want %+v", got, from)
	}
	hs.a.run("go.forward")
	hs.settle()
	if got := caretSpot(t, hs); got.path != to.path || got.line != to.line {
		t.Fatalf("forward at %+v, want %+v", got, to)
	}
	// Nothing more to go forward to.
	hs.a.run("go.forward")
	hs.settle()
	if got := caretSpot(t, hs); got.path != to.path {
		t.Fatalf("forward past the last at %+v", got)
	}
}

// Going to a line is a jump too; a jump after going back forgets the
// places forward.
func TestBackFromALineAndForgetForward(t *testing.T) {
	hs, d := defHarness(t)
	hs.place(d, 0, 0)
	hs.a.goToLine(6, 1)
	hs.settle()
	hs.a.run("go.back")
	hs.settle()
	if got := caretSpot(t, hs); got.line != 0 {
		t.Fatalf("back on line %d, want 0", got.line)
	}
	hs.a.goToLine(3, 1)
	hs.settle()
	if len(hs.a.jumps.forward) != 0 {
		t.Errorf("a jump kept %d places forward", len(hs.a.jumps.forward))
	}
}
