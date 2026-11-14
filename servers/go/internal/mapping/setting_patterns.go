package mapping

import (
	"strings"

	"slopensearch/internal/wildcard"
)

func MatchAnySettingPattern(patterns []string, key string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if wildcard.Match(p, key) || strings.HasPrefix(key, strings.TrimSuffix(p, "*")) && strings.HasSuffix(p, "*") {
			return true
		}
	}
	return false
}
