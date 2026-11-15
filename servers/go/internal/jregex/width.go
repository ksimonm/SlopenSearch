package jregex

// VariableLookbehind reports a look-behind with a branch whose length is not
// fixed. Oniguruma (grok) rejects those; java.util.regex accepts bounded ones.
func (re *Regexp) VariableLookbehind() bool { return variableLookbehind(re.root) }

func variableLookbehind(n *jNode) bool {
	if n == nil {
		return false
	}
	if n.kind == jnLook && !n.ahead {
		body := n.subs[0]
		branches := []*jNode{body}
		if body.kind == jnAlt {
			branches = body.subs
		}
		for _, b := range branches {
			if lo, hi := nodeWidth(b); lo != hi {
				return true
			}
		}
	}
	for _, s := range n.subs {
		if variableLookbehind(s) {
			return true
		}
	}
	return false
}

// nodeWidth returns the minimum and maximum match length (hi -1: unbounded).
func nodeWidth(n *jNode) (int, int) {
	switch n.kind {
	case jnLit, jnSet:
		return 1, 1
	case jnSeq:
		lo, hi := 0, 0
		for _, s := range n.subs {
			l, h := nodeWidth(s)
			lo += l
			if hi >= 0 {
				if h < 0 {
					hi = -1
				} else {
					hi += h
				}
			}
		}
		return lo, hi
	case jnAlt:
		lo, hi := -1, 0
		for _, s := range n.subs {
			l, h := nodeWidth(s)
			if lo < 0 || l < lo {
				lo = l
			}
			if hi >= 0 && (h < 0 || h > hi) {
				hi = h
			}
		}
		if lo < 0 {
			lo = 0
		}
		return lo, hi
	case jnGroup, jnAtomic:
		return nodeWidth(n.subs[0])
	case jnRepeat:
		l, h := nodeWidth(n.subs[0])
		lo := l * n.min
		if h == 0 {
			return lo, 0
		}
		if n.max < 0 || h < 0 {
			return lo, -1
		}
		return lo, h * n.max
	case jnBackref:
		return 0, -1
	}
	return 0, 0
}
