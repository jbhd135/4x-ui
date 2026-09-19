package service

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	xuilogger "github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"github.com/op/go-logging"
)

func TestDailyClientTrafficThreshold(t *testing.T) {
	if got, want := dailyClientTrafficThreshold(dailyClientTrafficLimit10GBBytes, 0), int64(10*1024*1024*1024); got != want {
		t.Fatalf("default daily threshold = %d, want %d", got, want)
	}
	override := int64(25 * 1024 * 1024 * 1024)
	if got := dailyClientTrafficThreshold(dailyClientTrafficLimit20GBBytes, override); got != override {
		t.Fatalf("manual override threshold = %d, want %d", got, override)
	}
	if got := dailyClientTrafficThreshold(0, 0); got != 0 {
		t.Fatalf("unlimited daily threshold = %d, want 0", got)
	}
}

func TestDailyClientTrafficLimitReached(t *testing.T) {
	limit := dailyClientTrafficLimit10GBBytes
	if dailyClientTrafficLimitReached(limit-1, limit, 0) {
		t.Fatal("traffic below the default limit should remain enabled")
	}
	if !dailyClientTrafficLimitReached(limit, limit, 0) {
		t.Fatal("traffic at the default limit should be blocked")
	}
	override := limit * 2
	if dailyClientTrafficLimitReached(limit, limit, override) {
		t.Fatal("traffic below a manual override should remain enabled")
	}
	if !dailyClientTrafficLimitReached(override, limit, override) {
		t.Fatal("traffic at a manual override should be blocked")
	}
	if dailyClientTrafficLimitReached(override, 0, 0) {
		t.Fatal("unlimited daily traffic should never be blocked")
	}
}

func TestValidDailyClientTrafficLimit(t *testing.T) {
	for _, limit := range []int64{
		0,
		dailyClientTrafficLimit10GBBytes,
		dailyClientTrafficLimit20GBBytes,
		dailyClientTrafficLimit30GBBytes,
	} {
		if !validDailyClientTrafficLimit(limit) {
			t.Fatalf("supported daily traffic limit %d was rejected", limit)
		}
	}

	for _, limit := range []int64{
		legacyDailyClientTrafficLimit5GBBytes,
		legacyDailyClientTrafficLimit15GBBytes,
		7 * 1024 * 1024 * 1024,
	} {
		if validDailyClientTrafficLimit(limit) {
			t.Fatalf("unsupported daily traffic limit %d was accepted", limit)
		}
	}
}

func TestStaleDailyOverrideDoesNotCarryAcrossDays(t *testing.T) {
	setupDailyTrafficTestDB(t)

	today := (&InboundService{}).monitorTrafficDate()
	inbound := &model.Inbound{
		Tag:               "stale-daily-override-inbound",
		Port:              23459,
		Protocol:          model.VMESS,
		Enable:            true,
		DailyTrafficLimit: dailyClientTrafficLimit10GBBytes,
		Settings:          `{"clients":[]}`,
	}
	if err := database.GetDB().Create(inbound).Error; err != nil {
		t.Fatal(err)
	}
	traffic := &xray.ClientTraffic{
		InboundId:          inbound.Id,
		Email:              "stale-daily-override-client",
		Enable:             true,
		DailyTrafficLimit:  dailyClientTrafficLimit10GBBytes,
		DailyOverrideLimit: 40 * 1024 * 1024 * 1024,
		// Empty is the legacy value after AutoMigrate adds the date column.
		DailyOverrideDate: "",
	}
	if err := database.GetDB().Create(traffic).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.DailyClientTraffic{
		Date:        today,
		InboundId:   inbound.Id,
		ClientEmail: traffic.Email,
		Down:        11 * 1024 * 1024 * 1024,
	}).Error; err != nil {
		t.Fatal(err)
	}

	service := &InboundService{}
	if _, err := service.ResetDailyClientTrafficLimits(); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().First(traffic, traffic.Id).Error; err != nil {
		t.Fatal(err)
	}
	if traffic.DailyOverrideLimit != 0 || traffic.DailyOverrideDate != "" {
		t.Fatalf("stale override was not cleared: limit=%d date=%q", traffic.DailyOverrideLimit, traffic.DailyOverrideDate)
	}
	if _, count, err := service.disableDailyLimitClients(database.GetDB()); err != nil {
		t.Fatal(err)
	} else if count != 1 {
		t.Fatalf("disabled client count = %d, want 1", count)
	}
}

func TestAddClientStatAssignsDefaultDailyLimit(t *testing.T) {
	setupDailyTrafficTestDB(t)

	inbound := &model.Inbound{
		Tag:      "client-default-daily-limit-inbound",
		Port:     23461,
		Protocol: model.VMESS,
		Enable:   true,
		Settings: `{"clients":[]}`,
	}
	if err := database.GetDB().Create(inbound).Error; err != nil {
		t.Fatal(err)
	}

	client := &model.Client{Email: "client-default-daily-limit", Enable: true}
	if err := (&InboundService{}).AddClientStat(database.GetDB(), inbound.Id, client); err != nil {
		t.Fatal(err)
	}

	var saved xray.ClientTraffic
	if err := database.GetDB().Where("inbound_id = ? AND email = ?", inbound.Id, client.Email).First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if got := saved.DailyTrafficLimit; got != dailyClientTrafficLimit10GBBytes {
		t.Fatalf("new client daily limit = %d, want %d", got, dailyClientTrafficLimit10GBBytes)
	}
}

func TestMigrateLegacyDailyTrafficLimits(t *testing.T) {
	setupDailyTrafficTestDB(t)

	inbounds := []model.Inbound{
		{
			Tag:               "legacy-5gb-daily-limit-inbound",
			Port:              23462,
			Protocol:          model.VMESS,
			DailyTrafficLimit: legacyDailyClientTrafficLimit5GBBytes,
			Settings:          `{"clients":[]}`,
		},
		{
			Tag:               "legacy-15gb-daily-limit-inbound",
			Port:              23463,
			Protocol:          model.VMESS,
			DailyTrafficLimit: legacyDailyClientTrafficLimit15GBBytes,
			Settings:          `{"clients":[]}`,
		},
		{
			Tag:               "unlimited-daily-limit-inbound",
			Port:              23464,
			Protocol:          model.VMESS,
			DailyTrafficLimit: 0,
			Settings:          `{"clients":[]}`,
		},
	}
	if err := database.GetDB().Create(&inbounds).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(&model.Inbound{}).
		Where("id = ?", inbounds[2].Id).
		Update("daily_traffic_limit", 0).Error; err != nil {
		t.Fatal(err)
	}

	clients := []xray.ClientTraffic{
		{InboundId: inbounds[0].Id, Email: "legacy-5gb-client", DailyTrafficLimit: legacyDailyClientTrafficLimit5GBBytes},
		{InboundId: inbounds[1].Id, Email: "legacy-15gb-client", DailyTrafficLimit: legacyDailyClientTrafficLimit15GBBytes},
		{InboundId: inbounds[2].Id, Email: "unlimited-client", DailyTrafficLimit: 0},
	}
	if err := database.GetDB().Create(&clients).Error; err != nil {
		t.Fatal(err)
	}

	if err := (&InboundService{}).migrateLegacyDailyTrafficLimits(); err != nil {
		t.Fatal(err)
	}

	inboundLimits := map[string]int64{
		"legacy-5gb-daily-limit-inbound":  dailyClientTrafficLimit10GBBytes,
		"legacy-15gb-daily-limit-inbound": dailyClientTrafficLimit20GBBytes,
		"unlimited-daily-limit-inbound":   0,
	}
	for tag, want := range inboundLimits {
		var inbound model.Inbound
		if err := database.GetDB().Where("tag = ?", tag).First(&inbound).Error; err != nil {
			t.Fatal(err)
		}
		if inbound.DailyTrafficLimit != want {
			t.Fatalf("inbound %q daily limit = %d, want %d", tag, inbound.DailyTrafficLimit, want)
		}
	}

	clientLimits := map[string]int64{
		"legacy-5gb-client":  dailyClientTrafficLimit10GBBytes,
		"legacy-15gb-client": dailyClientTrafficLimit20GBBytes,
		"unlimited-client":   0,
	}
	for email, want := range clientLimits {
		var client xray.ClientTraffic
		if err := database.GetDB().Where("email = ?", email).First(&client).Error; err != nil {
			t.Fatal(err)
		}
		if client.DailyTrafficLimit != want {
			t.Fatalf("client %q daily limit = %d, want %d", email, client.DailyTrafficLimit, want)
		}
	}
}

func setupDailyTrafficTestDB(t *testing.T) {
	t.Helper()
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	t.Setenv("XUI_LOG_FOLDER", dbDir)
	xuilogger.InitLogger(logging.ERROR)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("database.InitDB failed: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseDB(); err != nil {
			t.Logf("database.CloseDB warning: %v", err)
		}
	})
}

func TestTerminateInboundTCPConnections(t *testing.T) {
	previous := runSocketCommand
	t.Cleanup(func() {
		runSocketCommand = previous
	})

	var command string
	var args []string
	runSocketCommand = func(name string, commandArgs ...string) ([]byte, error) {
		command = name
		args = append([]string(nil), commandArgs...)
		return nil, nil
	}

	if err := terminateInboundTCPConnections(23456); err != nil {
		t.Fatalf("terminateInboundTCPConnections failed: %v", err)
	}
	if command != "ss" {
		t.Fatalf("command = %q, want ss", command)
	}
	wantArgs := []string{"-K", "state", "established", "sport = :23456"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestTerminateInboundTCPConnectionsRejectsInvalidPort(t *testing.T) {
	if err := terminateInboundTCPConnections(0); err == nil {
		t.Fatal("invalid port should be rejected")
	}
}

func TestDisableInvalidClientsByDailyTrafficLimit(t *testing.T) {
	setupDailyTrafficTestDB(t)

	settings, err := json.Marshal(map[string]any{
		"clients": []map[string]any{
			{"email": "daily-limit-test", "enable": true},
			{"email": "daily-limit-other", "enable": true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	inbound := &model.Inbound{
		Tag:               "daily-limit-test-inbound",
		Port:              23456,
		Protocol:          model.VMESS,
		Enable:            true,
		DailyTrafficLimit: dailyClientTrafficLimit10GBBytes,
		Settings:          string(settings),
	}
	if err := database.GetDB().Create(inbound).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&xray.ClientTraffic{
		InboundId:         inbound.Id,
		Email:             "daily-limit-test",
		Enable:            true,
		DailyTrafficLimit: dailyClientTrafficLimit10GBBytes,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&xray.ClientTraffic{
		InboundId:         inbound.Id,
		Email:             "daily-limit-other",
		Enable:            true,
		DailyTrafficLimit: dailyClientTrafficLimit10GBBytes,
	}).Error; err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(time.Local).Format("2006-01-02")
	if err := database.GetDB().Create(&model.DailyClientTraffic{
		Date:        today,
		InboundId:   inbound.Id,
		ClientEmail: "daily-limit-test",
		Down:        dailyClientTrafficLimit10GBBytes,
	}).Error; err != nil {
		t.Fatal(err)
	}

	needRestart, clientsDisabled, err := (&InboundService{}).AddTraffic(nil, nil)
	if err != nil {
		t.Fatalf("AddTraffic failed: %v", err)
	}
	if needRestart {
		t.Fatal("no Xray API restart should be required in the database-only test")
	}
	if !clientsDisabled {
		t.Fatal("daily-limit disable must report a disabled client")
	}

	var traffic xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", "daily-limit-test").First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if traffic.Enable {
		t.Fatal("client should be disabled after its daily limit is reached")
	}
	var savedInbound model.Inbound
	if err := database.GetDB().First(&savedInbound, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !savedInbound.Enable {
		t.Fatal("shared inbound must remain enabled after one client reaches its daily limit")
	}
	if traffic.DailyBlockedDate != today {
		t.Fatalf("client daily blocked date = %q, want %q", traffic.DailyBlockedDate, today)
	}
	var other xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", "daily-limit-other").First(&other).Error; err != nil {
		t.Fatal(err)
	}
	if !other.Enable || other.DailyBlockedDate != "" {
		t.Fatalf("other client on the shared inbound was changed: enable=%v blocked=%q", other.Enable, other.DailyBlockedDate)
	}

	if err := database.GetDB().Model(&traffic).Updates(map[string]any{
		"daily_blocked_date": "2000-01-01",
		"enable":             false,
	}).Error; err != nil {
		t.Fatal(err)
	}
	resetCount, err := (&InboundService{}).ResetDailyClientTrafficLimits()
	if err != nil {
		t.Fatalf("ResetDailyClientTrafficLimits failed: %v", err)
	}
	if resetCount != 1 {
		t.Fatalf("reset count = %d, want 1", resetCount)
	}
	if err := database.GetDB().First(&traffic, traffic.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !traffic.Enable {
		t.Fatal("client should be re-enabled after the day changes")
	}
}

func TestResetDailyClientTrafficLimitsRepairsSplitEnableState(t *testing.T) {
	setupDailyTrafficTestDB(t)

	settings, err := json.Marshal(map[string]any{
		"clients": []map[string]any{
			{"email": "stale-daily-client", "enable": false},
			{"email": "manual-disabled-client", "enable": false},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	inbound := &model.Inbound{
		Tag:      "stale-daily-client-inbound",
		Port:     23460,
		Protocol: model.VMESS,
		Enable:   true,
		Settings: string(settings),
	}
	if err := database.GetDB().Create(inbound).Error; err != nil {
		t.Fatal(err)
	}

	expiry := time.Now().Add(24 * time.Hour).UnixMilli()
	for _, traffic := range []*xray.ClientTraffic{
		{
			InboundId:         inbound.Id,
			Email:             "stale-daily-client",
			Enable:            true,
			ExpiryTime:        expiry,
			DailyTrafficLimit: dailyClientTrafficLimit10GBBytes,
		},
		{
			InboundId:         inbound.Id,
			Email:             "manual-disabled-client",
			Enable:            false,
			ExpiryTime:        expiry,
			DailyTrafficLimit: dailyClientTrafficLimit10GBBytes,
		},
	} {
		if err := database.GetDB().Create(traffic).Error; err != nil {
			t.Fatal(err)
		}
	}

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	for _, email := range []string{"stale-daily-client", "manual-disabled-client"} {
		if err := database.GetDB().Create(&model.DailyClientTraffic{
			Date:        yesterday,
			InboundId:   inbound.Id,
			ClientEmail: email,
			Down:        dailyClientTrafficLimit10GBBytes,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}

	resetCount, err := (&InboundService{}).ResetDailyClientTrafficLimits()
	if err != nil {
		t.Fatalf("ResetDailyClientTrafficLimits failed: %v", err)
	}
	if resetCount != 1 {
		t.Fatalf("reset count = %d, want 1", resetCount)
	}

	if err := database.GetDB().First(inbound, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	clients, err := (&InboundService{}).GetClients(inbound)
	if err != nil {
		t.Fatal(err)
	}
	enabled := make(map[string]bool, len(clients))
	for _, client := range clients {
		enabled[client.Email] = client.Enable
	}
	if !enabled["stale-daily-client"] {
		t.Fatal("stale daily-limit client was not repaired")
	}
	if enabled["manual-disabled-client"] {
		t.Fatal("manually disabled client should remain disabled")
	}
}

func TestUpdateInboundDailyTrafficLimitRestoresAndBlocks(t *testing.T) {
	setupDailyTrafficTestDB(t)

	today := time.Now().In(time.Local).Format("2006-01-02")
	inbound := &model.Inbound{
		Tag:      "daily-limit-selection-test",
		Port:     23457,
		Protocol: model.VMESS,
		Enable:   true,
		Settings: `{"clients":[{"email":"daily-limit-selection-client","enable":true}]}`,
	}
	if err := database.GetDB().Create(inbound).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&xray.ClientTraffic{
		InboundId:         inbound.Id,
		Email:             "daily-limit-selection-client",
		Enable:            true,
		DailyTrafficLimit: dailyClientTrafficLimit10GBBytes,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.DailyClientTraffic{
		Date:        today,
		InboundId:   inbound.Id,
		ClientEmail: "daily-limit-selection-client",
		Down:        25 * 1024 * 1024 * 1024,
	}).Error; err != nil {
		t.Fatal(err)
	}

	service := &InboundService{}
	updated, needRestart, err := service.UpdateClientDailyTrafficLimit(inbound.Id, "daily-limit-selection-client", dailyClientTrafficLimit30GBBytes)
	if err != nil {
		t.Fatal(err)
	}
	if needRestart || !updated.Enable || updated.DailyBlockedDate != "" {
		t.Fatalf("30 GB selection should leave the client enabled: restart=%v enable=%v blocked=%q", needRestart, updated.Enable, updated.DailyBlockedDate)
	}

	updated, needRestart, err = service.UpdateClientDailyTrafficLimit(inbound.Id, "daily-limit-selection-client", dailyClientTrafficLimit20GBBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !needRestart || updated.Enable || updated.DailyBlockedDate != today {
		t.Fatalf("20 GB selection should block the client: restart=%v enable=%v blocked=%q", needRestart, updated.Enable, updated.DailyBlockedDate)
	}

	updated, needRestart, err = service.UpdateClientDailyTrafficLimit(inbound.Id, "daily-limit-selection-client", dailyClientTrafficLimit10GBBytes)
	if err != nil {
		t.Fatal(err)
	}
	if needRestart || updated.Enable || updated.DailyBlockedDate != today {
		t.Fatalf("10 GB selection should leave the client blocked: restart=%v enable=%v blocked=%q", needRestart, updated.Enable, updated.DailyBlockedDate)
	}

	updated, needRestart, err = service.UpdateClientDailyTrafficLimit(inbound.Id, "daily-limit-selection-client", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !needRestart || !updated.Enable || updated.DailyTrafficLimit != 0 {
		t.Fatalf("unlimited selection should restore the client: restart=%v enable=%v limit=%d", needRestart, updated.Enable, updated.DailyTrafficLimit)
	}

	var saved xray.ClientTraffic
	if err := database.GetDB().Where("inbound_id = ? AND email = ?", inbound.Id, "daily-limit-selection-client").First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.DailyTrafficLimit != 0 {
		t.Fatalf("reloaded unlimited limit = %d, want 0", saved.DailyTrafficLimit)
	}

	// Traffic collection saves client rows as a slice. Unlimited must survive
	// that periodic write instead of being replaced by the model default.
	saved.Up++
	if err := database.GetDB().Save([]*xray.ClientTraffic{&saved}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Where("inbound_id = ? AND email = ?", inbound.Id, "daily-limit-selection-client").First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.DailyTrafficLimit != 0 {
		t.Fatalf("unlimited limit after traffic save = %d, want 0", saved.DailyTrafficLimit)
	}

	inbounds, err := service.GetInbounds(inbound.UserId)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbounds) != 1 || len(inbounds[0].ClientStats) != 1 {
		t.Fatalf("reloaded inbounds = %d, client stats = %d, want 1 and 1", len(inbounds), len(inbounds[0].ClientStats))
	}
	if got := inbounds[0].ClientStats[0].DailyTrafficLimit; got != 0 {
		t.Fatalf("API list unlimited limit = %d, want 0", got)
	}

	for _, limit := range []int64{legacyDailyClientTrafficLimit5GBBytes, legacyDailyClientTrafficLimit15GBBytes} {
		if _, _, err := service.UpdateClientDailyTrafficLimit(inbound.Id, "daily-limit-selection-client", limit); err == nil {
			t.Fatalf("unsupported daily traffic limit %d should be rejected", limit)
		}
	}
}

func TestGetInboundsIncludesTodayTraffic(t *testing.T) {
	setupDailyTrafficTestDB(t)

	inbound := &model.Inbound{
		UserId:            7,
		Tag:               "today-traffic-test-inbound",
		Port:              23458,
		Protocol:          model.VMESS,
		Enable:            true,
		DailyTrafficLimit: dailyClientTrafficLimit10GBBytes,
		Settings:          `{"clients":[]}`,
	}
	if err := database.GetDB().Create(inbound).Error; err != nil {
		t.Fatal(err)
	}
	clientStats := []xray.ClientTraffic{
		{InboundId: inbound.Id, Email: "today-a", Enable: true},
		{InboundId: inbound.Id, Email: "today-b", Enable: true},
	}
	if err := database.GetDB().Create(&clientStats).Error; err != nil {
		t.Fatal(err)
	}

	service := &InboundService{}
	today := service.monitorTrafficDate()
	rows := []model.DailyClientTraffic{
		{Date: today, InboundId: inbound.Id, ClientEmail: "today-a", Up: 100, Down: 200},
		{Date: today, InboundId: inbound.Id, ClientEmail: "today-b", Up: 300, Down: 400},
		{Date: "2000-01-01", InboundId: inbound.Id, ClientEmail: "old", Up: 5000, Down: 5000},
	}
	if err := database.GetDB().Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	inbounds, err := service.GetInbounds(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbounds) != 1 {
		t.Fatalf("inbound count = %d, want 1", len(inbounds))
	}
	if got, want := inbounds[0].TodayTraffic, int64(1000); got != want {
		t.Fatalf("today traffic = %d, want %d", got, want)
	}
	if got, want := inbounds[0].ClientStats[0].TodayTraffic+inbounds[0].ClientStats[1].TodayTraffic, int64(1000); got != want {
		t.Fatalf("client today traffic total = %d, want %d", got, want)
	}
}
