package store

import (
	"strings"
	"testing"
)

func TestGenerateRandomPasswordMeetsPolicyAndLength(t *testing.T) {
	for _, n := range []int{4, 8, 16, 32} {
		pw := GenerateRandomPassword(n)
		if len(pw) != n {
			t.Fatalf("length = %d, want %d", len(pw), n)
		}
		has := func(class string) bool {
			return strings.ContainsAny(pw, class)
		}
		if !has(pwLower) || !has(pwUpper) || !has(pwDigits) || !has(pwSymbols) {
			t.Fatalf("password %q misses a character class", pw)
		}
	}
	if len(GenerateRandomPassword(1)) != 4 {
		t.Fatal("n < 4 should be clamped to 4")
	}
}

func TestGenerateRandomPasswordIsNotConstant(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		seen[GenerateRandomPassword(16)] = true
	}
	if len(seen) < 45 {
		t.Fatalf("expected near-unique passwords, got %d distinct out of 50", len(seen))
	}
}

func TestInitialAdminPasswordPrefersEnv(t *testing.T) {
	if pw, fromEnv := InitialAdminPassword("  env-secret  "); !fromEnv || pw != "env-secret" {
		t.Fatalf("env value should win: fromEnv=%v pw=%q", fromEnv, pw)
	}
	pw, fromEnv := InitialAdminPassword("   ")
	if fromEnv {
		t.Fatal("blank env must fall back to random generation")
	}
	if len(pw) != 16 {
		t.Fatalf("random password length = %d, want 16", len(pw))
	}
}

func TestIsDevVersion(t *testing.T) {
	cases := map[string]bool{
		"":                  true,
		"dev":               true,
		"dev-dirty":         true,
		"v0.5.5-3-gabc1234": true,
		"v0.5.5-dirty":      true,
		"v0.5.5":            false,
		"v0.6.0-rc.1":       false,
		"0.6.0":             false,
	}
	for v, want := range cases {
		if got := IsDevVersion(v); got != want {
			t.Errorf("IsDevVersion(%q) = %v, want %v", v, got, want)
		}
	}
}
