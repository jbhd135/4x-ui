package service

import (
	"bytes"
	"errors"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"testing"
)

type fakeClientIPRoutingAPI struct {
	calls    [][]byte
	failures int
}

func (f *fakeClientIPRoutingAPI) ReplaceRouting(raw []byte) error {
	f.calls = append(f.calls, raw)
	if len(f.calls) <= f.failures {
		return errors.New("simulated partial/uncertain RPC failure")
	}
	return nil
}

func TestClientIPRoutingRollback(t *testing.T) {
	base := []byte(`{"rules":[{"type":"field","network":"tcp,udp","outboundTag":"direct"}]}`)
	routes, err := xray.BuildClientIPRoutes(base, []xray.ClientIPBlock{{Inbound: "vless", Email: "a", IPs: []string{"192.0.2.1"}}})
	if err != nil {
		t.Fatal(err)
	}
	restore, err := xray.BuildClientIPRoutes(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	for failures := 0; failures <= 2; failures++ {
		api := &fakeClientIPRoutingAPI{failures: failures}
		applied, err := replaceClientIPRouting(api, base, routes)
		if failures == 0 {
			if err != nil || len(api.calls) != 1 || !bytes.Equal(applied, routes) {
				t.Fatal("successful apply lost state")
			}
		} else {
			if err == nil || len(api.calls) != 2 || !bytes.Equal(api.calls[1], restore) {
				t.Fatal("failed apply did not restore complete base")
			}
			if failures == 1 && !bytes.Equal(applied, restore) {
				t.Fatal("successful rollback cache incorrect")
			}
			if failures == 2 && applied != nil {
				t.Fatal("uncertain state must force retry")
			}
		}
	}
}
