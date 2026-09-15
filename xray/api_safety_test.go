package xray

import "testing"

func TestRemoveUserWithoutInitReturnsError(t *testing.T) {
	var api XrayAPI
	if err := api.RemoveUser("inbound", "client"); err == nil {
		t.Fatal("RemoveUser should reject an uninitialized API client")
	}
}
