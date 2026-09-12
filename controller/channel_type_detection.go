package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	channelPlatformNewAPI  = "newapi"
	channelPlatformSub2API = "sub2api"
	channelPlatformUnknown = "unknown"
)

type DetectChannelTypeRequest struct {
	BaseURL string `json:"base_url" binding:"required"`
	Key     string `json:"key,omitempty"`
}

type ChannelTypeDetectionResult struct {
	Type       string   `json:"type"`
	Confidence string   `json:"confidence"`
	Evidence   []string `json:"evidence"`
}

type DetectAllChannelTypesRequest struct {
	IDs   []int `json:"ids,omitempty"`
	Force bool  `json:"force,omitempty"`
}

type ChannelTypeDetectionItem struct {
	ID         int      `json:"id"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Confidence string   `json:"confidence"`
	Evidence   []string `json:"evidence"`
}

type channelTypeProbe struct {
	Path       string
	StatusCode int
	Payload    map[string]any
}

func normalizeChannelDetectionURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("base_url 必须是完整的 HTTP(S) 地址")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("base_url 只支持 http 或 https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("base_url 不支持用户信息、查询参数或片段")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func probeChannelTypeEndpoint(ctx context.Context, client *http.Client, baseURL, path, key string) channelTypeProbe {
	probe := channelTypeProbe{Path: path}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return probe
	}
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(key) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
	}
	resp, err := client.Do(req)
	if err != nil {
		return probe
	}
	defer resp.Body.Close()
	probe.StatusCode = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err == nil && len(body) > 0 {
		var payload map[string]any
		if common.Unmarshal(body, &payload) == nil {
			probe.Payload = payload
		}
	}
	return probe
}

func payloadHas(payload map[string]any, key string) bool {
	_, ok := payload[key]
	return ok
}

func matchesSub2APIProbe(probe channelTypeProbe) bool {
	if probe.StatusCode >= 200 && probe.StatusCode < 300 && payloadHas(probe.Payload, "data") {
		return true
	}
	if probe.StatusCode != http.StatusUnauthorized && probe.StatusCode != http.StatusForbidden {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(fmt.Sprint(probe.Payload["message"])))
	if probe.Payload["code"] == "UNAUTHORIZED" && strings.Contains(message, "authorization header") {
		return true
	}
	success, hasSuccess := probe.Payload["success"].(bool)
	return hasSuccess && !success && (strings.Contains(message, "未登录") || strings.Contains(message, "unauthorized") || strings.Contains(message, "not logged"))
}

func matchesNewAPIProbe(probe channelTypeProbe) bool {
	return (probe.StatusCode >= 200 && probe.StatusCode < 300 && payloadHas(probe.Payload, "success") && payloadHas(probe.Payload, "data")) ||
		((probe.StatusCode == http.StatusUnauthorized || probe.StatusCode == http.StatusForbidden) && payloadHas(probe.Payload, "success") && payloadHas(probe.Payload, "message"))
}

func matchesNewAPIStatusProbe(probe channelTypeProbe) bool {
	if probe.StatusCode < 200 || probe.StatusCode >= 300 || probe.Payload == nil {
		return false
	}
	data, ok := probe.Payload["data"].(map[string]any)
	if !ok {
		return false
	}
	success, ok := probe.Payload["success"].(bool)
	return ok && success && payloadHas(data, "system_name") && payloadHas(data, "version")
}

func cachedChannelSiteType(channel *model.Channel) (string, bool) {
	if channel == nil {
		return "", false
	}
	if channel.SiteType != nil && (*channel.SiteType == channelPlatformNewAPI || *channel.SiteType == channelPlatformSub2API || *channel.SiteType == channelPlatformUnknown) {
		return *channel.SiteType, true
	}
	info := channel.GetOtherInfo()
	siteType, ok := info["site_type"].(string)
	if !ok || (siteType != channelPlatformNewAPI && siteType != channelPlatformSub2API && siteType != channelPlatformUnknown) {
		return "", false
	}
	return siteType, true
}

func setCachedChannelSiteType(channel *model.Channel, result ChannelTypeDetectionResult) error {
	if channel == nil || channel.Id == 0 {
		return nil
	}
	channel.SiteType = &result.Type
	return model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"site_type": result.Type,
	}).Error
}

func detectChannelPlatform(ctx context.Context, client *http.Client, rawURL string) (ChannelTypeDetectionResult, error) {
	baseURL, err := normalizeChannelDetectionURL(rawURL)
	if err != nil {
		return ChannelTypeDetectionResult{}, err
	}
	if client == nil {
		client = service.GetSSRFProtectedHTTPClient()
	}
	if client == nil {
		return ChannelTypeDetectionResult{}, fmt.Errorf("HTTP 客户端尚未初始化")
	}

	// Site identification is based only on public responses from the configured
	// API base URL. Inference keys must never be sent to platform account paths.
	sub2api := probeChannelTypeEndpoint(ctx, client, baseURL, "/api/v1/auth/me", "")
	newapi := probeChannelTypeEndpoint(ctx, client, baseURL, "/api/user/self", "")
	newapiStatus := probeChannelTypeEndpoint(ctx, client, baseURL, "/api/status", "")
	sub2apiMatch := matchesSub2APIProbe(sub2api)
	newapiMatch := matchesNewAPIProbe(newapi) || matchesNewAPIStatusProbe(newapiStatus)

	result := ChannelTypeDetectionResult{Type: channelPlatformUnknown, Confidence: "none", Evidence: make([]string, 0, 3)}
	if sub2apiMatch {
		result.Evidence = append(result.Evidence, fmt.Sprintf("%s returned a Sub2API auth envelope (HTTP %d)", sub2api.Path, sub2api.StatusCode))
	}
	if newapiMatch {
		if matchesNewAPIProbe(newapi) {
			result.Evidence = append(result.Evidence, fmt.Sprintf("%s returned a NewAPI user envelope (HTTP %d)", newapi.Path, newapi.StatusCode))
		} else {
			result.Evidence = append(result.Evidence, fmt.Sprintf("%s returned a NewAPI status envelope (HTTP %d)", newapiStatus.Path, newapiStatus.StatusCode))
		}
	}
	if sub2apiMatch && !newapiMatch {
		result.Type, result.Confidence = channelPlatformSub2API, "high"
	} else if newapiMatch && !sub2apiMatch {
		result.Type, result.Confidence = channelPlatformNewAPI, "high"
	} else if sub2apiMatch && newapiMatch {
		result.Confidence = "conflict"
		result.Evidence = append(result.Evidence, "both platform endpoints matched; manual verification required")
	}
	return result, nil
}

// DetectChannelType performs read-only endpoint probing and never changes channel data.
func DetectChannelType(c *gin.Context) {
	var req DetectChannelTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	result, err := detectChannelPlatform(ctx, service.GetSSRFProtectedHTTPClient(), req.BaseURL)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// DetectAllChannelTypes probes every configured channel without changing channel data.
func DetectAllChannelTypes(c *gin.Context) {
	var req DetectAllChannelTypesRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			common.ApiError(c, err)
			return
		}
	}

	var channels []*model.Channel
	var err error
	if len(req.IDs) > 0 {
		channels, err = model.GetChannelsByIds(req.IDs)
	} else {
		err = model.DB.Find(&channels).Error
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	client := service.GetSSRFProtectedHTTPClient()
	results := make([]ChannelTypeDetectionItem, len(channels))
	semaphore := make(chan struct{}, 4)
	var waitGroup sync.WaitGroup
	for index, channel := range channels {
		waitGroup.Add(1)
		go func(index int, channel *model.Channel) {
			defer waitGroup.Done()
			item := ChannelTypeDetectionItem{ID: channel.Id, Name: channel.Name, Type: channelPlatformUnknown, Confidence: "none", Evidence: []string{"base_url is empty"}}
			if cachedType, ok := cachedChannelSiteType(channel); ok && !req.Force {
				item.Type = cachedType
				item.Confidence = "cached"
				item.Evidence = []string{"using cached site type"}
				if channel.SiteType == nil {
					// Migrate metadata written by the previous implementation into
					// the independent site_type column without probing upstream.
					_ = setCachedChannelSiteType(channel, ChannelTypeDetectionResult{Type: cachedType})
				}
				results[index] = item
				return
			}
			if channel.BaseURL != nil && strings.TrimSpace(*channel.BaseURL) != "" {
				select {
				case semaphore <- struct{}{}:
					result, detectErr := detectChannelPlatform(ctx, client, *channel.BaseURL)
					<-semaphore
					if detectErr == nil {
						item.Type, item.Confidence, item.Evidence = result.Type, result.Confidence, result.Evidence
						if persistErr := setCachedChannelSiteType(channel, result); persistErr != nil {
							item.Evidence = append(item.Evidence, "cache write failed: "+persistErr.Error())
						}
					} else {
						item.Evidence = []string{"probe failed: " + detectErr.Error()}
					}
				case <-ctx.Done():
					item.Evidence = []string{"probe timed out"}
				}
			}
			if !req.Force || item.Type == channelPlatformUnknown {
				// Unknown results are cached too, so a permanently incompatible or
				// empty endpoint is not probed again on every page load.
				if _, cached := cachedChannelSiteType(channel); !cached || req.Force {
					_ = setCachedChannelSiteType(channel, ChannelTypeDetectionResult{
						Type: item.Type, Confidence: item.Confidence, Evidence: item.Evidence,
					})
				}
			}
			results[index] = item
		}(index, channel)
	}
	waitGroup.Wait()
	common.ApiSuccess(c, results)
}
