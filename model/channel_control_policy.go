package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ChannelProbeModeAuto      = "auto"
	ChannelProbeModeChat      = "chat"
	ChannelProbeModeResponses = "responses"
	ChannelProbeModeImage     = "image"

	defaultChannelRecoverySuccesses = 2
	maxChannelRecoverySuccesses     = 10
)

// ChannelControlPolicy makes the former per-group maintenance scripts part of
// the application configuration. A policy only affects the exact routing group
// named by Group; it never changes a channel's manual status or credentials.
type ChannelControlPolicy struct {
	Group string `json:"group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`

	Enabled bool `json:"enabled"`

	AdaptiveEnabled         bool `json:"adaptive_enabled"`
	AdaptiveWindowSeconds   int  `json:"adaptive_window_seconds"`
	AdaptiveMinSamples      int  `json:"adaptive_min_samples"`
	AdaptiveSlowThresholdMs int  `json:"adaptive_slow_threshold_ms"`
	AdaptiveMinWeight       uint `json:"adaptive_min_weight"`
	AdaptiveMaxWeight       uint `json:"adaptive_max_weight"`
	AdaptiveRecoveryWeight  uint `json:"adaptive_recovery_weight"`
	AdaptiveCooldownSeconds int  `json:"adaptive_cooldown_seconds"`

	ProbeEnabled bool   `json:"probe_enabled"`
	ProbeMode    string `json:"probe_mode" gorm:"type:varchar(16)"`
	ProbeModel   string `json:"probe_model" gorm:"type:varchar(255)"`

	RecoveryEnabled           bool `json:"recovery_enabled"`
	RecoverySuccessesRequired int  `json:"recovery_successes_required"`

	CreatedAt int64 `json:"created_at" gorm:"bigint;index"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint;index"`
}

// ChannelRecoveryState records only the probe outcome required to avoid
// recovering an automatically disabled channel from one accidental success.
// It deliberately has no request content, upstream key, or error payload.
type ChannelRecoveryState struct {
	ChannelID     int   `json:"channel_id" gorm:"primaryKey;autoIncrement:false"`
	SuccessStreak int   `json:"success_streak"`
	LastProbeAt   int64 `json:"last_probe_at" gorm:"bigint;index"`
	LastSuccessAt int64 `json:"last_success_at" gorm:"bigint"`
	LastFailureAt int64 `json:"last_failure_at" gorm:"bigint"`
	CreatedAt     int64 `json:"created_at" gorm:"bigint;index"`
	UpdatedAt     int64 `json:"updated_at" gorm:"bigint;index"`
}

// ChannelErrorGuardState persists a channel-level cooldown after a hard
// authentication/quota failure so every process observes the same circuit.
type ChannelErrorGuardState struct {
	ChannelID     int    `gorm:"primaryKey" json:"channel_id"`
	CooldownUntil int64  `gorm:"bigint;index" json:"cooldown_until"`
	LastTrippedAt int64  `gorm:"bigint" json:"last_tripped_at"`
	Signature     string `gorm:"type:varchar(96)" json:"signature"`
}

func SetChannelErrorGuardCooldown(ctx context.Context, channelID int, until int64, signature string) error {
	if channelID <= 0 || until <= 0 {
		return errors.New("channel cooldown requires channel id and expiry")
	}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"cooldown_until", "last_tripped_at", "signature"}),
	}).Create(&ChannelErrorGuardState{ChannelID: channelID, CooldownUntil: until, LastTrippedAt: common.GetTimestamp(), Signature: strings.TrimSpace(signature)}).Error
}

func GetChannelErrorGuardCooldowns(ctx context.Context, channelIDs []int) (map[int]int64, error) {
	result := make(map[int]int64)
	if len(channelIDs) == 0 {
		return result, nil
	}
	var states []ChannelErrorGuardState
	if err := DB.WithContext(ctx).Where("channel_id IN ?", channelIDs).Find(&states).Error; err != nil {
		return nil, err
	}
	for _, state := range states {
		result[state.ChannelID] = state.CooldownUntil
	}
	return result, nil
}

func (policy *ChannelControlPolicy) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if policy.CreatedAt == 0 {
		policy.CreatedAt = now
	}
	if policy.UpdatedAt == 0 {
		policy.UpdatedAt = now
	}
	return nil
}

func (state *ChannelRecoveryState) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if state.CreatedAt == 0 {
		state.CreatedAt = now
	}
	if state.UpdatedAt == 0 {
		state.UpdatedAt = now
	}
	return nil
}

func (policy *ChannelControlPolicy) Normalize() error {
	policy.Group = strings.TrimSpace(policy.Group)
	if policy.Group == "" || len([]rune(policy.Group)) > 64 || strings.Contains(policy.Group, ",") {
		return errors.New("group must be 1-64 characters and cannot contain a comma")
	}
	policy.ProbeMode = strings.ToLower(strings.TrimSpace(policy.ProbeMode))
	if policy.ProbeMode == "" {
		policy.ProbeMode = ChannelProbeModeAuto
	}
	switch policy.ProbeMode {
	case ChannelProbeModeAuto, ChannelProbeModeChat, ChannelProbeModeResponses, ChannelProbeModeImage:
	default:
		return fmt.Errorf("unsupported probe mode %q", policy.ProbeMode)
	}
	policy.ProbeModel = strings.TrimSpace(policy.ProbeModel)
	if len([]rune(policy.ProbeModel)) > 255 {
		return errors.New("probe model must not exceed 255 characters")
	}
	if policy.RecoverySuccessesRequired <= 0 {
		policy.RecoverySuccessesRequired = defaultChannelRecoverySuccesses
	}
	if policy.RecoverySuccessesRequired > maxChannelRecoverySuccesses {
		return fmt.Errorf("recovery successes required must not exceed %d", maxChannelRecoverySuccesses)
	}
	return nil
}

func GetChannelControlPolicies(ctx context.Context) ([]ChannelControlPolicy, error) {
	policies := make([]ChannelControlPolicy, 0)
	err := DB.WithContext(ctx).Order(commonGroupCol + " ASC").Find(&policies).Error
	return policies, err
}

func GetEnabledChannelControlPolicies(ctx context.Context) (map[string]ChannelControlPolicy, error) {
	policies := make([]ChannelControlPolicy, 0)
	if err := DB.WithContext(ctx).Where("enabled = ?", true).Find(&policies).Error; err != nil {
		return nil, err
	}
	result := make(map[string]ChannelControlPolicy, len(policies))
	for _, policy := range policies {
		if err := policy.Normalize(); err != nil {
			continue
		}
		result[policy.Group] = policy
	}
	return result, nil
}

func UpsertChannelControlPolicy(ctx context.Context, policy ChannelControlPolicy) (*ChannelControlPolicy, error) {
	if err := policy.Normalize(); err != nil {
		return nil, err
	}
	policy.UpdatedAt = common.GetTimestamp()
	err := DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "group"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"enabled",
			"adaptive_enabled",
			"adaptive_window_seconds",
			"adaptive_min_samples",
			"adaptive_slow_threshold_ms",
			"adaptive_min_weight",
			"adaptive_max_weight",
			"adaptive_recovery_weight",
			"adaptive_cooldown_seconds",
			"probe_enabled",
			"probe_mode",
			"probe_model",
			"recovery_enabled",
			"recovery_successes_required",
			"updated_at",
		}),
	}).Create(&policy).Error
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

func DeleteChannelControlPolicy(ctx context.Context, group string) (bool, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return false, errors.New("group is required")
	}
	result := DB.WithContext(ctx).Where(commonGroupCol+" = ?", group).Delete(&ChannelControlPolicy{})
	return result.RowsAffected > 0, result.Error
}

func ResolveChannelControlPolicy(channel *Channel, policies map[string]ChannelControlPolicy) (ChannelControlPolicy, bool) {
	if channel == nil || len(policies) == 0 {
		return ChannelControlPolicy{}, false
	}
	var resolved ChannelControlPolicy
	found := false
	for _, group := range channel.GetGroups() {
		policy, ok := policies[group]
		if !ok {
			continue
		}
		if !found {
			resolved = policy
			found = true
			continue
		}
		// A channel can belong to more than one policy group. Safety settings
		// are vetoes: one group opting out of probes/recovery must not be
		// overridden by another group merely because it appears later.
		resolved.ProbeEnabled = resolved.ProbeEnabled && policy.ProbeEnabled
		resolved.RecoveryEnabled = resolved.RecoveryEnabled && policy.RecoveryEnabled
		resolved.AdaptiveEnabled = resolved.AdaptiveEnabled && policy.AdaptiveEnabled
		if policy.RecoverySuccessesRequired > resolved.RecoverySuccessesRequired {
			resolved.RecoverySuccessesRequired = policy.RecoverySuccessesRequired
		}
	}
	return resolved, found
}

// RecordChannelRecoveryProbe increments a persisted success streak or resets
// it on any failed probe. The system-task lease serializes normal execution,
// and the transaction also makes manual/overlapping task runs deterministic.
func RecordChannelRecoveryProbe(ctx context.Context, channelID int, succeeded bool) (*ChannelRecoveryState, error) {
	if channelID <= 0 {
		return nil, errors.New("channel id is required")
	}
	now := common.GetTimestamp()
	state := &ChannelRecoveryState{}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing ChannelRecoveryState
		err := lockForUpdate(tx).Where("channel_id = ?", channelID).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			existing.ChannelID = channelID
		}
		existing.LastProbeAt = now
		existing.UpdatedAt = now
		if succeeded {
			existing.SuccessStreak++
			existing.LastSuccessAt = now
		} else {
			existing.SuccessStreak = 0
			existing.LastFailureAt = now
		}
		if err := tx.Save(&existing).Error; err != nil {
			return err
		}
		*state = existing
		return nil
	})
	if err != nil {
		return nil, err
	}
	return state, nil
}

func ClearChannelRecoveryState(ctx context.Context, channelID int) error {
	if channelID <= 0 {
		return nil
	}
	return DB.WithContext(ctx).Where("channel_id = ?", channelID).Delete(&ChannelRecoveryState{}).Error
}
