package xray

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestClientIPRoutesPreserveBaseAndValidateScope(t *testing.T) {
	base := []byte(`{"domainStrategy":"AsIs","balancers":[{"tag":"pool","selector":["direct"]}],"rules":[{"type":"field","inboundTag":["api"],"outboundTag":"api"},{"type":"field","user":["a"],"outboundTag":"home"},{"type":"field","domain":["example.org"],"outboundTag":"warp"},{"type":"field","network":"tcp,udp","outboundTag":"direct"}]}`)
	var before map[string]any
	_ = json.Unmarshal(base, &before)
	raw, err := BuildClientIPRoutes(base, []ClientIPBlock{{"vless", "a", []string{"192.0.2.2", "::ffff:192.0.2.2", "2001:db8::1"}}})
	if err != nil {
		t.Fatal(err)
	}
	var after map[string]any
	_ = json.Unmarshal(raw, &after)
	rules := after["rules"].([]any)
	if !reflect.DeepEqual(rules[1:], before["rules"]) || !reflect.DeepEqual(after["balancers"], before["balancers"]) || after["domainStrategy"] != "AsIs" {
		t.Fatal("base routing changed")
	}
	block := rules[0].(map[string]any)
	if !reflect.DeepEqual(block["user"], []any{"a"}) || !reflect.DeepEqual(block["inboundTag"], []any{"vless"}) || len(block["sourceIP"].([]any)) != 2 {
		t.Fatal("rule is not strictly scoped")
	}
	restored, err := BuildClientIPRoutes(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	var restore map[string]any
	_ = json.Unmarshal(restored, &restore)
	if !reflect.DeepEqual(before, restore) {
		t.Fatal("base not restored")
	}
	for _, bad := range []ClientIPBlock{{"", "a", []string{"192.0.2.1"}}, {"vless", "", []string{"192.0.2.1"}}, {"vless", "a", nil}, {"vless", "a", []string{"0.0.0.0/0"}}} {
		if _, err := BuildClientIPRoutes(base, []ClientIPBlock{bad}); err == nil {
			t.Fatalf("accepted unscoped restriction: %v", bad)
		}
	}
	var api CoreRoutingAPI
	custom, err := BuildClientIPRoutes(base, []ClientIPBlock{{"vless", "regexp:.*", []string{"192.0.2.1"}}})
	if err != nil {
		t.Fatal(err)
	}
	var customConfig map[string]any
	_ = json.Unmarshal(custom, &customConfig)
	if customConfig["rules"].([]any)[0].(map[string]any)["user"].([]any)[0] != `regexp:^regexp:\.\*$` {
		t.Fatal("custom name became a wildcard")
	}
	if api.ReplaceRouting(base) == nil {
		t.Fatal("uninitialized API accepted")
	}
}

func TestEnsureClientIPRoutingPreservesServicesAndDefaultOutbound(t *testing.T) {
	cfg := &Config{API: []byte(`{"tag":"api","services":["HandlerService","StatsService"],"listen":"127.0.0.1:62789"}`), OutboundConfigs: []byte(`[{"tag":"direct","protocol":"freedom"},{"tag":"home","protocol":"socks","settings":{"servers":[]}}]`)}
	if err := EnsureClientIPRouting(cfg); err != nil {
		t.Fatal(err)
	}
	originalAPI, originalOut := string(cfg.API), string(cfg.OutboundConfigs)
	if err := EnsureClientIPRouting(cfg); err != nil {
		t.Fatal(err)
	}
	if string(cfg.API) != originalAPI || string(cfg.OutboundConfigs) != originalOut {
		t.Fatal("not idempotent")
	}
	var api map[string]any
	_ = json.Unmarshal(cfg.API, &api)
	if len(api["services"].([]any)) != 3 || api["listen"] != "127.0.0.1:62789" {
		t.Fatal("API settings lost")
	}
	var outs []map[string]any
	_ = json.Unmarshal(cfg.OutboundConfigs, &outs)
	if outs[0]["tag"] != "direct" || outs[1]["protocol"] != "socks" || outs[2]["protocol"] != "blackhole" {
		t.Fatal("outbound changed")
	}
	cfg.OutboundConfigs = []byte(`[{"tag":"xui-client-ip-block","protocol":"freedom"}]`)
	if EnsureClientIPRouting(cfg) == nil {
		t.Fatal("accepted reserved tag collision")
	}
}
