// Package money provides Indian-style currency formatting.
package money

import (
	"math"
	"strconv"
	"strings"
)

// FormatINR renders a rupee amount the way Indian small businesses read it:
// ₹1.2 Cr, ₹36.5 L, ₹12,500 (Indian digit grouping below one lakh).
func FormatINR(amount float64) string {
	neg := amount < 0
	a := math.Abs(amount)
	var s string
	switch {
	case a >= 1e7:
		s = "₹" + trimNum(a/1e7) + " Cr"
	case a >= 1e5:
		s = "₹" + trimNum(a/1e5) + " L"
	default:
		s = "₹" + groupINR(math.Round(a))
	}
	if neg {
		return "-" + s
	}
	return s
}

// trimNum formats to one decimal place and drops a trailing ".0".
func trimNum(v float64) string {
	s := strconv.FormatFloat(v, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0")
}

// groupINR applies Indian digit grouping: last three digits, then pairs.
func groupINR(v float64) string {
	d := strconv.FormatFloat(v, 'f', 0, 64)
	if len(d) <= 3 {
		return d
	}
	head := d[:len(d)-3]
	tail := d[len(d)-3:]
	var parts []string
	for len(head) > 2 {
		parts = append([]string{head[len(head)-2:]}, parts...)
		head = head[:len(head)-2]
	}
	if head != "" {
		parts = append([]string{head}, parts...)
	}
	return strings.Join(parts, ",") + "," + tail
}
