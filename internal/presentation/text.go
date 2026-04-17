package presentation

import (
	"strings"
	"unicode"
)

func Title(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(value))
	capitalizeNext := true
	for _, r := range value {
		if capitalizeNext && unicode.IsLetter(r) {
			b.WriteRune(unicode.ToUpper(r))
			capitalizeNext = false
			continue
		}
		b.WriteRune(r)
		capitalizeNext = r == ' ' || r == '-' || r == '_'
	}
	return b.String()
}
