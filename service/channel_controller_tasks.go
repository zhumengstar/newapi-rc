package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// ChannelRecoverySummary describes a passive recovery pass. The actual probe
// is the existing built-in channel test, so provider-specific request shaping
// remains in one place and recovery never bypasses relay authentication.
type ChannelRecoverySummary struct {
	Attempted int   `json:"attempted"`
	Recovered int   `json:"recovered"`
	Pending   int   `json:"pending"`
	Failed    int   `json:"failed"`
	At        int64 `json:"at"`
}

// Recovery settings are intentionally opt-in at the application level. This
// keeps manually disabled channels untouched and lets operators enable recovery
// only after AutomaticEnableChannelEnabled has been reviewed.
func ChannelRecoveryEnabled() bool {
	if !common.AutomaticEnableChannelEnabled {
		return false
	}
	if common.GetEnvOrDefaultBool("CHANNEL_RECOVERY_TASK_ENABLED", false) {
		// Enabling the timer alone must not broaden scope. Operators must also
		// explicitly opt into probing channels without a group policy.
		if ChannelControlProbeAllEnabled() {
			return true
		}
	}
	policies, err := model.GetEnabledChannelControlPolicies(context.Background())
	if err != nil {
		return false
	}
	for _, policy := range policies {
		if policy.ProbeEnabled && policy.RecoveryEnabled {
			return true
		}
	}
	return false
}

func ChannelRecoveryIntervalSeconds() int {
	value := common.GetEnvOrDefault("CHANNEL_RECOVERY_INTERVAL_SECONDS", 1800)
	if value < 60 {
		return 1800
	}
	if value > 86400 {
		return 86400
	}
	return value
}

func ChannelHealthCheckEnabled() bool {
	if ChannelControlProbeAllEnabled() {
		return true
	}
	if common.GetEnvOrDefaultBool("CHANNEL_HEALTH_CHECK_ENABLED", false) {
		return true
	}
	policies, err := model.GetEnabledChannelControlPolicies(context.Background())
	if err != nil {
		return false
	}
	for _, policy := range policies {
		if policy.ProbeEnabled {
			return true
		}
	}
	return false
}

// ChannelControlProbeAllEnabled is deliberately separate from the task
// enablement switches. Health/recovery timers remain policy-scoped by default;
// this flag is the explicit opt-in for probing channels with no group policy.
func ChannelControlProbeAllEnabled() bool {
	return common.GetEnvOrDefaultBool("CHANNEL_CONTROL_PROBE_ALL_ENABLED", false)
}

func ChannelHealthCheckIntervalSeconds() int {
	value := common.GetEnvOrDefault("CHANNEL_HEALTH_CHECK_INTERVAL_SECONDS", 300)
	if value < 30 {
		return 300
	}
	if value > 86400 {
		return 86400
	}
	return value
}

// RunChannelRecoveryOnce selects only auto-disabled channels. The controller
// package supplies the test runner through the callback to avoid a service to
// controller import cycle.
func RunChannelRecoveryOnce(ctx context.Context, run func(context.Context) (int, int, int, error)) (ChannelRecoverySummary, error) {
	summary := ChannelRecoverySummary{At: common.GetTimestamp()}
	if run == nil {
		return summary, errors.New("channel recovery runner is not configured")
	}
	attempted, recovered, pending, err := run(ctx)
	summary.Attempted = attempted
	summary.Recovered = recovered
	summary.Pending = pending
	if attempted >= recovered+pending {
		summary.Failed = attempted - recovered - pending
	}
	return summary, err
}

type PriorityNormalizeSummary struct {
	ChannelsUpdated int   `json:"channels_updated"`
	Groups          int   `json:"groups"`
	Step            int64 `json:"step"`
	At              int64 `json:"at"`
}

func PriorityNormalizerEnabled() bool {
	return common.GetEnvOrDefaultBool("CHANNEL_PRIORITY_NORMALIZER_ENABLED", false)
}

func PriorityNormalizerIntervalSeconds() int {
	value := common.GetEnvOrDefault("CHANNEL_PRIORITY_NORMALIZER_INTERVAL_SECONDS", 21600)
	if value < 300 {
		return 21600
	}
	if value > 604800 {
		return 604800
	}
	return value
}

func priorityNormalizeStep() int64 {
	raw := strings.TrimSpace(os.Getenv("CHANNEL_PRIORITY_NORMALIZER_STEP"))
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		return 10
	}
	if value > 1000 {
		return 1000
	}
	return value
}

// RunPriorityNormalizerOnce gives each non-empty group one stable priority,
// ordered by its first channel id. It updates channels and abilities together
// so retry selection cannot observe a split priority during the operation.
func RunPriorityNormalizerOnce(ctx context.Context) (PriorityNormalizeSummary, error) {
	summary := PriorityNormalizeSummary{At: common.GetTimestamp(), Step: priorityNormalizeStep()}
	if model.DB == nil {
		return summary, errors.New("main database is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	var channels []model.Channel
	if err := model.DB.WithContext(ctx).Order("id ASC").Find(&channels).Error; err != nil {
		return summary, fmt.Errorf("load channels for priority normalization: %w", err)
	}
	groupFirstID := make(map[string]int)
	for _, channel := range channels {
		for _, group := range channel.GetGroups() {
			if first, ok := groupFirstID[group]; !ok || channel.Id < first {
				groupFirstID[group] = channel.Id
			}
		}
	}
	groups := make([]string, 0, len(groupFirstID))
	for group := range groupFirstID {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool { return groupFirstID[groups[i]] < groupFirstID[groups[j]] })
	summary.Groups = len(groups)
	updated, err := model.ApplyChannelGroupPriorityOrderWithStep(ctx, groups, summary.Step)
	summary.ChannelsUpdated = int(updated)
	if err != nil {
		return summary, fmt.Errorf("normalize channel priorities: %w", err)
	}
	if summary.ChannelsUpdated > 0 {
		model.InitChannelCache()
	}
	return summary, nil
}
