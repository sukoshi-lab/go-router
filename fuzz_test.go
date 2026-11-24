package router

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// fuzzPattern decodes a pattern from data, along with a regexp matching the
// same paths and capturing its params in order: literal chars, slashes, params
// with an optional literal suffix, and a wildcard ending the pattern. A segment
// holds at most one param or wildcard, as the syntax reads anything after a
// param up to the next slash as its literal suffix.
func fuzzPattern(data []byte) (pattern string, ref *regexp.Regexp, names []string, rest []byte) {
	var p, r strings.Builder
	p.WriteByte('/')
	r.WriteString("^/")
	segParam := false
	for n := 0; len(data) > 0 && n < 8; n++ {
		c := data[0]
		data = data[1:]
		switch c % 8 {
		case 0:
			n = 8 // end of pattern
		case 1, 2, 3:
			lit := string("ab.-"[c/8%4])
			p.WriteString(lit)
			r.WriteString(regexp.QuoteMeta(lit))
		case 4:
			p.WriteByte('/')
			r.WriteByte('/')
			segParam = false
		case 5, 6:
			if segParam {
				continue
			}
			name := fmt.Sprintf("p%d", n)
			names = append(names, name)
			p.WriteString("{" + name + "}")
			r.WriteString("([^/]+)")
			if suffix := []string{"", ".json", "-v1"}[c/8%3]; suffix != "" {
				p.WriteString(suffix)
				r.WriteString(regexp.QuoteMeta(suffix))
			}
			segParam = true
		case 7:
			if segParam {
				continue
			}
			name := fmt.Sprintf("w%d", n)
			names = append(names, name)
			p.WriteString("{" + name + ":*}")
			r.WriteString("(.*)")
			n = 8
		}
	}
	r.WriteString("$")
	return p.String(), regexp.MustCompile(r.String()), names, data
}

func fuzzPath(data []byte) string {
	const alphabet = "ab.-/jsonv1"
	var b strings.Builder
	b.WriteByte('/')
	for _, c := range data {
		b.WriteByte(alphabet[int(c)%len(alphabet)])
	}
	return b.String()
}

// FuzzRouter checks the router against regexps, one pattern at a time, and
// Walk over a tree of conflicting patterns against the patterns alone.
func FuzzRouter(f *testing.F) {
	f.Add([]byte{5, 4, 1}, []byte("a/a"))
	f.Add([]byte{1, 4, 5 + 8, 0, 5, 0, 7}, []byte("a/1.json"))
	f.Add([]byte{1, 9, 4, 5 + 16, 4, 1, 0, 1, 9, 4, 7}, []byte("ab/x-v1/a"))
	f.Fuzz(func(t *testing.T, patternData, pathData []byte) {
		type entry struct {
			pattern string
			ref     *regexp.Regexp
			names   []string
			alone   *Router[int]
		}
		var entries []entry
		seen := map[string]bool{}
		for len(patternData) > 0 && len(entries) < 4 {
			var e entry
			e.pattern, e.ref, e.names, patternData = fuzzPattern(patternData)
			if seen[e.pattern] {
				continue
			}
			seen[e.pattern] = true
			e.alone = NewRouter[int]()
			if err := e.alone.Set(e.pattern, len(entries)+1); err != nil {
				t.Fatalf("Set(%q): %v", e.pattern, err)
			}
			entries = append(entries, e)
		}
		path := fuzzPath(pathData)

		all := NewRouter[int](AllowConflict())
		want := map[int]bool{}
		for i, e := range entries {
			if err := all.Set(e.pattern, i+1); err != nil {
				t.Fatalf("Set(%q) with conflicts allowed, after %v: %v", e.pattern, entries[:i], err)
			}
			ps := map[string]string{}
			hit := e.alone.GetParam(path, ps) != 0
			m := e.ref.FindStringSubmatch(path)
			if hit != (m != nil) {
				t.Fatalf("%q on %q: router hit %v, regexp %s hit %v", e.pattern, path, hit, e.ref, m != nil)
			}
			if !hit {
				continue
			}
			want[i+1] = true
			for j, name := range e.names {
				if ps[name] != m[j+1] {
					t.Fatalf("%q on %q: param %s = %q, regexp %s captured %q", e.pattern, path, name, ps[name], e.ref, m[j+1])
				}
			}
			if w := walk(e.alone, path); len(w) != 1 || w[0] != i+1 {
				t.Fatalf("%q on %q: Walk alone = %v", e.pattern, path, w)
			}
		}

		got := map[int]bool{}
		for _, id := range walk(all, path) {
			if got[id] {
				t.Fatalf("path %q: %q walked twice", path, entries[id-1].pattern)
			}
			got[id] = true
		}
		for id := range want {
			if !got[id] {
				t.Fatalf("path %q: Walk missed %q, patterns %v", path, entries[id-1].pattern, entries)
			}
		}
		for id := range got {
			if !want[id] {
				t.Fatalf("path %q: Walk hit extra %q, patterns %v", path, entries[id-1].pattern, entries)
			}
		}
		if id := all.Get(path); id == 0 && len(want) != 0 || id != 0 && !want[id] {
			t.Fatalf("path %q: Get = %d, want one of %v, patterns %v", path, id, want, entries)
		}
	})
}

// fuzzAnyPattern decodes a pattern from data with no regard to its validity:
// names collide, params share a segment, wildcards may come anywhere.
func fuzzAnyPattern(data []byte) (pattern string, rest []byte) {
	var p strings.Builder
	p.WriteByte('/')
	for n := 0; len(data) > 0 && n < 10; n++ {
		c := data[0]
		data = data[1:]
		name := string("ab"[c/16%2])
		switch c % 10 {
		case 0:
			n = 10 // end of pattern
		case 1, 2, 3:
			p.WriteByte("ab.-"[c/16%4])
		case 4:
			p.WriteByte('/')
		case 5:
			p.WriteString("{" + name + "}")
		case 6:
			p.WriteString("{" + name + ":*}")
		case 7:
			p.WriteString("{" + name + ":" + []string{"[0-9]+", "[a-z]*", ".*", "a|b", "[^/]+"}[c/32%5] + "}")
		case 8:
			p.WriteString(".json")
		case 9:
			p.WriteString("-v1")
		}
	}
	return p.String(), data
}

// FuzzAllowConflict checks that a router allowing conflicts takes every valid
// pattern, and that Walk over it hits exactly the patterns matching alone.
func FuzzAllowConflict(f *testing.F) {
	f.Add([]byte{5, 4, 5 + 16, 0, 5, 8, 0, 7, 0, 6}, []byte("1/a.json"))
	f.Add([]byte{7, 0, 7 + 32, 0, 7 + 64, 0, 6 + 16}, []byte("ab"))
	f.Fuzz(func(t *testing.T, patternData, pathData []byte) {
		all := NewRouter[int](AllowConflict())
		var patterns []string
		var alone []*Router[int]
		seen := map[string]bool{}
		for len(patternData) > 0 && len(patterns) < 6 {
			var p string
			p, patternData = fuzzAnyPattern(patternData)
			if seen[p] {
				continue
			}
			seen[p] = true
			r := NewRouter[int]()
			if r.Set(p, len(patterns)+1) != nil {
				continue // invalid on its own
			}
			if err := all.Set(p, len(patterns)+1); err != nil {
				t.Fatalf("Set(%q) after %q: %v", p, patterns, err)
			}
			patterns = append(patterns, p)
			alone = append(alone, r)
		}
		path := fuzzPath(pathData)

		want := map[int]bool{}
		for i, r := range alone {
			if r.Get(path) != 0 {
				want[i+1] = true
			}
		}
		got := map[int]bool{}
		for _, id := range walk(all, path) {
			if got[id] {
				t.Fatalf("path %q: %q walked twice, patterns %q", path, patterns[id-1], patterns)
			}
			got[id] = true
		}
		for id := range want {
			if !got[id] {
				t.Fatalf("path %q: Walk missed %q, patterns %q", path, patterns[id-1], patterns)
			}
		}
		for id := range got {
			if !want[id] {
				t.Fatalf("path %q: Walk hit extra %q, patterns %q", path, patterns[id-1], patterns)
			}
		}
		if id := all.Get(path); id == 0 && len(want) != 0 || id != 0 && !want[id] {
			t.Fatalf("path %q: Get = %d, want one of %v, patterns %q", path, id, want, patterns)
		}
	})
}
