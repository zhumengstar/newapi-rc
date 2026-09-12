package service

import (
	"context"
	"github.com/QuantumNous/new-api/model"
	"sort"
	"time"
)

type PricingQuery struct {
	UserID                               int
	UserGroups                           []string
	BillingGroup, ModelName, BillingType string
}

// ResolveUserPricing returns the most specific active rule. Existing ratio
// settings remain the fallback, so enabling this table is backwards compatible.
func ResolveUserPricing(ctx context.Context, q PricingQuery) (*model.UserPricingRule, error) {
	if model.DB == nil {
		return nil, nil
	}
	var rules []model.UserPricingRule
	if err := model.DB.WithContext(ctx).Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	var best *model.UserPricingRule
	bestScore := -1
	for i := range rules {
		r := &rules[i]
		if !r.IsActive(now) || (r.UserID != nil && *r.UserID != q.UserID) {
			continue
		}
		if r.UserGroup != "" && !contains(q.UserGroups, r.UserGroup) {
			continue
		}
		if r.BillingGroup != "" && r.BillingGroup != q.BillingGroup {
			continue
		}
		if r.ModelName != "" && r.ModelName != q.ModelName {
			continue
		}
		if r.BillingType != "" && r.BillingType != q.BillingType {
			continue
		}
		score := 0
		if r.UserID != nil {
			score += 16
		}
		if r.UserGroup != "" {
			score += 8
		}
		if r.BillingGroup != "" {
			score += 4
		}
		if r.ModelName != "" {
			score += 2
		}
		if r.BillingType != "" {
			score++
		}
		if best == nil || score > bestScore || (score == bestScore && r.Priority > best.Priority) {
			best, bestScore = r, score
		}
	}
	return best, nil
}

func contains(values []string, wanted string) bool {
	for _, v := range values {
		if v == wanted {
			return true
		}
	}
	return false
}

func NormalizeUserGroups(primary string, groups []string) []string {
	out := make([]string, 0, len(groups)+1)
	seen := make(map[string]struct{}, len(groups)+1)
	for _, group := range append([]string{primary}, groups...) {
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		out = append(out, group)
	}
	sort.Strings(out)
	return out
}
