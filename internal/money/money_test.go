package money

import "testing"

func TestFormatINR(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "₹0"},
		{500, "₹500"},
		{12500, "₹12,500"},
		{99999, "₹99,999"},
		{100000, "₹1 L"},
		{1234567, "₹12.3 L"},
		{3650000, "₹36.5 L"},
		{10000000, "₹1 Cr"},
		{12000000, "₹1.2 Cr"},
		{250000000, "₹25 Cr"},
		{-12500, "-₹12,500"},
	}
	for _, tc := range cases {
		if got := FormatINR(tc.in); got != tc.want {
			t.Errorf("FormatINR(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIndianGrouping(t *testing.T) {
	cases := map[float64]string{
		1000:  "1,000",
		10000: "10,000",
		99999: "99,999",
	}
	for in, want := range cases {
		if got := groupINR(in); got != want {
			t.Errorf("groupINR(%v) = %q, want %q", in, got, want)
		}
	}
}
