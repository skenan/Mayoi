package symbols

import "testing"

func TestDefinitions(t *testing.T) {
	for _, tc := range []struct {
		name, text, find string
		assign           bool
		want             []int // the lines defining find
	}{
		{"go func", "package p\n\nfunc load(x int) error {\n\treturn load(x)\n}\n", "load", false, []int{2}},
		{"go method", "func (a *app) open(p string) {}\nfunc main() { a.open(p) }\n", "open", false, []int{0}},
		{"go type", "type Config struct {\n\tName string\n}\nvar c Config\n", "Config", false, []int{0}},
		{"go var and short", "var total int\nfunc f() {\n\tn, err := g()\n\tfor i, x := range xs {\n\t}\n\ttotal = n\n}\n", "err", false, []int{2}},
		{"go range", "for i, x := range xs {\n}\n", "x", false, []int{0}},
		{"go group", "const (\n\tmaxLines = 10\n\tminLines = 1\n)\nvar y = maxLines\n", "maxLines", false, []int{1}},
		{"go field", "type T struct {\n\tA, B int\n\tName string // the name\n}\nfunc f(t T) string { return t.Name }\n", "Name", false, []int{2}},
		{"ts const and function", "export const limit = 5\nexport async function fetchAll() {\n  return limit\n}\n", "limit", false, []int{0}},
		{"ts typed const", "export const defaultHomeCards: HomeCardDef[] = [\n  { id: 1 },\n];\nconst ids = defaultHomeCards.map(f)\n", "defaultHomeCards", false, []int{0}},
		{"ts typed object", "let opts: { a: number } = { a: 1 }\n", "opts", false, []int{0}},
		{"ts destructure", "const { a, b: renamed } = obj\nuse(renamed)\n", "renamed", false, []int{0}},
		{"ts class method", "class Store {\n  async save(item: Item): Promise<void> {\n  }\n  run() {\n    this.save(x)\n  }\n}\n", "save", false, []int{1}},
		{"js call is no method", "describe('x', () => {\n  foo(function () {\n  })\n  if (foo(a)) {\n  }\n})\n", "foo", false, nil},
		{"java method", "public class A {\n  public static int count(List<String> xs) {\n    return count(xs);\n  }\n}\n", "count", false, []int{1}},
		{"rust let mut", "fn main() {\n    let mut total = 0;\n    total += 1;\n}\n", "total", false, []int{1}},
		{"python", "MAX = 3\n\ndef run(n):\n    x = MAX\n    return MAX == x\n", "MAX", true, []int{0}},
		{"no assignment without assign", "x = 1\nprint(x)\n", "x", false, nil},
		{"whole word", "func loader() {}\n", "load", false, nil},
		{"function literal", "func() { Close() }()\nfunc Close() {}\n", "Close", false, []int{1}},
	} {
		var got []int
		for _, d := range Definitions(tc.text, tc.find, tc.assign) {
			got = append(got, d.Line)
		}
		if len(got) != len(tc.want) {
			t.Errorf("%s: lines %v, want %v", tc.name, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: lines %v, want %v", tc.name, got, tc.want)
				break
			}
		}
	}
}

func TestDefinitionColumn(t *testing.T) {
	d := Definitions("\tvar 名前, total = 1, 2\n", "total", false)
	if len(d) != 1 || d[0].Col != 9 || d[0].Kind != "variable" {
		t.Errorf("got %+v", d)
	}
}
