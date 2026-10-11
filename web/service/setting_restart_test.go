package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestRestartXrayOnClientDisableDefault(t *testing.T) {
	setupDailyTrafficTestDB(t)
	service := &SettingService{}

	restart, err := service.GetRestartXrayOnClientDisable()
	if err != nil {
		t.Fatal(err)
	}
	if restart {
		t.Fatal("missing setting must not enable global Xray restarts")
	}
	settings, err := service.GetAllSetting()
	if err != nil {
		t.Fatal(err)
	}
	if settings.RestartXrayOnClientDisable {
		t.Fatal("settings API must default to no global Xray restart")
	}
	var count int64
	if err := database.GetDB().Model(&model.Setting{}).
		Where("key = ?", "restartXrayOnClientDisable").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("reading the default must not create a stored setting")
	}
}

func TestRestartXrayOnClientDisablePreservesStoredChoice(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value bool
	}{
		{name: "explicitly enabled", value: true},
		{name: "explicitly disabled", value: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupDailyTrafficTestDB(t)
			service := &SettingService{}
			if err := service.SetRestartXrayOnClientDisable(tc.value); err != nil {
				t.Fatal(err)
			}
			reader := &SettingService{}
			restart, err := reader.GetRestartXrayOnClientDisable()
			if err != nil {
				t.Fatal(err)
			}
			if restart != tc.value {
				t.Fatalf("stored choice = %v, want %v", restart, tc.value)
			}
			settings, err := reader.GetAllSetting()
			if err != nil {
				t.Fatal(err)
			}
			if settings.RestartXrayOnClientDisable != tc.value {
				t.Fatalf("settings API choice = %v, want %v", settings.RestartXrayOnClientDisable, tc.value)
			}
		})
	}
}
