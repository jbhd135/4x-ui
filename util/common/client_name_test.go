package common

import (
	"encoding/json"
	"os"
	"testing"
)

func TestClientCodeFixtures(t *testing.T) {
	data, err := os.ReadFile("../../testdata/client_codes.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ Email, Code string }
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if got := ClientCode(c.Email); got != c.Code {
			t.Errorf("ClientCode(%q) = %q, want %q", c.Email, got, c.Code)
		}
	}
}

func TestClientCodeConflicts(t *testing.T) {
	existing := []string{"20261008-azxd4p", "临时测试"}
	for _, c := range []struct {
		names    []string
		conflict bool
	}{
		{[]string{"20261008-azxd4p"}, false},
		{[]string{"20270501-azxd4p"}, true},
		{[]string{"20270501-ABC123", "20261008-abc123"}, true},
		{[]string{"临时测试", "another test"}, false},
	} {
		if err := CheckClientCodeConflicts(existing, c.names); (err != nil) != c.conflict {
			t.Errorf("CheckClientCodeConflicts(%v) = %v", c.names, err)
		}
	}
}
