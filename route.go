package router

type Router[T any] struct {
	tree node[T]
	options
}

type options struct {
	allowConflict bool
}

// Option configures a Router created by NewRouter.
type Option func(*options)

// AllowConflict lets params or wildcards share a position though they may match
// the same path, like /a/{id} next to /a/{oid}, and so
// regexes of the same expression under different names, instead of failing
// Set with ErrExprConflict. Walk visits every one of them matching the path;
// Get returns the first matching one in sibling order.
func AllowConflict() Option {
	return func(o *options) { o.allowConflict = true }
}

// Set registers a value for the given URL pattern. It's not routine-safe.
func (r *Router[T]) Set(path string, handler T) error {
	n, err := r.add(path)
	if err != nil {
		return err
	}
	if n.assigned {
		return ErrConflict.With(path)
	}
	n.handler, n.assigned = handler, true
	return nil
}

// Upsert registers path like Set, except that a pattern already registered is
// no conflict: f gets the value there, the zero value for a new pattern, and
// returns the value to store. It's not routine-safe.
func (r *Router[T]) Upsert(path string, f func(old T) T) error {
	n, err := r.add(path)
	if err != nil {
		return err
	}
	n.handler, n.assigned = f(n.handler), true
	return nil
}

func (r *Router[T]) add(path string) (*node[T], error) {
	if path == "" || path[0] != '/' {
		return nil, ErrInvalidPath.With(path)
	}
	n, err := r.tree.add(path, path, r.allowConflict)
	r.tree.sort()
	return n, err
}

// GetParam matches the given path and returns the corresponding value,
// assigning the given params map with the matched parameters.
// If no pattern is found, the zero value is returned. It's routine-safe.
func (r *Router[T]) GetParam(path string, params map[string]string) (zero T) {
	if path == "" || path[0] != '/' || len(r.tree.children) == 0 {
		return zero
	}
	n := &r.tree
	if ch := n.children[0]; ch.b == '/' {
		// first node is almost always a literal("/")
		m := ch.m.(literal)
		end, ok := 1, len(m) == 1
		if !ok {
			end, _, ok = m.match(path)
		}
		if ok {
			n, path = ch, path[end:]
			if path == "" {
				zero = n.handler
			}
		}
	}
	n = n.get(path, params)
	if n == nil {
		return zero
	}
	return n.handler
}

// Get matches the given path and returns the corresponding value.
// If no pattern is found, the zero value is returned. It's routine-safe.
func (r *Router[T]) Get(path string) T {
	return r.GetParam(path, nil)
}

// Walk calls f with the value of every pattern matching the given path, until f
// returns false. A pattern's value comes after the values of the patterns
// nested under it; siblings go literals first, then params, regexes and wildcards.
// It's routine-safe.
func (r *Router[T]) Walk(path string, f func(T) (more bool)) {
	if path == "" || path[0] != '/' || len(r.tree.children) == 0 {
		return
	}
	n := &r.tree
	if ch := n.children[0]; ch.b == '/' {
		// first node is almost always a literal("/")
		m := ch.m.(literal)
		end, ok := 1, len(m) == 1
		if !ok {
			end, _, ok = m.match(path)
		}
		if ok {
			n, path = ch, path[end:]
		}
	}
	if n.walk(path, f) && path == "" && n.assigned {
		f(n.handler)
	}
}

func NewRouter[T any](opts ...Option) *Router[T] {
	r := &Router[T]{tree: node[T]{m: literal("")}}
	for _, opt := range opts {
		opt(&r.options)
	}
	return r
}
