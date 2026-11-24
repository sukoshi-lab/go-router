package router

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestRouterBasic(t *testing.T) {
	r := NewRouter[int]()
	r.Set("/", 1)
	r.Set("/a", 2)
	r.Set("/a/b", 3)
	r.Set("/a/b/c", 4)
	r.Set("/d", 5)
	r.Set("/d/e", 6)
	r.Set("/d/b", 7)
	for i, route := range []string{"/", "/a", "/a/b", "/a/b/c", "/d", "/d/e", "/d/b"} {
		if r.Get(route) != i+1 {
			t.Errorf("expected %d, got %d", i+1, r.Get(route))
		}
	}
}

func TestRouterOrder(t *testing.T) {
	r := NewRouter[int]()
	r.Set("/", 1)
	r.Set("/{a}", 2)
	r.Set("/a", 3)
	if hit := r.Get("/a"); hit != 3 {
		t.Errorf("expected to match literal, got %d", hit)
	}
}

// registering a pattern that equals an existing node used to index an empty path.
func TestSetPatternEqualToSplitNode(t *testing.T) {
	r := NewRouter[int]()
	for i, p := range []string{"/a/b", "/a{x:*}", "/a"} {
		if err := r.Set(p, i); err != nil {
			t.Fatalf("Set(%q): %v", p, err)
		}
	}
	for path, want := range map[string]int{"/a/b": 0, "/ax": 1} {
		if got := r.Get(path); got != want {
			t.Errorf("Get(%q) = %d, want %d", path, got, want)
		}
	}
}

func walk(r *Router[int], path string) (hits []int) {
	r.Walk(path, func(v int) bool {
		hits = append(hits, v)
		return true
	})
	return hits
}

func TestWalk(t *testing.T) {
	check := func(name string, got, want []int) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: got %v, want %v", name, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s: got %v, want %v", name, got, want)
			}
		}
	}

	r := NewRouter[int]()
	for i, p := range []string{"/{path:*}", "/ab{var}", "/absolute", "/{reg:.*}", "/abs{var}", "/{reg:([a-z]*)}"} {
		if err := r.Set(p, i+1); err != nil {
			t.Fatal(err)
		}
	}
	got := walk(r, "/absolute")
	if len(got) != 6 {
		t.Errorf("expected 6 matches, got %v", got)
	}

	r = NewRouter[int]()
	r.Set("/{path:*}", 1)
	r.Set("/payment/v1/{path:*}", 2)
	r.Set("/payment/v1/pay", 3)
	check("nested first", walk(r, "/payment/v1/pay"), []int{3, 2, 1})
	check("wildcard only", walk(r, "/payments/v1/pay"), []int{1})

	r = NewRouter[int]()
	r.Set("/a", 1)
	r.Set("/a{x:*}", 2)
	check("own handler with matching descendant", walk(r, "/a"), []int{2, 1})

	r = NewRouter[int]()
	r.Set("/x", 1)
	r.Set("/y", 2)
	check("unregistered root", walk(r, "/"), nil)
	r.Set("/", 3)
	check("registered root", walk(r, "/"), []int{3})

	check("not a path", walk(r, "x"), nil)
	check("empty path", walk(r, ""), nil)
	check("empty router", walk(NewRouter[int](), "/x"), nil)

	r = NewRouter[int]()
	r.Set("/a", 1)
	r.Set("/a/b", 2)
	r.Set("/a/{p}", 3)
	r.Set("/{q:*}", 4)
	calls := 0
	r.Walk("/a/b", func(int) bool { calls++; return false })
	if calls != 1 {
		t.Errorf("expected Walk to stop after f returned false, got %d calls", calls)
	}
}

// Walk over one tree must hit exactly the patterns that match on their own.
func TestWalkEqualsIndependentMatches(t *testing.T) {
	walkEqualsIndependent(t, []string{
		"/", "/a", "/a{x:*}", "/a/{x}", "/a/{x:*}", "/ab", "/ab{x}", "/{x}", "/{x:*}", "/{x:[a-z]+}",
		"/a/b", "/a/b/{x:*}", "/a/{x}/c", "/payment/v1/pay", "/payment/v1/{x:*}",
	})
}

func TestWalkEqualsIndependentMatchesConflict(t *testing.T) {
	walkEqualsIndependent(t, []string{
		"/a/{x}", "/a/{y}", "/a/{x}.json", "/a/{y}.xml", "/a/{x:*}", "/a/{y:*}", "/{x}", "/{y}",
		"/a/{x}/c", "/a/{y}/c", "/{x:[a-z]+}", "/a/b", "/payment/v1/{x:*}", "/payment/v1/{y:*}",
	}, AllowConflict())
}

func walkEqualsIndependent(t *testing.T, patterns []string, opts ...Option) {
	t.Helper()
	paths := []string{
		"/", "/a", "/ab", "/abc", "/a/", "/a/b", "/a/b/", "/a/b/c", "/a/b/c/d", "/a/x/c", "/b", "/zzz",
		"/a1", "/ab/c", "/payment/v1/pay", "/payment/v1/refund", "/payments/v1/pay",
		"/a/1.json", "/a/1.xml", "/a/1.txt",
	}
	rng := rand.New(rand.NewSource(1))
	for round := 0; round < 300; round++ {
		var picked []string
		for _, p := range patterns {
			if rng.Intn(2) == 0 {
				picked = append(picked, p)
			}
		}
		rng.Shuffle(len(picked), func(i, j int) { picked[i], picked[j] = picked[j], picked[i] })

		all := NewRouter[int](opts...)
		alone := map[int]*Router[int]{}
		for i, p := range picked {
			single := NewRouter[int]()
			if err := single.Set(p, i+1); err != nil {
				t.Fatalf("Set(%q): %v", p, err)
			}
			alone[i+1] = single
			if err := all.Set(p, i+1); err != nil {
				t.Fatalf("round %d: Set(%q) after %q: %v", round, p, picked[:i], err)
			}
		}
		for _, path := range paths {
			want := map[int]bool{}
			for id, single := range alone {
				if single.Get(path) != 0 {
					want[id] = true
				}
			}
			got := map[int]bool{}
			for _, id := range walk(all, path) {
				if got[id] {
					t.Fatalf("round %d, path %q: pattern %d walked twice, patterns %q", round, path, id, picked)
				}
				got[id] = true
			}
			for id := range want {
				if !got[id] {
					t.Errorf("round %d, path %q: missed %q, patterns %q", round, path, picked[id-1], picked)
				}
			}
			for id := range got {
				if !want[id] {
					t.Errorf("round %d, path %q: extra %q, patterns %q", round, path, picked[id-1], picked)
				}
			}
		}
	}
}

// a param followed by a literal in its segment used to never match.
func TestParamSuffix(t *testing.T) {
	r := NewRouter[int]()
	for i, p := range []string{"/a/{id}.json", "/b/{id}.json/c", "/c/{name}-v1", "/d/{id}/e"} {
		if err := r.Set(p, i+1); err != nil {
			t.Fatal(err)
		}
	}
	for path, want := range map[string]struct {
		hit int
		id  string
	}{
		"/a/1.json":      {1, "1"},
		"/a/1.json.json": {1, "1.json"},
		"/a/.json":       {0, ""},
		"/a/1.xml":       {0, ""},
		"/a/1.json/x":    {0, ""},
		"/a/json":        {0, ""},
		"/b/1.json/c":    {2, "1"},
		"/b/1.json":      {0, ""},
		"/c/abc-v1":      {3, "abc"},
		"/c/abc-v2":      {0, ""},
		"/d/1/e":         {4, "1"},
		"/d//e":          {0, ""}, // a param is never empty
	} {
		ps := map[string]string{}
		if got := r.GetParam(path, ps); got != want.hit {
			t.Errorf("GetParam(%q) = %d, want %d", path, got, want.hit)
		}
		if id := ps["id"] + ps["name"]; id != want.id {
			t.Errorf("GetParam(%q) param = %q, want %q", path, id, want.id)
		}
		if got := walk(r, path); (len(got) == 1 && got[0] == want.hit) != (want.hit != 0) || len(got) > 1 {
			t.Errorf("Walk(%q) = %v, want %d", path, got, want.hit)
		}
	}
}

// a regex param matches from where it starts; it used to match anywhere after.
func TestRegexParam(t *testing.T) {
	r := NewRouter[int]()
	for i, p := range []string{"/{x:[0-9]}1", "/a/{x:[0-9]+}", "/d/{x:.*}", "/n/{x:[0-9]+}1"} {
		if err := r.Set(p, i+1); err != nil {
			t.Fatal(err)
		}
	}
	for path, want := range map[string]struct {
		hit int
		x   string
	}{
		"/a1":    {0, ""},
		"/11":    {1, "1"},
		"/a/12":  {2, "12"},
		"/a/a12": {0, ""},
		"/d/x/y": {3, "x/y"}, // a regex may span slashes
		// a regex takes its leftmost-first match, never giving chars back to
		// the literal after it
		"/n/121": {0, ""},
	} {
		ps := map[string]string{}
		if got := r.GetParam(path, ps); got != want.hit || ps["x"] != want.x {
			t.Errorf("GetParam(%q) = %d, x = %q, want %d, %q", path, got, ps["x"], want.hit, want.x)
		}
	}
	if err := r.Set("/{y:[0-9]}1", 9); err == nil || !strings.Contains(err.Error(), "conflicts with") {
		t.Errorf("the same regex under another name: got %v, want ErrExprConflict", err)
	}
	if err := r.Set("/{x:[0-9]}1", 9); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Errorf("the same regex and name: got %v, want ErrConflict", err)
	}
	if err := r.Set("/{x:[a-z]}1", 9); err != nil {
		t.Errorf("another regex at the same position: %v", err)
	}
}

// params whose suffixes never end the same segment don't conflict.
func TestParamSuffixesApart(t *testing.T) {
	r := NewRouter[int]()
	for i, p := range []string{"/a/{id}.json", "/a/{id}.xml", "/a/{name}-v1", "/a/{name}-v2"} {
		if err := r.Set(p, i+1); err != nil {
			t.Fatalf("Set(%q): %v", p, err)
		}
	}
	for path, want := range map[string]int{"/a/1.json": 1, "/a/1.xml": 2, "/a/x-v1": 3, "/a/x-v2": 4, "/a/1.txt": 0} {
		if got := r.Get(path); got != want {
			t.Errorf("Get(%q) = %d, want %d", path, got, want)
		}
		if got := walk(r, path); want == 0 && len(got) != 0 || want != 0 && !reflect.DeepEqual(got, []int{want}) {
			t.Errorf("Walk(%q) = %v, want %d", path, got, want)
		}
	}
}

// params of different suffixes don't conflict, the longer suffix going first.
func TestParamSuffixPriority(t *testing.T) {
	for _, order := range [][]string{
		{"/a/{id}", "/a/{id}.json", "/a/{v}n"},
		{"/a/{v}n", "/a/{id}.json", "/a/{id}"},
		{"/a/{id}.json", "/a/{id}", "/a/{v}n"},
	} {
		r := NewRouter[string]()
		for _, p := range order {
			if err := r.Set(p, p); err != nil {
				t.Fatalf("Set(%q) in %q: %v", p, order, err)
			}
		}
		for path, want := range map[string]struct{ get, param string }{
			"/a/1.json": {"/a/{id}.json", "1"},
			"/a/1n":     {"/a/{v}n", "1"},
			"/a/1":      {"/a/{id}", "1"},
		} {
			ps := map[string]string{}
			if got := r.GetParam(path, ps); got != want.get || ps["id"]+ps["v"] != want.param {
				t.Errorf("order %q: GetParam(%q) = %q, %v, want %q, %q", order, path, got, ps, want.get, want.param)
			}
		}
	}
}

func TestUpsert(t *testing.T) {
	or := func(bit int) func(int) int { return func(old int) int { return old | bit } }
	r := NewRouter[int](AllowConflict())
	for _, c := range []struct {
		pattern string
		bit     int
	}{{"/a/{id}", 1}, {"/a/{id}", 2}, {"/a/{oid}", 4}, {"/b", 8}} {
		if err := r.Upsert(c.pattern, or(c.bit)); err != nil {
			t.Fatalf("Upsert(%q): %v", c.pattern, err)
		}
	}
	if err := r.Set("/b", 16); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Errorf("Set after Upsert of the same pattern: got %v, want ErrConflict", err)
	}
	if err := r.Upsert("/b", or(16)); err != nil {
		t.Errorf("Upsert after Set: %v", err)
	}
	got := walk(r, "/a/1")
	sort.Ints(got)
	if !reflect.DeepEqual(got, []int{3, 4}) {
		t.Errorf("Walk(/a/1) = %v, want [3 4]", got)
	}
	if got := r.Get("/b"); got != 24 {
		t.Errorf("Get(/b) = %d, want 24", got)
	}
	if err := r.Upsert("b", or(1)); err == nil {
		t.Error("Upsert of an invalid path: want an error")
	}
	if err := NewRouter[int]().Upsert("/{a}", or(1)); err != nil {
		t.Fatal(err)
	}
}

func TestAllowConflict(t *testing.T) {
	conflicting := [][2]string{
		{"/a/{id}", "/a/{oid}"},
		{"/fx/{x:*}", "/fx/{path:*}"},
		{"/a/{id}.json", "/a/{oid}.json"},
		{"/r/{x:[0-9]}", "/r/{y:[0-9]}"},
	}
	for _, pair := range conflicting {
		r := NewRouter[int]()
		if err := r.Set(pair[0], 1); err != nil {
			t.Fatal(err)
		}
		if err := r.Set(pair[1], 2); err == nil {
			t.Errorf("%q next to %q: expected a conflict without AllowConflict", pair[1], pair[0])
		}

		r = NewRouter[int](AllowConflict())
		if err := r.Set(pair[0], 1); err != nil {
			t.Fatal(err)
		}
		if err := r.Set(pair[1], 2); err != nil {
			t.Errorf("%q next to %q: %v", pair[1], pair[0], err)
		}
	}

	r := NewRouter[int](AllowConflict())
	for i, p := range []string{"/a/{id}", "/a/{oid}", "/a/{x:*}", "/a/{y:*}", "/a/{id}.json", "/a/{id}.xml"} {
		if err := r.Set(p, i+1); err != nil {
			t.Fatal(err)
		}
	}
	for path, want := range map[string][]int{
		"/a/1":      {1, 2, 3, 4},
		"/a/1/2":    {3, 4},
		"/a/1.json": {1, 2, 3, 4, 5},
		"/a/1.xml":  {1, 2, 3, 4, 6},
	} {
		got := walk(r, path)
		sort.Ints(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Walk(%q) = %v, want %v", path, got, want)
		}
	}
	if err := r.Set("/a/{id}", 5); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Errorf("the same pattern twice: got %v, want ErrConflict", err)
	}
}

// Below tests are taken from fasthttp, licensed under the BSD 3-Clause License.
type testRequests []struct {
	path       string
	nilHandler bool
	route      string
	ps         map[string]string
}

func checkRequests(t *testing.T, tree *Router[string], requests testRequests) {
	for _, request := range requests {
		params := make(map[string]string)
		handler := tree.GetParam(request.path, params)

		if handler == "" {
			if !request.nilHandler {
				t.Errorf("handle mismatch for route '%s': Expected non-nil handle", request.path)
			}
		} else if request.nilHandler {
			t.Errorf("handle mismatch for route '%s': Expected nil handle, got %s", request.path, handler)
		} else {
			if handler != request.route {
				t.Errorf("handle mismatch for route '%s': Wrong handle (%s != %s)", request.path, handler, request.route)
			}
		}
		if request.ps == nil {
			request.ps = make(map[string]string)
		}
		if !reflect.DeepEqual(params, request.ps) {
			t.Errorf("Route %s - User values == %v, want %v", request.path, params, request.ps)
		}
	}
}
func TestTreeAddAndGet(t *testing.T) {
	tree := NewRouter[string]()

	routes := [...]string{
		"/hi",
		"/contact/",
		"/co",
		"/c",
		"/a",
		"/ab",
		"/doc/",
		"/doc/go_faq.html",
		"/doc/go1.html",
		"/α",
		"/β",
		"/hello/test",
		"/hello/tooth",
		"/hello/{name}",
		"/regex/{c1:big_alt|alt|small_alt}/{rest:*}",
		// "/regex/{c2:(?<named>extra)_alt}/{rest:*}",
		"/regex/{path:*}",
		"/wildcard/sub/{rest:*}",
		"/wildcard/{rest:*}",
	}

	for _, route := range routes {
		tree.Set(route, route)
	}

	checkRequests(t, tree, testRequests{
		{"/a", false, "/a", nil},
		{"/", true, "", nil},
		{"/hi", false, "/hi", nil},
		{"/contact", true, "", nil}, // TSR
		{"/co", false, "/co", nil},
		{"/con", true, "", nil},  // key mismatch
		{"/cona", true, "", nil}, // key mismatch
		{"/no", true, "", nil},   // no matching child
		{"/ab", false, "/ab", nil},
		{"/α", false, "/α", nil},
		{"/β", false, "/β", nil},
		{"/hello/test", false, "/hello/test", nil},
		{"/hello/tooth", false, "/hello/tooth", nil},
		{"/hello/testastretta", false, "/hello/{name}", map[string]string{"name": "testastretta"}},
		{"/hello/tes", false, "/hello/{name}", map[string]string{"name": "tes"}},
		{"/hello/test/bye", true, "", nil},
		{"/regex/more_alt/hello", false, "/regex/{path:*}", map[string]string{"path": "more_alt/hello"}},
		{"/regex/small_alt/hello", false, "/regex/{c1:big_alt|alt|small_alt}/{rest:*}", map[string]string{"c1": "small_alt", "rest": "hello"}},
		{"/regex/small_alt/hello", false, "/regex/{c1:big_alt|alt|small_alt}/{rest:*}", map[string]string{"c1": "small_alt", "rest": "hello"}},
		// {"/regex/extra_alt/hello", false, "/regex/{c2:(?<named>extra)_alt}/{rest:*}", map[string]string{"c2": "extra_alt", "named": "extra", "rest": "hello"}}, // named group
		{"/wildcard/sub", false, "/wildcard/{rest:*}", map[string]string{"rest": "sub"}},
	})
}

func TestTreeWildcard(t *testing.T) {
	tree := NewRouter[string]()

	routes := [...]string{
		"/",
		"/cmd/{tool}/{sub}",
		"/cmd/{tool}/",
		"/src/{filepath:*}",
		"/src/data",
		"/search/",
		"/search/{query}",
		"/user_{name}",
		"/user_{name}/about",
		"/files/{dir}/{filepath:*}",
		"/doc/",
		"/doc/go_faq.html",
		"/doc/go1.html",
		"/info/{user}/public",
		"/info/{user}/project/{project}",
	}

	for _, route := range routes {
		tree.Set(route, route)
	}

	checkRequests(t, tree, testRequests{
		{"/", false, "/", nil},
		{"/cmd/test/", false, "/cmd/{tool}/", map[string]string{"tool": "test"}},
		{"/cmd/test", true, "", nil},
		{"/cmd/test/3", false, "/cmd/{tool}/{sub}", map[string]string{"tool": "test", "sub": "3"}},
		{"/src/", false, "/src/{filepath:*}", map[string]string{"filepath": ""}},
		{"/src/some/file.png", false, "/src/{filepath:*}", map[string]string{"filepath": "some/file.png"}},
		{"/search/", false, "/search/", nil},
		{"/search/someth!ng+in+ünìcodé", false, "/search/{query}", map[string]string{"query": "someth!ng+in+ünìcodé"}},
		{"/search/someth!ng+in+ünìcodé/", true, "", nil},
		{"/user_gopher", false, "/user_{name}", map[string]string{"name": "gopher"}},
		{"/user_gopher/about", false, "/user_{name}/about", map[string]string{"name": "gopher"}},
		{"/files/js/inc/framework.js", false, "/files/{dir}/{filepath:*}", map[string]string{"dir": "js", "filepath": "inc/framework.js"}},
		{"/info/gordon/public", false, "/info/{user}/public", map[string]string{"user": "gordon"}},
		{"/info/gordon/project/go", false, "/info/{user}/project/{project}", map[string]string{"user": "gordon", "project": "go"}},
		{"/info/gordon", true, "", nil},
	})
}

func TestTreeDuplicatePath(t *testing.T) {
	tree := NewRouter[string]()

	routes := [...]string{
		"/",
		"/doc/",
		"/src/{filepath:*}",
		"/search/{query}",
		"/user_{name}",
	}

	for _, route := range routes {
		handler := route
		recv := tree.Set(route, handler)

		if recv != nil {
			t.Fatalf("panic inserting route '%s': %v", route, recv)
		}

		// Add again
		recv = tree.Set(route, handler)
		if recv == nil {
			t.Fatalf("no panic while inserting duplicate route '%s", route)
		}
	}

	checkRequests(t, tree, testRequests{
		{"/", false, "/", nil},
		{"/doc/", false, "/doc/", nil},
		{"/src/some/file.png", false, "/src/{filepath:*}", map[string]string{"filepath": "some/file.png"}},
		{"/search/someth!ng+in+ünìcodé", false, "/search/{query}", map[string]string{"query": "someth!ng+in+ünìcodé"}},
		{"/user_gopher", false, "/user_{name}", map[string]string{"name": "gopher"}},
	})
}

func TestEmptyWildcardName(t *testing.T) {
	tree := NewRouter[string]()

	routes := [...]string{
		"/user{}",
		"/user{}/",
		"/cmd/{}/",
		"/src/{:*}",
	}

	for _, route := range routes {
		recv := tree.Set(route, route)
		if recv == nil {
			t.Errorf("no panic while inserting route with empty expression name '%s", route)
		}
	}
}

func TestTreeDoubleWildcard(t *testing.T) {
	const panicMsg = "the expressions must be separated by at least 1 char"

	routes := [...]string{
		"/{foo}{bar}",
		"/{foo}{bar}/",
		"/{foo}{bar:*}",
	}

	for _, route := range routes {
		tree := NewRouter[string]()
		recv := tree.Set(route, route)

		if recv == nil || !strings.Contains(recv.Error(), panicMsg) {
			t.Fatalf(`"Expected panic "%s" for route '%s', got "%v"`, panicMsg, route, recv)
		}
	}
}

func TestTreeTrailingSlashRedirect(t *testing.T) {
	tree := NewRouter[string]()

	routes := [...]string{
		"/hi",
		"/b/",
		"/search/{query}",
		"/cmd/{tool}/",
		"/src/{filepath:*}",
		"/x",
		"/x/y",
		"/y/",
		"/y/z",
		"/0/{id}",
		"/0/{id}/1",
		"/1/{id}/",
		"/1/{id}/2",
		"/aa",
		"/a/",
		"/admin",
		"/admin/{category}",
		"/admin/{category}/{page}",
		"/doc",
		"/doc/go_faq.html",
		"/doc/go1.html",
		"/no/a",
		"/no/b",
		"/api/hello/{name}",
		"/foo/data/hello",
		"/foo/",
	}
	for _, route := range routes {
		recv := tree.Set(route, route)
		if recv != nil {
			t.Fatalf("panic inserting route '%s': %v", route, recv)
		}
	}

	tsrRoutes := [...]string{
		"/hi/",
		"/b",
		"/search/gopher/",
		"/cmd/vet",
		"/src",
		"/x/",
		"/y",
		"/0/go/",
		"/1/go",
		"/a",
		"/admin/",
		"/admin/config/",
		"/admin/config/permissions/",
		"/doc/",
		"/foo/data/hello/",
		"/foo",
	}
	for _, route := range tsrRoutes {
		handler := tree.Get(route)
		if handler != "" {
			t.Fatalf("non-nil handler for TSR route '%s", route)
		}
	}

	noTsrRoutes := [...]string{
		"/",
		"/no",
		"/no/",
		"/_",
		"/_/",
		"/api/world/abc",
	}
	for _, route := range noTsrRoutes {
		handler := tree.Get(route)
		if handler != "" {
			t.Fatalf("non-nil handler for No-TSR route '%s", route)
		}
	}
}

func TestTreeRootTrailingSlashRedirect(t *testing.T) {
	tree := NewRouter[string]()

	recv := tree.Set("/{test}", "/{test}")

	if recv != nil {
		t.Fatalf("panic inserting test route: %v", recv)
	}

	handler := tree.Get("/")
	if handler != "" {
		t.Fatalf("non-nil handler")
	}
}

func TestTreeWildcardConflictEx(t *testing.T) {
	router := NewRouter[string]()
	routes := [...]string{
		"/con{tact}",
		"/who/are/{you:*}",
		"/who/foo/hello",
		"/whose/{users}/{name}",
		"/{filepath:*}",
		"/{id}",
	}
	for _, route := range routes {
		if err := router.Set(route, route); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	}

	conflicts := []struct {
		route string

		wantErr     bool
		wantErrText string
	}{
		{route: "/who/are/foo", wantErr: false},
		{route: "/who/are/foo/", wantErr: false},
		{route: "/who/are/foo/bar", wantErr: false},
		{route: "/conxxx", wantErr: false},
		{route: "/conooo/xxx", wantErr: false},
		{
			route:       "invalid/data",
			wantErr:     true,
			wantErrText: "path must begin with '/' in path 'invalid/data'",
		},
		{
			route:       "/con{tact}",
			wantErr:     true,
			wantErrText: "a handler is already registered for path '/con{tact}'",
		},
		{
			route:       "/con{something}",
			wantErr:     true,
			wantErrText: "path '/con{something}' conflicts with existing wildcard or param '{tact}'",
		},
		{
			route:       "/who/are/{you:*}",
			wantErr:     true,
			wantErrText: "a handler is already registered for path '/who/are/{you:*}'",
		},
		{
			route:       "/who/are/{me:*}",
			wantErr:     true,
			wantErrText: "path '/who/are/{me:*}' conflicts with existing wildcard or param '{you:*}'",
		},
		{
			route:       "/who/foo/hello",
			wantErr:     true,
			wantErrText: "a handler is already registered for path '/who/foo/hello'",
		},
		{
			route:       "/{static:*}",
			wantErr:     true,
			wantErrText: "path '/{static:*}' conflicts with existing wildcard or param '{filepath:*}'",
		},
		{
			route:       "/static/{filepath:*}/other",
			wantErr:     true,
			wantErrText: "wildcard routes are only allowed at the end of the path in path '/static/{filepath:*}/other'",
		},
		{
			route:       "/{user}/",
			wantErr:     true,
			wantErrText: "path '/{user}/' conflicts with existing wildcard or param '{id}'",
		},
		// {
		// 	route:       "/prefix{filepath:*}",
		// 	wantErr:     true,
		// 	wantErrText: "no / before wildcard in path '/prefix{filepath:*}'",
		// },
	}

	for _, conflict := range conflicts {
		err := router.Set(conflict.route, conflict.route)

		if conflict.wantErr == (err == nil) {
			t.Errorf("Unexpected error: %v [%s]", err, conflict.route)
		}

		if err != nil && conflict.wantErrText != fmt.Sprint(err) {
			t.Errorf("Invalid conflict error text (%v) [%s]", err, conflict.route)
		}
	}
}
