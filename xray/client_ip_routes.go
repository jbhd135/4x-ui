package xray

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/netip"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

const ClientIPBlockTag = "xui-client-ip-block"

// ClientIPBlock always scopes a restriction to both the account and its inbound.
type ClientIPBlock struct {
	Inbound string
	Email   string
	IPs     []string
}

func EnsureClientIPRouting(c *Config) error {
	api := map[string]json.RawMessage{}
	if len(c.API) > 0 && string(c.API) != "null" {
		if err := json.Unmarshal(c.API, &api); err != nil {
			return err
		}
	}
	var services []string
	if raw := api["services"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &services); err != nil {
			return err
		}
	}
	found := false
	for _, service := range services {
		found = found || service == "RoutingService"
	}
	if !found {
		services = append(services, "RoutingService")
	}
	api["services"], _ = json.Marshal(services)
	if len(api["tag"]) == 0 {
		api["tag"] = json.RawMessage(`"api"`)
	}
	var err error
	c.API, err = json.Marshal(api)
	if err != nil {
		return err
	}
	var outbounds []map[string]json.RawMessage
	if len(c.OutboundConfigs) > 0 && string(c.OutboundConfigs) != "null" {
		if err := json.Unmarshal(c.OutboundConfigs, &outbounds); err != nil {
			return err
		}
	}
	for _, outbound := range outbounds {
		var tag, protocol string
		_ = json.Unmarshal(outbound["tag"], &tag)
		_ = json.Unmarshal(outbound["protocol"], &protocol)
		if tag == ClientIPBlockTag {
			if protocol != "blackhole" {
				return fmt.Errorf("reserved outbound %s must be blackhole", tag)
			}
			return nil
		}
	}
	// Never make the limiting outbound the implicit default outbound.
	if len(outbounds) == 0 {
		return fmt.Errorf("client IP routing requires a default outbound")
	}
	outbounds = append(outbounds, map[string]json.RawMessage{
		"tag":      json.RawMessage(`"` + ClientIPBlockTag + `"`),
		"protocol": json.RawMessage(`"blackhole"`), "settings": json.RawMessage(`{}`),
	})
	c.OutboundConfigs, err = json.Marshal(outbounds)
	return err
}

// BuildClientIPRoutes preserves the complete base routing configuration. Xray's
// AddRule(shouldAppend=false) REPLACES all rules, rather than prepending them.
func BuildClientIPRoutes(base []byte, blocks []ClientIPBlock) ([]byte, error) {
	routing := map[string]json.RawMessage{}
	if len(base) > 0 && string(base) != "null" {
		if err := json.Unmarshal(base, &routing); err != nil {
			return nil, err
		}
	}
	var baseRules []json.RawMessage
	if raw := routing["rules"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &baseRules); err != nil {
			return nil, err
		}
	}
	ordered := append([]ClientIPBlock(nil), blocks...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Inbound != ordered[j].Inbound {
			return ordered[i].Inbound < ordered[j].Inbound
		}
		return ordered[i].Email < ordered[j].Email
	})
	rules := make([]json.RawMessage, 0, len(blocks)+len(baseRules))
	accounts := make(map[[2]string]bool)
	for _, block := range ordered {
		if block.Inbound == "" || block.Email == "" || len(block.IPs) == 0 {
			return nil, fmt.Errorf("unscoped client IP restriction")
		}
		key := [2]string{block.Inbound, block.Email}
		if accounts[key] {
			return nil, fmt.Errorf("duplicate account restriction")
		}
		accounts[key] = true
		user := block.Email
		// Xray interprets this prefix as a pattern; custom customer names must
		// still match exactly, never all accounts sharing the source address.
		if strings.HasPrefix(user, "regexp:") {
			user = "regexp:^" + regexp.QuoteMeta(user) + "$"
		}
		ips := make([]string, 0, len(block.IPs))
		seen := map[string]bool{}
		for _, raw := range block.IPs {
			ip, err := netip.ParseAddr(raw)
			if err != nil || ip.Zone() != "" {
				return nil, fmt.Errorf("invalid source IP %q", raw)
			}
			value := ip.Unmap().String()
			if !seen[value] {
				ips = append(ips, value)
				seen[value] = true
			}
		}
		sort.Strings(ips)
		rule, err := json.Marshal(map[string]any{
			"type": "field", "inboundTag": []string{block.Inbound}, "user": []string{user},
			"sourceIP": ips, "outboundTag": ClientIPBlockTag,
			"ruleTag": fmt.Sprintf("%s-%x", ClientIPBlockTag, sha256.Sum256([]byte(block.Inbound+"\x00"+block.Email))),
		})
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	rules = append(rules, baseRules...)
	routing["rules"], _ = json.Marshal(rules)
	return json.Marshal(routing)
}

// The running core compiles routing JSON itself. Its geodata protobuf changed
// in 26.4.25 and is not wire-compatible with the panel's embedded Go library.
// Using the core's API client also keeps future core updates compatible.
type CoreRoutingAPI struct {
	Binary string
	Port   int
}

func (api *CoreRoutingAPI) ReplaceRouting(raw []byte) error {
	if api == nil || api.Binary == "" || api.Port <= 0 || api.Port > 65535 {
		return fmt.Errorf("core routing API is not initialized")
	}
	payload, err := json.Marshal(map[string]json.RawMessage{"routing": raw})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, api.Binary, "api", "adrules",
		fmt.Sprintf("--server=127.0.0.1:%d", api.Port), "--timeout=5")
	cmd.Stdin = bytes.NewReader(payload)
	// No shell, no configuration file, and no credentials in process arguments.
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("core routing API: %w", ctx.Err())
		}
		if len(output) > 1024 {
			output = output[:1024]
		}
		return fmt.Errorf("core routing API: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
