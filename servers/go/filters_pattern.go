package main

// Regex based token filters, using Java regular expression semantics.

func filterPatternRegex(p map[string]any, pattern string) (*jRegex, *apiError) {
	flags, err := javaRegexFlags(fsString(p, "flags", ""))
	if err != nil {
		return nil, err
	}
	return compileJavaRegexAPI(pattern, flags)
}

// patternReplaceFilter ports PatternReplaceFilter.
func patternReplaceFilter(toks []analyzedToken, p map[string]any) ([]analyzedToken, *apiError) {
	pat := fsString(p, "pattern", "")
	if pat == "" {
		return nil, filterError("pattern is missing for [pattern_replace] token filter of type 'pattern_replace'")
	}
	re, err := filterPatternRegex(p, pat)
	if err != nil {
		return nil, err
	}
	repl := fsString(p, "replacement", "")
	all := fsBool(p, "all", true)
	for i := range toks {
		out, rerr := re.replace(toks[i].term, repl, all)
		if rerr != nil {
			return nil, rerr
		}
		toks[i].term = out
	}
	return toks, nil
}

// patternCaptureFilter ports PatternCaptureGroupTokenFilter: the original
// (optionally) followed by every non-empty capture group of every match.
func patternCaptureFilter(toks []analyzedToken, p map[string]any) ([]analyzedToken, *apiError) {
	var res []*jRegex
	for _, pat := range fsStrings(p, "patterns") {
		re, err := filterPatternRegex(p, pat)
		if err != nil {
			return nil, err
		}
		res = append(res, re)
	}
	preserve := fsBool(p, "preserve_original", true)
	out := make([]analyzedToken, 0, len(toks))
	for _, t := range toks {
		if preserve {
			out = append(out, t)
		}
		in := []rune(t.term)
		for _, re := range res {
			for _, m := range re.findAll(in) {
				for g := 1; 2*g+1 < len(m); g++ {
					if m[2*g] < 0 || m[2*g] == m[2*g+1] {
						continue
					}
					nt := t
					nt.term = string(in[m[2*g]:m[2*g+1]])
					if preserve && nt.term == t.term {
						continue
					}
					out = append(out, nt)
				}
			}
		}
	}
	return out, nil
}
