package model

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCachedLogCountCoalescesConcurrentLoads(t *testing.T) {
	t.Setenv("LOG_COUNT_CACHE_TTL_SECONDS", "60")
	key := buildLogCountCacheKey("test", t.Name(), time.Now().String())
	var loads atomic.Int32
	loader := func() (int64, error) {
		loads.Add(1)
		time.Sleep(10 * time.Millisecond)
		return 42, nil
	}

	var wg sync.WaitGroup
	values := make([]int64, 16)
	errors := make([]error, 16)
	for i := range values {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			values[index], errors[index] = getCachedLogCount(key, loader)
		}(i)
	}
	wg.Wait()

	for i := range values {
		require.NoError(t, errors[i])
		assert.Equal(t, int64(42), values[i])
	}
	assert.Equal(t, int32(1), loads.Load())

	value, err := getCachedLogCount(key, loader)
	require.NoError(t, err)
	assert.Equal(t, int64(42), value)
	assert.Equal(t, int32(1), loads.Load())
}
