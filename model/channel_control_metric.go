package model

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChannelControlMetric stores hourly aggregate health-control counters.
// Keeping this separate from request logs makes dashboards cheap and avoids
// exposing request contents.
type ChannelControlMetric struct {
	BucketAt         int64   `gorm:"primaryKey;bigint" json:"bucket_at"`
	Group            string  `gorm:"primaryKey;type:varchar(64)" json:"group"`
	Requests         int64   `gorm:"default:0" json:"requests"`
	Successes        int64   `gorm:"default:0" json:"successes"`
	Attempts         int64   `gorm:"default:0" json:"attempts"`
	ProbeAttempts    int64   `gorm:"default:0" json:"probe_attempts"`
	ProbeSuccesses   int64   `gorm:"default:0" json:"probe_successes"`
	ProbeCost        float64 `gorm:"default:0" json:"probe_cost"`
	RecoveryAttempts int64   `gorm:"default:0" json:"recovery_attempts"`
	RecoveryFailures int64   `gorm:"default:0" json:"recovery_failures"`
}

type ChannelControlMetricDelta struct {
	Requests         int64
	Successes        int64
	Attempts         int64
	ProbeAttempts    int64
	ProbeSuccesses   int64
	ProbeCost        float64
	RecoveryAttempts int64
	RecoveryFailures int64
}

func RecordChannelControlMetric(ctx context.Context, group string, delta ChannelControlMetricDelta) error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	if len(group) > 64 {
		group = group[:64]
	}
	bucket := time.Now().Unix() / 3600 * 3600
	row := ChannelControlMetric{BucketAt: bucket, Group: group, Requests: delta.Requests, Successes: delta.Successes, Attempts: delta.Attempts, ProbeAttempts: delta.ProbeAttempts, ProbeSuccesses: delta.ProbeSuccesses, ProbeCost: delta.ProbeCost, RecoveryAttempts: delta.RecoveryAttempts, RecoveryFailures: delta.RecoveryFailures}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "bucket_at"}, {Name: "group"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"requests": gorm.Expr("channel_control_metrics.requests + ?", delta.Requests), "successes": gorm.Expr("channel_control_metrics.successes + ?", delta.Successes), "attempts": gorm.Expr("channel_control_metrics.attempts + ?", delta.Attempts),
			"probe_attempts": gorm.Expr("channel_control_metrics.probe_attempts + ?", delta.ProbeAttempts), "probe_successes": gorm.Expr("channel_control_metrics.probe_successes + ?", delta.ProbeSuccesses), "probe_cost": gorm.Expr("channel_control_metrics.probe_cost + ?", delta.ProbeCost),
			"recovery_attempts": gorm.Expr("channel_control_metrics.recovery_attempts + ?", delta.RecoveryAttempts), "recovery_failures": gorm.Expr("channel_control_metrics.recovery_failures + ?", delta.RecoveryFailures),
		}),
	}).Create(&row).Error
}
