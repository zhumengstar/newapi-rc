package model

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"golang.org/x/sync/singleflight"
)

type cachedLogCount struct {
	value     int64
	expiresAt time.Time
}

var logCountCache = struct {
	sync.Mutex
	entries map[string]cachedLogCount
}{entries: make(map[string]cachedLogCount)}

var logCountGroup singleflight.Group

func logCountCacheTTL() time.Duration {
	seconds := common.GetEnvOrDefault("LOG_COUNT_CACHE_TTL_SECONDS", 60)
	if seconds < 0 {
		seconds = 0
	}
	return time.Duration(seconds) * time.Second
}

func normalizeTimestampForCountCache(ts int64) string {
	if ts <= 0 {
		return "0"
	}
	// 若结束时间处于当前时间附近或未来（如前端默认 endTime = now + 1h），统一归一化为 "latest"，
	// 彻底消除自动刷新或点击搜索时因秒级时间推进导致的缓存击穿。
	if ts >= time.Now().Unix()-15 {
		return "latest"
	}
	// 归一化到 10 秒窗口，避免高频轮询/实时查询中的时间微小漂移导致 COUNT(*) 击穿缓存
	return strconv.FormatInt((ts/10)*10, 10)
}

func buildLogCountCacheKey(prefix string, values ...string) string {
	var builder strings.Builder
	builder.WriteString(prefix)
	for _, value := range values {
		builder.WriteByte(0)
		builder.WriteString(value)
	}
	return builder.String()
}

func logScopeCacheValue(scoped bool, userIDs []int) string {
	if !scoped {
		return "all"
	}
	var builder strings.Builder
	builder.WriteString("scoped:")
	for i, userID := range userIDs {
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.Itoa(userID))
	}
	return builder.String()
}

func refreshCachedLogCount(key string, ttl time.Duration, loader func() (int64, error)) (int64, error) {
	value, err, _ := logCountGroup.Do(key, func() (interface{}, error) {
		count, loadErr := loader()
		if loadErr != nil {
			return int64(0), loadErr
		}

		now := time.Now()
		logCountCache.Lock()
		if len(logCountCache.entries) >= 1024 {
			for cacheKey, cacheEntry := range logCountCache.entries {
				if !now.Before(cacheEntry.expiresAt) {
					delete(logCountCache.entries, cacheKey)
				}
			}
			if len(logCountCache.entries) >= 1024 {
				logCountCache.entries = make(map[string]cachedLogCount)
			}
		}
		logCountCache.entries[key] = cachedLogCount{
			value:     count,
			expiresAt: now.Add(ttl),
		}
		logCountCache.Unlock()
		return count, nil
	})
	if err != nil {
		return 0, err
	}
	return value.(int64), nil
}

func setCachedLogCount(key string, count int64) {
	ttl := logCountCacheTTL()
	if ttl <= 0 {
		return
	}
	now := time.Now()
	logCountCache.Lock()
	logCountCache.entries[key] = cachedLogCount{
		value:     count,
		expiresAt: now.Add(ttl),
	}
	logCountCache.Unlock()
}

func getCachedLogCount(key string, loader func() (int64, error)) (int64, error) {
	ttl := logCountCacheTTL()
	if ttl <= 0 {
		return loader()
	}

	now := time.Now()
	logCountCache.Lock()
	entry, ok := logCountCache.entries[key]
	logCountCache.Unlock()
	if ok {
		if now.Before(entry.expiresAt) {
			return entry.value, nil
		}
		// Stale-While-Revalidate: 10 分钟窗口内立即 0ms 返回上次统计值，并在后台单飞异步更新
		if now.Before(entry.expiresAt.Add(10 * time.Minute)) {
			go func() {
				_, _ = refreshCachedLogCount(key, ttl, loader)
			}()
			return entry.value, nil
		}
	}

	return refreshCachedLogCount(key, ttl, loader)
}
