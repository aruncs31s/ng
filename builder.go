package numbergenerator

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	reTrailingDigits = regexp.MustCompile(`(?i)(\d+)$`)
)

type numberPart struct {
	position  int
	separator string
	value     string
}

// buildNumber assembles parts sorted by position.
// If includeTrailingSeparator is true, the last part's separator is included
// (used when building the prefix for the incremental query).
func buildNumber(parts []numberPart, includeTrailingSeparator bool) string {
	sort.Slice(parts, func(i, j int) bool { return parts[i].position < parts[j].position })
	var b strings.Builder
	for i, p := range parts {
		b.WriteString(p.value)
		if p.separator != "" && (i < len(parts)-1 || includeTrailingSeparator) {
			b.WriteString(p.separator)
		}
	}
	return b.String()
}

// buildWildcardPattern replaces the part at wildcardPosition with '%' and
// appends a trailing '%' to create a SQL LIKE pattern.
func buildWildcardPattern(parts []numberPart, wildcardPosition int) string {
	wc := make([]numberPart, 0, len(parts))
	for _, p := range parts {
		if p.position == wildcardPosition {
			wc = append(wc, numberPart{position: p.position, separator: p.separator, value: "%"})
		} else {
			wc = append(wc, p)
		}
	}
	return buildNumber(wc, true) + "%"
}

// getNumber returns the assembled number string with the counter at nextNum.
func getNumber(nextNum, width int, prefixParts []numberPart, cfg CounterConfig) string {
	numStr := strconv.Itoa(nextNum)
	if len(numStr) > width {
		width = len(numStr)
	}
	incrementValue := fmt.Sprintf("%0*d", width, nextNum)

	allParts := copyParts(prefixParts, numberPart{
		position:  cfg.Position,
		separator: cfg.Separator,
		value:     incrementValue,
	})
	return buildNumber(allParts, false)
}

// copyParts returns a new slice with the given parts plus the extra part appended.
func copyParts(parts []numberPart, extra numberPart) []numberPart {
	out := make([]numberPart, len(parts)+1)
	copy(out, parts)
	out[len(parts)] = extra
	return out
}

// getNext parses the trailing digits from last and returns nextNum + 1.
// If last is empty or has no trailing digits, returns 1.
func getNext(last, prefix string) int {
	if last == "" {
		return 1
	}
	trimmed := last
	if prefix != "" && strings.HasPrefix(strings.ToUpper(last), strings.ToUpper(prefix)) {
		trimmed = last[len(prefix):]
	}
	m := reTrailingDigits.FindStringSubmatch(trimmed)
	if len(m) >= 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n + 1
		}
	}
	return 1
}
