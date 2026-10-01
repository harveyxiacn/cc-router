package update

import "testing"

func TestIsNewerSemanticOrder(t *testing.T) {
	for _, tc := range []struct {
		candidate, current string
		newer              bool
	}{
		{"1.10.0", "1.9.0", true}, {"v1.0.0", "0.9.9", true},
		{"1.0.0", "1.0.0-beta.3", true}, {"1.0.0-beta.11", "1.0.0-beta.2", true},
		{"1.0.0-beta", "1.0.0-alpha.99", true}, {"1.0.0+build.2", "1.0.0+build.1", false},
		{"0.9.9", "1.0.0", false}, {"1.0.0-beta", "1.0.0", false},
	} {
		got, err := IsNewer(tc.candidate, tc.current)
		if err != nil || got != tc.newer {
			t.Errorf("IsNewer(%q,%q) = %v,%v", tc.candidate, tc.current, got, err)
		}
	}
	for _, invalid := range []string{"", "1.0", "1.01.0", "01.0.0", "1.0.0-01", "1.0.0-", "1.0.0+", "1.0.0\n", "1.0.0-foo_bar"} {
		if _, err := IsNewer(invalid, "1.0.0"); err == nil {
			t.Errorf("accepted %q", invalid)
		}
	}
}
