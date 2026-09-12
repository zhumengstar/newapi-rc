package service

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
)

const (
	channelErrorGuardDefaultWindow      = 10 * time.Minute
	channelErrorGuardHardWindow         = 5 * time.Minute
	channelErrorGuardQuotaWindow        = 2 * time.Minute
	channelErrorGuardMaxRows            = 20000
	channelErrorGuardTransientThreshold = 5
	channelErrorGuardTransientStreak    = 10
	channelErrorGuardHardThreshold      = 3
	channelErrorGuardHardStreak         = 2
	channelErrorGuardRecentRequests     = 20
	channelErrorGuardHardRecentRequests = 5
)

var channelErrorGuardMu sync.Mutex
var channelErrorGuardStatusPattern = regexp.MustCompile(`(?i)(?:status(?:_code)?|http(?: status)?)\s*[=:]?\s*(\d{3})`)

// ChannelErrorGuardSummary is intentionally aggregate-only because it is
// persisted in system_task.result and may be shown to administrators.
type ChannelErrorGuardSummary struct {
	ChannelsScanned int   `json:"channels_scanned"`
	LogsRead        int   `json:"logs_read"`
	RequestsSeen    int   `json:"requests_seen"`
	Candidates      int   `json:"candidates"`
	Disabled        int   `json:"disabled"`
	At              int64 `json:"at"`
}

type channelGuardEvent struct {
	ID        int
	CreatedAt int64
	ChannelID int
	Type      int
	Quota     int
	RequestID string
	Content   string
	Status    int
}

type channelGuardCandidate struct {
	ChannelID int
	Signature string
	Count     int
	Threshold int
	Recent    int
	Streak    int
	Hard      bool
}

// RunChannelErrorGuardOnce applies the rolling error guard used by the
// production helper. It only considers enabled, auto-ban channels and never
// changes priorities or weights. Recovery remains owned by channel testing.
func RunChannelErrorGuardOnce(ctx context.Context) (ChannelErrorGuardSummary, error) {
	channelErrorGuardMu.Lock()
	defer channelErrorGuardMu.Unlock()

	summary := ChannelErrorGuardSummary{At: common.GetTimestamp()}
	if model.DB == nil || model.LOG_DB == nil {
		return summary, fmt.Errorf("channel error guard databases are not initialized")
	}

	var channels []model.Channel
	if err := model.DB.WithContext(ctx).
		Where("status = ? AND auto_ban = ?", common.ChannelStatusEnabled, true).
		Find(&channels).Error; err != nil {
		return summary, fmt.Errorf("load guard channels: %w", err)
	}
	ids := make([]int, 0, len(channels))
	for _, channel := range channels {
		ids = append(ids, channel.Id)
	}
	cooldowns, err := model.GetChannelErrorGuardCooldowns(ctx, ids)
	if err != nil {
		return summary, fmt.Errorf("load channel guard cooldowns: %w", err)
	}
	now := summary.At
	active := channels[:0]
	for _, channel := range channels {
		if cooldowns[channel.Id] > now {
			continue
		}
		active = append(active, channel)
	}
	channels = active
	summary.ChannelsScanned = len(channels)
	if len(channels) == 0 {
		return summary, nil
	}

	ids = make([]int, 0, len(channels))
	channelByID := make(map[int]model.Channel, len(channels))
	for _, channel := range channels {
		ids = append(ids, channel.Id)
		channelByID[channel.Id] = channel
	}
	cutoff := summary.At - int64(channelErrorGuardDefaultWindow/time.Second)
	var logs []model.Log
	if err := model.LOG_DB.WithContext(ctx).
		Where("channel_id IN ? AND type IN ? AND created_at >= ?", ids,
			[]int{model.LogTypeConsume, model.LogTypeError}, cutoff).
		Order("id DESC").Limit(channelErrorGuardMaxRows).Find(&logs).Error; err != nil {
		return summary, fmt.Errorf("load guard logs: %w", err)
	}
	summary.LogsRead = len(logs)

	events := deduplicateChannelGuardEvents(logs)
	summary.RequestsSeen = len(events)
	byChannel := make(map[int][]channelGuardEvent)
	for _, event := range events {
		byChannel[event.ChannelID] = append(byChannel[event.ChannelID], event)
	}

	for channelID, channelEvents := range byChannel {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		candidates := findChannelGuardCandidates(channelEvents, summary.At)
		if len(candidates) == 0 {
			continue
		}
		candidate := candidates[0]
		summary.Candidates++
		channel, ok := channelByID[channelID]
		if !ok {
			continue
		}
		reason := formatChannelGuardReason(candidate)
		channelError := types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", channel.GetAutoBan())
		DisableChannel(*channelError, reason)
		// DisableChannel is deliberately idempotent. Read the final state for an
		// aggregate summary; a concurrent writer may legitimately own the change.
		var current model.Channel
		if err := model.DB.WithContext(ctx).Select("status").Where("id = ?", channelID).First(&current).Error; err == nil && current.Status == common.ChannelStatusAutoDisabled {
			summary.Disabled++
		}
	}
	return summary, nil
}

func deduplicateChannelGuardEvents(logs []model.Log) []channelGuardEvent {
	byRequest := make(map[string]channelGuardEvent, len(logs))
	for _, log := range logs {
		key := strings.TrimSpace(log.RequestId)
		if key == "" {
			key = fmt.Sprintf("log:%d", log.Id)
		} else {
			key = fmt.Sprintf("%d:%s", log.ChannelId, key)
		}
		status := logStatusCode(log.Other)
		if status == 0 {
			status = contentStatusCode(log.Content)
		}
		event := channelGuardEvent{ID: log.Id, CreatedAt: log.CreatedAt, ChannelID: log.ChannelId, Type: log.Type, Quota: log.Quota, RequestID: key, Content: log.Content, Status: status}
		previous, exists := byRequest[key]
		if !exists || (event.Type == model.LogTypeConsume && event.Quota > 0 && !(previous.Type == model.LogTypeConsume && previous.Quota > 0)) {
			byRequest[key] = event
		}
	}
	result := make([]channelGuardEvent, 0, len(byRequest))
	for _, event := range byRequest {
		result = append(result, event)
	}
	return result
}

func contentStatusCode(content string) int {
	match := channelErrorGuardStatusPattern.FindStringSubmatch(content)
	if len(match) != 2 {
		return 0
	}
	status, _ := strconv.Atoi(match[1])
	return status
}

func logStatusCode(other string) int {
	values, err := common.StrToMap(other)
	if err != nil || values == nil {
		return 0
	}
	value, ok := values["status_code"]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}

func channelGuardSignature(event channelGuardEvent) (string, bool, bool) {
	if event.Type == model.LogTypeConsume {
		return "", false, false
	}
	lower := strings.ToLower(event.Content)
	category := ""
	switch {
	case strings.Contains(lower, "payload too large"), strings.Contains(lower, "request entity too large"), strings.Contains(lower, "model not found"), strings.Contains(lower, "no available channel"), strings.Contains(lower, "分组无权访问"):
		return "", false, false
	case strings.Contains(lower, "insufficient quota"), strings.Contains(lower, "insufficient balance"), strings.Contains(lower, "insufficient_balance"), strings.Contains(lower, "余额不足"), strings.Contains(lower, "额度不足"):
		category = "quota"
	case strings.Contains(lower, "invalid api key"), strings.Contains(lower, "invalid token"), strings.Contains(lower, "security token"):
		category = "invalid_token"
	case strings.Contains(lower, "permission denied"), strings.Contains(lower, "not authorized"), strings.Contains(lower, "operation not allowed"):
		category = "permission"
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "timed out"), strings.Contains(lower, "deadline exceeded"):
		category = "timeout"
	case strings.Contains(lower, "rate limit"), strings.Contains(lower, "too many requests"):
		category = "rate_limit"
	case event.Status == http.StatusUnauthorized || event.Status == http.StatusForbidden:
		category = "permission"
	case event.Status == http.StatusRequestTimeout || event.Status == http.StatusTooManyRequests || event.Status >= 500:
		category = "upstream"
	default:
		return "", false, false
	}
	hard := category == "quota" || category == "permission" || category == "invalid_token"
	return fmt.Sprintf("http_%d:%s", event.Status, category), true, hard
}

func findChannelGuardCandidates(events []channelGuardEvent, now int64) []channelGuardCandidate {
	if len(events) == 0 {
		return nil
	}
	// The database query is newest-first, but sorting here also makes the pure
	// decision function deterministic for tests and alternate callers.
	sortChannelGuardEvents(events)
	latest := events[len(events)-1].CreatedAt
	if latest == 0 {
		latest = now
	}
	var candidates []channelGuardCandidate
	signatures := make(map[string]bool)
	for _, event := range events {
		if signature, eligible, _ := channelGuardSignature(event); eligible {
			signatures[signature] = true
		}
	}
	for signature := range signatures {
		hard := false
		for _, event := range events {
			if value, eligible, isHard := channelGuardSignature(event); eligible && value == signature {
				hard = isHard
				break
			}
		}
		window := int64(channelErrorGuardDefaultWindow / time.Second)
		recentLimit, threshold, requiredStreak := channelErrorGuardRecentRequests, channelErrorGuardTransientThreshold, channelErrorGuardTransientStreak
		if hard {
			recentLimit, threshold, requiredStreak = channelErrorGuardHardRecentRequests, channelErrorGuardHardThreshold, channelErrorGuardHardStreak
			window = int64(channelErrorGuardHardWindow / time.Second)
			if strings.HasSuffix(signature, ":quota") {
				window = int64(channelErrorGuardQuotaWindow / time.Second)
			}
		}
		windowed := events
		if len(events) > recentLimit {
			windowed = events[len(events)-recentLimit:]
		}
		count := 0
		for _, event := range windowed {
			if event.CreatedAt >= latest-window {
				if value, eligible, _ := channelGuardSignature(event); eligible && value == signature {
					count++
				}
			}
		}
		streak := 0
		for i := len(events) - 1; i >= 0; i-- {
			value, eligible, _ := channelGuardSignature(events[i])
			if !eligible || value != signature {
				break
			}
			streak++
		}
		triggered := (hard && count >= threshold && streak >= requiredStreak) || (!hard && (count >= threshold || streak >= requiredStreak))
		if triggered {
			candidates = append(candidates, channelGuardCandidate{Signature: signature, Count: count, Threshold: threshold, Recent: recentLimit, Streak: streak, Hard: hard})
		}
	}
	return candidates
}

func sortChannelGuardEvents(events []channelGuardEvent) {
	for i := 1; i < len(events); i++ {
		current := events[i]
		j := i - 1
		for j >= 0 && (events[j].CreatedAt > current.CreatedAt || (events[j].CreatedAt == current.CreatedAt && events[j].ID > current.ID)) {
			events[j+1] = events[j]
			j--
		}
		events[j+1] = current
	}
}

func formatChannelGuardReason(candidate channelGuardCandidate) string {
	trigger := fmt.Sprintf("近%d次窗口内达到%d次", candidate.Recent, candidate.Threshold)
	if candidate.Streak >= 2 {
		trigger += fmt.Sprintf("，连续%d次", candidate.Streak)
	}
	return fmt.Sprintf("系统错误守护：%s相同上游错误（%s）", trigger, candidate.Signature)
}
