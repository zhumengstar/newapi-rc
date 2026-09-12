package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

const (
	adaptiveRoutingDefaultWindow   = 300
	adaptiveRoutingMaxWindow       = 3600
	adaptiveRoutingDefaultSamples  = 5
	adaptiveRoutingDefaultSlowMs   = 4000
	adaptiveRoutingDefaultMin      = 1
	adaptiveRoutingDefaultMax      = 1000
	adaptiveRoutingDefaultRecovery = 5
	adaptiveRoutingDefaultCooldown = 180
	// Keep adaptive changes within the same per-group/model budget used by
	// manual channel weight edits. Otherwise the scheduler would overwrite a
	// group's 300-point allocation on its next run.
	adaptiveRoutingTotalWeight = uint(300)
	adaptiveRoutingMaxLogRows  = 20000
	adaptiveRoutingMaxSamples  = 10000
	adaptiveRoutingMaxSlowMs   = 3600000
	adaptiveRoutingMaxCooldown = 86400
)

var adaptiveRoutingRunMu sync.Mutex

// AdaptiveRoutingSummary is stored in the system task result and intentionally
// contains aggregate values only, so a scheduled run cannot expose request
// bodies or credentials.
type AdaptiveRoutingSummary struct {
	EvaluatedChannels   int   `json:"evaluated_channels"`
	UpdatedAbilities    int   `json:"updated_abilities"`
	SkippedBuckets      int   `json:"skipped_buckets"`
	InsufficientBuckets int   `json:"insufficient_buckets"`
	WindowSeconds       int   `json:"window_seconds"`
	LogRowsRead         int   `json:"log_rows_read"`
	At                  int64 `json:"at"`
}

type adaptiveObservationKey struct {
	RequestID string
	ChannelID int
	Group     string
	Model     string
}

type adaptiveObservation struct {
	Key        adaptiveObservationKey
	Success    bool
	TTFTMs     float64
	HasTTFT    bool
	TotalMs    float64
	HasTotal   bool
	IsImage    bool
	ModelPrice float64
	HasPrice   bool
}

type adaptiveMetricKey struct {
	ChannelID int
	Group     string
	Model     string
}

type adaptiveMetric struct {
	Requests        int
	Successes       int
	TTFTSumMs       float64
	TTFTCount       int
	TotalMs         float64
	TotalCount      int
	ImageRequests   int
	ModelPriceSum   float64
	ModelPriceCount int
}

type adaptiveBucketKey struct {
	Group    string
	Model    string
	Priority int64
}

type adaptiveCandidate struct {
	ChannelID       int
	Group           string
	Model           string
	CurrentWeight   uint
	MinWeight       uint
	MaxWeight       uint
	MinSamples      int
	SlowThresholdMs int
	UseTotalTime    bool
	CooldownSeconds int
	LastAppliedAt   int64
	RecoveryWeight  uint
	Warmup          bool
	Score           float64
	CostScore       float64
	Metric          adaptiveMetric
}

// HasAdaptiveRoutingChannels keeps the scheduler dormant unless at least one
// channel opts in from the channel management form or a configured legacy
// controller group is present.
func HasAdaptiveRoutingChannels() bool {
	if model.DB == nil {
		return false
	}
	if !common.GetEnvOrDefaultBool("CHANNEL_ADAPTIVE_ROUTING_ENABLED", true) {
		return false
	}
	legacyGroups := configuredAdaptiveGroups()
	policies, err := model.GetEnabledChannelControlPolicies(context.Background())
	if err != nil {
		return false
	}
	if len(legacyGroups) == 0 && !hasAdaptiveControlPolicy(policies) {
		var count int64
		return model.DB.Model(&model.Channel{}).
			Where("status = ? AND adaptive_enabled = ?", common.ChannelStatusEnabled, true).
			Count(&count).Error == nil && count > 0
	}
	var channels []model.Channel
	if err := model.DB.Find(&channels).Error; err != nil {
		return false
	}
	for _, channel := range channels {
		if channel.Status == common.ChannelStatusEnabled &&
			(channel.AdaptiveEnabled || adaptiveGroupConfigured(channel.Group, legacyGroups) || channelHasAdaptiveControlPolicy(&channel, policies)) {
			return true
		}
	}
	return false
}

func hasAdaptiveControlPolicy(policies map[string]model.ChannelControlPolicy) bool {
	for _, policy := range policies {
		if policy.AdaptiveEnabled {
			return true
		}
	}
	return false
}

func channelHasAdaptiveControlPolicy(channel *model.Channel, policies map[string]model.ChannelControlPolicy) bool {
	for _, group := range channel.GetGroups() {
		if policy, ok := policies[group]; ok && policy.AdaptiveEnabled {
			return true
		}
	}
	return false
}

func abilityUsesAdaptiveRouting(channel model.Channel, group string, legacyGroups []string, policies map[string]model.ChannelControlPolicy) bool {
	if channel.AdaptiveEnabled {
		return true
	}
	if adaptiveGroupConfigured(group, legacyGroups) {
		return true
	}
	policy, ok := policies[group]
	return ok && policy.AdaptiveEnabled
}

// configuredAdaptiveGroups enables the same quality/latency/cost controller
// for legacy script-managed groups without requiring a schema migration. The
// channel-level adaptive fields still apply as defaults and per-channel limits.
func configuredAdaptiveGroups() []string {
	raw := os.Getenv("CHANNEL_ADAPTIVE_GROUPS")
	if strings.TrimSpace(raw) == "" {
		raw = os.Getenv("CHANNEL_WEIGHT_CONTROLLER_GROUPS")
	}
	parts := strings.Split(raw, ",")
	groups := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		group := strings.TrimSpace(part)
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
	}
	return groups
}

func adaptiveGroupConfigured(group string, configured []string) bool {
	channel := model.Channel{Group: group}
	for _, channelGroup := range channel.GetGroups() {
		for _, candidate := range configured {
			if candidate == channelGroup {
				return true
			}
		}
	}
	return false
}

// RunAdaptiveRoutingOnce evaluates recent consume/error logs and updates only
// abilities.weight. Channel status is owned by the native channel test flow;
// priority and manual settings are never changed here.
func RunAdaptiveRoutingOnce(ctx context.Context) (AdaptiveRoutingSummary, error) {
	adaptiveRoutingRunMu.Lock()
	defer adaptiveRoutingRunMu.Unlock()

	summary := AdaptiveRoutingSummary{At: common.GetTimestamp()}
	if model.DB == nil {
		return summary, errors.New("main database is not initialized")
	}
	if model.LOG_DB == nil {
		return summary, errors.New("log database is not initialized")
	}

	legacyGroups := configuredAdaptiveGroups()
	policies, err := model.GetEnabledChannelControlPolicies(ctx)
	if err != nil {
		return summary, fmt.Errorf("load channel control policies: %w", err)
	}
	var channels []model.Channel
	if err := model.DB.WithContext(ctx).Where("adaptive_enabled = ?", true).Find(&channels).Error; err != nil {
		return summary, fmt.Errorf("load adaptive channels: %w", err)
	}
	if len(legacyGroups) > 0 || hasAdaptiveControlPolicy(policies) {
		var allChannels []model.Channel
		if err := model.DB.WithContext(ctx).Find(&allChannels).Error; err != nil {
			return summary, fmt.Errorf("load configured adaptive groups: %w", err)
		}
		seen := make(map[int]struct{}, len(channels))
		for _, channel := range channels {
			seen[channel.Id] = struct{}{}
		}
		for _, channel := range allChannels {
			if adaptiveGroupConfigured(channel.Group, legacyGroups) || channelHasAdaptiveControlPolicy(&channel, policies) {
				if _, exists := seen[channel.Id]; !exists {
					channels = append(channels, channel)
					seen[channel.Id] = struct{}{}
				}
			}
		}
	}
	if len(channels) == 0 {
		return summary, nil
	}
	summary.EvaluatedChannels = len(channels)

	channelByID := make(map[int]model.Channel, len(channels))
	allChannelByID := make(map[int]model.Channel)
	channelIDs := make([]int, 0, len(channels))
	maxWindow := adaptiveRoutingDefaultWindow
	for _, channel := range channels {
		channelByID[channel.Id] = channel
		channelIDs = append(channelIDs, channel.Id)
		if window := normalizeAdaptiveWindow(channel.AdaptiveWindowSeconds); window > maxWindow {
			maxWindow = window
		}
	}
	for _, policy := range policies {
		if policy.AdaptiveEnabled {
			if window := normalizeAdaptiveWindow(policy.AdaptiveWindowSeconds); window > maxWindow {
				maxWindow = window
			}
		}
	}
	if maxWindow > adaptiveRoutingMaxWindow {
		maxWindow = adaptiveRoutingMaxWindow
	}
	summary.WindowSeconds = maxWindow

	// Read the complete channel set once so a bucket can be protected when it
	// contains an active channel that has not opted into adaptive routing. This
	// keeps the feature opt-in and prevents it from silently taking over that
	// channel's share of traffic.
	var allChannels []model.Channel
	if err := model.DB.WithContext(ctx).Find(&allChannels).Error; err != nil {
		return summary, fmt.Errorf("load all channels: %w", err)
	}
	for _, channel := range allChannels {
		allChannelByID[channel.Id] = channel
	}

	observations, rowsRead, err := loadAdaptiveObservations(ctx, channelIDs, summary.At-int64(maxWindow))
	if err != nil {
		return summary, err
	}
	summary.LogRowsRead = rowsRead
	metrics := buildAdaptiveMetrics(observations)

	var abilities []model.Ability
	if err := model.DB.WithContext(ctx).
		Where("channel_id IN ? AND enabled = ?", channelIDs, true).
		Find(&abilities).Error; err != nil {
		return summary, fmt.Errorf("load channel abilities: %w", err)
	}

	buckets := make(map[adaptiveBucketKey][]adaptiveCandidate)
	for _, ability := range abilities {
		channel, ok := channelByID[ability.ChannelId]
		if !ok || channel.Status != common.ChannelStatusEnabled || !abilityUsesAdaptiveRouting(channel, ability.Group, legacyGroups, policies) {
			continue
		}
		priority := channel.GetRoutingPriority()
		if channel.CostTier <= 0 && ability.Priority != nil {
			priority = *ability.Priority
		}
		candidate := adaptiveCandidate{
			ChannelID:       channel.Id,
			Group:           ability.Group,
			Model:           ability.Model,
			CurrentWeight:   ability.Weight,
			MinWeight:       normalizeAdaptiveMinWeight(channel.AdaptiveMinWeight),
			MaxWeight:       normalizeAdaptiveMaxWeight(channel.AdaptiveMaxWeight),
			MinSamples:      normalizeAdaptiveMinSamples(channel.AdaptiveMinSamples),
			SlowThresholdMs: normalizeAdaptiveSlowThreshold(channel.AdaptiveSlowThresholdMs),
			UseTotalTime:    usesAdaptiveTotalTime(channel.Type),
			CooldownSeconds: normalizeAdaptiveCooldown(channel.AdaptiveCooldownSeconds),
			LastAppliedAt:   channel.AdaptiveLastAppliedAt,
			RecoveryWeight:  normalizeAdaptiveRecoveryWeight(channel.AdaptiveRecoveryWeight),
			Warmup:          adaptiveChannelNeedsWarmup(channel),
		}
		candidate.CostScore = adaptiveChannelCostScore(channel)
		if policy, ok := policies[ability.Group]; ok && policy.AdaptiveEnabled {
			candidate.MinWeight = normalizeAdaptiveMinWeight(policy.AdaptiveMinWeight)
			candidate.MaxWeight = normalizeAdaptiveMaxWeight(policy.AdaptiveMaxWeight)
			candidate.MinSamples = normalizeAdaptiveMinSamples(policy.AdaptiveMinSamples)
			candidate.SlowThresholdMs = normalizeAdaptiveSlowThreshold(policy.AdaptiveSlowThresholdMs)
			candidate.CooldownSeconds = normalizeAdaptiveCooldown(policy.AdaptiveCooldownSeconds)
			candidate.RecoveryWeight = normalizeAdaptiveRecoveryWeight(policy.AdaptiveRecoveryWeight)
		}
		if candidate.MaxWeight < candidate.MinWeight {
			candidate.MaxWeight = candidate.MinWeight
		}
		candidate.Metric = metrics[adaptiveMetricKey{ChannelID: channel.Id, Group: ability.Group, Model: ability.Model}]
		candidate.UseTotalTime = candidate.UseTotalTime || candidate.Metric.ImageRequests > 0
		key := adaptiveBucketKey{Group: ability.Group, Model: ability.Model, Priority: priority}
		buckets[key] = append(buckets[key], candidate)
	}
	for key, candidates := range buckets {
		buckets[key] = applyAdaptiveCostScores(candidates)
	}
	// A mixed bucket would otherwise normalize only the opted-in channels to
	// the full group budget and effectively exclude the non-adaptive channel. Leave such buckets
	// unchanged until all of their active channels opt in.
	fixedBuckets := make(map[adaptiveBucketKey]bool)
	var allEnabledAbilities []model.Ability
	if err := model.DB.WithContext(ctx).
		Where("enabled = ?", true).
		Find(&allEnabledAbilities).Error; err != nil {
		return summary, fmt.Errorf("load enabled abilities: %w", err)
	}
	for _, ability := range allEnabledAbilities {
		priority := int64(0)
		if channel, ok := allChannelByID[ability.ChannelId]; ok {
			priority = channel.GetRoutingPriority()
		}
		if priority == 0 && ability.Priority != nil {
			priority = *ability.Priority
		}
		key := adaptiveBucketKey{Group: ability.Group, Model: ability.Model, Priority: priority}
		if _, ok := buckets[key]; !ok {
			continue
		}
		channel, ok := allChannelByID[ability.ChannelId]
		if ok && channel.Status == common.ChannelStatusEnabled && !abilityUsesAdaptiveRouting(channel, ability.Group, legacyGroups, policies) {
			fixedBuckets[key] = true
		}
	}

	weightUpdates := make([]adaptiveWeightUpdate, 0)
	reasons := make(map[int][]string, len(channels))
	now := summary.At
	for key, candidates := range buckets {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if len(candidates) < 2 {
			summary.SkippedBuckets++
			continue
		}
		if fixedBuckets[key] {
			summary.SkippedBuckets++
			for _, candidate := range candidates {
				reasons[candidate.ChannelID] = append(reasons[candidate.ChannelID], fmt.Sprintf("%s/%s: contains non-adaptive channel", key.Group, key.Model))
			}
			continue
		}
		if adaptiveBucketInCooldown(candidates, now) {
			summary.SkippedBuckets++
			continue
		}

		ready := true
		for _, candidate := range candidates {
			if candidate.Metric.Requests < candidate.MinSamples {
				ready = false
				reasons[candidate.ChannelID] = append(reasons[candidate.ChannelID], fmt.Sprintf("%s/%s: %d/%d samples", key.Group, key.Model, candidate.Metric.Requests, candidate.MinSamples))
			}
		}
		if !ready {
			summary.InsufficientBuckets++
			continue
		}
		if allAdaptiveCandidatesNeedWarmup(candidates) {
			for i := range candidates {
				candidates[i].Warmup = false
			}
		}

		scores := make([]float64, len(candidates))
		for i := range candidates {
			if candidates[i].Warmup {
				candidates[i].MaxWeight = minUInt(candidates[i].MaxWeight, candidates[i].RecoveryWeight)
				if candidates[i].MaxWeight < candidates[i].MinWeight {
					candidates[i].MaxWeight = candidates[i].MinWeight
				}
			}
			candidates[i].Score = adaptiveCandidateScore(candidates[i], 0)
			scores[i] = candidates[i].Score
		}
		weights := normalizeAdaptiveWeights(scores, candidates, adaptiveRoutingTotalWeight)
		for i, weight := range weights {
			if weight == candidates[i].CurrentWeight {
				continue
			}
			weightUpdates = append(weightUpdates, adaptiveWeightUpdate{
				Group:     candidates[i].Group,
				Model:     candidates[i].Model,
				ChannelID: candidates[i].ChannelID,
				Weight:    weight,
			})
			reasons[candidates[i].ChannelID] = append(reasons[candidates[i].ChannelID], fmt.Sprintf("%s/%s: score %.3f -> weight %d", key.Group, key.Model, candidates[i].Score, weight))
		}
	}

	// Keep the timestamp/reason fields auditable even when the sparse-data guard
	// intentionally leaves weights untouched.
	err = model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, update := range weightUpdates {
			result := tx.Model(&model.Ability{}).
				Where(map[string]any{"enabled": true, "group": update.Group, "model": update.Model, "channel_id": update.ChannelID}).
				Update("weight", update.Weight)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				summary.UpdatedAbilities += int(result.RowsAffected)
			}
		}
		for _, channel := range channels {
			if channel.Status != common.ChannelStatusEnabled {
				continue
			}
			reason := strings.Join(limitAdaptiveReasons(reasons[channel.Id]), "; ")
			if reason == "" {
				reason = "no eligible bucket changed"
			}
			if err := tx.Model(&model.Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
				"adaptive_last_evaluated_at": now,
				"adaptive_last_reason":       reason,
			}).Error; err != nil {
				return err
			}
		}
		for _, update := range weightUpdates {
			if err := tx.Model(&model.Channel{}).Where("id = ?", update.ChannelID).Updates(map[string]any{
				"adaptive_last_applied_at": now,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return summary, fmt.Errorf("apply adaptive routing weights: %w", err)
	}
	if summary.UpdatedAbilities > 0 {
		model.InitChannelCache()
	}
	logger.LogInfo(ctx, fmt.Sprintf("adaptive routing completed: channels=%d abilities=%d logs=%d window=%ds", len(channels), summary.UpdatedAbilities, summary.LogRowsRead, summary.WindowSeconds))
	return summary, nil
}

type adaptiveWeightUpdate struct {
	Group     string
	Model     string
	ChannelID int
	Weight    uint
}

func loadAdaptiveObservations(ctx context.Context, channelIDs []int, cutoff int64) ([]adaptiveObservation, int, error) {
	if len(channelIDs) == 0 {
		return nil, 0, nil
	}
	var logs []model.Log
	err := model.LOG_DB.WithContext(ctx).
		Where("type IN ? AND created_at >= ? AND channel_id IN ?", []int{model.LogTypeConsume, model.LogTypeError}, cutoff, channelIDs).
		Order("created_at DESC, id DESC").
		Limit(adaptiveRoutingMaxLogRows).
		Find(&logs).Error
	if err != nil {
		return nil, 0, fmt.Errorf("load recent logs: %w", err)
	}

	deduplicated := make(map[adaptiveObservationKey]adaptiveObservation, len(logs))
	for _, log := range logs {
		if log.ChannelId == 0 || log.Group == "" || log.ModelName == "" {
			continue
		}
		requestID := strings.TrimSpace(log.RequestId)
		if requestID == "" {
			requestID = fmt.Sprintf("log:%d", log.Id)
		}
		key := adaptiveObservationKey{RequestID: requestID, ChannelID: log.ChannelId, Group: log.Group, Model: log.ModelName}
		observation := adaptiveObservation{Key: key, Success: log.Type == model.LogTypeConsume}
		observation.IsImage = isAdaptiveImageRequest(log.Other)
		observation.TTFTMs, observation.HasTTFT = extractAdaptiveMetric(log.Other, "frt")
		if log.UseTime > 0 {
			observation.TotalMs = float64(log.UseTime) * 1000
			observation.HasTotal = true
			if !observation.HasTTFT {
				observation.TTFTMs = observation.TotalMs
				observation.HasTTFT = true
			}
		}

		previous, exists := deduplicated[key]
		if !exists || (!previous.Success && observation.Success) {
			deduplicated[key] = observation
		}
	}

	observations := make([]adaptiveObservation, 0, len(deduplicated))
	for _, observation := range deduplicated {
		observations = append(observations, observation)
	}
	return observations, len(logs), nil
}

func buildAdaptiveMetrics(observations []adaptiveObservation) map[adaptiveMetricKey]adaptiveMetric {
	metrics := make(map[adaptiveMetricKey]adaptiveMetric)
	for _, observation := range observations {
		key := adaptiveMetricKey{ChannelID: observation.Key.ChannelID, Group: observation.Key.Group, Model: observation.Key.Model}
		metric := metrics[key]
		metric.Requests++
		if observation.Success {
			metric.Successes++
		}
		if observation.HasTTFT && observation.TTFTMs > 0 && math.IsInf(observation.TTFTMs, 0) == false && math.IsNaN(observation.TTFTMs) == false {
			metric.TTFTSumMs += observation.TTFTMs
			metric.TTFTCount++
		}
		if observation.HasTotal && observation.TotalMs > 0 && !math.IsInf(observation.TotalMs, 0) && !math.IsNaN(observation.TotalMs) {
			metric.TotalMs += observation.TotalMs
			metric.TotalCount++
		}
		if observation.IsImage {
			metric.ImageRequests++
		}
		metrics[key] = metric
	}
	return metrics
}

func isAdaptiveImageRequest(other string) bool {
	values, err := common.StrToMap(other)
	if err != nil || values == nil {
		return false
	}
	path, ok := values["request_path"].(string)
	if !ok {
		return false
	}
	path = strings.ToLower(path)
	return strings.Contains(path, "/images/") || strings.Contains(path, "image_generation") || strings.Contains(path, "/image")
}

func extractAdaptiveMetric(other string, key string) (float64, bool) {
	values, err := common.StrToMap(other)
	if err != nil || values == nil {
		return 0, false
	}
	value, ok := values[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return typed, typed > 0 && !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case float32:
		converted := float64(typed)
		return converted, converted > 0 && !math.IsNaN(converted) && !math.IsInf(converted, 0)
	case int:
		return float64(typed), typed > 0
	case int64:
		return float64(typed), typed > 0
	case string:
		converted, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return converted, err == nil && converted > 0 && !math.IsNaN(converted) && !math.IsInf(converted, 0)
	default:
		return 0, false
	}
}

func adaptiveCandidateScore(candidate adaptiveCandidate, _ float64) float64 {
	successRate := float64(candidate.Metric.Successes) / float64(candidate.Metric.Requests)
	latencyScore := 1.0
	metricTotal, metricCount := candidate.Metric.TTFTSumMs, candidate.Metric.TTFTCount
	if candidate.UseTotalTime {
		metricTotal, metricCount = candidate.Metric.TotalMs, candidate.Metric.TotalCount
	}
	if metricCount > 0 {
		averageTTFT := metricTotal / float64(metricCount)
		latencyScore = clampAdaptiveScore(float64(candidate.SlowThresholdMs)/averageTTFT, 0.05, 1)
	}
	// Cost is a channel property, not a value inferred from billing logs.
	// CostScore is normalized within the same group/model bucket so cheaper
	// upstream channels receive a modest preference without overwhelming health.
	costScore := candidate.CostScore
	if costScore <= 0 || math.IsNaN(costScore) || math.IsInf(costScore, 0) {
		costScore = 0.5
	}
	score := 0.50*successRate + 0.30*latencyScore + 0.20*costScore
	if metricCount > 0 && metricTotal/float64(metricCount) > float64(candidate.SlowThresholdMs) {
		score = math.Min(score, 0.01)
	}
	return clampAdaptiveScore(score, 0.01, 1)
}

func usesAdaptiveTotalTime(channelType int) bool {
	switch channelType {
	case constant.ChannelTypeMidjourney, constant.ChannelTypeMidjourneyPlus,
		constant.ChannelTypeKling, constant.ChannelTypeJimeng, constant.ChannelTypeVidu,
		constant.ChannelTypeDoubaoVideo, constant.ChannelTypeSora, constant.ChannelTypeReplicate:
		return true
	default:
		return false
	}
}

func adaptiveChannelCostScore(channel model.Channel) float64 {
	input, output := channel.InputPrice, channel.OutputPrice
	if input > 0 || output > 0 {
		return input + output
	}
	if channel.ChannelRatio != nil && *channel.ChannelRatio > 0 && !math.IsNaN(*channel.ChannelRatio) && !math.IsInf(*channel.ChannelRatio, 0) {
		return *channel.ChannelRatio
	}
	return 0
}

func applyAdaptiveCostScores(candidates []adaptiveCandidate) []adaptiveCandidate {
	known := make([]float64, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.CostScore > 0 {
			known = append(known, candidate.CostScore)
		}
	}
	if len(known) < 2 {
		for i := range candidates {
			candidates[i].CostScore = 0.5
		}
		return candidates
	}
	minCost, maxCost := known[0], known[0]
	for _, cost := range known[1:] {
		minCost = math.Min(minCost, cost)
		maxCost = math.Max(maxCost, cost)
	}
	if maxCost <= minCost {
		for i := range candidates {
			candidates[i].CostScore = 0.5
		}
		return candidates
	}
	for i := range candidates {
		if candidates[i].CostScore <= 0 {
			candidates[i].CostScore = 0.5
			continue
		}
		// Invert normalized cost: cheapest=1, most expensive=0.05.
		normalized := (candidates[i].CostScore - minCost) / (maxCost - minCost)
		candidates[i].CostScore = clampAdaptiveScore(1-normalized, 0.05, 1)
	}
	return candidates
}

func normalizeAdaptiveWeights(scores []float64, candidates []adaptiveCandidate, total uint) []uint {
	weights := make([]uint, len(scores))
	if len(scores) == 0 || len(scores) != len(candidates) {
		return weights
	}

	// A user can configure a large minimum for each channel. Bound the
	// effective minimum per bucket so the scheduler can still honor the fixed
	// point total-weight contract.
	boundedCandidates := make([]adaptiveCandidate, len(candidates))
	copy(boundedCandidates, candidates)
	perCandidateLimit := total / uint(len(boundedCandidates))
	for i := range boundedCandidates {
		boundedCandidates[i].MinWeight = minUInt(
			boundedCandidates[i].MinWeight,
			perCandidateLimit,
		)
		boundedCandidates[i].MaxWeight = minUInt(
			boundedCandidates[i].MaxWeight,
			total,
		)
		if boundedCandidates[i].MaxWeight < boundedCandidates[i].MinWeight {
			boundedCandidates[i].MaxWeight = boundedCandidates[i].MinWeight
		}
	}

	minTotal, maxTotal := uint(0), uint(0)
	for i, candidate := range boundedCandidates {
		weights[i] = candidate.MinWeight
		minTotal += candidate.MinWeight
		maxTotal += candidate.MaxWeight
	}
	target := total
	if target < minTotal {
		target = minTotal
	}
	if target > maxTotal {
		target = maxTotal
	}
	if target == minTotal {
		return weights
	}

	scoreTotal := 0.0
	for _, score := range scores {
		scoreTotal += math.Max(score, 0.01)
	}
	for i, candidate := range boundedCandidates {
		share := (float64(target-minTotal) * math.Max(scores[i], 0.01)) / scoreTotal
		addition := uint(math.Floor(share))
		weights[i] = minUInt(candidate.MaxWeight, candidate.MinWeight+addition)
	}
	for sumAdaptiveWeights(weights) < target {
		index := bestWeightIndex(scores, weights, boundedCandidates, true)
		if index < 0 {
			break
		}
		weights[index]++
	}
	for sumAdaptiveWeights(weights) > target {
		index := bestWeightIndex(scores, weights, boundedCandidates, false)
		if index < 0 {
			break
		}
		weights[index]--
	}
	return weights
}

func bestWeightIndex(scores []float64, weights []uint, candidates []adaptiveCandidate, add bool) int {
	best := -1
	for i := range weights {
		if add && weights[i] >= candidates[i].MaxWeight {
			continue
		}
		if !add && weights[i] <= candidates[i].MinWeight {
			continue
		}
		if best == -1 || (add && scores[i] > scores[best]) || (!add && scores[i] < scores[best]) {
			best = i
		}
	}
	return best
}

func sumAdaptiveWeights(weights []uint) uint {
	result := uint(0)
	for _, weight := range weights {
		result += weight
	}
	return result
}

func adaptiveBucketInCooldown(candidates []adaptiveCandidate, now int64) bool {
	for _, candidate := range candidates {
		if candidate.LastAppliedAt > 0 && now-candidate.LastAppliedAt < int64(candidate.CooldownSeconds) {
			return true
		}
	}
	return false
}

func limitAdaptiveReasons(reasons []string) []string {
	if len(reasons) <= 4 {
		return reasons
	}
	return append(reasons[:4], fmt.Sprintf("+%d more", len(reasons)-4))
}

func clampAdaptiveScore(value float64, minValue float64, maxValue float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return minValue
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func minUInt(a uint, b uint) uint {
	if a < b {
		return a
	}
	return b
}

func normalizeAdaptiveWindow(value int) int {
	if value < 60 {
		return adaptiveRoutingDefaultWindow
	}
	if value > adaptiveRoutingMaxWindow {
		return adaptiveRoutingMaxWindow
	}
	return value
}

func normalizeAdaptiveMinSamples(value int) int {
	if value <= 0 {
		return adaptiveRoutingDefaultSamples
	}
	if value > adaptiveRoutingMaxSamples {
		return adaptiveRoutingMaxSamples
	}
	return value
}

func normalizeAdaptiveSlowThreshold(value int) int {
	if value <= 0 {
		return adaptiveRoutingDefaultSlowMs
	}
	if value > adaptiveRoutingMaxSlowMs {
		return adaptiveRoutingMaxSlowMs
	}
	return value
}

func normalizeAdaptiveMinWeight(value uint) uint {
	if value == 0 {
		return adaptiveRoutingDefaultMin
	}
	return minUInt(value, adaptiveRoutingTotalWeight)
}

func normalizeAdaptiveMaxWeight(value uint) uint {
	if value == 0 {
		return adaptiveRoutingDefaultMax
	}
	return minUInt(value, adaptiveRoutingTotalWeight)
}

func normalizeAdaptiveRecoveryWeight(value uint) uint {
	if value == 0 {
		return adaptiveRoutingDefaultRecovery
	}
	return minUInt(value, adaptiveRoutingTotalWeight)
}

func normalizeAdaptiveCooldown(value int) int {
	if value < 0 {
		return adaptiveRoutingDefaultCooldown
	}
	if value > adaptiveRoutingMaxCooldown {
		return adaptiveRoutingMaxCooldown
	}
	return value
}

func adaptiveChannelNeedsWarmup(channel model.Channel) bool {
	if channel.AdaptiveLastAppliedAt == 0 {
		return true
	}
	values, err := common.StrToMap(channel.OtherInfo)
	if err != nil || values == nil {
		return false
	}
	statusTime, ok := adaptiveNumericValue(values["status_time"])
	return ok && int64(statusTime) > channel.AdaptiveLastAppliedAt && channel.Status == common.ChannelStatusEnabled
}

func allAdaptiveCandidatesNeedWarmup(candidates []adaptiveCandidate) bool {
	if len(candidates) == 0 {
		return false
	}
	for _, candidate := range candidates {
		if !candidate.Warmup {
			return false
		}
	}
	return true
}

func adaptiveNumericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case float32:
		converted := float64(typed)
		return converted, !math.IsNaN(converted) && !math.IsInf(converted, 0)
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case string:
		converted, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return converted, err == nil && !math.IsNaN(converted) && !math.IsInf(converted, 0)
	default:
		return 0, false
	}
}
