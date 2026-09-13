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
	seconds := common.GetEnvOrDefault("LOG_COUNT_CACHE_TTL_SECONDS", 5)
	if seconds < 0 {
		seconds = 0
	}
	return time.Duration(seconds) * time.Second
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

func getCachedLogCount(key string, loader func() (int64, error)) (int64, error) {
	ttl := logCountCacheTTL()
	if ttl <= 0 {
		return loader()
	}

	now := time.Now()
	logCountCache.Lock()
	entry, ok := logCountCache.entries[key]
	logCountCache.Unlock()
	if ok && now.Before(entry.expiresAt) {
		return entry.value, nil
	}

	value, err, _ := logCountGroup.Do(key, func() (interface{}, error) {
		now := time.Now()
		logCountCache.Lock()
		entry, ok := logCountCache.entries[key]
		logCountCache.Unlock()
		if ok && now.Before(entry.expiresAt) {
			return entry.value, nil
		}

		count, loadErr := loader()
		if loadErr != nil {
			return int64(0), loadErr
		}

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
