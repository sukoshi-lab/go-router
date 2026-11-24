package router

import (
	"sort"
	"strings"
)

// add returns the node of path, creating it if needed, for the caller to assign.
func (n *node[T]) add(path, fullPath string, allowConflict bool) (*node[T], error) {
	for path != "" {
		next, end, err := next(path)
		if err != nil {
			return nil, err
		}
		inserted := false
		for _, child := range n.children {
			if child.m.equal(next) {
				n = child
				inserted = true
				path = path[end:]
				break
			}
		}

		if inserted {
			continue
		}
		switch next := next.(type) {
		case literal:
			maxi, maxl := -1, 0
			for i, child := range n.children {
				l, ok := child.m.(literal)
				if !ok || string(l)[0] != path[0] {
					continue
				}
				if common := lcp(string(l), string(next)); common > maxl {
					maxi, maxl = i, common
				}
			}
			if maxi != -1 {
				// split node
				child := n.children[maxi]
				n = child.cut(maxl)
				path = path[maxl:]
				inserted = true
				break
			}
		default:
			if allowConflict {
				break
			}
			for _, child := range n.children {
				if next.conflicts(child.m) {
					return nil, ErrExprConflict.With(fullPath, child.m.string())
				}
			}
		}
		if inserted {
			continue
		}
		path = path[end:]
		if _, ok := next.(wildcard); ok && path != "" {
			return nil, ErrWildcardNotAtEnd.With(fullPath)
		}
		newch := &node[T]{m: next}
		if next, ok := next.(literal); ok {
			newch.b = next[0]
		}
		n.children = append(n.children, newch)
		n = newch
	}
	return n, nil
}

func (n *node[T]) get(path string, params map[string]string) *node[T] {
	for i := 0; i < len(n.children); i++ {
		child, end, key, ok := n.children[i], 0, "", false
		if child.b != 0 {
			if path == "" || path[0] != child.b {
				continue
			}
			if end, key, ok = child.m.(literal).match(path); !ok {
				continue
			}
			i = n.lastlit
		} else if end, key, ok = child.m.match(path); !ok {
			continue
		}
		var next *node[T]
		if len(child.children) != 0 {
			next = child.get(path[end:], params)
		}
		if next == nil {
			if !child.assigned || end != len(path) {
				continue
			}
			next = child
		}
		if params != nil && key != "" {
			value := path[:end]
			if p, ok := child.m.(param); ok {
				value = value[:len(value)-len(p.after)]
			}
			params[key] = strings.Clone(value)
		}
		return next
	}
	return nil
}

// walk calls f with every handler matching path, a child's descendants before
// the child itself. It returns false once f asked to stop.
func (n *node[T]) walk(path string, f func(T) (more bool)) bool {
	for i := 0; i < len(n.children); i++ {
		child, end, ok := n.children[i], 0, false
		if child.b != 0 {
			if path == "" || path[0] != child.b {
				continue
			}
			if end, _, ok = child.m.(literal).match(path); !ok {
				continue
			}
			i = n.lastlit
		} else if end, _, ok = child.m.match(path); !ok {
			continue
		}
		if len(child.children) != 0 && !child.walk(path[end:], f) {
			return false
		}
		if child.assigned && end == len(path) && !f(child.handler) {
			return false
		}
	}
	return true
}

func (n *node[T]) cut(i int) *node[T] {
	l, ok := n.m.(literal)
	if !ok {
		panic("cannot cut non-literal node")
	}
	if i == len(l) {
		return n
	}
	n.children = []*node[T]{{
		m: l[i:], b: l[i], children: n.children,
		handler: n.handler, assigned: n.assigned,
	}}
	var zero T
	n.handler = zero
	n.assigned = false
	n.m = l[:i]
	return n
}

func (n *node[T]) sort() {
	sort.Sort(n)
	for i, child := range n.children {
		if _, ok := child.m.(literal); ok {
			n.lastlit = i
		}
		child.sort()
	}
}
