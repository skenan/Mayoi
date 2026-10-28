package main

import (
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"mayoi/internal/remote"
)

// terminalFile opens a printed path without consulting the search index,
// so files Git ignores can be opened too. Relative paths are the shell's,
// falling back to the folder the terminal started in when it reports none.
func (a *app) terminalFile(s *session, link string) bool {
	if strings.Contains(link, "://") || strings.HasPrefix(link, "mailto:") {
		return false
	}
	dir := s.dir
	if s.term != nil {
		if reported := s.term.Dir(); filepath.IsAbs(reported) {
			dir = reported
		}
	}
	resolve := func(name string) string {
		if strings.HasPrefix(name, "~/") || strings.HasPrefix(name, `~\`) {
			name = filepath.Join(homeDir(), name[2:])
		}
		if !filepath.IsAbs(name) {
			name = filepath.Join(dir, name)
		}
		return filepath.Clean(name)
	}
	path := resolve(link)
	info, err := os.Stat(path)
	line, col := 0, 0
	if err != nil {
		// Prefer a literal filename if it exists, even one ending in :12.
		name, l, c, ok := splitPosition(link)
		if !ok {
			return false
		}
		line, col, path = l, c, resolve(name)
		info, err = os.Stat(path)
	}
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	a.openFileLine(path, line, col)
	return true
}

// remoteTerminalFile opens a path a terminal of a host printed, as
// terminalFile does one of this machine, once a stat on the host finds a
// file there. Until then it cannot tell, so it takes every path.
func (a *app) remoteTerminalFile(s *session, c *remote.Conn, link string) bool {
	if strings.Contains(link, "://") || strings.HasPrefix(link, "mailto:") || !c.Connected() {
		return false
	}
	// Relative paths are the shell's: the folder it reports, else the
	// one its process is in on the host.
	_, dir, _ := remote.Split(s.dir)
	pid := s.pid
	if s.term != nil {
		if reported := s.term.Dir(); path.IsAbs(reported) {
			dir, pid = reported, 0
		}
	}
	home := func(name string) string {
		if rest, ok := strings.CutPrefix(name, "~/"); ok {
			return path.Join(c.Info().Home, rest)
		}
		return name
	}
	whole := home(link)
	name, line, col, ok := splitPosition(link)
	paths := []string{whole}
	if ok {
		name = home(name)
		paths = append(paths, name)
	}
	go func() {
		in, infos, err := c.StatInShell(pid, dir, paths)
		a.post(func() {
			if err != nil {
				return
			}
			open := func(name string, line, col int) {
				if !path.IsAbs(name) {
					name = path.Join(in, name)
				}
				a.openFileLine(remote.Path(c.Host, name), line, col)
			}
			// Prefer a literal filename, as terminalFile does.
			if f := infos[whole]; f != nil && f.Mode().IsRegular() {
				open(whole, 0, 0)
			} else if f := infos[name]; ok && f != nil && f.Mode().IsRegular() {
				open(name, line, col)
			}
		})
	}()
	return true
}

// splitPosition parts a link at a :line or :line:column suffix, both
// 1-based, ok false when it has none.
func splitPosition(link string) (name string, line, col int, ok bool) {
	at := strings.LastIndexByte(link, ':')
	if at < 0 {
		return "", 0, 0, false
	}
	line, err := strconv.Atoi(link[at+1:])
	if err != nil || line < 0 {
		return "", 0, 0, false
	}
	name = link[:at]
	if at = strings.LastIndexByte(name, ':'); at >= 0 {
		// A column of 0, which some tools print, opens the line.
		if n, err := strconv.Atoi(name[at+1:]); err == nil && n > 0 {
			line, col, name = n, line, name[:at]
		}
	}
	if line < 1 {
		return "", 0, 0, false
	}
	return name, line, col, true
}

// openFileLine opens a file with the caret at a line and column, 1-based,
// or where it was with a line of 0. A remote file still being read takes
// the caret once it is read.
func (a *app) openFileLine(path string, line, col int) {
	if line == 0 {
		a.openFile(path, true)
		return
	}
	// The line is in the text, whatever markdownPreview says.
	a.openText(func() { a.openFile(path, true) })
	d := a.activeEditor()
	switch {
	case d == nil || d.path != path:
	case d.loading:
		d.goTo = [2]int{line, col}
	default:
		a.goToLine(line, col)
	}
}
