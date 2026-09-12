package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/model"
)

type ChannelControlMetricsSummary struct {
	WindowSeconds       int     `json:"window_seconds"`
	Requests            int64   `json:"requests"`
	Successes           int64   `json:"successes"`
	SuccessRate         float64 `json:"success_rate"`
	Attempts            int64   `json:"attempts"`
	AverageAttempts     float64 `json:"average_attempts"`
	ProbeAttempts       int64   `json:"probe_attempts"`
	ProbeSuccesses      int64   `json:"probe_successes"`
	ProbeCost           float64 `json:"probe_cost"`
	RecoveryAttempts    int64   `json:"recovery_attempts"`
	RecoveryFailures    int64   `json:"recovery_failures"`
	RecoveryFailureRate float64 `json:"recovery_failure_rate"`
	Groups              int     `json:"groups"`
}

func RecordChannelControlRequestMetric(ctx context.Context, group string, success bool, attempts int64) {
	delta := model.ChannelControlMetricDelta{Requests: 1, Attempts: attempts}
	if success {
		delta.Successes = 1
	}
	if err := model.RecordChannelControlMetric(ctx, group, delta); err != nil {
		modelLogMetricError("request", err)
	}
}

func RecordChannelControlProbeMetric(ctx context.Context, group string, success bool, cost float64, recovery bool) {
	delta := model.ChannelControlMetricDelta{ProbeAttempts: 1, ProbeCost: cost}
	if success {
		delta.ProbeSuccesses = 1
	}
	if recovery {
		delta.RecoveryAttempts = 1
		if !success {
			delta.RecoveryFailures = 1
		}
	}
	if err := model.RecordChannelControlMetric(ctx, group, delta); err != nil {
		modelLogMetricError("probe", err)
	}
}

func GetChannelControlMetrics(ctx context.Context, windowSeconds int) (ChannelControlMetricsSummary, error) {
	if model.DB == nil {
		return ChannelControlMetricsSummary{}, fmt.Errorf("main database is not initialized")
	}
	if windowSeconds < 3600 {
		windowSeconds = 3600
	}
	if windowSeconds > 30*24*3600 {
		windowSeconds = 30 * 24 * 3600
	}
	cutoff := time.Now().Add(-time.Duration(windowSeconds) * time.Second).Unix()
	var rows []model.ChannelControlMetric
	if err := model.DB.WithContext(ctx).Where("bucket_at >= ?", cutoff).Find(&rows).Error; err != nil {
		return ChannelControlMetricsSummary{}, err
	}
	summary := ChannelControlMetricsSummary{WindowSeconds: windowSeconds}
	groups := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		groups[row.Group] = struct{}{}
		summary.Requests += row.Requests
		summary.Successes += row.Successes
		summary.Attempts += row.Attempts
		summary.ProbeAttempts += row.ProbeAttempts
		summary.ProbeSuccesses += row.ProbeSuccesses
		summary.ProbeCost += row.ProbeCost
		summary.RecoveryAttempts += row.RecoveryAttempts
		summary.RecoveryFailures += row.RecoveryFailures
	}
	summary.Groups = len(groups)
	if summary.Requests > 0 {
		summary.SuccessRate = float64(summary.Successes) / float64(summary.Requests)
		summary.AverageAttempts = float64(summary.Attempts) / float64(summary.Requests)
	}
	if summary.RecoveryAttempts > 0 {
		summary.RecoveryFailureRate = float64(summary.RecoveryFailures) / float64(summary.RecoveryAttempts)
	}
	return summary, nil
}

func modelLogMetricError(kind string, err error) {
	// Keep metric failures non-fatal to request and health-control paths.
	_ = kind
	_ = err
}
