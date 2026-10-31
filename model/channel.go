package model

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Channel struct {
	Id                 int      `json:"id"`
	Type               int      `json:"type" gorm:"default:0"`
	Key                string   `json:"key" gorm:"not null"`
	OpenAIOrganization *string  `json:"openai_organization"`
	TestModel          *string  `json:"test_model"`
	Status             int      `json:"status" gorm:"default:1"`
	Name               string   `json:"name" gorm:"index"`
	Contact            string   `json:"contact" gorm:"type:varchar(255);index"`
	Weight             *uint    `json:"weight" gorm:"default:0"`
	CreatedTime        int64    `json:"created_time" gorm:"bigint"`
	TestTime           int64    `json:"test_time" gorm:"bigint"`
	ResponseTime       int      `json:"response_time"` // in milliseconds
	BaseURL            *string  `json:"base_url" gorm:"column:base_url;default:''"`
	Other              string   `json:"other"`
	Balance            float64  `json:"balance"` // in USD
	BalanceUpdatedTime int64    `json:"balance_updated_time" gorm:"bigint"`
	ChannelRatio       *float64 `json:"channel_ratio"`
	// ChannelRatioProvided distinguishes an omitted ratio from an explicit null
	// in partial update requests. It is never persisted or returned to clients.
	ChannelRatioProvided bool    `json:"-" gorm:"-"`
	Models               string  `json:"models"`
	Group                string  `json:"group" gorm:"type:varchar(64);default:'default'"`
	UsedQuota            int64   `json:"used_quota" gorm:"bigint;default:0"`
	ModelMapping         *string `json:"model_mapping" gorm:"type:text"`
	//MaxInputTokens     *int    `json:"max_input_tokens" gorm:"default:0"`
	StatusCodeMapping *string `json:"status_code_mapping" gorm:"type:varchar(1024);default:''"`
	Priority          *int64  `json:"priority" gorm:"bigint;default:0"`
	// CostTier is the provider cost layer used by routing. InputPrice and
	// OutputPrice are optional upstream unit-price hints, not user billing data.
	CostTier    int     `json:"cost_tier" gorm:"index"`
	InputPrice  float64 `json:"input_price"`
	OutputPrice float64 `json:"output_price"`
	AutoBan     *int    `json:"auto_ban" gorm:"default:1"`
	// RPMLimit caps requests routed through this channel in a fixed
	// one-minute window. Zero disables the channel-level limit.
	RPMLimit int `json:"rpm_limit" gorm:"default:0;index"`
	// Adaptive routing adjusts enabled ability weights from recent per-channel
	// request quality. It never changes channel status or priority.
	AdaptiveEnabled         bool   `json:"adaptive_enabled"`
	AdaptiveWindowSeconds   int    `json:"adaptive_window_seconds"`
	AdaptiveMinSamples      int    `json:"adaptive_min_samples"`
	AdaptiveSlowThresholdMs int    `json:"adaptive_slow_threshold_ms"`
	AdaptiveMinWeight       uint   `json:"adaptive_min_weight"`
	AdaptiveMaxWeight       uint   `json:"adaptive_max_weight"`
	AdaptiveRecoveryWeight  uint   `json:"adaptive_recovery_weight"`
	AdaptiveCooldownSeconds int    `json:"adaptive_cooldown_seconds"`
	AdaptiveLastEvaluatedAt int64  `json:"adaptive_last_evaluated_at" gorm:"bigint"`
	AdaptiveLastAppliedAt   int64  `json:"adaptive_last_applied_at" gorm:"bigint"`
	AdaptiveLastReason      string `json:"adaptive_last_reason" gorm:"type:text"`
	OtherInfo               string `json:"other_info"`
	// SiteType is the independently editable NewAPI/Sub2API site classification.
	SiteType       *string `json:"site_type" gorm:"type:varchar(16);index"`
	Tag            *string `json:"tag" gorm:"index"`
	Setting        *string `json:"setting" gorm:"type:text"` // 渠道额外设置
	ParamOverride  *string `json:"param_override" gorm:"type:text"`
	HeaderOverride *string `json:"header_override" gorm:"type:text"`
	Remark         *string `json:"remark" gorm:"type:varchar(255)" validate:"max=255"`
	// add after v0.8.5
	ChannelInfo ChannelInfo `json:"channel_info" gorm:"type:json"`

	OtherSettings string `json:"settings" gorm:"column:settings"` // 其他设置，存储azure版本等不需要检索的信息，详见dto.ChannelOtherSettings
	// Balance credentials are encrypted at rest and never serialized in API responses.
	BalanceAccessToken           string `json:"-" gorm:"column:balance_access_token;type:text"`
	BalanceUsername              string `json:"-" gorm:"column:balance_username;type:varchar(255)"`
	BalancePassword              string `json:"-" gorm:"column:balance_password;type:text"`
	BalanceMode                  string `json:"balance_mode" gorm:"column:balance_mode;type:varchar(16)"`
	BalanceAccessTokenConfigured bool   `json:"balance_access_token_configured" gorm:"-"`
	BalanceLoginConfigured       bool   `json:"balance_login_configured" gorm:"-"`

	// cache info
	Keys []string `json:"-" gorm:"-"`
}

type ChannelInfo struct {
	IsMultiKey             bool                  `json:"is_multi_key"`                        // 是否多Key模式
	MultiKeySize           int                   `json:"multi_key_size"`                      // 多Key模式下的Key数量
	MultiKeyStatusList     map[int]int           `json:"multi_key_status_list"`               // key状态列表，key index -> status
	MultiKeyDisabledReason map[int]string        `json:"multi_key_disabled_reason,omitempty"` // key禁用原因列表，key index -> reason
	MultiKeyDisabledTime   map[int]int64         `json:"multi_key_disabled_time,omitempty"`   // key禁用时间列表，key index -> time
	MultiKeyPollingIndex   int                   `json:"multi_key_polling_index"`             // 多Key模式下轮询的key索引
	MultiKeyMode           constant.MultiKeyMode `json:"multi_key_mode"`
}

type ChannelSortOptions struct {
	SortBy     string
	SortOrder  string
	IDSort     bool
	GroupOrder []string
}

var channelSortColumns = map[string]string{
	"id":            "id",
	"name":          "name",
	"site_type":     "site_type",
	"type":          "type",
	"group":         "group",
	"priority":      "priority",
	"weight":        "weight",
	"balance":       "balance",
	"channel_ratio": "channel_ratio",
	"used_quota":    "used_quota",
	"response_time": "response_time",
	"test_time":     "test_time",
}

func NewChannelSortOptions(sortBy string, sortOrder string, idSort bool, groupOrder ...string) ChannelSortOptions {
	normalizedSortBy := strings.ToLower(strings.TrimSpace(sortBy))
	normalizedSortOrder := strings.ToLower(strings.TrimSpace(sortOrder))
	if _, ok := channelSortColumns[normalizedSortBy]; !ok {
		normalizedSortBy = ""
		normalizedSortOrder = ""
	} else if normalizedSortOrder != "asc" {
		normalizedSortOrder = "desc"
	}

	return ChannelSortOptions{
		SortBy:     normalizedSortBy,
		SortOrder:  normalizedSortOrder,
		IDSort:     idSort,
		GroupOrder: normalizeChannelGroupOrder(groupOrder),
	}
}

func (options ChannelSortOptions) Apply(query *gorm.DB) *gorm.DB {
	if (options.SortBy == "" || options.SortBy == "group") && len(options.GroupOrder) > 0 {
		orderedGroups := options.GroupOrder
		if options.SortBy == "group" && options.SortOrder == "asc" {
			orderedGroups = make([]string, len(options.GroupOrder))
			for i, g := range options.GroupOrder {
				orderedGroups[len(options.GroupOrder)-1-i] = g
			}
		}
		caseSQL := "CASE"
		args := make([]any, 0, len(orderedGroups)*5+1)
		for index, group := range orderedGroups {
			caseSQL += " WHEN " + commonGroupCol + " = ? OR " + commonGroupCol + " LIKE ? OR " + commonGroupCol + " LIKE ? OR " + commonGroupCol + " LIKE ? THEN CAST(? AS INTEGER)"
			args = append(args, group, group+",%", "%,"+group, "%,"+group+",%", index)
		}
		caseSQL += " ELSE CAST(? AS INTEGER) END, priority DESC, weight DESC, id"
		args = append(args, len(orderedGroups))
		// Keep the complete ordering expression in one clause. GORM prepends
		// columns from subsequent Order calls, which would make priority sort
		// before the group rank and split groups apart.
		return query.Clauses(clause.OrderBy{Expression: clause.Expr{SQL: caseSQL, Vars: args}})
	}
	if columnName, ok := channelSortColumns[options.SortBy]; ok {
		if columnName == "channel_ratio" {
			// Keep NULL ratios after concrete values on every supported database.
			// PostgreSQL otherwise puts NULLs first for descending order.
			query = query.Order(gorm.Expr("CASE WHEN channel_ratio IS NULL THEN 1 ELSE 0 END ASC"))
		}
		isDesc := options.SortOrder != "asc"
		if columnName == "group" {
			query = query.Order(clause.OrderByColumn{
				Column: clause.Column{Name: commonGroupCol, Raw: true},
				Desc:   isDesc,
			})
			query = query.Order(clause.OrderByColumn{
				Column: clause.Column{Name: "priority"},
				Desc:   true,
			})
			query = query.Order(clause.OrderByColumn{
				Column: clause.Column{Name: "weight"},
				Desc:   true,
			})
		} else {
			query = query.Order(clause.OrderByColumn{
				Column: clause.Column{Name: columnName},
				Desc:   isDesc,
			})
		}
		if columnName == "priority" {
			query = query.Order(clause.OrderByColumn{
				Column: clause.Column{Name: "weight"},
				Desc:   isDesc,
			})
		} else if columnName == "weight" {
			query = query.Order(clause.OrderByColumn{
				Column: clause.Column{Name: "priority"},
				Desc:   isDesc,
			})
		}
		// Keep pagination deterministic when multiple channels have the same
		// value for the selected column.
		if columnName != "id" {
			query = query.Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}})
		}
		return query
	}
	if options.IDSort {
		return query.Order(clause.OrderByColumn{
			Column: clause.Column{Name: "id"},
			Desc:   true,
		})
	}
	return query.Order(clause.OrderByColumn{
		Column: clause.Column{Name: "priority"},
		Desc:   true,
	})
}

func normalizeChannelGroupOrder(groups []string) []string {
	ordered := make([]string, 0, len(groups))
	seen := make(map[string]struct{}, len(groups))
	for _, raw := range groups {
		for _, part := range strings.Split(raw, ",") {
			group := strings.TrimSpace(part)
			if group == "" {
				continue
			}
			if _, exists := seen[group]; exists {
				continue
			}
			seen[group] = struct{}{}
			ordered = append(ordered, group)
		}
	}
	return ordered
}

func resolveChannelSortOptions(idSort bool, sortOptions []ChannelSortOptions) ChannelSortOptions {
	if len(sortOptions) == 0 {
		return NewChannelSortOptions("", "", idSort)
	}
	options := sortOptions[0]
	options.IDSort = options.IDSort || idSort
	return options
}

func NormalizeChannelGroupFilter(group string) string {
	group = strings.TrimSpace(group)
	if group == "" || strings.EqualFold(group, "all") || strings.EqualFold(group, "null") {
		return ""
	}
	return group
}

func channelGroupFilterCondition() string {
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		return `CONCAT(',', ` + commonGroupCol + `, ',') LIKE ? ESCAPE '!'`
	}
	return `(',' || ` + commonGroupCol + ` || ',') LIKE ? ESCAPE '!'`
}

func channelGroupFilterPattern(group string) string {
	group = strings.NewReplacer(
		"!", "!!",
		"%", "!%",
		"_", "!_",
	).Replace(group)
	return "%," + group + ",%"
}

func ApplyChannelGroupFilter(query *gorm.DB, group string) *gorm.DB {
	group = NormalizeChannelGroupFilter(group)
	if group == "" {
		return query
	}
	parts := strings.Split(group, ",")
	var conditions []string
	var args []any
	likeOp := mainLikeOp()
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || strings.EqualFold(p, "all") || strings.EqualFold(p, "null") {
			continue
		}
		escaped := strings.NewReplacer(
			"!", "!!",
			"%", "!%",
			"_", "!_",
		).Replace(p)
		// 支持多选和模糊搜索：分组名称包含该关键词即可
		conditions = append(conditions, fmt.Sprintf("%s %s ? ESCAPE '!'", commonGroupCol, likeOp))
		args = append(args, "%"+escaped+"%")
	}
	if len(conditions) == 0 {
		return query
	}
	return query.Where("("+strings.Join(conditions, " OR ")+")", args...)
}

// Value implements driver.Valuer interface
// 必须返回 string 而非 []byte:PG simple protocol 下 []byte 参数按 bytea
// 编码,写 json 列会触发 SQLSTATE 22P02。
func (c ChannelInfo) Value() (driver.Value, error) {
	b, err := common.Marshal(&c)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements sql.Scanner interface
func (c *ChannelInfo) Scan(value any) error {
	return common.Unmarshal(jsonScanBytes(value), c)
}

func (channel *Channel) GetKeys() []string {
	if channel.Key == "" {
		return []string{}
	}
	if len(channel.Keys) > 0 {
		return channel.Keys
	}
	trimmed := strings.TrimSpace(channel.Key)
	// If the key starts with '[', try to parse it as a JSON array (e.g., for Vertex AI scenarios)
	if strings.HasPrefix(trimmed, "[") {
		var arr []json.RawMessage
		if err := common.Unmarshal([]byte(trimmed), &arr); err == nil {
			res := make([]string, len(arr))
			for i, v := range arr {
				res[i] = string(v)
			}
			return res
		}
	}
	// Otherwise, fall back to splitting by newline
	keys := strings.Split(strings.Trim(channel.Key, "\n"), "\n")
	return keys
}

func (channel *Channel) GetNextEnabledKey() (string, int, *types.NewAPIError) {
	// If not in multi-key mode, return the original key string directly.
	if !channel.ChannelInfo.IsMultiKey {
		return channel.Key, 0, nil
	}

	// Obtain all keys (split by \n)
	keys := channel.GetKeys()
	if len(keys) == 0 {
		// No keys available, return error, should disable the channel
		return "", 0, types.NewError(errors.New("no keys available"), types.ErrorCodeChannelNoAvailableKey)
	}

	lock := GetChannelPollingLock(channel.Id)
	lock.Lock()
	defer lock.Unlock()

	statusList := channel.ChannelInfo.MultiKeyStatusList
	// helper to get key status, default to enabled when missing
	getStatus := func(idx int) int {
		if statusList == nil {
			return common.ChannelStatusEnabled
		}
		if status, ok := statusList[idx]; ok {
			return status
		}
		return common.ChannelStatusEnabled
	}

	// Collect indexes of enabled keys
	enabledIdx := make([]int, 0, len(keys))
	for i := range keys {
		if getStatus(i) == common.ChannelStatusEnabled {
			enabledIdx = append(enabledIdx, i)
		}
	}
	// If no specific status list or none enabled, return an explicit error so caller can
	// properly handle a channel with no available keys (e.g. mark channel disabled).
	// Returning the first key here caused requests to keep using an already-disabled key.
	if len(enabledIdx) == 0 {
		return "", 0, types.NewError(errors.New("no enabled keys"), types.ErrorCodeChannelNoAvailableKey)
	}

	switch channel.ChannelInfo.MultiKeyMode {
	case constant.MultiKeyModeRandom:
		// Randomly pick one enabled key
		selectedIdx := enabledIdx[rand.Intn(len(enabledIdx))]
		return keys[selectedIdx], selectedIdx, nil
	case constant.MultiKeyModePolling:
		// Use channel-specific lock to ensure thread-safe polling

		channelInfo, err := CacheGetChannelInfo(channel.Id)
		if err != nil {
			return "", 0, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		}
		defer func() {
			if common.DebugEnabled {
				logger.LogDebug(nil, "channel %d polling index: %d", channel.Id, channel.ChannelInfo.MultiKeyPollingIndex)
			}
			if !common.MemoryCacheEnabled {
				_ = channel.SaveChannelInfo()
			} else {
				// CacheUpdateChannel(channel)
			}
		}()
		// Start from the saved polling index and look for the next enabled key
		start := channelInfo.MultiKeyPollingIndex
		if start < 0 || start >= len(keys) {
			start = 0
		}
		for i := range keys {
			idx := (start + i) % len(keys)
			if getStatus(idx) == common.ChannelStatusEnabled {
				// update polling index for next call (point to the next position)
				channel.ChannelInfo.MultiKeyPollingIndex = (idx + 1) % len(keys)
				return keys[idx], idx, nil
			}
		}
		// Fallback – should not happen, but return first enabled key
		return keys[enabledIdx[0]], enabledIdx[0], nil
	default:
		// Unknown mode, default to first enabled key (or original key string)
		return keys[enabledIdx[0]], enabledIdx[0], nil
	}
}

func (channel *Channel) SaveChannelInfo() error {
	return DB.Model(channel).Update("channel_info", channel.ChannelInfo).Error
}

func (channel *Channel) GetModels() []string {
	if channel.Models == "" {
		return []string{}
	}
	return strings.Split(strings.Trim(channel.Models, ","), ",")
}

func (channel *Channel) GetGroups() []string {
	if channel.Group == "" {
		return []string{}
	}
	parts := strings.Split(strings.Trim(channel.Group, ","), ",")
	groups := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		group := strings.TrimSpace(part)
		if group == "" {
			continue
		}
		if _, exists := seen[group]; exists {
			continue
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
	}
	return groups
}

func GetChannelGroupsWithQuotas() ([]string, map[string]int64, error) {
	var channels []Channel
	if err := DB.Model(&Channel{}).Select(commonGroupCol).Find(&channels).Error; err != nil {
		return nil, nil, err
	}

	groupSet := make(map[string]struct{})
	for i := range channels {
		for _, group := range channels[i].GetGroups() {
			if group != "" {
				groupSet[group] = struct{}{}
			}
		}
	}

	groups := make([]string, 0, len(groupSet))
	for group := range groupSet {
		groups = append(groups, group)
	}

	_, quotaMap := GetRankedGroupsWithQuotas()
	ratioMap := ratio_setting.GetGroupRatioCopy()
	SortGroupsSliceByQuotaAndRatio(groups, quotaMap, ratioMap)

	filteredQuotas := make(map[string]int64, len(groups))
	for _, g := range groups {
		if q := quotaMap[g]; q > 0 {
			filteredQuotas[g] = q
		}
	}
	return groups, filteredQuotas, nil
}

func GetChannelGroups() ([]string, error) {
	groups, _, err := GetChannelGroupsWithQuotas()
	return groups, err
}

func GetGroupModelsMap() (map[string][]string, error) {
	var channels []Channel
	if err := DB.Model(&Channel{}).Where("status = ?", common.ChannelStatusEnabled).Select(commonGroupCol, "models").Find(&channels).Error; err != nil {
		return nil, err
	}

	groupModels := make(map[string]map[string]struct{})
	for i := range channels {
		for _, group := range channels[i].GetGroups() {
			if group == "" {
				continue
			}
			if _, ok := groupModels[group]; !ok {
				groupModels[group] = make(map[string]struct{})
			}
			for _, m := range strings.Split(channels[i].Models, ",") {
				m = strings.TrimSpace(m)
				if m != "" {
					groupModels[group][m] = struct{}{}
				}
			}
		}
	}

	res := make(map[string][]string, len(groupModels))
	for g, mset := range groupModels {
		list := make([]string, 0, len(mset))
		for m := range mset {
			list = append(list, m)
		}
		sort.Strings(list)
		res[g] = list
	}
	return res, nil
}

type ChannelGroupItem struct {
	Id           int    `json:"id"`
	Name         string `json:"name"`
	Type         int    `json:"type"`
	Status       int    `json:"status"`
	Priority     int64  `json:"priority"`
	Weight       uint   `json:"weight"`
	ResponseTime int    `json:"response_time"`
}

func GetGroupChannelsMap() (map[string][]ChannelGroupItem, error) {
	var channels []Channel
	if err := DB.Model(&Channel{}).Select("id", "name", "type", "status", "priority", "weight", "response_time", commonGroupCol).Find(&channels).Error; err != nil {
		return nil, err
	}

	res := make(map[string][]ChannelGroupItem)
	for i := range channels {
		p := int64(0)
		if channels[i].Priority != nil {
			p = *channels[i].Priority
		}
		w := uint(0)
		if channels[i].Weight != nil {
			w = *channels[i].Weight
		}
		item := ChannelGroupItem{
			Id:           channels[i].Id,
			Name:         channels[i].Name,
			Type:         channels[i].Type,
			Status:       channels[i].Status,
			Priority:     p,
			Weight:       w,
			ResponseTime: channels[i].ResponseTime,
		}
		for _, group := range channels[i].GetGroups() {
			if group == "" {
				continue
			}
			res[group] = append(res[group], item)
		}
	}
	// 按 ID 升序排序
	for g := range res {
		sort.Slice(res[g], func(i, j int) bool {
			return res[g][i].Id < res[g][j].Id
		})
	}
	return res, nil
}

func GetGroupChannelIds(group string) []int {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	res := make([]int, 0)
	for id, ch := range channelsIDM {
		if ch != nil {
			for _, g := range ch.GetGroups() {
				if g == group {
					res = append(res, id)
					break
				}
			}
		}
	}
	return res
}

func GetChannelIdsWithBaseURL(baseURL string) []int {
	if baseURL == "" {
		return nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	res := make([]int, 0)
	cleanURL := strings.TrimRight(strings.ToLower(baseURL), "/")
	for id, ch := range channelsIDM {
		if ch != nil && strings.TrimRight(strings.ToLower(ch.GetBaseURL()), "/") == cleanURL {
			res = append(res, id)
		}
	}
	return res
}

func GetGroupChannelIdsWithBaseURL(group string, baseURL string) []int {
	if baseURL == "" {
		return nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	res := make([]int, 0)
	cleanURL := strings.TrimRight(strings.ToLower(baseURL), "/")
	for id, ch := range channelsIDM {
		if ch != nil && strings.TrimRight(strings.ToLower(ch.GetBaseURL()), "/") == cleanURL {
			for _, g := range ch.GetGroups() {
				if g == group {
					res = append(res, id)
					break
				}
			}
		}
	}
	return res
}

func GetCacheChannel(id int) *Channel {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	return channelsIDM[id]
}


// UpdateChannelGroupAdaptiveEnabled updates every channel that belongs to the
// exact group. Group membership is stored as a comma-separated list, so use
// the shared cross-database filter instead of a dialect-specific expression.
func UpdateChannelGroupAdaptiveEnabled(group string, enabled bool) (int64, error) {
	group = NormalizeChannelGroupFilter(group)
	if group == "" || strings.Contains(group, ",") {
		return 0, errors.New("group is required")
	}
	result := ApplyChannelGroupFilter(DB.Model(&Channel{}), group).
		Update("adaptive_enabled", enabled)
	return result.RowsAffected, result.Error
}

// UpdateChannelAdaptiveEnabled updates only one channel's adaptive-routing flag.
func UpdateChannelAdaptiveEnabled(id int, enabled bool) error {
	if id <= 0 {
		return errors.New("channel ID is invalid")
	}
	result := DB.Model(&Channel{}).Where("id = ?", id).
		Update("adaptive_enabled", enabled)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ApplyChannelGroupPriorityOrder assigns each group a reserved priority range:
// 1-9, 11-19, 21-29... . A channel in multiple groups receives the earliest
// matching group's range, while each ability uses the top value of its group's
// range. Existing channel priorities are kept as an offset inside the new
// range whenever possible.
func ApplyChannelGroupPriorityOrder(groups []string) (int64, error) {
	return ApplyChannelGroupPriorityOrderWithStep(context.Background(), groups, 10)
}

// ApplyChannelGroupPriorityOrderWithStep is the shared atomic implementation
// for both an operator-supplied group order and scheduled normalization.
func ApplyChannelGroupPriorityOrderWithStep(ctx context.Context, groups []string, step int64) (int64, error) {
	if step < 2 {
		return 0, errors.New("priority step must reserve at least one value")
	}
	ordered := make([]string, 0, len(groups))
	seen := make(map[string]struct{}, len(groups))
	for _, raw := range groups {
		group := NormalizeChannelGroupFilter(raw)
		if group == "" {
			continue
		}
		if strings.Contains(group, ",") {
			return 0, errors.New("group must be a single group")
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		ordered = append(ordered, group)
	}
	if len(ordered) == 0 {
		return 0, errors.New("groups are required")
	}

	var updated int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var channels []Channel
		if err := lockForUpdate(tx.Model(&Channel{})).Select("id", commonGroupCol, "priority", "weight").Find(&channels).Error; err != nil {
			return err
		}
		channelPriority := make(map[int]int64)
		for index, group := range ordered {
			rangeStart := int64(index)*step + 1
			groupPriority := rangeStart + step - 2
			for _, channel := range channels {
				for _, member := range channel.GetGroups() {
					if member == group {
						if _, exists := channelPriority[channel.Id]; !exists {
							offset := int64(0)
							if channel.Priority != nil {
								offset = *channel.Priority % step
							}
							if offset < 1 || offset > step-1 {
								offset = step - 1
							}
							channelPriority[channel.Id] = rangeStart + offset - 1
						}
						break
					}
				}
			}
			if err := tx.Model(&Ability{}).Where(commonGroupCol+" = ?", group).Update("priority", groupPriority).Error; err != nil {
				return err
			}
		}
		for channelID, priority := range channelPriority {
			result := tx.Model(&Channel{}).Where("id = ?", channelID).Update("priority", priority)
			if result.Error != nil {
				return result.Error
			}
			updated += result.RowsAffected
		}
		return nil
	})
	return updated, err
}

// RebalanceChannelWeightsForChannel is a no-op now.
// Channel weights are independently controlled by administrators and must not be auto-adjusted.
func RebalanceChannelWeightsForChannel(channelID int) error {
	return nil
}

func rebalanceChannelGroupWeights(tx *gorm.DB, groups []string, channels []Channel, fixedChannelID int) error {
	return nil
}

func (channel *Channel) GetOtherInfo() map[string]any {
	otherInfo := make(map[string]any)
	if channel.OtherInfo != "" {
		err := common.Unmarshal([]byte(channel.OtherInfo), &otherInfo)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to unmarshal other info: channel_id=%d, tag=%s, name=%s, error=%v", channel.Id, channel.GetTag(), channel.Name, err))
		}
	}
	return otherInfo
}

func (channel *Channel) SetOtherInfo(otherInfo map[string]any) {
	otherInfoBytes, err := json.Marshal(otherInfo)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to marshal other info: channel_id=%d, tag=%s, name=%s, error=%v", channel.Id, channel.GetTag(), channel.Name, err))
		return
	}
	channel.OtherInfo = string(otherInfoBytes)
}

func (channel *Channel) GetTag() string {
	if channel.Tag == nil {
		return ""
	}
	return *channel.Tag
}

func (channel *Channel) SetTag(tag string) {
	channel.Tag = &tag
}

func (channel *Channel) GetAutoBan() bool {
	if channel.AutoBan == nil {
		return false
	}
	return *channel.AutoBan == 1
}

func (channel *Channel) Save() error {
	return DB.Save(channel).Error
}

// saveStatusState persists only the fields owned by the channel status flow.
// Keeping this allowlist here prevents a stale channel snapshot from
// overwriting credentials, accounting counters, or channel configuration.
func (channel *Channel) saveStatusState() error {
	if channel.Id == 0 {
		return errors.New("channel ID is 0")
	}
	updates := map[string]any{
		"status":     channel.Status,
		"other_info": channel.OtherInfo,
	}
	if channel.ChannelInfo.IsMultiKey {
		updates["channel_info"] = channel.ChannelInfo
	}
	return DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(updates).Error
}

func GetAllChannels(startIdx int, num int, selectAll bool, idSort bool, sortOptions ...ChannelSortOptions) ([]*Channel, error) {
	var channels []*Channel
	var err error
	order := resolveChannelSortOptions(idSort, sortOptions)
	if selectAll {
		err = order.Apply(DB).Find(&channels).Error
	} else {
		err = order.Apply(DB).Limit(num).Offset(startIdx).Omit("key").Find(&channels).Error
	}
	for _, channel := range channels {
		channel.NormalizeBalanceSettings()
	}
	return channels, err
}

func GetChannelsByTag(tag string, idSort bool, selectAll bool, sortOptions ...ChannelSortOptions) ([]*Channel, error) {
	var channels []*Channel
	order := resolveChannelSortOptions(idSort, sortOptions)
	query := order.Apply(DB.Where("tag = ?", tag))
	if !selectAll {
		query = query.Omit("key")
	}
	err := query.Find(&channels).Error
	for _, channel := range channels {
		channel.NormalizeBalanceSettings()
	}
	return channels, err
}

func SearchChannels(keyword string, group string, model string, idSort bool, sortOptions ...ChannelSortOptions) ([]*Channel, error) {
	var channels []*Channel
	modelsCol := "`models`"

	// 如果是 PostgreSQL，使用双引号
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		modelsCol = `"models"`
	}

	baseURLCol := "`base_url`"
	// 如果是 PostgreSQL，使用双引号
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		baseURLCol = `"base_url"`
	}

	order := resolveChannelSortOptions(idSort, sortOptions)

	// 构造基础查询
	baseQuery := DB.Model(&Channel{}).Omit("key")

	// 构造WHERE子句
	likeOp := mainLikeOp()
	whereClause := fmt.Sprintf("(id = ? OR name %s ? OR %s = ? OR %s %s ? OR %s %s ?) AND %s %s ?",
		likeOp, commonKeyCol, baseURLCol, likeOp, commonGroupCol, likeOp, modelsCol, likeOp)
	args := []any{common.String2Int(keyword), "%" + keyword + "%", keyword, "%" + keyword + "%", "%" + keyword + "%", "%" + model + "%"}
	baseQuery = ApplyChannelGroupFilter(baseQuery.Where(whereClause, args...), group)

	// 执行查询
	err := order.Apply(baseQuery).Find(&channels).Error
	if err != nil {
		return nil, err
	}
	for _, channel := range channels {
		channel.NormalizeBalanceSettings()
	}
	return channels, nil
}

// GetChannelById loads a channel directly from the database, bypassing the
// in-memory channel cache.
//
// WARNING: do NOT call this on request hot paths (middleware, distribution,
// relay submit/retry, polling). Every call is a synchronous DB query and will
// not see cache-only state. Use CacheGetChannel instead: it serves from the
// in-memory cache and falls back to this function automatically when
// MemoryCacheEnabled is false. Direct use is appropriate only where fresh DB
// state is required, e.g. admin CRUD, channel testing, or cache (re)building.
func GetChannelById(id int, selectAll bool) (*Channel, error) {
	channel := &Channel{Id: id}
	var err error = nil
	if selectAll {
		err = DB.First(channel, "id = ?", id).Error
	} else {
		err = DB.Omit("key").First(channel, "id = ?", id).Error
	}
	if err != nil {
		return nil, err
	}
	channel.NormalizeBalanceSettings()
	return channel, nil
}

func BatchInsertChannels(channels []Channel) error {
	if len(channels) == 0 {
		return nil
	}
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, chunk := range lo.Chunk(channels, 50) {
		if err := tx.Create(&chunk).Error; err != nil {
			tx.Rollback()
			return err
		}
		for _, channel_ := range chunk {
			if err := channel_.AddAbilities(tx); err != nil {
				tx.Rollback()
				return err
			}
		}
	}
	return tx.Commit().Error
}

func BatchDeleteChannels(ids []int) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	// 使用事务 分批删除channel表和abilities表
	tx := DB.Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	var deletedCount int64
	for _, chunk := range lo.Chunk(ids, 200) {
		result := tx.Where("id in (?)", chunk).Delete(&Channel{})
		if result.Error != nil {
			tx.Rollback()
			return 0, result.Error
		}
		deletedCount += result.RowsAffected
		if err := tx.Where("channel_id in (?)", chunk).Delete(&Ability{}).Error; err != nil {
			tx.Rollback()
			return 0, err
		}
	}
	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	return deletedCount, nil
}

func (channel *Channel) GetPriority() int64 {
	if channel.Priority == nil {
		return 0
	}
	return *channel.Priority
}

// GetRoutingPriority is the cost-layer priority used by failover selection.
// CostTier takes precedence when explicitly configured; legacy channels keep
// their existing priority semantics.
func (channel *Channel) GetRoutingPriority() int64 {
	if channel == nil {
		return 0
	}
	if channel.CostTier > 0 {
		return int64(channel.CostTier)
	}
	return channel.GetPriority()
}

func (channel *Channel) GetWeight() int {
	if channel.Weight == nil {
		return 0
	}
	return int(*channel.Weight)
}

func (channel *Channel) GetBaseURL() string {
	if channel.BaseURL == nil {
		return ""
	}
	url := *channel.BaseURL
	if url == "" {
		url = constant.GetChannelBaseURL(channel.Type)
	}
	return url
}

func (channel *Channel) GetModelMapping() string {
	if channel.ModelMapping == nil {
		return ""
	}
	return *channel.ModelMapping
}

func (channel *Channel) GetStatusCodeMapping() string {
	if channel.StatusCodeMapping == nil {
		return ""
	}
	return *channel.StatusCodeMapping
}

func (channel *Channel) Insert() error {
	var err error
	err = DB.Create(channel).Error
	if err != nil {
		return err
	}
	err = channel.AddAbilities(nil)
	return err
}

func (channel *Channel) Update() error {
	// If this is a multi-key channel, recalculate MultiKeySize based on the current key list to avoid inconsistency after editing keys
	if channel.ChannelInfo.IsMultiKey {
		var keyStr string
		if channel.Key != "" {
			keyStr = channel.Key
		} else {
			// If key is not provided, read the existing key from the database
			if existing, err := GetChannelById(channel.Id, true); err == nil {
				keyStr = existing.Key
			}
		}
		// Parse the key list (supports newline separation or JSON array)
		keys := []string{}
		if keyStr != "" {
			trimmed := strings.TrimSpace(keyStr)
			if strings.HasPrefix(trimmed, "[") {
				var arr []json.RawMessage
				if err := common.Unmarshal([]byte(trimmed), &arr); err == nil {
					keys = make([]string, len(arr))
					for i, v := range arr {
						keys[i] = string(v)
					}
				}
			}
			if len(keys) == 0 { // fallback to newline split
				keys = strings.Split(strings.Trim(keyStr, "\n"), "\n")
			}
		}
		channel.ChannelInfo.MultiKeySize = len(keys)
		// Clean up status data that exceeds the new key count to prevent index out of range
		if channel.ChannelInfo.MultiKeyStatusList != nil {
			for idx := range channel.ChannelInfo.MultiKeyStatusList {
				if idx >= channel.ChannelInfo.MultiKeySize {
					delete(channel.ChannelInfo.MultiKeyStatusList, idx)
				}
			}
		}
	}
	// Keep the scalar ratio update in the same transaction as the rest of the
	// channel edit. Struct Updates intentionally omits nil/zero fields, so an
	// explicit null (clear) or zero ratio must be written through a map update.
	err := DB.Transaction(func(tx *gorm.DB) error {
		channelUpdates := tx.Model(channel)
		if channel.ChannelRatioProvided {
			channelUpdates = channelUpdates.Omit("channel_ratio")
		}
		if err := channelUpdates.Updates(channel).Error; err != nil {
			return err
		}
		if !channel.ChannelRatioProvided {
			return nil
		}

		var ratioValue any
		if channel.ChannelRatio != nil {
			ratioValue = *channel.ChannelRatio
		}
		return tx.Model(&Channel{}).
			Where("id = ?", channel.Id).
			Updates(map[string]any{"channel_ratio": ratioValue}).Error
	})
	if err != nil {
		return err
	}
	if err := DB.Model(channel).First(channel, "id = ?", channel.Id).Error; err != nil {
		return err
	}
	err = channel.UpdateAbilities(nil)
	return err
}

// UpdateChannelRatio updates only the channel-level upstream multiplier. A
// direct editor sends a partial channel payload, so it must not run the normal
// channel update path or recalculate abilities from an incomplete snapshot.
func (channel *Channel) UpdateChannelRatio() error {
	if channel == nil || channel.Id == 0 {
		return errors.New("channel ID is 0")
	}

	var ratioValue any
	if channel.ChannelRatio != nil {
		ratioValue = *channel.ChannelRatio
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Channel{}).
			Where("id = ?", channel.Id).
			Updates(map[string]any{"channel_ratio": ratioValue}).Error; err != nil {
			return err
		}
		return tx.First(channel, "id = ?", channel.Id).Error
	})
}

func (channel *Channel) UpdateResponseTime(responseTime int64) {
	err := DB.Model(channel).Select("response_time", "test_time").Updates(Channel{
		TestTime:     common.GetTimestamp(),
		ResponseTime: int(responseTime),
	}).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to update response time: channel_id=%d, error=%v", channel.Id, err))
	}
}

func (channel *Channel) UpdateBalance(balance float64) {
	err := DB.Model(channel).Select("balance_updated_time", "balance").Updates(Channel{
		BalanceUpdatedTime: common.GetTimestamp(),
		Balance:            balance,
	}).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to update balance: channel_id=%d, error=%v", channel.Id, err))
	}
}

func (channel *Channel) SetBalanceCredentials(token, username, password string) error {
	var err error
	if channel.BalanceAccessToken, err = common.EncryptSecret(strings.TrimSpace(token)); err != nil {
		return err
	}
	channel.BalanceUsername = strings.TrimSpace(username)
	if channel.BalancePassword, err = common.EncryptSecret(password); err != nil {
		return err
	}
	return nil
}

func (channel *Channel) GetBalanceCredentials() (token, username, password string) {
	token, username, password, _ = channel.GetBalanceCredentialsWithError()
	return token, username, password
}

// GetBalanceCredentialsWithError distinguishes an empty credential from a
// stored ciphertext that can no longer be decrypted (for example after a
// CRYPTO_SECRET change). Callers updating credentials must fail closed rather
// than silently replacing the unreadable secret.
func (channel *Channel) GetBalanceCredentialsWithError() (token, username, password string, err error) {
	token, err = common.DecryptSecret(channel.BalanceAccessToken)
	if err != nil {
		return "", channel.BalanceUsername, "", fmt.Errorf("decrypt balance access token: %w", err)
	}
	password, err = common.DecryptSecret(channel.BalancePassword)
	if err != nil {
		return "", channel.BalanceUsername, "", fmt.Errorf("decrypt balance password: %w", err)
	}
	return token, channel.BalanceUsername, password, nil
}

func (channel *Channel) NormalizeBalanceSettings() {
	if channel.BalanceMode != "manual" {
		channel.BalanceMode = "auto"
	}
	channel.BalanceAccessTokenConfigured = channel.BalanceAccessToken != ""
	channel.BalanceLoginConfigured = channel.BalanceUsername != "" && channel.BalancePassword != ""
}

func (channel *Channel) Delete() error {
	var err error
	err = DB.Delete(channel).Error
	if err != nil {
		return err
	}
	err = channel.DeleteAbilities()
	return err
}

var channelStatusLock sync.Mutex

// channelPollingLocks stores locks for each channel.id to ensure thread-safe polling
var channelPollingLocks sync.Map

// GetChannelPollingLock returns or creates a mutex for the given channel ID
func GetChannelPollingLock(channelId int) *sync.Mutex {
	if lock, exists := channelPollingLocks.Load(channelId); exists {
		return lock.(*sync.Mutex)
	}
	// Create new lock for this channel
	newLock := &sync.Mutex{}
	actual, _ := channelPollingLocks.LoadOrStore(channelId, newLock)
	return actual.(*sync.Mutex)
}

// CleanupChannelPollingLocks removes locks for channels that no longer exist
// This is optional and can be called periodically to prevent memory leaks
func CleanupChannelPollingLocks() {
	var activeChannelIds []int
	DB.Model(&Channel{}).Pluck("id", &activeChannelIds)

	activeChannelSet := make(map[int]bool)
	for _, id := range activeChannelIds {
		activeChannelSet[id] = true
	}

	channelPollingLocks.Range(func(key, value any) bool {
		channelId := key.(int)
		if !activeChannelSet[channelId] {
			channelPollingLocks.Delete(channelId)
		}
		return true
	})
}

func handlerMultiKeyUpdate(channel *Channel, usingKey string, status int, reason string) {
	keys := channel.GetKeys()
	if len(keys) == 0 {
		channel.Status = status
	} else {
		keyIndex := -1
		for i, key := range keys {
			if key == usingKey {
				keyIndex = i
				break
			}
		}
		if keyIndex < 0 {
			if usingKey != "" {
				common.SysLog(fmt.Sprintf("failed to update multi-key status: channel_id=%d, using key not found", channel.Id))
				return
			}
			channel.Status = status
			info := channel.GetOtherInfo()
			info["status_reason"] = reason
			info["status_time"] = common.GetTimestamp()
			channel.SetOtherInfo(info)
			return
		}
		if channel.ChannelInfo.MultiKeyStatusList == nil {
			channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		}
		if status == common.ChannelStatusEnabled {
			delete(channel.ChannelInfo.MultiKeyStatusList, keyIndex)
		} else {
			channel.ChannelInfo.MultiKeyStatusList[keyIndex] = status
			if channel.ChannelInfo.MultiKeyDisabledReason == nil {
				channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)
			}
			if channel.ChannelInfo.MultiKeyDisabledTime == nil {
				channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
			}
			channel.ChannelInfo.MultiKeyDisabledReason[keyIndex] = reason
			channel.ChannelInfo.MultiKeyDisabledTime[keyIndex] = common.GetTimestamp()
		}
		if !hasEnabledMultiKey(keys, channel.ChannelInfo.MultiKeyStatusList) {
			channel.Status = common.ChannelStatusAutoDisabled
			info := channel.GetOtherInfo()
			info["status_reason"] = "All keys are disabled"
			info["status_time"] = common.GetTimestamp()
			channel.SetOtherInfo(info)
		} else if status == common.ChannelStatusEnabled {
			channel.Status = common.ChannelStatusEnabled
		}
	}
}

func hasEnabledMultiKey(keys []string, statusList map[int]int) bool {
	for i := range keys {
		if statusList == nil {
			return true
		}
		status, ok := statusList[i]
		if !ok || status == common.ChannelStatusEnabled {
			return true
		}
	}
	return false
}

func UpdateChannelStatus(channelId int, usingKey string, status int, reason string) bool {
	if common.MemoryCacheEnabled {
		channelStatusLock.Lock()
		defer channelStatusLock.Unlock()
	}

	// ChannelInfo stores both multi-key status and the polling cursor. Hold the
	// same per-channel lock from the first read through persistence so neither
	// writer can save a stale JSON snapshot over the other.
	pollingLock := GetChannelPollingLock(channelId)
	pollingLock.Lock()
	defer pollingLock.Unlock()

	if common.MemoryCacheEnabled {
		channelCache, _ := CacheGetChannel(channelId)
		if channelCache == nil {
			return false
		}
		if channelCache.ChannelInfo.IsMultiKey {
			beforeStatus := channelCache.Status
			// 如果是多Key模式，更新缓存中的状态
			handlerMultiKeyUpdate(channelCache, usingKey, status, reason)
			if beforeStatus != channelCache.Status {
				CacheUpdateChannelStatus(channelId, channelCache.Status)
			}
			//CacheUpdateChannel(channelCache)
			//return true
		} else {
			// 如果缓存渠道存在，且状态已是目标状态，直接返回
			if channelCache.Status == status {
				return false
			}
			CacheUpdateChannelStatus(channelId, status)
		}
	}

	shouldUpdateAbilities := false
	defer func() {
		if shouldUpdateAbilities {
			err := UpdateAbilityStatus(channelId, status == common.ChannelStatusEnabled)
			if err != nil {
				common.SysLog(fmt.Sprintf("failed to update ability status: channel_id=%d, error=%v", channelId, err))
			}
		}
	}()
	channel, err := GetChannelById(channelId, true)
	if err != nil {
		return false
	} else {
		if channel.Status == status {
			return false
		}

		if channel.ChannelInfo.IsMultiKey {
			beforeStatus := channel.Status
			handlerMultiKeyUpdate(channel, usingKey, status, reason)
			if beforeStatus != channel.Status {
				shouldUpdateAbilities = true
			}
		} else {
			info := channel.GetOtherInfo()
			info["status_reason"] = reason
			info["status_time"] = common.GetTimestamp()
			channel.SetOtherInfo(info)
			channel.Status = status
			shouldUpdateAbilities = true
		}
		err = channel.saveStatusState()
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to update channel status: channel_id=%d, status=%d, error=%v", channel.Id, status, err))
			return false
		}
		if err := ClearChannelRecoveryState(context.Background(), channel.Id); err != nil {
			common.SysError(fmt.Sprintf("failed to clear channel recovery state: channel_id=%d, error=%v", channel.Id, err))
		}
	}
	return true
}

func EnableChannelByTag(tag string) error {
	err := DB.Model(&Channel{}).Where("tag = ?", tag).Update("status", common.ChannelStatusEnabled).Error
	if err != nil {
		return err
	}
	err = UpdateAbilityStatusByTag(tag, true)
	return err
}

func DisableChannelByTag(tag string) error {
	err := DB.Model(&Channel{}).Where("tag = ?", tag).Update("status", common.ChannelStatusManuallyDisabled).Error
	if err != nil {
		return err
	}
	err = UpdateAbilityStatusByTag(tag, false)
	return err
}

func EditChannelByTag(tag string, newTag *string, modelMapping *string, models *string, group *string, priority *int64, weight *uint, paramOverride *string, headerOverride *string) error {
	updateData := Channel{}
	shouldReCreateAbilities := false
	updatedTag := tag
	// 如果 newTag 不为空且不等于 tag，则更新 tag
	if newTag != nil && *newTag != tag {
		updateData.Tag = newTag
		updatedTag = *newTag
	}
	if modelMapping != nil {
		updateData.ModelMapping = modelMapping
	}
	if models != nil && *models != "" {
		shouldReCreateAbilities = true
		updateData.Models = *models
	}
	if group != nil && *group != "" {
		shouldReCreateAbilities = true
		updateData.Group = *group
	}
	if priority != nil {
		updateData.Priority = priority
	}
	if weight != nil {
		updateData.Weight = weight
	}
	if paramOverride != nil {
		updateData.ParamOverride = paramOverride
	}
	if headerOverride != nil {
		updateData.HeaderOverride = headerOverride
	}

	err := DB.Model(&Channel{}).Where("tag = ?", tag).Updates(updateData).Error
	if err != nil {
		return err
	}
	if shouldReCreateAbilities {
		channels, err := GetChannelsByTag(updatedTag, false, false)
		if err == nil {
			for _, channel := range channels {
				err = channel.UpdateAbilities(nil)
				if err != nil {
					common.SysLog(fmt.Sprintf("failed to update abilities: channel_id=%d, tag=%s, error=%v", channel.Id, channel.GetTag(), err))
				}
			}
		}
	} else {
		err := UpdateAbilityByTag(tag, newTag, priority, weight)
		if err != nil {
			return err
		}
	}
	return nil
}

func UpdateChannelUsedQuota(id int, quota int) {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeChannelUsedQuota, id, quota)
		return
	}
	updateChannelUsedQuota(id, quota)
}

func updateChannelUsedQuota(id int, quota int) {
	err := DB.Model(&Channel{}).Where("id = ?", id).Update("used_quota", gorm.Expr("used_quota + ?", quota)).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to update channel used quota: channel_id=%d, delta_quota=%d, error=%v", id, quota, err))
	}
}

func DeleteChannelByStatus(status int64) (int64, error) {
	result := DB.Where("status = ?", status).Delete(&Channel{})
	return result.RowsAffected, result.Error
}

func DeleteDisabledChannel() (int64, error) {
	result := DB.Where("status = ? or status = ?", common.ChannelStatusAutoDisabled, common.ChannelStatusManuallyDisabled).Delete(&Channel{})
	return result.RowsAffected, result.Error
}

func GetPaginatedTags(offset int, limit int) ([]*string, error) {
	return GetPaginatedChannelTags(DB.Model(&Channel{}), offset, limit)
}

func GetPaginatedChannelTags(query *gorm.DB, offset int, limit int) ([]*string, error) {
	var tags []*string
	err := query.
		Select("DISTINCT tag").
		Where("tag is not null AND tag != ''").
		Order(clause.OrderByColumn{Column: clause.Column{Name: "tag"}}).
		Offset(offset).
		Limit(limit).
		Find(&tags).Error
	return tags, err
}

func SearchTags(keyword string, group string, model string, idSort bool) ([]*string, error) {
	var tags []*string
	modelsCol := "`models`"

	// 如果是 PostgreSQL，使用双引号
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		modelsCol = `"models"`
	}

	baseURLCol := "`base_url`"
	// 如果是 PostgreSQL，使用双引号
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		baseURLCol = `"base_url"`
	}

	order := "priority desc"
	if idSort {
		order = "id desc"
	}

	// 构造基础查询
	baseQuery := DB.Model(&Channel{}).Omit("key")

	// 构造WHERE子句
	likeOp := mainLikeOp()
	whereClause := fmt.Sprintf("(id = ? OR name %s ? OR %s = ? OR %s %s ? OR %s %s ?) AND %s %s ?",
		likeOp, commonKeyCol, baseURLCol, likeOp, commonGroupCol, likeOp, modelsCol, likeOp)
	args := []any{common.String2Int(keyword), "%" + keyword + "%", keyword, "%" + keyword + "%", "%" + keyword + "%", "%" + model + "%"}
	baseQuery = ApplyChannelGroupFilter(baseQuery.Where(whereClause, args...), group)

	subQuery := baseQuery.
		Select("tag").
		Where("tag != ''").
		Order(order)

	err := DB.Table("(?) as sub", subQuery).
		Select("DISTINCT tag").
		Find(&tags).Error

	if err != nil {
		return nil, err
	}

	return tags, nil
}

func (channel *Channel) ValidateSettings() error {
	channelParams := &dto.ChannelSettings{}
	if channel.Setting != nil && *channel.Setting != "" {
		err := common.Unmarshal([]byte(*channel.Setting), channelParams)
		if err != nil {
			return err
		}
	}
	if _, err := common.ParseProxyURLStrict(channelParams.Proxy); err != nil {
		return fmt.Errorf("invalid channel proxy: %w", err)
	}
	if err := channelParams.ValidateHTTPTransport(); err != nil {
		return err
	}
	channelOtherSettings := &dto.ChannelOtherSettings{}
	if channel.OtherSettings != "" {
		err := common.UnmarshalJsonStr(channel.OtherSettings, channelOtherSettings)
		if err != nil {
			return err
		}
	}
	if err := channelOtherSettings.ValidateToolLossPolicy(); err != nil {
		return err
	}
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		if channelOtherSettings.AdvancedCustom == nil {
			return fmt.Errorf("advanced_custom is required")
		}
	}
	if channelOtherSettings.AdvancedCustom != nil {
		if err := channelOtherSettings.AdvancedCustom.Validate(); err != nil {
			return err
		}
	}
	if channel.Type == constant.ChannelTypeAdvancedCustom && channelOtherSettings.UpstreamModelUpdateCheckEnabled {
		if _, ok := channelOtherSettings.AdvancedCustom.ModelListRoute(); !ok {
			return fmt.Errorf("advanced custom channels require a %s route when upstream model update checks are enabled", dto.AdvancedCustomModelListPath)
		}
	}
	return nil
}

func (channel *Channel) GetSetting() dto.ChannelSettings {
	setting := dto.ChannelSettings{}
	if channel.Setting != nil && *channel.Setting != "" {
		err := common.Unmarshal([]byte(*channel.Setting), &setting)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to unmarshal setting: channel_id=%d, error=%v", channel.Id, err))
			channel.Setting = nil // 清空设置以避免后续错误
			_ = channel.Save()    // 保存修改
		}
	}
	return setting
}

func (channel *Channel) SetSetting(setting dto.ChannelSettings) {
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to marshal setting: channel_id=%d, error=%v", channel.Id, err))
		return
	}
	channel.Setting = common.GetPointer[string](string(settingBytes))
}

func (channel *Channel) GetOtherSettings() dto.ChannelOtherSettings {
	setting := dto.ChannelOtherSettings{}
	if channel.OtherSettings != "" {
		err := common.UnmarshalJsonStr(channel.OtherSettings, &setting)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to unmarshal setting: channel_id=%d, error=%v", channel.Id, err))
			channel.OtherSettings = "{}" // 清空设置以避免后续错误
			_ = channel.Save()           // 保存修改
		}
	}
	return setting
}

func (channel *Channel) SetOtherSettings(setting dto.ChannelOtherSettings) {
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to marshal setting: channel_id=%d, error=%v", channel.Id, err))
		return
	}
	channel.OtherSettings = string(settingBytes)
}

func (channel *Channel) GetParamOverride() map[string]any {
	paramOverride := make(map[string]any)
	if channel.ParamOverride != nil && *channel.ParamOverride != "" {
		err := common.Unmarshal([]byte(*channel.ParamOverride), &paramOverride)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to unmarshal param override: channel_id=%d, error=%v", channel.Id, err))
		}
	}
	return paramOverride
}

func (channel *Channel) GetHeaderOverride() map[string]any {
	headerOverride := make(map[string]any)
	if channel.HeaderOverride != nil && *channel.HeaderOverride != "" {
		err := common.Unmarshal([]byte(*channel.HeaderOverride), &headerOverride)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to unmarshal header override: channel_id=%d, error=%v", channel.Id, err))
		}
	}
	return headerOverride
}

func GetChannelsByIds(ids []int) ([]*Channel, error) {
	var channels []*Channel
	err := DB.Where("id in (?)", ids).Find(&channels).Error
	return channels, err
}

func BatchSetChannelTag(ids []int, tag *string) error {
	// 开启事务
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	// 更新标签
	err := tx.Model(&Channel{}).Where("id in (?)", ids).Update("tag", tag).Error
	if err != nil {
		tx.Rollback()
		return err
	}

	// update ability status
	channels, err := GetChannelsByIds(ids)
	if err != nil {
		tx.Rollback()
		return err
	}

	for _, channel := range channels {
		err = channel.UpdateAbilities(tx)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	// 提交事务
	return tx.Commit().Error
}

// CountAllChannels returns total channels in DB
func CountAllChannels() (int64, error) {
	var total int64
	err := DB.Model(&Channel{}).Count(&total).Error
	return total, err
}

// CountAllTags returns number of non-empty distinct tags
func CountAllTags() (int64, error) {
	return CountChannelTags(DB.Model(&Channel{}))
}

func CountChannelTags(query *gorm.DB) (int64, error) {
	var total int64
	err := query.Where("tag is not null AND tag != ''").Distinct("tag").Count(&total).Error
	return total, err
}

// Get channels of specified type with pagination
func GetChannelsByType(startIdx int, num int, idSort bool, channelType int) ([]*Channel, error) {
	var channels []*Channel
	order := "priority desc"
	if idSort {
		order = "id desc"
	}
	err := DB.Where("type = ?", channelType).Order(order).Limit(num).Offset(startIdx).Omit("key").Find(&channels).Error
	return channels, err
}

// Count channels of specific type
func CountChannelsByType(channelType int) (int64, error) {
	var count int64
	err := DB.Model(&Channel{}).Where("type = ?", channelType).Count(&count).Error
	return count, err
}

// Return map[type]count for all channels
func CountChannelsGroupByType() (map[int64]int64, error) {
	type result struct {
		Type  int64 `gorm:"column:type"`
		Count int64 `gorm:"column:count"`
	}
	var results []result
	err := DB.Model(&Channel{}).Select("type, count(*) as count").Group("type").Find(&results).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[int64]int64)
	for _, r := range results {
		counts[r.Type] = r.Count
	}
	return counts, nil
}

// GetAllChannelModels returns distinct model names configured across all channels (both enabled and disabled) and abilities
func GetAllChannelModels() []string {
	channelModelSet := make(map[string]struct{})
	var channelModelsList []string
	_ = DB.Model(&Channel{}).Pluck("models", &channelModelsList).Error
	for _, mList := range channelModelsList {
		for _, m := range strings.Split(mList, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				channelModelSet[m] = struct{}{}
			}
		}
	}
	var abilityModels []string
	_ = DB.Table("abilities").Distinct("model").Pluck("model", &abilityModels).Error
	for _, m := range abilityModels {
		m = strings.TrimSpace(m)
		if m != "" {
			channelModelSet[m] = struct{}{}
		}
	}
	result := make([]string, 0, len(channelModelSet))
	for m := range channelModelSet {
		result = append(result, m)
	}
	sort.Strings(result)
	return result
}

// GetGroupBasePriority finds a reference priority for the group from abilities or channels.
// It safely handles reserved keyword quoting for SQL dialects (PostgreSQL / MySQL) via commonGroupCol and ApplyChannelGroupFilter.
func GetGroupBasePriority(group string) (*int64, error) {
	firstGroup := strings.TrimSpace(strings.Split(group, ",")[0])
	if firstGroup == "" {
		return nil, nil
	}
	if commonGroupCol == "" {
		initCol()
	}
	var ability Ability
	err := DB.Where(commonGroupCol+" = ?", firstGroup).Order("priority ASC").First(&ability).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil && ability.Priority != nil {
		return ability.Priority, nil
	}

	var ch Channel
	err = ApplyChannelGroupFilter(DB.Model(&Channel{}), firstGroup).Order("priority ASC").First(&ch).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil && ch.Priority != nil {
		return ch.Priority, nil
	}
	return nil, nil
}

type ChannelApiAddress struct {
	BaseURL      string `json:"base_url" gorm:"column:base_url"`
	UsedQuota    int64  `json:"used_quota" gorm:"column:used_quota"`
	PeriodQuota  int64  `json:"period_quota" gorm:"column:period_quota"`
	ChannelCount int    `json:"channel_count" gorm:"column:channel_count"`
}

var (
	channelApiAddrCacheMu sync.RWMutex
	channelApiAddrCache   = make(map[string]channelApiAddrCacheEntry)
)

type channelApiAddrCacheEntry struct {
	items []*ChannelApiAddress
	exp   time.Time
}

func channelSupportsModel(channelModels, modelName string) bool {
	if modelName == "" {
		return true
	}
	for _, m := range strings.Split(channelModels, ",") {
		m = strings.TrimSpace(m)
		if m == modelName || m == "*" {
			return true
		}
	}
	return false
}

func GetChannelApiAddresses(group string, extra ...interface{}) ([]*ChannelApiAddress, error) {
	var modelName string
	var startTimestamp, endTimestamp int64

	for _, arg := range extra {
		switch v := arg.(type) {
		case string:
			if modelName == "" {
				modelName = strings.TrimSpace(v)
			}
		case int64:
			if startTimestamp == 0 {
				startTimestamp = v
			} else if endTimestamp == 0 {
				endTimestamp = v
			}
		case int:
			if startTimestamp == 0 {
				startTimestamp = int64(v)
			} else if endTimestamp == 0 {
				endTimestamp = int64(v)
			}
		}
	}
	if startTimestamp == 0 && endTimestamp == 0 {
		startTimestamp = shanghaiTodayStartUnix()
	}

	cacheKey := fmt.Sprintf("%s:%s:%d:%d", group, modelName, startTimestamp, endTimestamp)
	now := time.Now()
	channelApiAddrCacheMu.RLock()
	if entry, found := channelApiAddrCache[cacheKey]; found && now.Before(entry.exp) {
		channelApiAddrCacheMu.RUnlock()
		return entry.items, nil
	}
	channelApiAddrCacheMu.RUnlock()

	// 1. 优先从 channels 表获取基准数据（耗时 < 1ms，确保任何异常下 100% 都有可用 API 地址）
	type channelItem struct {
		Id        int     `gorm:"column:id"`
		BaseURL   *string `gorm:"column:base_url"`
		UsedQuota int64   `gorm:"column:used_quota"`
		Models    string  `gorm:"column:models"`
	}
	var channels []channelItem
	chQuery := DB.Model(&Channel{}).
		Select("id, base_url, used_quota, models").
		Where("base_url IS NOT NULL AND base_url != ''")
	if strings.TrimSpace(group) != "" {
		chQuery = ApplyChannelGroupFilter(chQuery, strings.TrimSpace(group))
	}
	if err := chQuery.Find(&channels).Error; err != nil {
		return nil, err
	}

	// 如果指定了 modelName，优先按渠道支持的模型进行联动过滤
	if modelName != "" {
		var matched []channelItem
		for _, ch := range channels {
			if channelSupportsModel(ch.Models, modelName) {
				matched = append(matched, ch)
			}
		}
		if len(matched) > 0 {
			channels = matched
		}
	}

	addrMap := make(map[string]*ChannelApiAddress)
	channelIDs := make([]int, 0, len(channels))
	for _, ch := range channels {
		if ch.BaseURL == nil || strings.TrimSpace(*ch.BaseURL) == "" {
			continue
		}
		url := strings.TrimSpace(*ch.BaseURL)
		item, exists := addrMap[url]
		if !exists {
			item = &ChannelApiAddress{BaseURL: url}
			addrMap[url] = item
		}
		item.ChannelCount++
		item.UsedQuota += ch.UsedQuota
		channelIDs = append(channelIDs, ch.Id)
	}

	if len(addrMap) == 0 {
		return []*ChannelApiAddress{}, nil
	}

	// 2. 智能区间消耗判定：
	// 如果是大时间跨度（> 24 小时，或历史大周期）：
	// 渠道本身的 used_quota（总消耗额度）就是长周期下最精准、最有价值的排序指标！
	// 此时绝对不能扫描几千万行 logs 表，直接短路使用 UsedQuota，0.5 毫秒瞬间返回！
	// 仅当时间跨度 <= 24 小时（例如查“今天”或小时段）时，且存在 target channelIDs，才从 logs 表聚合近期活跃消耗，并附带 1.5 秒安全超时降级保护。
	isShortTimeRange := false
	if startTimestamp > 0 && endTimestamp > 0 {
		isShortTimeRange = (endTimestamp - startTimestamp) <= 86400
	} else if startTimestamp >= shanghaiTodayStartUnix() {
		isShortTimeRange = true
	}

	channelQuotaMap := make(map[int]int64)
	if isShortTimeRange && LOG_DB != nil && len(channelIDs) > 0 {
		type channelQuotaRow struct {
			ChannelID   int   `gorm:"column:channel_id"`
			PeriodQuota int64 `gorm:"column:period_quota"`
		}
		var rows []channelQuotaRow
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()

		logTx := LOG_DB.WithContext(ctx).Table("logs").
			Select("channel_id, COALESCE(SUM(quota), 0) AS period_quota").
			Where("channel_id IN (?)", channelIDs).
			Where("type = ?", LogTypeConsume)
		if modelName != "" {
			logTx = logTx.Where("model_name = ?", modelName)
		}
		if startTimestamp > 0 {
			logTx = logTx.Where("created_at >= ?", startTimestamp)
		}
		if endTimestamp > 0 {
			logTx = logTx.Where("created_at <= ?", endTimestamp)
		}
		if err := logTx.Group("channel_id").Having("SUM(quota) > 0").Find(&rows).Error; err == nil {
			for _, r := range rows {
				channelQuotaMap[r.ChannelID] = r.PeriodQuota
			}
		}
	}

	for _, ch := range channels {
		if ch.BaseURL == nil {
			continue
		}
		url := strings.TrimSpace(*ch.BaseURL)
		item, exists := addrMap[url]
		if !exists {
			continue
		}
		if pq, ok := channelQuotaMap[ch.Id]; ok {
			item.PeriodQuota += pq
		}
	}

	items := make([]*ChannelApiAddress, 0, len(addrMap))
	for _, item := range addrMap {
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].PeriodQuota != items[j].PeriodQuota {
			return items[i].PeriodQuota > items[j].PeriodQuota
		}
		if items[i].UsedQuota != items[j].UsedQuota {
			return items[i].UsedQuota > items[j].UsedQuota
		}
		return items[i].BaseURL < items[j].BaseURL
	})

	cacheTTL := 60 * time.Second
	if !isShortTimeRange {
		cacheTTL = 5 * time.Minute
	}

	channelApiAddrCacheMu.Lock()
	channelApiAddrCache[cacheKey] = channelApiAddrCacheEntry{
		items: items,
		exp:   now.Add(cacheTTL),
	}
	channelApiAddrCacheMu.Unlock()

	return items, nil
}
