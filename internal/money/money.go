// Package money formats integer cents as Brazilian reais for text that leaves
// the screen, like charge messages and notifications (RN-21).
package money

import (
	"fmt"
	"strings"
)

// Format writes cents as Brazilian reais, like R$ 1.234,56.
func Format(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	whole := fmt.Sprintf("%d", cents/100)
	var groups []string
	for len(whole) > 3 {
		groups = append([]string{whole[len(whole)-3:]}, groups...)
		whole = whole[:len(whole)-3]
	}
	groups = append([]string{whole}, groups...)
	return fmt.Sprintf("%sR$ %s,%02d", sign, strings.Join(groups, "."), cents%100)
}
