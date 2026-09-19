package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

var userRPMScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return count
`)

var userMPMScript = redis.NewScript(`
local delta = tonumber(ARGV[1])
local count = redis.call('INCRBY', KEYS[1], delta)
if tonumber(count) == delta then
    redis.call('EXPIRE', KEYS[1], ARGV[2])
end
return count
`)

var userRPMMemory = struct {
	sync.RWMutex
	counts map[int]map[int64]int64 // userID -> (minuteWindow -> count)
}{
	counts: make(map[int]map[int64]int64),
}

var userMPMMemory = struct {
	sync.RWMutex
	counts map[int]map[int64]int64 // userID -> (minuteWindow -> consumedQuota)
}{
	counts: make(map[int]map[int64]int64),
}

// RecordUserRPM 原子递增用户在当前分钟的请求计数
func RecordUserRPM(userID int) {
	if userID <= 0 {
		return
	}
	now := time.Now().Unix()
	currMin := now / 60

	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("user:rpm:%d:%d", userID, currMin)
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_, _ = userRPMScript.Run(ctx, common.RDB, []string{key}, 130).Result()
		return
	}

	// 内存降级路径
	userRPMMemory.Lock()
	defer userRPMMemory.Unlock()

	userWindows, ok := userRPMMemory.counts[userID]
	if !ok {
		userWindows = make(map[int64]int64)
		userRPMMemory.counts[userID] = userWindows
	}

	for win := range userWindows {
		if win < currMin-1 {
			delete(userWindows, win)
		}
	}
	userWindows[currMin]++
}

// RecordUserMPM 原子累加用户在当前分钟的消耗 Quota
func RecordUserMPM(userID int, quota int64) {
	if userID <= 0 || quota <= 0 {
		return
	}
	now := time.Now().Unix()
	currMin := now / 60

	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("user:mpm:%d:%d", userID, currMin)
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_, _ = userMPMScript.Run(ctx, common.RDB, []string{key}, quota, 130).Result()
		return
	}

	// 内存降级路径
	userMPMMemory.Lock()
	defer userMPMMemory.Unlock()

	userWindows, ok := userMPMMemory.counts[userID]
	if !ok {
		userWindows = make(map[int64]int64)
		userMPMMemory.counts[userID] = userWindows
	}

	for win := range userWindows {
		if win < currMin-1 {
			delete(userWindows, win)
		}
	}
	userWindows[currMin] += quota
}

// GetUsersRPMAndMPM 批量通过滑动窗口平滑加权获取用户最近一分钟的请求速率 RPM 和消耗 Quota MPM
func GetUsersRPMAndMPM(userIDs []int) (map[int]int64, map[int]int64) {
	rpmMap := make(map[int]int64, len(userIDs))
	mpmMap := make(map[int]int64, len(userIDs))
	if len(userIDs) == 0 {
		return rpmMap, mpmMap
	}

	now := time.Now().Unix()
	currMin := now / 60
	prevMin := currMin - 1
	secInMin := now % 60
	prevWeight := float64(60-secInMin) / 60.0

	// 1. 优先使用 Redis 批量单个 MGet 读取所有用户的 RPM 和 MPM 窗口
	if common.RedisEnabled && common.RDB != nil {
		keys := make([]string, 0, len(userIDs)*4)
		for _, id := range userIDs {
			// 每个用户 4 个 key: rpmCurr, rpmPrev, mpmCurr, mpmPrev
			keys = append(keys,
				fmt.Sprintf("user:rpm:%d:%d", id, currMin),
				fmt.Sprintf("user:rpm:%d:%d", id, prevMin),
				fmt.Sprintf("user:mpm:%d:%d", id, currMin),
				fmt.Sprintf("user:mpm:%d:%d", id, prevMin),
			)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		vals, err := common.RDB.MGet(ctx, keys...).Result()
		if err == nil && len(vals) == len(keys) {
			for i, id := range userIDs {
				parseVal := func(val any) int64 {
					if val != nil {
						if s, ok := val.(string); ok {
							if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
								return parsed
							}
						}
					}
					return 0
				}

				base := 4 * i
				rpmCurr := parseVal(vals[base])
				rpmPrev := parseVal(vals[base+1])
				mpmCurr := parseVal(vals[base+2])
				mpmPrev := parseVal(vals[base+3])

				rpmMap[id] = rpmCurr + int64(math.Round(float64(rpmPrev)*prevWeight))
				mpmMap[id] = mpmCurr + int64(math.Round(float64(mpmPrev)*prevWeight))
			}
			return rpmMap, mpmMap
		}
	}

	// 2. 内存降级读取
	userRPMMemory.RLock()
	for _, id := range userIDs {
		if userWindows, ok := userRPMMemory.counts[id]; ok {
			rpmMap[id] = userWindows[currMin] + int64(math.Round(float64(userWindows[prevMin])*prevWeight))
		} else {
			rpmMap[id] = 0
		}
	}
	userRPMMemory.RUnlock()

	userMPMMemory.RLock()
	for _, id := range userIDs {
		if userWindows, ok := userMPMMemory.counts[id]; ok {
			mpmMap[id] = userWindows[currMin] + int64(math.Round(float64(userWindows[prevMin])*prevWeight))
		} else {
			mpmMap[id] = 0
		}
	}
	userMPMMemory.RUnlock()

	return rpmMap, mpmMap
}

// GetUsersRPM 兼容旧接口
func GetUsersRPM(userIDs []int) map[int]int64 {
	rpmMap, _ := GetUsersRPMAndMPM(userIDs)
	return rpmMap
}
