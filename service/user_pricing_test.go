package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNormalizeUserGroups(t *testing.T) {
	got := NormalizeUserGroups("vip", []string{"developer", "vip", "developer"})
	assert.Equal(t, []string{"developer", "vip"}, got)
}

func TestUserPricingRuleIsActive(t *testing.T) {
	rule := model.UserPricingRule{Enabled: true, ValidFrom: 10, ValidTo: 20}
	if rule.IsActive(9) || !rule.IsActive(10) || !rule.IsActive(20) || rule.IsActive(21) {
		t.Fatal("unexpected validity window")
	}
}
