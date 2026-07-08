package main

import "testing"

func TestParseModelosdVersion(t *testing.T) {
	cases := []struct {
		ua   string
		want []int
	}{
		{"/modeloswire:0.5.0/modelosd:1.0.7.5/", []int{1, 0, 7, 5}},
		{"/modeloswire:0.5.0/modelosd:1.0.6/", []int{1, 0, 6}},
		{"/modeloswire:0.5.0/modelosd:1.0.7-beta/", []int{1, 0, 7}},
		{"/modeloswire:0.5.0/neutrino:0.12.0-beta/", nil},  // SPV — not gated
		{"/modeloswire:0.5.0/modelos-crawler:0.1.0/", nil}, // crawler — not gated
		{"garbage", nil},
		{"/modelosd:1.0.x/", nil}, // malformed → nil (no false reject)
	}
	for _, c := range cases {
		got := parseModelosdVersion(c.ua)
		if len(got) != len(c.want) {
			t.Fatalf("parseModelosdVersion(%q) = %v, want %v", c.ua, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("parseModelosdVersion(%q) = %v, want %v", c.ua, got, c.want)
			}
		}
	}
}

func TestCompareVersions(t *testing.T) {
	min := []int{1, 0, 7, 5}
	cases := []struct {
		v     []int
		below bool // true if v < min (should be rejected)
	}{
		{[]int{1, 0, 6}, true},     // 1.0.6 < 1.0.7.5 → reject
		{[]int{1, 0, 7}, true},     // 1.0.7 == 1.0.7.0 < 1.0.7.5 → reject
		{[]int{1, 0, 7, 5}, false}, // exact → allow
		{[]int{1, 0, 7, 6}, false}, // newer → allow
		{[]int{1, 1, 0}, false},    // newer minor → allow
		{[]int{2, 0, 0}, false},    // newer major → allow
		{[]int{1, 0, 7, 4}, true},  // 1.0.7.4 < 1.0.7.5 → reject
	}
	for _, c := range cases {
		got := compareVersions(c.v, min) < 0
		if got != c.below {
			t.Fatalf("compareVersions(%v, %v)<0 = %v, want %v", c.v, min, got, c.below)
		}
	}
}

// TestEnforcedMinPeerVersion verifies the built-in enforced floor is a valid,
// parseable version and rejects/accepts the expected cohorts. The current floor
// is 1.0.7 (drops buggy 1.0.6, keeps the 1.0.7/1.0.7.x cohort so we are not
// isolated). Update this alongside EnforcedMinPeerVersion.
func TestEnforcedMinPeerVersion(t *testing.T) {
	floor := parseVersionString(EnforcedMinPeerVersion)
	if floor == nil {
		t.Fatalf("EnforcedMinPeerVersion %q does not parse", EnforcedMinPeerVersion)
	}
	// 1.0.6 must be rejected; 1.0.7 and above must be accepted.
	reject := parseModelosdVersion("/modeloswire:0.5.0/modelosd:1.0.6/")
	if compareVersions(reject, floor) >= 0 {
		t.Errorf("1.0.6 should be below the enforced floor %s", EnforcedMinPeerVersion)
	}
	for _, ok := range []string{"1.0.7", "1.0.7.5", "1.0.7.7", "1.1.0"} {
		v := parseModelosdVersion("/modeloswire:0.5.0/modelosd:" + ok + "/")
		if compareVersions(v, floor) < 0 {
			t.Errorf("%s should meet the enforced floor %s", ok, EnforcedMinPeerVersion)
		}
	}
	// SPV/light clients are never gated.
	if parseModelosdVersion("/modeloswire:0.5.0/neutrino:0.12.0-beta/") != nil {
		t.Error("neutrino (SPV) must not be gated by min-version")
	}
}
