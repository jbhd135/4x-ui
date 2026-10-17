package job

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/op/go-logging"
)

var loggerInitOnce sync.Once

func setupIntegrationDB(t *testing.T) {
	t.Helper()
	loggerInitOnce.Do(func() { logger.InitLogger(logging.ERROR) })
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	t.Setenv("XUI_LOG_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
}

func TestClientIPDatabasePreservesAccountAndHistory(t *testing.T) {
	setupIntegrationDB(t)
	settings := `{"clients":[{"id":"test-uuid","email":"customer-a","enable":true,"limitIp":1,"expiryTime":1900000000000},{"email":"disabled","enable":false,"limitIp":1}]}`
	inbound := model.Inbound{Tag: testAccount.Inbound, Enable: true, Protocol: model.VLESS, Port: 4321, Settings: settings}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	old, _ := json.Marshal([]IPWithTimestamp{{"192.0.2.99", testStart.Add(-5 * time.Minute).Unix()}})
	row := model.InboundClientIps{ClientEmail: testAccount.Email, Ips: string(old)}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	limits, err := loadClientIPLimits()
	if err != nil {
		t.Fatal(err)
	}
	if len(limits) != 1 || limits[testAccount] != 1 {
		t.Fatalf("limits: %v", limits)
	}
	j := NewCheckClientIpJob(nil)
	if got := j.evaluate(limits, observe(testAccount, testStart, "192.0.2.1"), testStart); len(got) != 0 {
		t.Fatal("history blocked current IP")
	}
	if err := j.saveHistory(observe(testAccount, testStart, "192.0.2.1"), testStart); err != nil {
		t.Fatal(err)
	}
	continuouslyOverLimit(j)
	if err := database.GetDB().First(&inbound, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	if inbound.Settings != settings || !inbound.Enable {
		t.Fatal("enforcement modified customer credentials/settings")
	}
	if err := database.GetDB().First(&row, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	var ips []IPWithTimestamp
	if err := json.Unmarshal([]byte(row.Ips), &ips); err != nil {
		t.Fatal(err)
	}
	if len(ips) != 2 {
		t.Fatalf("history lost: %v", ips)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("XUI_LOG_FOLDER"), "3xipl.log")); !os.IsNotExist(err) {
		t.Fatal("legacy IP-ban log was written")
	}
	if err := database.GetDB().Model(&inbound).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	limits, err = loadClientIPLimits()
	if err != nil {
		t.Fatal(err)
	}
	if got := j.evaluate(limits, nil, testStart.Add(110*time.Second)); len(got) != 0 {
		t.Fatal("disabled account restriction retained")
	}
}

func TestClientIPLogReadRotationPartialAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	line := func(ip string) string {
		return fmt.Sprintf("2026/10/17 12:00:00.123456 from tcp:%s:12345 accepted tcp:example.com:443 [inbound-test -> direct] email: customer-a\n", ip)
	}
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	j := NewCheckClientIpJob(nil)
	write(line("192.0.2.1") + line("[2001:db8::1]") + "2026/10/17 12:00")
	got, err := j.readObservations(path, testStart)
	if err != nil || len(got) != 2 {
		t.Fatalf("read: %v %v", got, err)
	}
	if got, err := j.readObservations(path, testStart); err != nil || len(got) != 0 {
		t.Fatalf("repeat/partial line: %v %v", got, err)
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	write(line("192.0.2.2") + line("192.0.2.2") + line("127.0.0.1"))
	got, err = j.readObservations(path, testStart)
	if err != nil || len(got) != 1 || got[0].IP != "192.0.2.2" {
		t.Fatalf("rotation/dedup: %v %v", got, err)
	}
	write(line("192.0.2.3"))
	got, err = j.readObservations(path, testStart)
	if err != nil || len(got) != 1 || got[0].IP != "192.0.2.3" {
		t.Fatalf("truncation: %v %v", got, err)
	}
	write("malformed\n")
	got, err = j.readObservations(path, testStart)
	if err != nil || len(got) != 0 {
		t.Fatalf("malformed: %v %v", got, err)
	}
}
