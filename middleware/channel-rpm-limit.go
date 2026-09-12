package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const channelRPMWindow = time.Minute

var channelRPMMemory = struct {
	sync.Mutex
	counts map[string]channelRPMEntry
}{counts: make(map[string]channelRPMEntry)}

var channelRPMRedisScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then redis.call('EXPIRE', KEYS[1], 61) end
if count > tonumber(ARGV[1]) then
  redis.call('DECR', KEYS[1])
  return 0
end
return 1
`)

type channelRPMEntry struct {
	window int64
	count  int
}

// AllowChannelRPM atomically admits one request for a channel in a fixed
// one-minute window. A zero limit
// disables the check. Redis makes the limit effective across all instances;
// the in-memory path is used when Redis is unavailable.
func AllowChannelRPM(ctx *gin.Context, channelID, limit int) bool {
	if limit <= 0 || channelID <= 0 {
		return true
	}
	window := time.Now().Unix() / int64(channelRPMWindow.Seconds())
	key := fmt.Sprintf("channel:rpm:%d:%d", channelID, window)
	if common.RedisEnabled && common.RDB != nil {
		allowed, err := channelRPMRedisScript.Run(context.Background(), common.RDB, []string{key}, limit).Int()
		if err == nil {
			return allowed == 1
		}
	}

	channelRPMMemory.Lock()
	defer channelRPMMemory.Unlock()
	for key, entry := range channelRPMMemory.counts {
		if entry.window < window-1 {
			delete(channelRPMMemory.counts, key)
		}
	}
	entry := channelRPMMemory.counts[key]
	if entry.window != window {
		entry = channelRPMEntry{window: window}
	}
	if entry.count >= limit {
		return false
	}
	entry.count++
	channelRPMMemory.counts[key] = entry
	return true
}

func enforceChannelRPM(c *gin.Context, channelID, limit int) bool {
	if AllowChannelRPM(c, channelID, limit) {
		return true
	}
	abortWithOpenAiMessage(c, http.StatusTooManyRequests,
		"channel request rate limit exceeded: maximum "+strconv.Itoa(limit)+" requests per minute")
	return false
}
