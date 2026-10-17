package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	router "github.com/xtls/xray-core/app/router/command"
	xnet "github.com/xtls/xray-core/common/net"
	"golang.org/x/net/proxy"
)

func freeTestPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func startTestCore(t *testing.T, binary string, config any, readyPort int) {
	t.Helper()
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "run", "-config", path)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Log(output.String())
		}
	})
	for n := 0; n < 100; n++ {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", readyPort), 50*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("test core did not start")
}

// Opt-in real-core test: private fixtures and loopback only, never live customers.
func TestClientIPRoutingRealCore(t *testing.T) {
	binary := os.Getenv("XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("set XRAY_TEST_BINARY to the supported Xray 26.4.25 executable")
	}
	apiPort, serverPort := freeTestPort(t), freeTestPort(t)
	aPort, bPort := freeTestPort(t), freeTestPort(t)
	ids := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}
	// Test destinations are loopback-only. Override the core's private-target
	// protection only in this isolated fixture, never in production configs.
	cfg := &Config{API: []byte(`{"tag":"api","services":["RoutingService","HandlerService"]}`), OutboundConfigs: []byte(`[{"tag":"direct","protocol":"freedom","settings":{"ipsBlocked":[]}},{"tag":"home","protocol":"freedom","settings":{"ipsBlocked":[]}},{"tag":"warp","protocol":"freedom","settings":{"ipsBlocked":[]}}]`)}
	if err := EnsureClientIPRouting(cfg); err != nil {
		t.Fatal(err)
	}
	base := []byte(`{"domainStrategy":"AsIs","rules":[{"type":"field","inboundTag":["api"],"outboundTag":"api"},{"type":"field","user":["a"],"outboundTag":"home"},{"type":"field","domain":["example.org"],"outboundTag":"warp"},{"type":"field","network":"tcp,udp","outboundTag":"direct"}]}`)
	server := map[string]any{
		"log": map[string]any{"loglevel": "debug"},
		"api": json.RawMessage(cfg.API), "outbounds": json.RawMessage(cfg.OutboundConfigs), "routing": json.RawMessage(base),
		"inbounds": []any{
			map[string]any{"tag": "api", "listen": "127.0.0.1", "port": apiPort, "protocol": "dokodemo-door", "settings": map[string]any{"address": "127.0.0.1"}},
			map[string]any{"tag": "vless", "listen": "127.0.0.1", "port": serverPort, "protocol": "vless", "settings": map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": ids[0], "email": "a"}, map[string]any{"id": ids[1], "email": "b"}}}},
		},
	}
	startTestCore(t, binary, server, apiPort)
	var api XrayAPI
	if err := api.Init(apiPort); err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	routingAPI := CoreRoutingAPI{Binary: binary, Port: apiPort}
	client := map[string]any{"log": map[string]any{"loglevel": "debug"}}
	var ins, outs, rules []any
	for i, port := range []int{aPort, bPort} {
		tag := fmt.Sprintf("client-%d", i)
		ins = append(ins, map[string]any{"tag": tag, "listen": "127.0.0.1", "port": port, "protocol": "socks", "settings": map[string]any{"auth": "noauth"}})
		outs = append(outs, map[string]any{"tag": tag, "protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": "127.0.0.1", "port": serverPort, "users": []any{map[string]any{"id": ids[i], "encryption": "none"}}}}}})
		rules = append(rules, map[string]any{"type": "field", "inboundTag": []string{tag}, "outboundTag": tag})
	}
	client["inbounds"], client["outbounds"], client["routing"] = ins, outs, map[string]any{"rules": rules}
	startTestCore(t, binary, client, aPort)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer target.Close()
	request := func(port int) error {
		dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", port), nil, &net.Dialer{Timeout: time.Second})
		if err != nil {
			return err
		}
		transport := &http.Transport{DialContext: dialer.(proxy.ContextDialer).DialContext, DisableKeepAlives: true}
		defer transport.CloseIdleConnections()
		response, err := (&http.Client{Transport: transport, Timeout: 2 * time.Second}).Get(target.URL)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		if response.StatusCode != 204 {
			return fmt.Errorf("HTTP %d", response.StatusCode)
		}
		return nil
	}
	for _, port := range []int{aPort, bPort} {
		if err := request(port); err != nil {
			t.Fatal("baseline request:", err)
		}
	}
	route := func(user, ip, inbound, domain string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		result, err := router.NewRoutingServiceClient(api.grpcClient).TestRoute(ctx, &router.TestRouteRequest{
			RoutingContext: &router.RoutingContext{InboundTag: inbound, User: user, SourceIPs: [][]byte{net.ParseIP(ip).To16()}, TargetDomain: domain, TargetPort: 443, Network: xnet.Network_TCP},
		})
		if err != nil {
			t.Fatal(err)
		}
		return result.OutboundTag
	}
	blocked, err := BuildClientIPRoutes(base, []ClientIPBlock{{"vless", "a", []string{"127.0.0.1", "2001:db8::2"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := routingAPI.ReplaceRouting(blocked); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ user, ip, inbound, domain, want string }{
		{"a", "127.0.0.1", "vless", "example.com", ClientIPBlockTag},
		{"a", "2001:db8::2", "vless", "example.com", ClientIPBlockTag},
		{"a", "127.0.0.2", "vless", "example.com", "home"},
		{"b", "127.0.0.1", "vless", "example.com", "direct"},
		{"a", "127.0.0.1", "other", "example.com", "home"},
		{"b", "127.0.0.1", "vless", "example.org", "warp"},
		{"", "127.0.0.1", "api", "example.com", "api"},
	} {
		if got := route(c.user, c.ip, c.inbound, c.domain); got != c.want {
			t.Fatalf("route %v: got %s", c, got)
		}
	}
	if err := request(aPort); err == nil {
		t.Fatal("restricted account still opens new connections")
	}
	if err := request(bPort); err != nil {
		t.Fatal("other account on same IP disrupted:", err)
	}
	restored, err := BuildClientIPRoutes(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := routingAPI.ReplaceRouting(restored); err != nil {
		t.Fatal(err)
	}
	if err := request(aPort); err != nil {
		t.Fatal("recovery failed:", err)
	}
	if err := request(bPort); err != nil {
		t.Fatal("other account disrupted on recovery:", err)
	}
	if err := routingAPI.ReplaceRouting([]byte(`{"rules":[{"type":"field","sourceIP":["not-an-ip"],"outboundTag":"direct"}]}`)); err == nil {
		t.Fatal("invalid routing accepted")
	}
	if err := request(bPort); err != nil {
		t.Fatal("validation failure changed routing:", err)
	}
}
