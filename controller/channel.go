package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/ollama"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type OpenAIModel struct {
	ID         string         `json:"id"`
	Object     string         `json:"object"`
	Created    int64          `json:"created"`
	OwnedBy    string         `json:"owned_by"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Permission []struct {
		ID                 string `json:"id"`
		Object             string `json:"object"`
		Created            int64  `json:"created"`
		AllowCreateEngine  bool   `json:"allow_create_engine"`
		AllowSampling      bool   `json:"allow_sampling"`
		AllowLogprobs      bool   `json:"allow_logprobs"`
		AllowSearchIndices bool   `json:"allow_search_indices"`
		AllowView          bool   `json:"allow_view"`
		AllowFineTuning    bool   `json:"allow_fine_tuning"`
		Organization       string `json:"organization"`
		Group              string `json:"group"`
		IsBlocking         bool   `json:"is_blocking"`
	} `json:"permission"`
	Root   string `json:"root"`
	Parent string `json:"parent"`
}

type OpenAIModelsResponse struct {
	Data    []OpenAIModel `json:"data"`
	Success bool          `json:"success"`
}

func parseStatusFilter(statusParam string) int {
	switch strings.ToLower(statusParam) {
	case "enabled", "1":
		return common.ChannelStatusEnabled
	case "disabled", "0":
		return 0
	default:
		return -1
	}
}

func clearChannelInfo(channel *model.Channel) {
	if channel.ChannelInfo.IsMultiKey {
		channel.ChannelInfo.MultiKeyDisabledReason = nil
		channel.ChannelInfo.MultiKeyDisabledTime = nil
	}
}

func applyCachedChannelSiteType(channel *model.Channel) {
	if channel == nil {
		return
	}
	if channel.SiteType != nil && (*channel.SiteType == channelPlatformNewAPI || *channel.SiteType == channelPlatformSub2API || *channel.SiteType == channelPlatformUnknown) {
		return
	}
	// Read legacy cached metadata written by earlier builds and expose it in the
	// new independent field. It will be migrated to the column on the next scan.
	info := channel.GetOtherInfo()
	if siteType, ok := info["site_type"].(string); ok && (siteType == channelPlatformNewAPI || siteType == channelPlatformSub2API || siteType == channelPlatformUnknown) {
		channel.SiteType = &siteType
	}
}

func applyChannelStatusFilter(query *gorm.DB, statusFilter int) *gorm.DB {
	if statusFilter == common.ChannelStatusEnabled {
		return query.Where("status = ?", common.ChannelStatusEnabled)
	}
	if statusFilter == 0 {
		return query.Where("status != ?", common.ChannelStatusEnabled)
	}
	return query
}

func buildChannelListQuery(group string, statusFilter int, typeFilter int) *gorm.DB {
	query := model.DB.Model(&model.Channel{})
	query = model.ApplyChannelGroupFilter(query, group)
	query = applyChannelStatusFilter(query, statusFilter)
	if typeFilter >= 0 {
		query = query.Where("type = ?", typeFilter)
	}
	return query
}

func GetChannelOps(c *gin.Context) {
	common.ApiSuccess(c, gin.H{
		"retry_times": common.RetryTimes,
	})
}

func GetChannelDefaultBaseURLs(c *gin.Context) {
	baseURLs := make(map[int]string)
	for channelType, baseURL := range constant.ChannelBaseURLs {
		if baseURL != "" {
			baseURLs[channelType] = baseURL
		}
	}
	common.ApiSuccess(c, baseURLs)
}

var channelModelFamilyTerms = map[string][]string{
	"OpenAI":    {"gpt", "chatgpt", "o1-", "o3-", "o4-", "dall-e", "sora", "codex"},
	"Claude":    {"claude"},
	"Gemini":    {"gemini", "gemma", "imagen", "veo", "nano-banana", "nano banana"},
	"Grok":      {"grok"},
	"DeepSeek":  {"deepseek"},
	"Qwen":      {"qwen", "qwq"},
	"ByteDance": {"doubao", "seedream", "seedance"},
	"Zhipu":     {"glm", "cogview", "cogvideo"},
	"Kimi":      {"kimi", "moonshot"},
	"MiniMax":   {"minimax"},
	"Mistral":   {"mistral", "codestral", "ministral", "pixtral"},
	"Meta":      {"llama", "meta-llama"},
	"Cohere":    {"command", "c4ai"},
	"Baidu":     {"ernie", "wenxin"},
	"Hunyuan":   {"hunyuan"},
}

func getChannelModelFamily(modelName string) string {
	name := strings.ToLower(strings.TrimSpace(modelName))
	if name == "o1" || name == "o3" || name == "o4" {
		return "OpenAI"
	}
	for family, terms := range channelModelFamilyTerms {
		for _, term := range terms {
			if strings.HasPrefix(name, term) {
				return family
			}
		}
	}
	return "Other"
}

func getChannelModelFamilies(models string) map[string]struct{} {
	families := make(map[string]struct{})
	for _, modelName := range strings.Split(strings.Trim(models, ","), ",") {
		if strings.TrimSpace(modelName) != "" {
			families[getChannelModelFamily(modelName)] = struct{}{}
		}
	}
	return families
}

func applyChannelModelFamilyFilter(query *gorm.DB, family string) *gorm.DB {
	if family == "" || family == "all" {
		return query
	}
	models := make([]string, 0)
	for _, modelName := range model.GetEnabledModels() {
		if getChannelModelFamily(modelName) == family {
			models = append(models, modelName)
		}
	}
	if len(models) == 0 {
		return query.Where("1 = 0")
	}
	subQuery := model.DB.Model(&model.Ability{}).
		Select("channel_id").
		Where("enabled = ? AND model IN ?", true, models)
	return query.Where("id IN (?)", subQuery)
}

const (
	channelModelTypeText       = "Text"
	channelModelTypeMultimodal = "Multimodal"
	channelModelTypeImage      = "Image"
	channelModelTypeVideo      = "Video"

	channelBillingTypePerRequest = "PerRequest"
	channelBillingTypePerToken   = "PerToken"
)

func getChannelModelType(modelName string) string {
	name := strings.ToLower(strings.TrimSpace(modelName))
	videoTerms := []string{"video", "veo", "sora", "seedance", "kling", "hailuo", "wan2", "cogvideo"}
	for _, term := range videoTerms {
		if strings.Contains(name, term) {
			return channelModelTypeVideo
		}
	}
	imageTerms := []string{"image", "imagen", "dall-e", "seedream", "flux", "banana", "cogview", "midjourney", "mj_", "sd-", "sd_", "stable-diffusion"}
	for _, term := range imageTerms {
		if strings.Contains(name, term) {
			return channelModelTypeImage
		}
	}
	multimodalTerms := []string{
		"vision", "multimodal", "vl", "omni", "4o",
		"gemini", "claude-3", "gpt-4-turbo",
		"glm-4v", "internvl", "minicpm-v",
	}
	for _, term := range multimodalTerms {
		if strings.Contains(name, term) {
			return channelModelTypeMultimodal
		}
	}
	return channelModelTypeText
}

func getChannelPricingMap() map[string]model.Pricing {
	pricing := make(map[string]model.Pricing)
	for _, item := range model.GetPricing() {
		pricing[item.ModelName] = item
	}
	return pricing
}

func getChannelModelTypes(models string) map[string]struct{} {
	types := make(map[string]struct{})
	for _, modelName := range strings.Split(strings.Trim(models, ","), ",") {
		modelName = strings.TrimSpace(modelName)
		if modelName != "" {
			types[getChannelModelType(modelName)] = struct{}{}
		}
	}
	return types
}

func applyChannelModelTypeFilter(query *gorm.DB, modelType string) *gorm.DB {
	if modelType == "" || modelType == "all" {
		return query
	}
	models := make([]string, 0)
	for _, modelName := range model.GetEnabledModels() {
		if getChannelModelType(modelName) == modelType {
			models = append(models, modelName)
		}
	}
	if len(models) == 0 {
		return query.Where("1 = 0")
	}
	subQuery := model.DB.Model(&model.Ability{}).
		Select("channel_id").
		Where("enabled = ? AND model IN ?", true, models)
	return query.Where("id IN (?)", subQuery)
}

func getChannelBillingType(modelName string, pricing map[string]model.Pricing) string {
	if item, ok := pricing[modelName]; ok && item.QuotaType == 1 {
		return channelBillingTypePerRequest
	}
	return channelBillingTypePerToken
}

func getChannelBillingTypes(models string, pricing map[string]model.Pricing) map[string]struct{} {
	types := make(map[string]struct{})
	for _, modelName := range strings.Split(strings.Trim(models, ","), ",") {
		modelName = strings.TrimSpace(modelName)
		if modelName != "" {
			types[getChannelBillingType(modelName, pricing)] = struct{}{}
		}
	}
	return types
}

func applyChannelBillingTypeFilter(query *gorm.DB, billingType string) *gorm.DB {
	if billingType == "" || billingType == "all" {
		return query
	}
	pricing := getChannelPricingMap()
	models := make([]string, 0)
	for _, modelName := range model.GetEnabledModels() {
		if getChannelBillingType(modelName, pricing) == billingType {
			models = append(models, modelName)
		}
	}
	if len(models) == 0 {
		return query.Where("1 = 0")
	}
	subQuery := model.DB.Model(&model.Ability{}).
		Select("channel_id").
		Where("enabled = ? AND model IN ?", true, models)
	return query.Where("id IN (?)", subQuery)
}

func getChannelModelFamilyCounts(query *gorm.DB) (map[string]int64, error) {
	var rows []struct {
		Models string
	}
	if err := query.Select("models").Find(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int64)
	counts["all"] = int64(len(rows))
	for _, row := range rows {
		for family := range getChannelModelFamilies(row.Models) {
			counts[family]++
		}
	}
	return counts, nil
}

func getChannelModelTypeCounts(query *gorm.DB) (map[string]int64, error) {
	var rows []struct {
		Models string
	}
	if err := query.Select("models").Find(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int64)
	counts["all"] = int64(len(rows))
	for _, row := range rows {
		for modelType := range getChannelModelTypes(row.Models) {
			counts[modelType]++
		}
	}
	return counts, nil
}

func getChannelBillingTypeCounts(query *gorm.DB) (map[string]int64, error) {
	var rows []struct {
		Models string
	}
	if err := query.Select("models").Find(&rows).Error; err != nil {
		return nil, err
	}
	pricing := getChannelPricingMap()
	counts := make(map[string]int64)
	counts["all"] = int64(len(rows))
	for _, row := range rows {
		for billingType := range getChannelBillingTypes(row.Models, pricing) {
			counts[billingType]++
		}
	}
	return counts, nil
}

func countChannelModelFamilies(channels []*model.Channel) map[string]int64 {
	counts := make(map[string]int64)
	counts["all"] = int64(len(channels))
	for _, channel := range channels {
		for family := range getChannelModelFamilies(channel.Models) {
			counts[family]++
		}
	}
	return counts
}

func countChannelModelTypes(channels []*model.Channel) map[string]int64 {
	counts := make(map[string]int64)
	counts["all"] = int64(len(channels))
	for _, channel := range channels {
		for modelType := range getChannelModelTypes(channel.Models) {
			counts[modelType]++
		}
	}
	return counts
}

func countChannelBillingTypes(channels []*model.Channel) map[string]int64 {
	pricing := getChannelPricingMap()
	counts := make(map[string]int64)
	counts["all"] = int64(len(channels))
	for _, channel := range channels {
		for billingType := range getChannelBillingTypes(channel.Models, pricing) {
			counts[billingType]++
		}
	}
	return counts
}

func filterChannelsByModelFamily(channels []*model.Channel, family string) []*model.Channel {
	if family == "" || family == "all" {
		return channels
	}
	filtered := make([]*model.Channel, 0, len(channels))
	for _, channel := range channels {
		if _, ok := getChannelModelFamilies(channel.Models)[family]; ok {
			filtered = append(filtered, channel)
		}
	}
	return filtered
}

func filterChannelsByModelType(channels []*model.Channel, modelType string) []*model.Channel {
	if modelType == "" || modelType == "all" {
		return channels
	}
	filtered := make([]*model.Channel, 0, len(channels))
	for _, channel := range channels {
		if _, ok := getChannelModelTypes(channel.Models)[modelType]; ok {
			filtered = append(filtered, channel)
		}
	}
	return filtered
}

func filterChannelsByBillingType(channels []*model.Channel, billingType string) []*model.Channel {
	if billingType == "" || billingType == "all" {
		return channels
	}
	pricing := getChannelPricingMap()
	filtered := make([]*model.Channel, 0, len(channels))
	for _, channel := range channels {
		if _, ok := getChannelBillingTypes(channel.Models, pricing)[billingType]; ok {
			filtered = append(filtered, channel)
		}
	}
	return filtered
}

func GetAllChannels(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	channelData := make([]*model.Channel, 0)
	idSort, _ := strconv.ParseBool(c.Query("id_sort"))
	sortOptions := model.NewChannelSortOptions(c.Query("sort_by"), c.Query("sort_order"), idSort, c.Query("group_order"))
	enableTagMode, _ := strconv.ParseBool(c.Query("tag_mode"))
	groupFilter := model.NormalizeChannelGroupFilter(c.Query("group"))
	statusParam := c.Query("status")
	// statusFilter: -1 all, 1 enabled, 0 disabled (include auto & manual)
	statusFilter := parseStatusFilter(statusParam)
	// type filter
	typeStr := c.Query("type")
	typeFilter := -1
	if typeStr != "" {
		if t, err := strconv.Atoi(typeStr); err == nil {
			typeFilter = t
		}
	}
	modelFamily := c.DefaultQuery("model_family", "all")
	modelType := c.DefaultQuery("model_type", "all")
	billingType := c.DefaultQuery("billing_type", "all")
	buildFilteredQuery := func(typeValue int) *gorm.DB {
		query := buildChannelListQuery(groupFilter, statusFilter, typeValue)
		query = applyChannelModelFamilyFilter(query, modelFamily)
		query = applyChannelModelTypeFilter(query, modelType)
		return applyChannelBillingTypeFilter(query, billingType)
	}

	var total int64

	if enableTagMode {
		tags, err := model.GetPaginatedChannelTags(buildFilteredQuery(typeFilter), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
		if err != nil {
			common.SysError("failed to get paginated tags: " + err.Error())
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取标签失败，请稍后重试"})
			return
		}
		total, err = model.CountChannelTags(buildFilteredQuery(typeFilter))
		if err != nil {
			common.SysError("failed to count tags: " + err.Error())
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取标签数量失败，请稍后重试"})
			return
		}
		for _, tag := range tags {
			if tag == nil || *tag == "" {
				continue
			}
			var tagChannels []*model.Channel
			err := sortOptions.Apply(buildFilteredQuery(typeFilter).Where("tag = ?", *tag)).
				Omit("key").
				Find(&tagChannels).Error
			if err != nil {
				common.SysError("failed to get channels by tag: " + err.Error())
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取标签渠道失败，请稍后重试"})
				return
			}
			channelData = append(channelData, tagChannels...)
		}
	} else {
		if err := buildFilteredQuery(typeFilter).Count(&total).Error; err != nil {
			common.SysError("failed to count channels: " + err.Error())
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取渠道数量失败，请稍后重试"})
			return
		}

		err := sortOptions.Apply(buildFilteredQuery(typeFilter)).
			Limit(pageInfo.GetPageSize()).
			Offset(pageInfo.GetStartIdx()).
			Omit("key").
			Find(&channelData).Error
		if err != nil {
			common.SysError("failed to get channels: " + err.Error())
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取渠道列表失败，请稍后重试"})
			return
		}
	}

	for _, datum := range channelData {
		clearChannelInfo(datum)
		applyCachedChannelSiteType(datum)
	}

	typeCountQuery := buildChannelListQuery(groupFilter, statusFilter, -1)
	typeCountQuery = applyChannelModelTypeFilter(typeCountQuery, modelType)
	typeCountQuery = applyChannelBillingTypeFilter(typeCountQuery, billingType)
	var results []struct {
		Type  int64
		Count int64
	}
	if err := typeCountQuery.Select("type, count(*) as count").Group("type").Find(&results).Error; err != nil {
		common.SysError("failed to count channel types: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取渠道类型统计失败，请稍后重试"})
		return
	}
	typeCounts := make(map[int64]int64)
	for _, r := range results {
		typeCounts[r.Type] = r.Count
	}
	familyCountQuery := buildChannelListQuery(groupFilter, statusFilter, -1)
	familyCountQuery = applyChannelModelTypeFilter(familyCountQuery, modelType)
	familyCountQuery = applyChannelBillingTypeFilter(familyCountQuery, billingType)
	familyCounts, err := getChannelModelFamilyCounts(familyCountQuery)
	if err != nil {
		common.SysError("failed to count channel model families: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取模型家族统计失败，请稍后重试"})
		return
	}
	modelTypeCountQuery := buildChannelListQuery(groupFilter, statusFilter, typeFilter)
	modelTypeCountQuery = applyChannelModelFamilyFilter(modelTypeCountQuery, modelFamily)
	modelTypeCountQuery = applyChannelBillingTypeFilter(modelTypeCountQuery, billingType)
	modelTypeCounts, err := getChannelModelTypeCounts(modelTypeCountQuery)
	if err != nil {
		common.SysError("failed to count channel model types: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取模型类型统计失败，请稍后重试"})
		return
	}
	billingTypeCountQuery := buildChannelListQuery(groupFilter, statusFilter, typeFilter)
	billingTypeCountQuery = applyChannelModelFamilyFilter(billingTypeCountQuery, modelFamily)
	billingTypeCountQuery = applyChannelModelTypeFilter(billingTypeCountQuery, modelType)
	billingTypeCounts, err := getChannelBillingTypeCounts(billingTypeCountQuery)
	if err != nil {
		common.SysError("failed to count channel billing types: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取计费类型统计失败，请稍后重试"})
		return
	}
	common.ApiSuccess(c, gin.H{
		"items":               channelData,
		"total":               total,
		"page":                pageInfo.GetPage(),
		"page_size":           pageInfo.GetPageSize(),
		"type_counts":         typeCounts,
		"model_family_counts": familyCounts,
		"model_type_counts":   modelTypeCounts,
		"billing_type_counts": billingTypeCounts,
	})
	return
}

func buildFetchModelsHeaders(channel *model.Channel, key string) (http.Header, error) {
	var headers http.Header
	switch channel.Type {
	case constant.ChannelTypeAnthropic:
		headers = GetClaudeAuthHeader(key)
	default:
		headers = GetAuthHeader(key)
	}

	if err := applyFetchModelsHeaderOverrides(channel, key, headers); err != nil {
		return nil, err
	}
	return headers, nil
}

func applyFetchModelsHeaderOverrides(channel *model.Channel, key string, headers http.Header) error {
	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:          key,
			HeadersOverride: channel.GetHeaderOverride(),
		},
	}
	overrides, err := relaychannel.ResolveHeaderOverride(info, nil)
	if err != nil {
		return err
	}
	for name, value := range overrides {
		headers.Set(name, value)
	}

	return nil
}

func FetchUpstreamModels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	ids, err := fetchChannelUpstreamModelIDs(channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取模型列表失败: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    ids,
	})
}

func FixChannelsAbilities(c *gin.Context) {
	success, fails, err := model.FixAbility()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"success": success,
			"fails":   fails,
		},
	})
}

func SearchChannels(c *gin.Context) {
	keyword := c.Query("keyword")
	group := c.Query("group")
	modelKeyword := c.Query("model")
	statusParam := c.Query("status")
	statusFilter := parseStatusFilter(statusParam)
	idSort, _ := strconv.ParseBool(c.Query("id_sort"))
	sortOptions := model.NewChannelSortOptions(c.Query("sort_by"), c.Query("sort_order"), idSort, c.Query("group_order"))
	enableTagMode, _ := strconv.ParseBool(c.Query("tag_mode"))
	channelData := make([]*model.Channel, 0)
	if enableTagMode {
		tags, err := model.SearchTags(keyword, group, modelKeyword, idSort)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		for _, tag := range tags {
			if tag != nil && *tag != "" {
				var tagChannels []*model.Channel
				err := sortOptions.Apply(buildChannelListQuery(group, -1, -1).Where("tag = ?", *tag)).
					Omit("key").
					Find(&tagChannels).Error
				if err != nil {
					c.JSON(http.StatusOK, gin.H{
						"success": false,
						"message": err.Error(),
					})
					return
				}
				channelData = append(channelData, tagChannels...)
			}
		}
	} else {
		channels, err := model.SearchChannels(keyword, group, modelKeyword, idSort, sortOptions)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		channelData = channels
	}

	if statusFilter == common.ChannelStatusEnabled || statusFilter == 0 {
		filtered := make([]*model.Channel, 0, len(channelData))
		for _, ch := range channelData {
			if statusFilter == common.ChannelStatusEnabled && ch.Status != common.ChannelStatusEnabled {
				continue
			}
			if statusFilter == 0 && ch.Status == common.ChannelStatusEnabled {
				continue
			}
			filtered = append(filtered, ch)
		}
		channelData = filtered
	}

	modelFamily := c.DefaultQuery("model_family", "all")
	modelType := c.DefaultQuery("model_type", "all")
	billingType := c.DefaultQuery("billing_type", "all")
	modelFamilyCounts := countChannelModelFamilies(
		filterChannelsByBillingType(
			filterChannelsByModelType(channelData, modelType),
			billingType,
		),
	)
	// calculate type counts for search results
	typeCounts := make(map[int64]int64)
	baseChannelsForType := filterChannelsByBillingType(
		filterChannelsByModelType(channelData, modelType),
		billingType,
	)
	for _, channel := range baseChannelsForType {
		typeCounts[int64(channel.Type)]++
	}
	modelTypeCounts := countChannelModelTypes(
		filterChannelsByBillingType(
			filterChannelsByModelFamily(channelData, modelFamily),
			billingType,
		),
	)
	billingTypeCounts := countChannelBillingTypes(
		filterChannelsByModelType(
			filterChannelsByModelFamily(channelData, modelFamily),
			modelType,
		),
	)
	channelData = filterChannelsByModelFamily(channelData, modelFamily)
	channelData = filterChannelsByModelType(channelData, modelType)
	channelData = filterChannelsByBillingType(channelData, billingType)

	typeParam := c.Query("type")
	typeFilter := -1
	if typeParam != "" {
		if tp, err := strconv.Atoi(typeParam); err == nil {
			typeFilter = tp
		}
	}

	if typeFilter >= 0 {
		filtered := make([]*model.Channel, 0, len(channelData))
		for _, ch := range channelData {
			if ch.Type == typeFilter {
				filtered = append(filtered, ch)
			}
		}
		channelData = filtered
	}

	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	total := len(channelData)
	startIdx := min((page-1)*pageSize, total)
	endIdx := min(startIdx+pageSize, total)

	pagedData := channelData[startIdx:endIdx]

	for _, datum := range pagedData {
		clearChannelInfo(datum)
		applyCachedChannelSiteType(datum)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":               pagedData,
			"total":               total,
			"type_counts":         typeCounts,
			"model_family_counts": modelFamilyCounts,
			"model_type_counts":   modelTypeCounts,
			"billing_type_counts": billingTypeCounts,
		},
	})
	return
}

func GetChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.GetChannelById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel != nil {
		clearChannelInfo(channel)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channel,
	})
	return
}

// GetChannelKey 获取渠道密钥（需要通过安全验证中间件）
// 此函数依赖 SecureVerificationRequired 中间件，确保用户已通过安全验证
func GetChannelKey(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelId <= 0 {
		common.ApiErrorMsg(c, "渠道ID格式错误")
		return
	}

	// 获取渠道信息（包含密钥）
	channel, err := model.GetChannelById(channelId, true)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiErrorI18n(c, i18n.MsgChannelNotExists)
		return
	}
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}

	// 记录操作审计日志（高危：查看渠道密钥）
	recordManageAudit(c, "channel.key_view", map[string]any{
		"id":   channelId,
		"name": channel.Name,
	})

	// 返回渠道密钥
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "获取成功",
		"data": map[string]any{
			"key": channel.Key,
		},
	})
}

// validateTwoFactorAuth 统一的2FA验证函数
func validateTwoFactorAuth(twoFA *model.TwoFA, code string) bool {
	// 尝试验证TOTP
	if cleanCode, err := common.ValidateNumericCode(code); err == nil {
		if isValid, _ := twoFA.ValidateTOTPAndUpdateUsage(cleanCode); isValid {
			return true
		}
	}

	// 尝试验证备用码
	if isValid, err := twoFA.ValidateBackupCodeAndUpdateUsage(code); err == nil && isValid {
		return true
	}

	return false
}

const maxChannelRatio = model.MaxChannelRatio

// validateChannel 通用的渠道校验函数
func validateChannel(channel *model.Channel, isAdd bool) error {
	if channel == nil {
		return fmt.Errorf("channel cannot be empty")
	}
	if channel.ChannelRatio != nil &&
		(*channel.ChannelRatio < 0 || math.IsNaN(*channel.ChannelRatio) ||
			math.IsInf(*channel.ChannelRatio, 0) || *channel.ChannelRatio > maxChannelRatio) {
		return fmt.Errorf("channel ratio must be between 0 and %d", maxChannelRatio)
	}
	for field, value := range map[string]float64{
		"input price":  channel.InputPrice,
		"output price": channel.OutputPrice,
	} {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("%s must be a finite non-negative number", field)
		}
	}
	if channel.CostTier < 0 {
		return fmt.Errorf("cost tier must be non-negative")
	}
	if channel.RPMLimit < 0 || channel.RPMLimit > 1000000 {
		return fmt.Errorf("rpm limit must be between 0 and 1000000")
	}
	if channel.Priority != nil && *channel.Priority < 0 {
		return fmt.Errorf("priority must be non-negative")
	}
	if channel.SiteType != nil && *channel.SiteType != "" && *channel.SiteType != channelPlatformNewAPI && *channel.SiteType != channelPlatformSub2API && *channel.SiteType != channelPlatformUnknown {
		return fmt.Errorf("site type must be empty, newapi, sub2api, or unknown")
	}

	// 校验 channel settings
	if err := channel.ValidateSettings(); err != nil {
		return fmt.Errorf("渠道额外设置[channel setting] 格式错误：%s", err.Error())
	}
	if channel.Type == constant.ChannelTypeTaskPlugin {
		pluginKey := strings.TrimSpace(channel.GetSetting().TaskPluginKey)
		if pluginKey == "" {
			return fmt.Errorf("task plugin key is required")
		}
		if len(pluginKey) > 30 {
			return fmt.Errorf("task plugin key must not exceed 30 characters")
		}
		plugin, ok := jsplugin.DefaultRegistry.Get(pluginKey)
		if !ok {
			return fmt.Errorf("task plugin %q is not registered", pluginKey)
		}
		if channel.BaseURL == nil || strings.TrimSpace(*channel.BaseURL) == "" {
			// The plugin default is persisted onto the channel instead of being
			// resolved per request, so the destination host stays an auditable
			// channel property that only an administrator edit can change.
			if plugin.Meta.BaseURL == "" {
				return fmt.Errorf("base URL is required for task plugin channels")
			}
			defaultBaseURL := plugin.Meta.BaseURL
			channel.BaseURL = &defaultBaseURL
		}
	}

	if channel.Type == constant.ChannelTypeNewAPI && strings.TrimSpace(channel.GetBaseURL()) == "" {
		return fmt.Errorf("New API channel base URL cannot be empty")
	}
	if channel.Type == constant.ChannelTypeSub2API && strings.TrimSpace(channel.GetBaseURL()) == "" {
		return fmt.Errorf("Sub2API channel base URL cannot be empty")
	}

	// 如果是添加操作，检查 channel 和 key 是否为空
	if isAdd {
		if channel.Key == "" {
			return fmt.Errorf("channel cannot be empty")
		}

		// 检查模型名称长度是否超过 255
		for _, m := range channel.GetModels() {
			if len(m) > 255 {
				return fmt.Errorf("模型名称过长: %s", m)
			}
		}
	}

	// VertexAI 特殊校验
	if channel.Type == constant.ChannelTypeVertexAi {
		if channel.Other == "" {
			return fmt.Errorf("部署地区不能为空")
		}

		regionMap, err := common.StrToMap(channel.Other)
		if err != nil {
			return fmt.Errorf("部署地区必须是标准的Json格式，例如{\"default\": \"us-central1\", \"region2\": \"us-east1\"}")
		}

		if regionMap["default"] == nil {
			return fmt.Errorf("部署地区必须包含default字段")
		}
	}

	// Codex OAuth key validation (optional, only when JSON object is provided)
	if channel.Type == constant.ChannelTypeCodex {
		trimmedKey := strings.TrimSpace(channel.Key)
		if isAdd || trimmedKey != "" {
			if !strings.HasPrefix(trimmedKey, "{") {
				return fmt.Errorf("Codex key must be a valid JSON object")
			}
			var keyMap map[string]any
			if err := common.Unmarshal([]byte(trimmedKey), &keyMap); err != nil {
				return fmt.Errorf("Codex key must be a valid JSON object")
			}
			if v, ok := keyMap["access_token"]; !ok || v == nil || strings.TrimSpace(fmt.Sprintf("%v", v)) == "" {
				return fmt.Errorf("Codex key JSON must include access_token")
			}
			if v, ok := keyMap["account_id"]; !ok || v == nil || strings.TrimSpace(fmt.Sprintf("%v", v)) == "" {
				return fmt.Errorf("Codex key JSON must include account_id")
			}
		}
	}

	return nil
}

func RefreshCodexChannelCredential(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	oauthKey, ch, err := service.RefreshCodexChannelCredential(ctx, channelId, service.CodexCredentialRefreshOptions{ResetCaches: true})
	if err != nil {
		common.SysError("failed to refresh codex channel credential: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "刷新凭证失败，请稍后重试"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "refreshed",
		"data": gin.H{
			"expires_at":   oauthKey.Expired,
			"last_refresh": oauthKey.LastRefresh,
			"account_id":   oauthKey.AccountID,
			"email":        oauthKey.Email,
			"channel_id":   ch.Id,
			"channel_type": ch.Type,
			"channel_name": ch.Name,
		},
	})
}

type AddChannelRequest struct {
	Mode                      string                `json:"mode"`
	MultiKeyMode              constant.MultiKeyMode `json:"multi_key_mode"`
	BatchAddSetKeyPrefix2Name bool                  `json:"batch_add_set_key_prefix_2_name"`
	Channel                   *model.Channel        `json:"channel"`
	BalanceAccessToken        string                `json:"balance_access_token,omitempty"`
	BalanceUsername           string                `json:"balance_username,omitempty"`
	BalancePassword           string                `json:"balance_password,omitempty"`
	BalanceMode               string                `json:"balance_mode,omitempty"`
}

// buildChannelsForAdd expands a channel into one record per key and group.
// A channel may still be edited with multiple groups for backward compatibility,
// but new channels are stored separately so each group has an independent ID,
// ability set, status, and routing weight.
func buildChannelsForAdd(source *model.Channel, keys []string, setKeyPrefix bool) []model.Channel {
	if source == nil {
		return nil
	}

	groups := source.GetGroups()
	if len(groups) == 0 {
		groups = []string{source.Group}
	}
	// Avoid creating duplicate channels when the form submits a repeated group.
	uniqueGroups := make([]string, 0, len(groups))
	seenGroups := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if _, exists := seenGroups[group]; exists {
			continue
		}
		seenGroups[group] = struct{}{}
		uniqueGroups = append(uniqueGroups, group)
	}
	if len(uniqueGroups) == 0 {
		uniqueGroups = []string{source.Group}
	}

	channels := make([]model.Channel, 0, len(keys)*len(uniqueGroups))
	for _, key := range keys {
		if key == "" {
			continue
		}
		name := source.Name
		if setKeyPrefix && len(keys) > 1 {
			keyPrefix := key
			if len(keyPrefix) > 8 {
				keyPrefix = keyPrefix[:8]
			}
			name = fmt.Sprintf("%s %s", name, keyPrefix)
		}
		for _, group := range uniqueGroups {
			localChannel := *source
			localChannel.Key = key
			localChannel.Name = name
			localChannel.Group = group
			channels = append(channels, localChannel)
		}
	}
	return channels
}

func getVertexArrayKeys(keys string) ([]string, error) {
	if keys == "" {
		return nil, nil
	}
	var keyArray []any
	err := common.Unmarshal([]byte(keys), &keyArray)
	if err != nil {
		return nil, fmt.Errorf("批量添加 Vertex AI 必须使用标准的JsonArray格式，例如[{key1}, {key2}...]，请检查输入: %w", err)
	}
	cleanKeys := make([]string, 0, len(keyArray))
	for _, key := range keyArray {
		var keyStr string
		switch v := key.(type) {
		case string:
			keyStr = strings.TrimSpace(v)
		default:
			bytes, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("Vertex AI key JSON 编码失败: %w", err)
			}
			keyStr = string(bytes)
		}
		if keyStr != "" {
			cleanKeys = append(cleanKeys, keyStr)
		}
	}
	if len(cleanKeys) == 0 {
		return nil, fmt.Errorf("批量添加 Vertex AI 的 keys 不能为空")
	}
	return cleanKeys, nil
}

func AddChannel(c *gin.Context) {
	addChannelRequest := AddChannelRequest{}
	err := c.ShouldBindJSON(&addChannelRequest)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if addChannelRequest.Channel != nil && addChannelRequest.Channel.Type == constant.ChannelTypeTaskPlugin &&
		!authz.Can(c.GetInt("id"), c.GetInt("role"), authz.TaskPluginBind) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "task plugin channels require the task_plugin.bind permission",
		})
		return
	}

	baseURLFromPluginDefault := addChannelRequest.Channel != nil &&
		addChannelRequest.Channel.Type == constant.ChannelTypeTaskPlugin &&
		(addChannelRequest.Channel.BaseURL == nil || strings.TrimSpace(*addChannelRequest.Channel.BaseURL) == "")
	// 使用统一的校验函数
	if err := validateChannel(addChannelRequest.Channel, true); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	addChannelRequest.Channel.CreatedTime = common.GetTimestamp()
	if addChannelRequest.BalanceAccessToken != "" || addChannelRequest.BalanceUsername != "" || addChannelRequest.BalancePassword != "" {
		if err := addChannelRequest.Channel.SetBalanceCredentials(addChannelRequest.BalanceAccessToken, addChannelRequest.BalanceUsername, addChannelRequest.BalancePassword); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if addChannelRequest.BalanceMode == "manual" {
		addChannelRequest.Channel.BalanceMode = "manual"
	} else {
		addChannelRequest.Channel.BalanceMode = "auto"
	}
	keys := make([]string, 0)
	switch addChannelRequest.Mode {
	case "multi_to_single":
		addChannelRequest.Channel.ChannelInfo.IsMultiKey = true
		addChannelRequest.Channel.ChannelInfo.MultiKeyMode = addChannelRequest.MultiKeyMode
		if addChannelRequest.Channel.Type == constant.ChannelTypeVertexAi && addChannelRequest.Channel.GetOtherSettings().VertexKeyType != dto.VertexKeyTypeAPIKey {
			array, err := getVertexArrayKeys(addChannelRequest.Channel.Key)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
			addChannelRequest.Channel.ChannelInfo.MultiKeySize = len(array)
			addChannelRequest.Channel.Key = strings.Join(array, "\n")
		} else {
			cleanKeys := make([]string, 0)
			for key := range strings.SplitSeq(addChannelRequest.Channel.Key, "\n") {
				if key == "" {
					continue
				}
				key = strings.TrimSpace(key)
				cleanKeys = append(cleanKeys, key)
			}
			addChannelRequest.Channel.ChannelInfo.MultiKeySize = len(cleanKeys)
			addChannelRequest.Channel.Key = strings.Join(cleanKeys, "\n")
		}
		keys = []string{addChannelRequest.Channel.Key}
	case "batch":
		if addChannelRequest.Channel.Type == constant.ChannelTypeVertexAi && addChannelRequest.Channel.GetOtherSettings().VertexKeyType != dto.VertexKeyTypeAPIKey {
			// multi json
			keys, err = getVertexArrayKeys(addChannelRequest.Channel.Key)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
		} else {
			keys = strings.Split(addChannelRequest.Channel.Key, "\n")
		}
	case "single":
		keys = []string{addChannelRequest.Channel.Key}
	default:
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "不支持的添加模式",
		})
		return
	}

	channels := buildChannelsForAdd(
		addChannelRequest.Channel,
		keys,
		addChannelRequest.BatchAddSetKeyPrefix2Name,
	)
	for index := range channels {
		requestedPriority := int64(0)
		if channels[index].Priority != nil {
			requestedPriority = *channels[index].Priority
		}
		priority, priorityErr := constrainNewChannelPriority(channels[index].Group, requestedPriority)
		if priorityErr != nil {
			common.ApiError(c, priorityErr)
			return
		}
		channels[index].Priority = &priority
	}
	err = model.BatchInsertChannels(channels)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.RebalanceChannelWeightsForChannel(0); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	createAudit := map[string]any{
		"name":   addChannelRequest.Channel.Name,
		"type":   addChannelRequest.Channel.Type,
		"groups": addChannelRequest.Channel.GetGroups(),
		"count":  len(channels),
	}
	if baseURLFromPluginDefault {
		createAudit["base_url_source"] = "plugin_default"
	}
	recordManageAudit(c, "channel.create", createAudit)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"count": len(channels),
		},
	})
	return
}

func DeleteChannel(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	channelName := ""
	channelProxy := ""
	channelLookupFailed := false
	if existing, err := model.GetChannelById(id, false); err == nil && existing != nil {
		channelName = existing.Name
		channelProxy = existing.GetSetting().Proxy
	} else {
		channelLookupFailed = true
	}
	channel := model.Channel{Id: id}
	err := channel.Delete()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	if channelLookupFailed {
		service.ResetProxyClientCache()
	} else {
		service.InvalidateProxyClient(channelProxy)
	}
	recordManageAudit(c, "channel.delete", map[string]any{
		"id":   id,
		"name": channelName,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func DeleteDisabledChannel(c *gin.Context) {
	rows, err := model.DeleteDisabledChannel()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	if rows > 0 {
		service.ResetProxyClientCache()
	}
	recordManageAudit(c, "channel.delete_disabled", map[string]any{
		"count": rows,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
	return
}

type ChannelTag struct {
	Tag            string  `json:"tag"`
	NewTag         *string `json:"new_tag"`
	Priority       *int64  `json:"priority"`
	Weight         *uint   `json:"weight"`
	ModelMapping   *string `json:"model_mapping"`
	Models         *string `json:"models"`
	Groups         *string `json:"groups"`
	ParamOverride  *string `json:"param_override"`
	HeaderOverride *string `json:"header_override"`
}

func DisableTagChannels(c *gin.Context) {
	channelTag := ChannelTag{}
	err := c.ShouldBindJSON(&channelTag)
	if err != nil || channelTag.Tag == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.DisableChannelByTag(channelTag.Tag)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	recordManageAudit(c, "channel.tag_disable", map[string]any{
		"tag": channelTag.Tag,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func EnableTagChannels(c *gin.Context) {
	channelTag := ChannelTag{}
	err := c.ShouldBindJSON(&channelTag)
	if err != nil || channelTag.Tag == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.EnableChannelByTag(channelTag.Tag)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	recordManageAudit(c, "channel.tag_enable", map[string]any{
		"tag": channelTag.Tag,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func EditTagChannels(c *gin.Context) {
	channelTag := ChannelTag{}
	err := c.ShouldBindJSON(&channelTag)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	if channelTag.Tag == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "tag不能为空",
		})
		return
	}
	if (channelTag.ParamOverride != nil || channelTag.HeaderOverride != nil) &&
		!authz.Can(c.GetInt("id"), c.GetInt("role"), authz.ChannelSensitiveWrite) {
		common.ApiErrorI18n(c, i18n.MsgAuthInsufficientPrivilege)
		return
	}
	if channelTag.ParamOverride != nil {
		trimmed := strings.TrimSpace(*channelTag.ParamOverride)
		if trimmed != "" && !json.Valid([]byte(trimmed)) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "参数覆盖必须是合法的 JSON 格式",
			})
			return
		}
		channelTag.ParamOverride = common.GetPointer[string](trimmed)
	}
	if channelTag.HeaderOverride != nil {
		trimmed := strings.TrimSpace(*channelTag.HeaderOverride)
		if trimmed != "" && !json.Valid([]byte(trimmed)) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "请求头覆盖必须是合法的 JSON 格式",
			})
			return
		}
		channelTag.HeaderOverride = common.GetPointer[string](trimmed)
	}
	// Priority ranges depend on each channel's first group, so do not apply one
	// unbounded bulk value before per-channel clamping below.
	bulkPriority := (*int64)(nil)
	err = model.EditChannelByTag(channelTag.Tag, channelTag.NewTag, channelTag.ModelMapping, channelTag.Models, channelTag.Groups, bulkPriority, channelTag.Weight, channelTag.ParamOverride, channelTag.HeaderOverride)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	updatedTag := channelTag.Tag
	if channelTag.NewTag != nil && *channelTag.NewTag != "" {
		updatedTag = *channelTag.NewTag
	}
	if channelTag.Priority != nil {
		channels, queryErr := model.GetChannelsByTag(updatedTag, false, true)
		if queryErr != nil {
			common.ApiError(c, queryErr)
			return
		}
		for _, taggedChannel := range channels {
			priority, priorityErr := constrainChannelPriority(taggedChannel.Id, taggedChannel.Group, *channelTag.Priority)
			if priorityErr != nil {
				common.ApiError(c, priorityErr)
				return
			}
			if priorityErr = model.DB.Model(&model.Channel{}).Where("id = ?", taggedChannel.Id).Update("priority", priority).Error; priorityErr != nil {
				common.ApiError(c, priorityErr)
				return
			}
			if priorityErr = model.DB.Model(&model.Ability{}).Where("channel_id = ?", taggedChannel.Id).Update("priority", priority).Error; priorityErr != nil {
				common.ApiError(c, priorityErr)
				return
			}
		}
	}
	if channelTag.Weight != nil {
		if err := model.RebalanceChannelWeightsForChannel(0); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	model.InitChannelCache()
	recordManageAudit(c, "channel.tag_edit", map[string]any{
		"tag": channelTag.Tag,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

// getGroupBasePriority finds a reference priority for the group from abilities or channels.
func getGroupBasePriority(group string) (*int64, error) {
	return model.GetGroupBasePriority(group)
}

func isPriorityInGroupRange(groupPriority *int64, priority int64) bool {
	if groupPriority == nil || *groupPriority <= 0 {
		return true
	}
	base := ((*groupPriority - 1) / 10) * 10
	return priority >= base+1 && priority <= base+9
}

// remapPriorityByBase maps an existing priority into the target base's [base+1, base+9] range.
// It preserves the intra-group tier (% 10 offset):
// e.g. 81 (%10=1, tier 1) in a base=40 group becomes 41 (40 + 1).
// 85 (%10=5, tier 5) in a base=40 group becomes 45 (40 + 5).
// 89 (%10=9, tier 9) in a base=40 group becomes 49 (40 + 9).
func remapPriorityByBase(targetBasePriority int64, currentPriority int64) int64 {
	if targetBasePriority <= 0 {
		return currentPriority
	}
	base := ((targetBasePriority - 1) / 10) * 10
	offset := currentPriority % 10
	if offset < 1 || offset > 9 {
		offset = 1
	}
	return base + offset
}

func remapPriorityToGroupRange(targetGroup string, currentPriority int64) (int64, error) {
	groupPriority, err := getGroupBasePriority(targetGroup)
	if err != nil {
		return currentPriority, err
	}
	if groupPriority == nil || *groupPriority <= 0 {
		return currentPriority, nil
	}
	return remapPriorityByBase(*groupPriority, currentPriority), nil
}

// GetChannelGroupPriorities returns a mapping of group name -> base priority
func GetChannelGroupPriorities(c *gin.Context) {
	groups, err := model.GetChannelGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result := make(map[string]int64)
	for _, g := range groups {
		if prio, err := getGroupBasePriority(g); err == nil && prio != nil {
			result[g] = *prio
		}
	}
	common.ApiSuccess(c, result)
}

// constrainChannelPriority keeps a channel inside the reserved nine-value
// range of its target routing group. Channels without abilities are legacy or
// incomplete records; use the target group or ability range rather than accepting
// an unbounded value.
func constrainChannelPriority(channelID int, targetGroup string, requested int64) (int64, error) {
	if targetGroup != "" {
		firstGroup := strings.TrimSpace(strings.Split(targetGroup, ",")[0])
		if firstGroup != "" {
			groupPriority, err := getGroupBasePriority(firstGroup)
			if err != nil {
				return 0, err
			}
			if groupPriority != nil {
				return constrainPriorityToGroupRange(groupPriority, requested), nil
			}
		}
	}
	var ability model.Ability
	err := model.DB.Where("channel_id = ?", channelID).Order("priority ASC").First(&ability).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	if err == nil {
		return constrainPriorityToGroupRange(ability.Priority, requested), nil
	}
	return constrainPriorityToGroupRange(nil, requested), nil
}

func constrainNewChannelPriority(group string, requested int64) (int64, error) {
	firstGroup := strings.TrimSpace(strings.Split(group, ",")[0])
	groupPriority, err := getGroupBasePriority(firstGroup)
	if err != nil {
		return 0, err
	}
	return constrainPriorityToGroupRange(groupPriority, requested), nil
}

func constrainPriorityToGroupRange(groupPriority *int64, requested int64) int64 {
	if groupPriority == nil || *groupPriority <= 0 {
		return 1
	}
	base := ((*groupPriority - 1) / 10) * 10
	if requested < base+1 {
		return base + 1
	}
	if requested > base+9 {
		return base + 9
	}
	return requested
}

type ChannelBatch struct {
	Ids []int   `json:"ids"`
	Tag *string `json:"tag"`
}

func DeleteChannelBatch(c *gin.Context) {
	channelBatch := ChannelBatch{}
	err := c.ShouldBindJSON(&channelBatch)
	if err != nil || len(channelBatch.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	deletedCount, err := model.BatchDeleteChannels(channelBatch.Ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	if deletedCount > 0 {
		service.ResetProxyClientCache()
	}
	recordManageAudit(c, "channel.delete_batch", map[string]any{
		"count": deletedCount,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    deletedCount,
	})
	return
}

type PatchChannel struct {
	model.Channel
	MultiKeyMode *string `json:"multi_key_mode"`
	KeyMode      *string `json:"key_mode"` // 多key模式下密钥覆盖或者追加
}

// updateAdaptiveRoutingConfig explicitly updates adaptive fields supplied by
// the client. GORM omits zero values when Updates receives a struct, which
// would otherwise make it impossible to turn the feature off or reset a value
// to zero through the channel management form.
func updateAdaptiveRoutingConfig(channel *PatchChannel, requestData map[string]any) error {
	updates := make(map[string]any, 8)
	if _, ok := requestData["adaptive_enabled"]; ok {
		updates["adaptive_enabled"] = channel.AdaptiveEnabled
	}
	if _, ok := requestData["adaptive_window_seconds"]; ok {
		updates["adaptive_window_seconds"] = channel.AdaptiveWindowSeconds
	}
	if _, ok := requestData["adaptive_min_samples"]; ok {
		updates["adaptive_min_samples"] = channel.AdaptiveMinSamples
	}
	if _, ok := requestData["adaptive_slow_threshold_ms"]; ok {
		updates["adaptive_slow_threshold_ms"] = channel.AdaptiveSlowThresholdMs
	}
	if _, ok := requestData["adaptive_min_weight"]; ok {
		updates["adaptive_min_weight"] = channel.AdaptiveMinWeight
	}
	if _, ok := requestData["adaptive_max_weight"]; ok {
		updates["adaptive_max_weight"] = channel.AdaptiveMaxWeight
	}
	if _, ok := requestData["adaptive_recovery_weight"]; ok {
		updates["adaptive_recovery_weight"] = channel.AdaptiveRecoveryWeight
	}
	if _, ok := requestData["adaptive_cooldown_seconds"]; ok {
		updates["adaptive_cooldown_seconds"] = channel.AdaptiveCooldownSeconds
	}
	if len(updates) == 0 {
		return nil
	}
	return model.DB.Model(&model.Channel{}).
		Where("id = ?", channel.Id).
		Updates(updates).Error
}

func isChannelRatioOnlyUpdate(requestData map[string]any) bool {
	if _, provided := requestData["channel_ratio"]; !provided {
		return false
	}
	for field := range requestData {
		if field != "id" && field != "channel_ratio" {
			return false
		}
	}
	return true
}

type ChannelStatusRequest struct {
	Status int `json:"status"`
}

type ChannelStatusBatchRequest struct {
	Ids    []int `json:"ids"`
	Status int   `json:"status"`
}

func UpdateChannel(c *gin.Context) {
	channel := PatchChannel{}
	rawBody, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := common.Unmarshal(rawBody, &channel); err != nil {
		common.ApiError(c, err)
		return
	}
	var requestData map[string]any
	if err := common.Unmarshal(rawBody, &requestData); err != nil {
		common.ApiError(c, err)
		return
	}
	_, channel.ChannelRatioProvided = requestData["channel_ratio"]
	if _, ok := requestData["status"]; ok {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	clearChannelReadOnlyFields(&channel, requestData)

	if channel.Type == constant.ChannelTypeTaskPlugin &&
		!authz.Can(c.GetInt("id"), c.GetInt("role"), authz.TaskPluginBind) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "task plugin channels require the task_plugin.bind permission",
		})
		return
	}

	baseURLFromPluginDefault := channel.Type == constant.ChannelTypeTaskPlugin &&
		(channel.BaseURL == nil || strings.TrimSpace(*channel.BaseURL) == "")
	// 使用统一的校验函数
	if err := validateChannel(&channel.Channel, false); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	// Preserve existing ChannelInfo to ensure multi-key channels keep correct state even if the client does not send ChannelInfo in the request.
	originChannel, err := model.GetChannelById(channel.Id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	originProxy := originChannel.GetSetting().Proxy
	if channel.Weight != nil && *channel.Weight > 300 {
		// A single channel can never consume more than the group's 300-point
		// budget. Clamp first, then rebalance the complete group below.
		*channel.Weight = 300
	}
	// Detect group change (切组)
	targetGroup := originChannel.Group
	if g, ok := requestData["group"].(string); ok && strings.TrimSpace(g) != "" {
		targetGroup = strings.TrimSpace(g)
	} else if channel.Group != "" {
		targetGroup = channel.Group
	}
	groupChanged := targetGroup != originChannel.Group

	if groupChanged {
		_, priorityProvided := requestData["priority"]
		if !priorityProvided {
			origPriority := int64(0)
			if originChannel.Priority != nil {
				origPriority = *originChannel.Priority
			}
			newPriority, err := remapPriorityToGroupRange(targetGroup, origPriority)
			if err == nil {
				channel.Priority = &newPriority
			}
		} else if channel.Priority != nil {
			groupPriority, _ := getGroupBasePriority(targetGroup)
			if groupPriority != nil && !isPriorityInGroupRange(groupPriority, *channel.Priority) {
				newPriority, err := remapPriorityToGroupRange(targetGroup, *channel.Priority)
				if err == nil {
					*channel.Priority = newPriority
				}
			}
		}
	}

	if channel.Priority != nil {
		priority, priorityErr := constrainChannelPriority(channel.Id, targetGroup, *channel.Priority)
		if priorityErr != nil {
			common.ApiError(c, priorityErr)
			return
		}
		*channel.Priority = priority
	}
	proxyChanged := false
	if _, settingProvided := requestData["setting"]; settingProvided {
		newProxy, _ := service.NormalizeProxyURL(channel.GetSetting().Proxy)
		normalizedOriginProxy, originProxyErr := service.NormalizeProxyURL(originProxy)
		proxyChanged = originProxyErr != nil || normalizedOriginProxy != newProxy
	}

	// Always copy the original ChannelInfo so that fields like IsMultiKey and MultiKeySize are retained.
	channel.ChannelInfo = originChannel.ChannelInfo

	if channelHasSensitiveChanges(&channel, originChannel, requestData) &&
		!authz.Can(c.GetInt("id"), c.GetInt("role"), authz.ChannelSensitiveWrite) {
		common.ApiErrorI18n(c, i18n.MsgAuthInsufficientPrivilege)
		return
	}

	// If the request explicitly specifies a new MultiKeyMode, apply it on top of the original info.
	if channel.MultiKeyMode != nil && *channel.MultiKeyMode != "" {
		channel.ChannelInfo.MultiKeyMode = constant.MultiKeyMode(*channel.MultiKeyMode)
	}

	// 处理多key模式下的密钥追加/覆盖逻辑
	if channel.KeyMode != nil && channel.ChannelInfo.IsMultiKey {
		switch *channel.KeyMode {
		case "append":
			// 追加模式：将新密钥添加到现有密钥列表
			if originChannel.Key != "" {
				var newKeys []string
				var existingKeys []string

				// 解析现有密钥
				if strings.HasPrefix(strings.TrimSpace(originChannel.Key), "[") {
					// JSON数组格式
					var arr []json.RawMessage
					if err := json.Unmarshal([]byte(strings.TrimSpace(originChannel.Key)), &arr); err == nil {
						existingKeys = make([]string, len(arr))
						for i, v := range arr {
							existingKeys[i] = string(v)
						}
					}
				} else {
					// 换行分隔格式
					existingKeys = strings.Split(strings.Trim(originChannel.Key, "\n"), "\n")
				}

				// 处理 Vertex AI 的特殊情况
				if channel.Type == constant.ChannelTypeVertexAi && channel.GetOtherSettings().VertexKeyType != dto.VertexKeyTypeAPIKey {
					// 尝试解析新密钥为JSON数组
					if strings.HasPrefix(strings.TrimSpace(channel.Key), "[") {
						array, err := getVertexArrayKeys(channel.Key)
						if err != nil {
							c.JSON(http.StatusOK, gin.H{
								"success": false,
								"message": "追加密钥解析失败: " + err.Error(),
							})
							return
						}
						newKeys = array
					} else {
						// 单个JSON密钥
						newKeys = []string{channel.Key}
					}
				} else {
					// 普通渠道的处理
					inputKeys := strings.SplitSeq(channel.Key, "\n")
					for key := range inputKeys {
						key = strings.TrimSpace(key)
						if key != "" {
							newKeys = append(newKeys, key)
						}
					}
				}

				seen := make(map[string]struct{}, len(existingKeys)+len(newKeys))
				for _, key := range existingKeys {
					normalized := strings.TrimSpace(key)
					if normalized == "" {
						continue
					}
					seen[normalized] = struct{}{}
				}
				dedupedNewKeys := make([]string, 0, len(newKeys))
				for _, key := range newKeys {
					normalized := strings.TrimSpace(key)
					if normalized == "" {
						continue
					}
					if _, ok := seen[normalized]; ok {
						continue
					}
					seen[normalized] = struct{}{}
					dedupedNewKeys = append(dedupedNewKeys, normalized)
				}

				allKeys := append(existingKeys, dedupedNewKeys...)
				channel.Key = strings.Join(allKeys, "\n")
			}
		case "replace":
			// 覆盖模式：直接使用新密钥（默认行为，不需要特殊处理）
		}
	}
	if isChannelRatioOnlyUpdate(requestData) {
		err = channel.UpdateChannelRatio()
	} else {
		err = channel.Update()
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel.Weight != nil && !isChannelRatioOnlyUpdate(requestData) {
		if err := model.RebalanceChannelWeightsForChannel(channel.Id); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if err := updateAdaptiveRoutingConfig(&channel, requestData); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	if proxyChanged {
		service.InvalidateProxyClient(originProxy)
	}
	// 记录变更的字段名（语言无关的字段标识），密钥仅记录"已更换"绝不记录内容。
	changedFields := make([]string, 0)
	if channel.Models != originChannel.Models {
		changedFields = append(changedFields, "models")
	}
	if channel.Group != originChannel.Group {
		changedFields = append(changedFields, "group")
	}
	if channel.Type != originChannel.Type {
		changedFields = append(changedFields, "type")
	}
	if !equalStringPtr(channel.BaseURL, originChannel.BaseURL) {
		changedFields = append(changedFields, "base_url")
	}
	if channel.Key != "" && channel.Key != originChannel.Key {
		changedFields = append(changedFields, "key")
	}
	for _, field := range []string{
		"channel_ratio",
		"adaptive_enabled",
		"adaptive_window_seconds",
		"adaptive_min_samples",
		"adaptive_slow_threshold_ms",
		"adaptive_min_weight",
		"adaptive_max_weight",
		"adaptive_recovery_weight",
		"adaptive_cooldown_seconds",
	} {
		if _, ok := requestData[field]; ok {
			changedFields = append(changedFields, field)
		}
	}
	updateAudit := map[string]any{
		"id":             channel.Id,
		"name":           channel.Name,
		"changed_fields": changedFields,
	}
	if baseURLFromPluginDefault {
		updateAudit["base_url_source"] = "plugin_default"
	}
	recordManageAudit(c, "channel.update", updateAudit)
	channel.Key = ""
	clearChannelInfo(&channel.Channel)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channel,
	})
	return
}

func UpdateChannelStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	req := ChannelStatusRequest{}
	if err := c.ShouldBindJSON(&req); err != nil || !isManageableChannelStatus(req.Status) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	changed := model.UpdateChannelStatus(id, "", req.Status, "manual operation")
	if changed {
		model.InitChannelCache()
	}
	recordManageAudit(c, "channel.status_update", map[string]any{
		"id":      id,
		"status":  req.Status,
		"changed": changed,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    changed,
	})
}

func BatchUpdateChannelStatus(c *gin.Context) {
	req := ChannelStatusBatchRequest{}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 || !isManageableChannelStatus(req.Status) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	changedCount := 0
	for _, id := range req.Ids {
		if model.UpdateChannelStatus(id, "", req.Status, "manual batch operation") {
			changedCount++
		}
	}
	if changedCount > 0 {
		model.InitChannelCache()
	}
	recordManageAudit(c, "channel.status_update_batch", map[string]any{
		"count":  changedCount,
		"total":  len(req.Ids),
		"status": req.Status,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    changedCount,
	})
}

func isManageableChannelStatus(status int) bool {
	return status == common.ChannelStatusEnabled || status == common.ChannelStatusManuallyDisabled || status == common.ChannelStatusAutoDisabled
}

// equalStringPtr 比较两个 *string 是否相等（均为 nil 视为相等）。
func equalStringPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

type fetchModelsRequest struct {
	ChannelID      int     `json:"channel_id"`
	BaseURL        *string `json:"base_url"`
	Type           int     `json:"type"`
	Key            string  `json:"key"`
	AdvancedCustom *string `json:"advanced_custom"`
	HeaderOverride *string `json:"header_override"`
	Proxy          *string `json:"proxy"`
}

func buildAdvancedCustomModelPreviewChannel(req fetchModelsRequest) (*model.Channel, error) {
	var channel *model.Channel
	if req.ChannelID > 0 {
		savedChannel, err := model.GetChannelById(req.ChannelID, true)
		if err != nil {
			return nil, err
		}
		if savedChannel.Type != constant.ChannelTypeAdvancedCustom {
			return nil, fmt.Errorf("channel %d is not an advanced custom channel", req.ChannelID)
		}
		channel = savedChannel
	} else {
		key := strings.TrimSpace(req.Key)
		if key != "" {
			key = strings.Split(key, "\n")[0]
		}
		channel = &model.Channel{
			Type: req.Type,
			Key:  key,
		}
	}

	if channel.Type != constant.ChannelTypeAdvancedCustom {
		return nil, fmt.Errorf("channel type must be advanced custom")
	}
	if req.BaseURL != nil {
		baseURL := strings.TrimSpace(*req.BaseURL)
		channel.BaseURL = &baseURL
	}

	settings := channel.GetOtherSettings()
	if req.AdvancedCustom != nil {
		rawConfig := strings.TrimSpace(*req.AdvancedCustom)
		if rawConfig == "" {
			return nil, fmt.Errorf("advanced_custom is required")
		}
		var config dto.AdvancedCustomConfig
		if err := common.UnmarshalJsonStr(rawConfig, &config); err != nil {
			return nil, err
		}
		settings.AdvancedCustom = &config
	} else if req.ChannelID <= 0 {
		return nil, fmt.Errorf("advanced_custom is required")
	}
	channel.SetOtherSettings(settings)

	if req.HeaderOverride != nil {
		rawHeaderOverride := strings.TrimSpace(*req.HeaderOverride)
		if rawHeaderOverride != "" {
			var headerOverride map[string]any
			if err := common.UnmarshalJsonStr(rawHeaderOverride, &headerOverride); err != nil {
				return nil, fmt.Errorf("header_override must be a JSON object: %w", err)
			}
		}
		channel.HeaderOverride = &rawHeaderOverride
	}
	if req.Proxy != nil {
		channelSettings := channel.GetSetting()
		channelSettings.Proxy = strings.TrimSpace(*req.Proxy)
		channel.SetSetting(channelSettings)
	}

	if err := validateChannel(channel, false); err != nil {
		return nil, err
	}
	return channel, nil
}

func FetchModels(c *gin.Context) {
	var req fetchModelsRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request",
		})
		return
	}

	var channel *model.Channel
	if req.Type == constant.ChannelTypeAdvancedCustom || req.ChannelID > 0 {
		var err error
		channel, err = buildAdvancedCustomModelPreviewChannel(req)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	} else {
		baseURL := ""
		if req.BaseURL != nil {
			baseURL = strings.TrimSpace(*req.BaseURL)
		}
		if baseURL == "" {
			baseURL = constant.GetChannelBaseURL(req.Type)
		}

		key := strings.TrimSpace(req.Key)
		if req.Type != constant.ChannelTypeCodex {
			key = strings.Split(key, "\n")[0]
		}
		channel = &model.Channel{
			Type:    req.Type,
			Key:     key,
			BaseURL: &baseURL,
		}
	}

	models, err := fetchChannelUpstreamModelIDs(channel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取模型列表失败: %s", err.Error()),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    models,
	})
}

func BatchSetChannelTag(c *gin.Context) {
	channelBatch := ChannelBatch{}
	err := c.ShouldBindJSON(&channelBatch)
	if err != nil || len(channelBatch.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.BatchSetChannelTag(channelBatch.Ids, channelBatch.Tag)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	recordManageAudit(c, "channel.tag_batch_set", map[string]any{
		"count": len(channelBatch.Ids),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    len(channelBatch.Ids),
	})
	return
}

func GetTagModels(c *gin.Context) {
	tag := c.Query("tag")
	if tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "tag不能为空",
		})
		return
	}

	channels, err := model.GetChannelsByTag(tag, false, false) // idSort=false, selectAll=false
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	var longestModels string
	maxLength := 0

	// Find the longest models string among all channels with the given tag
	for _, channel := range channels {
		if channel.Models != "" {
			currentModels := strings.Split(channel.Models, ",")
			if len(currentModels) > maxLength {
				maxLength = len(currentModels)
				longestModels = channel.Models
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    longestModels,
	})
	return
}

// CopyChannel handles cloning an existing channel with its key.
// POST /api/channel/copy/:id
// Optional query params:
//
//	suffix         - string appended to the original name (default "_复制")
//	reset_balance  - bool, when true will reset balance & used_quota to 0 (default true)
func CopyChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid id"})
		return
	}

	suffix := c.DefaultQuery("suffix", "_复制")
	targetGroup := strings.TrimSpace(c.Query("group"))
	if targetGroup == "" {
		targetGroup = strings.TrimSpace(c.Query("target_group"))
	}
	resetBalance := true
	if rbStr := c.DefaultQuery("reset_balance", "true"); rbStr != "" {
		if v, err := strconv.ParseBool(rbStr); err == nil {
			resetBalance = v
		}
	}

	// fetch original channel with key
	origin, err := model.GetChannelById(id, true)
	if err != nil {
		common.SysError("failed to get channel by id: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取渠道信息失败，请稍后重试"})
		return
	}
	if origin.Type == constant.ChannelTypeTaskPlugin &&
		!authz.Can(c.GetInt("id"), c.GetInt("role"), authz.TaskPluginBind) {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "task plugin channels require the task_plugin.bind permission"})
		return
	}

	// clone channel
	clone := *origin // shallow copy is sufficient as we will overwrite primitives
	clone.Id = 0     // let DB auto-generate
	clone.CreatedTime = common.GetTimestamp()
	clone.Name = origin.Name + suffix
	clone.TestTime = 0
	clone.ResponseTime = 0
	if resetBalance {
		clone.Balance = 0
		clone.UsedQuota = 0
	}
	if targetGroup != "" && targetGroup != origin.Group {
		clone.Group = targetGroup
		origPriority := int64(0)
		if origin.Priority != nil {
			origPriority = *origin.Priority
		}
		newPriority, err := remapPriorityToGroupRange(targetGroup, origPriority)
		if err == nil {
			clone.Priority = &newPriority
		}
	}

	if err := clone.ValidateSettings(); err != nil {
		common.SysError("failed to validate cloned channel: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Failed to copy channel: invalid channel settings"})
		return
	}

	// insert
	if err := clone.Insert(); err != nil {
		common.SysError("failed to clone channel: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "复制渠道失败，请稍后重试"})
		return
	}
	model.InitChannelCache()
	recordManageAudit(c, "channel.copy", map[string]any{
		"sourceId": id,
		"id":       clone.Id,
		"name":     clone.Name,
	})
	// success
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"id": clone.Id}})
}

// MultiKeyManageRequest represents the request for multi-key management operations
type MultiKeyManageRequest struct {
	ChannelId int    `json:"channel_id"`
	Action    string `json:"action"`              // "disable_key", "enable_key", "delete_key", "delete_disabled_keys", "get_key_status"
	KeyIndex  *int   `json:"key_index,omitempty"` // for disable_key, enable_key, and delete_key actions
	Page      int    `json:"page,omitempty"`      // for get_key_status pagination
	PageSize  int    `json:"page_size,omitempty"` // for get_key_status pagination
	Status    *int   `json:"status,omitempty"`    // for get_key_status filtering: 1=enabled, 2=manual_disabled, 3=auto_disabled, nil=all
}

// MultiKeyStatusResponse represents the response for key status query
type MultiKeyStatusResponse struct {
	Keys       []KeyStatus `json:"keys"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
	// Statistics
	EnabledCount        int `json:"enabled_count"`
	ManualDisabledCount int `json:"manual_disabled_count"`
	AutoDisabledCount   int `json:"auto_disabled_count"`
}

type KeyStatus struct {
	Index        int    `json:"index"`
	Status       int    `json:"status"` // 1: enabled, 2: disabled
	DisabledTime int64  `json:"disabled_time,omitempty"`
	Reason       string `json:"reason,omitempty"`
	KeyPreview   string `json:"key_preview"` // first 10 chars of key for identification
}

// ManageMultiKeys handles multi-key management operations
func ManageMultiKeys(c *gin.Context) {
	request := MultiKeyManageRequest{}
	err := c.ShouldBindJSON(&request)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	channel, err := model.GetChannelById(request.ChannelId, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "渠道不存在",
		})
		return
	}

	if !channel.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该渠道不是多密钥模式",
		})
		return
	}
	if multiKeyActionRequiresSensitiveWrite(request.Action) &&
		!authz.Can(c.GetInt("id"), c.GetInt("role"), authz.ChannelSensitiveWrite) {
		common.ApiErrorI18n(c, i18n.MsgAuthInsufficientPrivilege)
		return
	}

	// get_key_status 为只读查询，不记录审计；其余为修改操作，记录审计并跳过中间件兜底。
	if request.Action == "get_key_status" {
		markAuditLogged(c)
	} else {
		recordManageAudit(c, "channel.multi_key_manage", map[string]any{
			"action": request.Action,
			"id":     channel.Id,
		})
	}

	lock := model.GetChannelPollingLock(channel.Id)
	lock.Lock()
	defer lock.Unlock()

	switch request.Action {
	case "get_key_status":
		keys := channel.GetKeys()

		// Default pagination parameters
		page := request.Page
		pageSize := request.PageSize
		if page <= 0 {
			page = 1
		}
		if pageSize <= 0 {
			pageSize = 50 // Default page size
		}

		// Statistics for all keys (unchanged by filtering)
		var enabledCount, manualDisabledCount, autoDisabledCount int

		// Build all key status data first
		var allKeyStatusList []KeyStatus
		for i, key := range keys {
			status := 1 // default enabled
			var disabledTime int64
			var reason string

			if channel.ChannelInfo.MultiKeyStatusList != nil {
				if s, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists {
					status = s
				}
			}

			// Count for statistics (all keys)
			switch status {
			case 1:
				enabledCount++
			case 2:
				manualDisabledCount++
			case 3:
				autoDisabledCount++
			}

			if status != 1 {
				if channel.ChannelInfo.MultiKeyDisabledTime != nil {
					disabledTime = channel.ChannelInfo.MultiKeyDisabledTime[i]
				}
				if channel.ChannelInfo.MultiKeyDisabledReason != nil {
					reason = channel.ChannelInfo.MultiKeyDisabledReason[i]
				}
			}

			// Create key preview (first 10 chars)
			keyPreview := key
			if len(key) > 10 {
				keyPreview = key[:10] + "..."
			}

			allKeyStatusList = append(allKeyStatusList, KeyStatus{
				Index:        i,
				Status:       status,
				DisabledTime: disabledTime,
				Reason:       reason,
				KeyPreview:   keyPreview,
			})
		}

		// Apply status filter if specified
		var filteredKeyStatusList []KeyStatus
		if request.Status != nil {
			for _, keyStatus := range allKeyStatusList {
				if keyStatus.Status == *request.Status {
					filteredKeyStatusList = append(filteredKeyStatusList, keyStatus)
				}
			}
		} else {
			filteredKeyStatusList = allKeyStatusList
		}

		// Calculate pagination based on filtered results
		filteredTotal := len(filteredKeyStatusList)
		totalPages := (filteredTotal + pageSize - 1) / pageSize
		if totalPages == 0 {
			totalPages = 1
		}
		if page > totalPages {
			page = totalPages
		}

		// Calculate range for current page
		start := (page - 1) * pageSize
		end := min(start+pageSize, filteredTotal)

		// Get the page data
		var pageKeyStatusList []KeyStatus
		if start < filteredTotal {
			pageKeyStatusList = filteredKeyStatusList[start:end]
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": MultiKeyStatusResponse{
				Keys:                pageKeyStatusList,
				Total:               filteredTotal, // Total of filtered results
				Page:                page,
				PageSize:            pageSize,
				TotalPages:          totalPages,
				EnabledCount:        enabledCount,        // Overall statistics
				ManualDisabledCount: manualDisabledCount, // Overall statistics
				AutoDisabledCount:   autoDisabledCount,   // Overall statistics
			},
		})
		return

	case "disable_key":
		if request.KeyIndex == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "未指定要禁用的密钥索引",
			})
			return
		}

		keyIndex := *request.KeyIndex
		if keyIndex < 0 || keyIndex >= channel.ChannelInfo.MultiKeySize {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "密钥索引超出范围",
			})
			return
		}

		if channel.ChannelInfo.MultiKeyStatusList == nil {
			channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		}
		if channel.ChannelInfo.MultiKeyDisabledTime == nil {
			channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
		}
		if channel.ChannelInfo.MultiKeyDisabledReason == nil {
			channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)
		}

		channel.ChannelInfo.MultiKeyStatusList[keyIndex] = 2 // disabled

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "密钥已禁用",
		})
		return

	case "enable_key":
		if request.KeyIndex == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "未指定要启用的密钥索引",
			})
			return
		}

		keyIndex := *request.KeyIndex
		if keyIndex < 0 || keyIndex >= channel.ChannelInfo.MultiKeySize {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "密钥索引超出范围",
			})
			return
		}

		// 从状态列表中删除该密钥的记录，使其回到默认启用状态
		if channel.ChannelInfo.MultiKeyStatusList != nil {
			delete(channel.ChannelInfo.MultiKeyStatusList, keyIndex)
		}
		if channel.ChannelInfo.MultiKeyDisabledTime != nil {
			delete(channel.ChannelInfo.MultiKeyDisabledTime, keyIndex)
		}
		if channel.ChannelInfo.MultiKeyDisabledReason != nil {
			delete(channel.ChannelInfo.MultiKeyDisabledReason, keyIndex)
		}

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "密钥已启用",
		})
		return

	case "enable_all_keys":
		// 清空所有禁用状态，使所有密钥回到默认启用状态
		var enabledCount int
		if channel.ChannelInfo.MultiKeyStatusList != nil {
			enabledCount = len(channel.ChannelInfo.MultiKeyStatusList)
		}

		channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
		channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": fmt.Sprintf("已启用 %d 个密钥", enabledCount),
		})
		return

	case "disable_all_keys":
		// 禁用所有启用的密钥
		if channel.ChannelInfo.MultiKeyStatusList == nil {
			channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		}
		if channel.ChannelInfo.MultiKeyDisabledTime == nil {
			channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
		}
		if channel.ChannelInfo.MultiKeyDisabledReason == nil {
			channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)
		}

		var disabledCount int
		for i := 0; i < channel.ChannelInfo.MultiKeySize; i++ {
			status := 1 // default enabled
			if s, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists {
				status = s
			}

			// 只禁用当前启用的密钥
			if status == 1 {
				channel.ChannelInfo.MultiKeyStatusList[i] = 2 // disabled
				disabledCount++
			}
		}

		if disabledCount == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "没有可禁用的密钥",
			})
			return
		}

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": fmt.Sprintf("已禁用 %d 个密钥", disabledCount),
		})
		return

	case "delete_key":
		if request.KeyIndex == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "未指定要删除的密钥索引",
			})
			return
		}

		keyIndex := *request.KeyIndex
		if keyIndex < 0 || keyIndex >= channel.ChannelInfo.MultiKeySize {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "密钥索引超出范围",
			})
			return
		}

		keys := channel.GetKeys()
		var remainingKeys []string
		var newStatusList = make(map[int]int)
		var newDisabledTime = make(map[int]int64)
		var newDisabledReason = make(map[int]string)

		newIndex := 0
		for i, key := range keys {
			// 跳过要删除的密钥
			if i == keyIndex {
				continue
			}

			remainingKeys = append(remainingKeys, key)

			// 保留其他密钥的状态信息，重新索引
			if channel.ChannelInfo.MultiKeyStatusList != nil {
				if status, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists && status != 1 {
					newStatusList[newIndex] = status
				}
			}
			if channel.ChannelInfo.MultiKeyDisabledTime != nil {
				if t, exists := channel.ChannelInfo.MultiKeyDisabledTime[i]; exists {
					newDisabledTime[newIndex] = t
				}
			}
			if channel.ChannelInfo.MultiKeyDisabledReason != nil {
				if r, exists := channel.ChannelInfo.MultiKeyDisabledReason[i]; exists {
					newDisabledReason[newIndex] = r
				}
			}
			newIndex++
		}

		if len(remainingKeys) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "不能删除最后一个密钥",
			})
			return
		}

		// Update channel with remaining keys
		channel.Key = strings.Join(remainingKeys, "\n")
		channel.ChannelInfo.MultiKeySize = len(remainingKeys)
		channel.ChannelInfo.MultiKeyStatusList = newStatusList
		channel.ChannelInfo.MultiKeyDisabledTime = newDisabledTime
		channel.ChannelInfo.MultiKeyDisabledReason = newDisabledReason

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "密钥已删除",
		})
		return

	case "delete_disabled_keys":
		keys := channel.GetKeys()
		var remainingKeys []string
		var deletedCount int
		var newStatusList = make(map[int]int)
		var newDisabledTime = make(map[int]int64)
		var newDisabledReason = make(map[int]string)

		newIndex := 0
		for i, key := range keys {
			status := 1 // default enabled
			if channel.ChannelInfo.MultiKeyStatusList != nil {
				if s, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists {
					status = s
				}
			}

			// 只删除自动禁用（status == 3）的密钥，保留启用（status == 1）和手动禁用（status == 2）的密钥
			if status == 3 {
				deletedCount++
			} else {
				remainingKeys = append(remainingKeys, key)
				// 保留非自动禁用密钥的状态信息，重新索引
				if status != 1 {
					newStatusList[newIndex] = status
					if channel.ChannelInfo.MultiKeyDisabledTime != nil {
						if t, exists := channel.ChannelInfo.MultiKeyDisabledTime[i]; exists {
							newDisabledTime[newIndex] = t
						}
					}
					if channel.ChannelInfo.MultiKeyDisabledReason != nil {
						if r, exists := channel.ChannelInfo.MultiKeyDisabledReason[i]; exists {
							newDisabledReason[newIndex] = r
						}
					}
				}
				newIndex++
			}
		}

		if deletedCount == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "没有需要删除的自动禁用密钥",
			})
			return
		}

		// Update channel with remaining keys
		channel.Key = strings.Join(remainingKeys, "\n")
		channel.ChannelInfo.MultiKeySize = len(remainingKeys)
		channel.ChannelInfo.MultiKeyStatusList = newStatusList
		channel.ChannelInfo.MultiKeyDisabledTime = newDisabledTime
		channel.ChannelInfo.MultiKeyDisabledReason = newDisabledReason

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": fmt.Sprintf("已删除 %d 个自动禁用的密钥", deletedCount),
			"data":    deletedCount,
		})
		return

	default:
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "不支持的操作",
		})
		return
	}
}

func multiKeyActionRequiresSensitiveWrite(action string) bool {
	return action == "delete_key" || action == "delete_disabled_keys"
}

// OllamaPullModel 拉取 Ollama 模型
func OllamaPullModel(c *gin.Context) {
	var req struct {
		ChannelID int    `json:"channel_id"`
		ModelName string `json:"model_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request parameters",
		})
		return
	}

	if req.ChannelID == 0 || req.ModelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Channel ID and model name are required",
		})
		return
	}

	// 获取渠道信息
	channel, err := model.GetChannelById(req.ChannelID, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Channel not found",
		})
		return
	}

	// 检查是否是 Ollama 渠道
	if channel.Type != constant.ChannelTypeOllama {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "This operation is only supported for Ollama channels",
		})
		return
	}

	baseURL := constant.GetChannelBaseURL(channel.Type)
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	key := strings.Split(channel.Key, "\n")[0]
	err = ollama.PullOllamaModel(baseURL, key, req.ModelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Failed to pull model: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Model %s pulled successfully", req.ModelName),
	})
}

// OllamaPullModelStream 流式拉取 Ollama 模型
func OllamaPullModelStream(c *gin.Context) {
	var req struct {
		ChannelID int    `json:"channel_id"`
		ModelName string `json:"model_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request parameters",
		})
		return
	}

	if req.ChannelID == 0 || req.ModelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Channel ID and model name are required",
		})
		return
	}

	// 获取渠道信息
	channel, err := model.GetChannelById(req.ChannelID, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Channel not found",
		})
		return
	}

	// 检查是否是 Ollama 渠道
	if channel.Type != constant.ChannelTypeOllama {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "This operation is only supported for Ollama channels",
		})
		return
	}

	baseURL := constant.GetChannelBaseURL(channel.Type)
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	// 设置 SSE 头部
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	key := strings.Split(channel.Key, "\n")[0]

	// 创建进度回调函数
	progressCallback := func(progress ollama.OllamaPullResponse) {
		data, _ := json.Marshal(progress)
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(data))
		c.Writer.Flush()
	}

	// 执行拉取
	err = ollama.PullOllamaModelStream(baseURL, key, req.ModelName, progressCallback)

	if err != nil {
		errorData, _ := json.Marshal(gin.H{
			"error": err.Error(),
		})
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(errorData))
	} else {
		successData, _ := json.Marshal(gin.H{
			"message": fmt.Sprintf("Model %s pulled successfully", req.ModelName),
		})
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(successData))
	}

	// 发送结束标志
	fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
	c.Writer.Flush()
}

// OllamaDeleteModel 删除 Ollama 模型
func OllamaDeleteModel(c *gin.Context) {
	var req struct {
		ChannelID int    `json:"channel_id"`
		ModelName string `json:"model_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request parameters",
		})
		return
	}

	if req.ChannelID == 0 || req.ModelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Channel ID and model name are required",
		})
		return
	}

	// 获取渠道信息
	channel, err := model.GetChannelById(req.ChannelID, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Channel not found",
		})
		return
	}

	// 检查是否是 Ollama 渠道
	if channel.Type != constant.ChannelTypeOllama {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "This operation is only supported for Ollama channels",
		})
		return
	}

	baseURL := constant.GetChannelBaseURL(channel.Type)
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	key := strings.Split(channel.Key, "\n")[0]
	err = ollama.DeleteOllamaModel(baseURL, key, req.ModelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Failed to delete model: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Model %s deleted successfully", req.ModelName),
	})
}

// OllamaVersion 获取 Ollama 服务版本信息
func OllamaVersion(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid channel id",
		})
		return
	}

	channel, err := model.GetChannelById(id, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Channel not found",
		})
		return
	}

	if channel.Type != constant.ChannelTypeOllama {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "This operation is only supported for Ollama channels",
		})
		return
	}

	baseURL := constant.GetChannelBaseURL(channel.Type)
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	key := strings.Split(channel.Key, "\n")[0]
	version, err := ollama.FetchOllamaVersion(baseURL, key)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取Ollama版本失败: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"version": version,
		},
	})
}
