package sub

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func TestFormatClientExpiryRemark(t *testing.T) {
	now := time.Date(2026, time.May, 24, 10, 0, 0, 0, time.Local)
	tests := []struct {
		name   string
		remark string
		want   string
		ok     bool
	}{
		{
			name:   "formats current year MMDD",
			remark: "0615",
			want:   "过期日期:2026年6月15日",
			ok:     true,
		},
		{
			name:   "formats explicit year YYYYMMDD",
			remark: "20270524",
			want:   "过期日期:2027年5月24日",
			ok:     true,
		},
		{
			name:   "formats short year YYMMDD",
			remark: "270524",
			want:   "过期日期:2027年5月24日",
			ok:     true,
		},
		{
			name:   "clamps invalid month day",
			remark: "0631",
			want:   "过期日期:2026年6月30日",
			ok:     true,
		},
		{
			name:   "clamps non leap February",
			remark: "0230",
			want:   "过期日期:2026年2月28日",
			ok:     true,
		},
		{
			name:   "uses earliest date token",
			remark: "0603.20270501",
			want:   "过期日期:2026年6月3日",
			ok:     true,
		},
		{
			name:   "ignores non date remark",
			remark: "vip-node",
			ok:     false,
		},
		{
			name:   "ignores invalid month",
			remark: "1321",
			ok:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := formatClientExpiryRemark(tt.remark, now)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("formatClientExpiryRemark(%q) = %q, %v; want %q, %v", tt.remark, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestGenRemarkKeepsClientSuffixForExpiryRemark(t *testing.T) {
	service := &SubService{remarkModel: "-ieo"}
	inbound := &model.Inbound{Remark: "0615"}

	got := service.genRemark(inbound, "20260615-ayd3o269", "")
	want := fmt.Sprintf("ayd3o269 | %02d-06-15", time.Now().Year()%100)
	if got != want {
		t.Fatalf("genRemark() = %q, want %q", got, want)
	}
}

func TestClientLookupRemarksAndFormats(t *testing.T) {
	expiry := time.Date(2026, 10, 8, 23, 59, 59, 0, time.Local).UnixMilli()
	for _, tt := range []struct {
		email  string
		expiry int64
		want   string
	}{
		{"20261008-azxd4p", expiry, "azxd4p | 26-10-08"},
		{"20261008-azxd4p", expiry + 86400000, "azxd4p | 26-10-09"},
		{"20261008-azxd4p", 0, "azxd4p | 不限期"},
		{"20261008-azxd4p", -86400000, "azxd4p | 启用后1天"},
		{"临时测试", 0, "cf83bbad4 | 临时测试"},
		{"test", expiry, "cafd071e5 | test | 26-10-08"},
	} {
		t.Run(tt.want, func(t *testing.T) {
			client := model.Client{Email: tt.email, ID: "11111111-2222-4333-8444-555555555555", Security: "auto", ExpiryTime: tt.expiry, Enable: true}
			settings, _ := json.Marshal(map[string]any{"clients": []model.Client{client}})
			inbound := &model.Inbound{Remark: "shared-reality", Protocol: model.VLESS, Port: 443, Settings: string(settings), StreamSettings: `{"network":"tcp","security":"none","tcpSettings":{"header":{"type":"none"}}}`}
			s := &SubService{remarkModel: "-i", address: "example.com"}
			if got := s.genRemark(inbound, client.Email, ""); got != tt.want {
				t.Fatalf("name = %q, want %q", got, tt.want)
			}
			if got := s.genRemark(inbound, client.Email, "B"); got != tt.want+" | B" {
				t.Fatalf("extra name = %q", got)
			}
			u, err := url.Parse(s.getLink(inbound, client.Email))
			if err != nil || u.Fragment != tt.want || u.User.Username() != client.ID {
				t.Fatalf("VLESS alias/credential mismatch: %v", err)
			}
			clash := NewSubClashService(s).getProxies(inbound, client, "example.com")
			if len(clash) != 1 || clash[0]["name"] != tt.want || clash[0]["uuid"] != client.ID {
				t.Fatal("Clash alias/credential mismatch")
			}
			configs := NewSubJsonService("", "", "", "", s).getConfig(inbound, client, "example.com")
			var config map[string]any
			if len(configs) != 1 {
				t.Fatal("expected one JSON configuration")
			}
			if err := json.Unmarshal(configs[0], &config); err != nil || config["remarks"] != tt.want {
				t.Fatal("JSON alias mismatch")
			}
			inbound.Protocol = model.VMESS
			payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s.getLink(inbound, client.Email), "vmess://"))
			var vmess map[string]any
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(payload, &vmess); err != nil || vmess["ps"] != tt.want || vmess["id"] != client.ID {
				t.Fatal("VMess alias/credential mismatch")
			}
			s.showInfo = true
			inbound.ClientStats = []xray.ClientTraffic{{Email: client.Email, Enable: false}}
			if got := s.genRemark(inbound, client.Email, ""); got != tt.want+" | ⛔️N/A" {
				t.Fatalf("disabled name must still start with code: %q", got)
			}
		})
	}
}

func TestFormatClientIdentifierSuffix(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  string
	}{
		{name: "current full date id", email: "20261123-test01", want: "test01"},
		{name: "legacy month day id", email: "1123-test02", want: "test02"},
		{name: "custom temporary name", email: "临时测试", want: "临时测试"},
		{name: "blank", email: " ", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatClientIdentifierSuffix(tt.email); got != tt.want {
				t.Fatalf("formatClientIdentifierSuffix(%q) = %q, want %q", tt.email, got, tt.want)
			}
		})
	}
}
