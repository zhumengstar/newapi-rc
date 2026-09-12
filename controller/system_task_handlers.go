package controller

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// RegisterScheduledSystemTasks wires the periodic channel test, upstream model
// update, and async task polling (Midjourney / Suno / video) jobs into the
// system task framework so a DB lease dedups execution across multiple master
// instances and each run is recorded as one task row. Call this before
// service.StartSystemTaskRunner.
func RegisterScheduledSystemTasks() {
	service.RegisterSystemTaskHandler(channelTestHandler{})
	service.RegisterSystemTaskHandler(modelUpdateHandler{})
	service.RegisterSystemTaskHandler(midjourneyPollHandler{})
	service.RegisterSystemTaskHandler(asyncTaskPollHandler{})
	service.RegisterSystemTaskHandler(adaptiveRoutingHandler{})
	service.RegisterSystemTaskHandler(channelBalanceHandler{})
	service.RegisterSystemTaskHandler(channelErrorGuardHandler{})
	service.RegisterSystemTaskHandler(channelRecoveryHandler{})
	service.RegisterSystemTaskHandler(channelHealthHandler{})
	service.RegisterSystemTaskHandler(priorityNormalizeHandler{})
}

// channelErrorGuardHandler evaluates recent deduplicated upstream errors and
// applies the configured automatic-disable policy. It is dormant when the
// global automatic-disable switch is off, preserving the existing safety
// control for installations that only want observation.
type channelErrorGuardHandler struct{}

func (channelErrorGuardHandler) Type() string { return model.SystemTaskTypeChannelErrorGuard }
func (channelErrorGuardHandler) Enabled() bool {
	return common.AutomaticDisableChannelEnabled && common.GetEnvOrDefaultBool("CHANNEL_ERROR_GUARD_ENABLED", true)
}
func (channelErrorGuardHandler) Interval() time.Duration {
	seconds := common.GetEnvOrDefault("CHANNEL_ERROR_GUARD_INTERVAL_SECONDS", 10)
	if seconds < 1 {
		seconds = 10
	}
	return time.Duration(seconds) * time.Second
}
func (channelErrorGuardHandler) NewPayload() any { return nil }
func (channelErrorGuardHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := service.RunChannelErrorGuardOnce(ctx)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

type channelBalanceHandler struct{}

type channelBalanceTaskPayload struct {
	IncludeDisabled bool `json:"include_disabled,omitempty"`
}

func (channelBalanceHandler) Type() string { return model.SystemTaskTypeChannelBalance }
func (channelBalanceHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("CHANNEL_BALANCE_REFRESH_ENABLED", true)
}
func (channelBalanceHandler) Interval() time.Duration {
	minutes := 60
	if raw := os.Getenv("CHANNEL_BALANCE_REFRESH_INTERVAL_MINUTES"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			minutes = value
		}
	} else if raw := os.Getenv("CHANNEL_UPDATE_FREQUENCY"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			minutes = value
		}
	}
	return time.Duration(minutes) * time.Minute
}
func (channelBalanceHandler) NewPayload() any { return nil }
func (channelBalanceHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := channelBalanceTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	if err := ctx.Err(); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary, err := updateAllChannelsBalance(ctx, payload.IncludeDisabled, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// adaptiveRoutingHandler adjusts only ability weights for channels that opt in
// through channel management or a configured legacy controller group. Native
// channel testing remains the owner of status changes and recovery.
type adaptiveRoutingHandler struct{}

func (adaptiveRoutingHandler) Type() string { return model.SystemTaskTypeAdaptiveRouting }

func (adaptiveRoutingHandler) Enabled() bool {
	return service.HasAdaptiveRoutingChannels()
}

func (adaptiveRoutingHandler) Interval() time.Duration { return time.Minute }

func (adaptiveRoutingHandler) NewPayload() any { return nil }

func (adaptiveRoutingHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := service.RunAdaptiveRoutingOnce(ctx)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// channelRecoveryHandler replaces the legacy recovery timers. It deliberately
// runs the existing passive channel test path, which skips manual disables and
// only enables channels after a successful provider-specific test.
type channelRecoveryHandler struct{}

func (channelRecoveryHandler) Type() string  { return model.SystemTaskTypeChannelRecovery }
func (channelRecoveryHandler) Enabled() bool { return service.ChannelRecoveryEnabled() }
func (channelRecoveryHandler) Interval() time.Duration {
	return time.Duration(service.ChannelRecoveryIntervalSeconds()) * time.Second
}
func (channelRecoveryHandler) NewPayload() any { return nil }
func (channelRecoveryHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	recovery, err := service.RunChannelRecoveryOnce(ctx, func(runCtx context.Context) (int, int, int, error) {
		testSummary, runErr := runChannelTestTask(runCtx, operation_setting.ChannelTestModePassiveRecovery, false, service.NewSystemTaskProgressReporter(task, runnerID))
		return testSummary.Tested, testSummary.Enabled, testSummary.PendingRecovery, runErr
	})
	status := model.SystemTaskStatusSucceeded
	if err != nil {
		status = model.SystemTaskStatusFailed
	}
	finishSystemTaskHandler(task, runnerID, status, recovery, err)
}

// channelHealthHandler is the proactive counterpart to error protection. It
// exercises enabled and auto-disabled auto-ban channels through the normal
// provider test adapters, allowing failed channels to be quarantined and
// healthy channels to recover when automatic enablement is enabled.
type channelHealthHandler struct{}

func (channelHealthHandler) Type() string  { return model.SystemTaskTypeChannelHealth }
func (channelHealthHandler) Enabled() bool { return service.ChannelHealthCheckEnabled() }
func (channelHealthHandler) Interval() time.Duration {
	return time.Duration(service.ChannelHealthCheckIntervalSeconds()) * time.Second
}
func (channelHealthHandler) NewPayload() any { return nil }
func (channelHealthHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := runChannelTestTask(ctx, operation_setting.ChannelTestModeAutoBanOnly, false, service.NewSystemTaskProgressReporter(task, runnerID))
	status := model.SystemTaskStatusSucceeded
	if err != nil {
		status = model.SystemTaskStatusFailed
	}
	finishSystemTaskHandler(task, runnerID, status, summary, err)
}

// priorityNormalizeHandler replaces the one-shot priority-normalizer script
// when explicitly enabled. It is disabled by default because priority is an
// operational setting and should not be changed implicitly after deployment.
type priorityNormalizeHandler struct{}

func (priorityNormalizeHandler) Type() string  { return model.SystemTaskTypePriorityNormalize }
func (priorityNormalizeHandler) Enabled() bool { return service.PriorityNormalizerEnabled() }
func (priorityNormalizeHandler) Interval() time.Duration {
	return time.Duration(service.PriorityNormalizerIntervalSeconds()) * time.Second
}
func (priorityNormalizeHandler) NewPayload() any { return nil }
func (priorityNormalizeHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := service.RunPriorityNormalizerOnce(ctx)
	status := model.SystemTaskStatusSucceeded
	if err != nil {
		status = model.SystemTaskStatusFailed
	}
	finishSystemTaskHandler(task, runnerID, status, summary, err)
}

// channelTestHandler runs the scheduled "test all channels" job. Enablement and
// cadence still come from the monitor settings; only the execution path moved
// into the system task runner.
type channelTestHandler struct{}

func (channelTestHandler) Type() string { return model.SystemTaskTypeChannelTest }

func (channelTestHandler) Enabled() bool {
	return operation_setting.GetMonitorSetting().AutoTestChannelEnabled
}

func (channelTestHandler) Interval() time.Duration {
	minutes := operation_setting.GetMonitorSetting().AutoTestChannelMinutes
	if minutes <= 0 {
		minutes = 10
	}
	return time.Duration(minutes * float64(time.Minute))
}

func (channelTestHandler) NewPayload() any { return nil }

// channelTestTaskPayload controls one channel_test run. A nil/empty payload is a
// scheduled run, which uses the configured monitor ChannelTestMode and does not
// notify. A manual "test all channels" trigger sets Mode=scheduled_all and
// Notify=true to reproduce the legacy manual behavior (test every channel and
// notify root on completion).
type channelTestTaskPayload struct {
	Mode   string `json:"mode,omitempty"`
	Notify bool   `json:"notify,omitempty"`
}

func (channelTestHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := channelTestTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary, err := runChannelTestTask(ctx, payload.Mode, payload.Notify, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// modelUpdateHandler runs the scheduled upstream model update detection job.
type modelUpdateHandler struct{}

func (modelUpdateHandler) Type() string { return model.SystemTaskTypeModelUpdate }

func (modelUpdateHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", true)
}

func (modelUpdateHandler) Interval() time.Duration {
	intervalMinutes := common.GetEnvOrDefault(
		"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_INTERVAL_MINUTES",
		channelUpstreamModelUpdateTaskDefaultIntervalMinutes,
	)
	if intervalMinutes < 1 {
		intervalMinutes = channelUpstreamModelUpdateTaskDefaultIntervalMinutes
	}
	return time.Duration(intervalMinutes) * time.Minute
}

func (modelUpdateHandler) NewPayload() any { return nil }

// modelUpdateTaskPayload controls one model_update run. A scheduled run
// (Manual=false) respects the per-channel minimum check interval and may
// auto-apply detected models when a channel has auto-sync enabled. A manual
// "detect all" trigger sets Manual=true to reproduce the legacy detect-all
// semantics: force a re-check regardless of the interval and never auto-apply,
// so the admin reviews and applies changes explicitly.
type modelUpdateTaskPayload struct {
	Manual bool `json:"manual,omitempty"`
}

func (modelUpdateHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := modelUpdateTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary := runChannelUpstreamModelUpdateTaskOnce(ctx, payload.Manual, !payload.Manual, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// midjourneyPollHandler runs one Midjourney polling pass per scheduled run.
// Enabled() folds the "are there unfinished tasks?" check into enablement so the
// scheduler creates no row when the system is idle; only when at least one
// Midjourney task is in progress does a row get scheduled.
type midjourneyPollHandler struct{}

func (midjourneyPollHandler) Type() string { return model.SystemTaskTypeMidjourneyPoll }

func (midjourneyPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedMidjourneyTasks()
}

func (midjourneyPollHandler) Interval() time.Duration { return 15 * time.Second }

func (midjourneyPollHandler) NewPayload() any { return nil }

func (midjourneyPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := runMidjourneyTaskUpdateOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// asyncTaskPollHandler runs one async-task (Suno/video) polling pass per
// scheduled run. Like midjourneyPollHandler, Enabled() folds in the unfinished
// task existence check so an idle system schedules no rows.
type asyncTaskPollHandler struct{}

func (asyncTaskPollHandler) Type() string { return model.SystemTaskTypeAsyncTaskPoll }

func (asyncTaskPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedSyncTasks()
}

func (asyncTaskPollHandler) Interval() time.Duration { return 15 * time.Second }

func (asyncTaskPollHandler) NewPayload() any { return nil }

func (asyncTaskPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := service.RunTaskPollingOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

func finishSystemTaskHandler(task *model.SystemTask, runnerID string, status model.SystemTaskStatus, result any, runErr error) {
	errorMessage := ""
	if runErr != nil {
		errorMessage = runErr.Error()
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); err != nil {
		common.SysLog(fmt.Sprintf("system task %s failed to persist result: %v", task.TaskID, err))
	}
}
