package symbols

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Definition is a line of a text that defines a name: where, the line's
// text, trimmed, and the kind of what it defines.
type Definition struct {
	Line int
	Col  int // where the name starts on its line, in runes
	Text string
	Kind string
}

var (
	// varDecl finds the declarations of variables and constants: let,
	// const, var and the like, then the names.
	varDecl = regexp.MustCompile(`^\s*(?:(?:export|pub(?:\([^)]*\))?|public|private|protected|static|declare|readonly)\s+)*(?:var|let|const|val|local|static)\s+(.*)$`)
	// goGroup opens a group of Go's declarations, as const (.
	goGroup = regexp.MustCompile(`^\s*(?:const|var|type)\s*\($`)
	// structOpen opens the fields of a struct or of an interface.
	structOpen = regexp.MustCompile(`\b(?:struct|interface)\s*\{\s*$`)
	// classMethod finds a method of a class without a keyword, as
	// JavaScript, TypeScript, Java and C# write them: the name and its
	// parameters, then the opening brace.
	classMethod = regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|async|get|set|readonly|override|abstract|final|virtual|synchronized|[\w$<>\[\],.?]+)\s+)*\*?([\w$]+)\s*(?:<[^>]*>)?\s*\([^;()]*\)\s*(?::[^{;=]*|throws [^{;]*)?\{\s*$`)
	// assignment finds a name given a value, which defines it in Python,
	// Ruby or a shell.
	assignment = regexp.MustCompile(`^\s*([\w$]+)\s*(?::[^=]*)?=[^=]`)
)

// notTypes are the words before a name and its parameters that make a
// call of it, not a method: return f(x) {…} is no declaration.
var notTypes = map[string]bool{
	"return": true, "new": true, "else": true, "throw": true, "await": true, "case": true, "yield": true,
	"go": true, "defer": true, "delete": true, "typeof": true, "not": true, "and": true, "or": true,
	"in": true, "if": true, "for": true, "while": true, "switch": true, "catch": true, "echo": true, "print": true,
}

// Definitions finds the lines of text that define name, in most
// languages: a function, a type or a class of that name, a variable or a
// constant, a field of a struct, a method of a class. assign has an
// assignment define a name too, as in Python.
func Definitions(text, name string, assign bool) []Definition {
	if name == "" || !strings.Contains(text, name) {
		return nil
	}
	var out []Definition
	group, inStruct := false, false
	structIndent := 0
	line := 0
	for rest := text; rest != ""; line++ {
		l := rest
		if i := strings.IndexByte(rest, '\n'); i >= 0 {
			l, rest = rest[:i], rest[i+1:]
		} else {
			rest = ""
		}
		if len(l) > 400 {
			continue
		}
		indent := len(l) - len(strings.TrimLeft(l, " \t"))
		trimmed := strings.TrimSpace(l)
		// The blocks of Go's groups and of structs, which name what they
		// define first on their lines.
		switch {
		case group && strings.HasPrefix(trimmed, ")"):
			group = false
			continue
		case inStruct && strings.HasPrefix(trimmed, "}") && indent <= structIndent:
			inStruct = false
			continue
		}
		if !strings.Contains(l, name) {
			if goGroup.MatchString(l) {
				group = true
			} else if structOpen.MatchString(l) {
				inStruct, structIndent = true, indent
			}
			continue
		}
		kind := ""
		switch {
		case group:
			if firstWord(trimmed) == name {
				kind = "variable"
			}
		case inStruct:
			if names, ok := fieldNames(trimmed); ok && slices.Contains(names, name) {
				kind = "field"
			}
		}
		if kind == "" {
			kind = defines(l, name, assign)
		}
		if goGroup.MatchString(l) {
			group = true
		} else if structOpen.MatchString(l) && !inStruct {
			inStruct, structIndent = true, indent
		}
		if kind == "" {
			continue
		}
		col := 0
		if at := wordIndex(l, name); at >= 0 {
			col = utf8.RuneCountInString(l[:at])
		}
		out = append(out, Definition{Line: line, Col: col, Text: trimmed, Kind: kind})
	}
	return out
}

// defines reports the kind of what a line defines of name, "" for none.
func defines(l, name string, assign bool) string {
	if sym, ok := declare(l); ok && sym.Name == name {
		return sym.Kind
	}
	if m := varDecl.FindStringSubmatch(l); m != nil && slices.Contains(declaredNames(m[1]), name) {
		return "variable"
	}
	if lhs, _, ok := strings.Cut(l, ":="); ok && slices.Contains(shortNames(lhs), name) {
		return "variable"
	}
	if m := classMethod.FindStringSubmatch(l); m != nil && m[1] == name && !notTypes[firstWord(strings.TrimSpace(l))] {
		return "method"
	}
	if m := assignment.FindStringSubmatch(l); assign && m != nil && m[1] == name {
		return "variable"
	}
	return ""
}

// declaredNames are the names a declaration of variables gives, before
// its value: a, b of var a, b int, and those of { a, b: c } = o.
func declaredNames(decl string) []string {
	decl, _, _ = strings.Cut(decl, "=")
	decl = strings.TrimSpace(decl)
	// Braces first destructure; those after a name are of its type, as
	// const cards: Card[].
	destructure := strings.HasPrefix(decl, "{") || strings.HasPrefix(decl, "[")
	var out []string
	for part := range strings.SplitSeq(strings.Trim(strings.TrimSpace(decl), "{}[]() "), ",") {
		part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "mut "))
		if destructure {
			if _, renamed, ok := strings.Cut(part, ":"); ok {
				part = renamed
			}
		}
		out = append(out, firstWord(strings.TrimSpace(part)))
	}
	return out
}

// shortNames are the names of Go's short declaration before :=, past the
// keyword it may follow: x, err of for x, err := range.
func shortNames(lhs string) []string {
	lhs = strings.TrimSpace(lhs)
	for _, kw := range []string{"} else if ", "else if ", "if ", "for ", "switch ", "case "} {
		lhs = strings.TrimPrefix(lhs, kw)
	}
	var out []string
	for part := range strings.SplitSeq(lhs, ",") {
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

// fieldNames are the names of a field of a Go struct, of a line of its
// body: a, b of a, b int. An embedded type is no field.
func fieldNames(line string) ([]string, bool) {
	if strings.HasPrefix(line, "//") {
		return nil, false
	}
	names, typ, ok := strings.Cut(line, " ")
	if !ok || strings.TrimSpace(typ) == "" {
		return nil, false
	}
	for strings.HasSuffix(names, ",") {
		more, rest, _ := strings.Cut(strings.TrimSpace(typ), " ")
		names, typ = names+more, rest
	}
	var out []string
	for n := range strings.SplitSeq(names, ",") {
		out = append(out, strings.TrimSpace(n))
	}
	return out, true
}

// firstWord is the name s starts with.
func firstWord(s string) string {
	end := strings.IndexFunc(s, func(r rune) bool { return !isWordRune(r) && r != '$' })
	if end < 0 {
		return s
	}
	return s[:end]
}

// wordIndex is where name first stands as a whole word in s, -1 for
// nowhere.
func wordIndex(s, name string) int {
	for at := 0; at < len(s); {
		i := strings.Index(s[at:], name)
		if i < 0 {
			return -1
		}
		i += at
		before, _ := utf8.DecodeLastRuneInString(s[:i])
		after, _ := utf8.DecodeRuneInString(s[i+len(name):])
		if (i == 0 || !isWordRune(before) && before != '$') && (i+len(name) == len(s) || !isWordRune(after) && after != '$') {
			return i
		}
		at = i + len(name)
	}
	return -1
}
