package router

// Len implements sort.Interface.
func (n *node[T]) Len() int {
	return len(n.children)
}

// Less implements sort.Interface.
func (n *node[T]) Less(i int, j int) bool {
	l, r := n.children[i], n.children[j]
	if l.m.equal(r.m) {
		return len(l.children) > len(r.children) // more children first
	}
	if pl, pr := typeID(l.m), typeID(r.m); pl != pr {
		return pl < pr // literals first, then params, then regexes
	}
	if l, ok := l.m.(literal); ok {
		return l > r.m.(literal) // sort by literal length
	}
	if l, ok := l.m.(param); ok {
		if r := r.m.(param); len(l.after) != len(r.after) {
			return len(l.after) > len(r.after) // longer suffix first
		}
	}
	return i > j
}

// Swap implements sort.Interface.
func (n *node[T]) Swap(i int, j int) {
	n.children[i], n.children[j] = n.children[j], n.children[i]
}
