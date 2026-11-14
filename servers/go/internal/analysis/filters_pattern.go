package analysis

import (
	"slopensearch/internal/jregex"

	"slopensearch/internal/apierr"
)

// Regex based token filters, using Java regular expression semantics.

func filterPatternRegex(p map[string]any, pattern string) (*jregex.Regexp, *apierr.Error) {
	flags, err := javaRegexFlags(fsString(p, "flags", ""))
	if err != nil {
		return nil, err
	}
	return compileJavaRegexAPI(pattern, flags)
}

// patternReplaceFilter ports PatternReplaceFilter.
func patternReplaceFilter(toks []Token, p map[string]any) ([]Token, *apierr.Error) {
	pat := fsString(p, "pattern", "")
	if pat == "" {
		return nil, filterError("pattern is missing for [pattern_replace] token filter of type 'pattern_replace'")
	}
	re, err := filterPatternRegex(p, pat)
	if err != nil {
		return nil, err
	}
	repl := fsString(p, "replacement", "")
	all := ParamBool(p, "all", true)
	for i := range toks {
		out, rerr := re.Replace(toks[i].Term, repl, all)
		if rerr != nil {
			return nil, jregexError(rerr)
		}
		toks[i].Term = out
	}
	return toks, nil
}

// patternCaptureFilter ports PatternCaptureGroupTokenFilter: the original
// (optionally) followed by every non-empty capture group of every match.
func patternCaptureFilter(toks []Token, p map[string]any) ([]Token, *apierr.Error) {
	var res []*jregex.Regexp
	for _, pat := range fsStrings(p, "patterns") {
		re, err := filterPatternRegex(p, pat)
		if err != nil {
			return nil, err
		}
		res = append(res, re)
	}
	preserve := ParamBool(p, "preserve_original", true)
	out := make([]Token, 0, len(toks))
	for _, t := range toks {
		if preserve {
			out = append(out, t)
		}
		in := []rune(t.Term)
		for _, re := range res {
			for _, m := range re.FindAll(in) {
				for g := 1; 2*g+1 < len(m); g++ {
					if m[2*g] < 0 || m[2*g] == m[2*g+1] {
						continue
					}
					nt := t
					nt.Term = string(in[m[2*g]:m[2*g+1]])
					if preserve && nt.Term == t.Term {
						continue
					}
					out = append(out, nt)
				}
			}
		}
	}
	return out, nil
}
