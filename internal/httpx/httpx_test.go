package httpx

import "testing"

func TestTruncateRunesKeepsCharactersWhole(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"你好世界abc", 3, "你好世"},
		{"你好", 5, "你好"},
		{"abc", 3, "abc"},
		{"😀😀", 1, "😀"},
	}
	for _, tc := range cases {
		if got := TruncateRunes(tc.in, tc.max); got != tc.want {
			t.Fatalf("TruncateRunes(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
		}
	}
}
