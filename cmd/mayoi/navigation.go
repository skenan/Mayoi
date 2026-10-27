package main

// maxJumps is how many places going back remembers.
const maxJumps = 50

// spot is a place of a file the caret was at: its line, and runes into
// it, which edits elsewhere in the file leave where they are.
type spot struct {
	path      string
	line, col int
}

// jumps are the places the caret jumped from, as going to a definition,
// which going back returns to, and those going back left, which going
// forward returns to.
type jumps struct {
	back, forward []spot
}

// here is the place of the caret of the editor shown, false for none.
func (a *app) here() (spot, bool) {
	d := a.activeEditor()
	if d == nil || d.path == "" {
		return spot{}, false
	}
	snap := d.buf.Snapshot()
	at := min(d.pending0(), snap.Len())
	line := snap.LineAt(at)
	s, _ := snap.LineRange(line)
	return spot{d.path, line, at - s}, true
}

// markJump remembers where the caret is before it jumps elsewhere, for
// going back, and forgets the places going forward would go.
func (a *app) markJump() {
	sp, ok := a.here()
	if !ok {
		return
	}
	j := &a.jumps
	if n := len(j.back); n == 0 || j.back[n-1] != sp {
		j.back = append(j.back, sp)
		if len(j.back) > maxJumps {
			j.back = j.back[1:]
		}
	}
	j.forward = nil
}

// goBack returns to the place the caret last jumped from.
func (a *app) goBack() { a.stepJump(&a.jumps.back, &a.jumps.forward) }

// goForward returns to the place going back last left.
func (a *app) goForward() { a.stepJump(&a.jumps.forward, &a.jumps.back) }

// stepJump goes to the last place of from, past the one the caret is at,
// keeping where it is in to.
func (a *app) stepJump(from, to *[]spot) {
	cur, ok := a.here()
	for len(*from) > 0 {
		sp := (*from)[len(*from)-1]
		*from = (*from)[:len(*from)-1]
		if ok && sp == cur {
			continue
		}
		if ok {
			*to = append(*to, cur)
		}
		a.goToSpot(sp)
		return
	}
}

// goToSpot shows a place of a file, with the caret there.
func (a *app) goToSpot(sp spot) {
	a.openText(func() { a.openFile(sp.path, true) })
	d := a.activeEditor()
	if d == nil || d.path != sp.path {
		return
	}
	snap := d.buf.Snapshot()
	line := min(max(sp.line, 0), snap.LineCount()-1)
	s, e := snap.LineRange(line)
	at := min(s+sp.col, e)
	d.selectRange(at, at, true)
	d.lightLanding()
}
