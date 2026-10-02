package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestClientLookupCodeValidationAllowsRenewal(t *testing.T) {
	setupDailyTrafficTestDB(t)
	inbound := model.Inbound{Port: 443, Protocol: model.VLESS, Settings: `{"clients":[{"email":"20261008-azxd4p"}]}`}
	if err := database.GetDB().Create(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	s := &InboundService{}
	renewed := []model.Client{{Email: "20270501-azxd4p"}}
	if _, err := s.checkEmailsExistForClients(renewed); err == nil {
		t.Fatal("new customer must not reuse an existing customer's code")
	}
	if duplicate, err := s.checkEmailsExistForClients(renewed, "20261008-azxd4p"); err != nil || duplicate != "" {
		t.Fatalf("renewal of the same client must keep its code: %q, %v", duplicate, err)
	}
	if duplicate, err := s.checkEmailsExistForClients([]model.Client{{Email: "20261008-azxd4p"}}); err != nil || duplicate == "" {
		t.Fatalf("exact duplicate email validation must remain intact: %q, %v", duplicate, err)
	}
}
