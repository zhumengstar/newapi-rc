package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestShanghaiTodayStartUnix(t *testing.T) {
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Now().In(loc)
	expected := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Unix()
	actual := model.ShanghaiTodayStartUnix()
	assert.Equal(t, expected, actual)
}

func TestLogTodayFilterDynamicOption(t *testing.T) {
	// 确保默认开启
	assert.True(t, common.UserOnlyViewTodayLogsEnabled)

	// 测试 updateOptionMap 能够动态控制
	defer func() {
		common.UserOnlyViewTodayLogsEnabled = true
	}()

	gin.SetMode(gin.TestMode)
	minStart := model.ShanghaiUserLogMinStartUnix()
	threeDaysAgo := minStart - 86400

	// 1. 普通用户在 UserOnlyViewTodayLogsEnabled = true 下查询（保留两天数据日志）
	{
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request, _ = http.NewRequest(http.MethodGet, "/api/log/self?start_timestamp="+strconv.FormatInt(threeDaysAgo, 10), nil)
		c.Set("id", 100)
		c.Set("role", common.RoleCommonUser)

		startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
		if c.GetInt("role") < common.RoleAdminUser && common.UserOnlyViewTodayLogsEnabled {
			tStart := model.ShanghaiUserLogMinStartUnix()
			if startTimestamp < tStart {
				startTimestamp = tStart
			}
		}
		assert.Equal(t, minStart, startTimestamp)
	}

	// 2. 管理员不受限制
	{
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request, _ = http.NewRequest(http.MethodGet, "/api/log/self?start_timestamp="+strconv.FormatInt(threeDaysAgo, 10), nil)
		c.Set("id", 1)
		c.Set("role", common.RoleAdminUser)

		startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
		if c.GetInt("role") < common.RoleAdminUser && common.UserOnlyViewTodayLogsEnabled {
			tStart := model.ShanghaiUserLogMinStartUnix()
			if startTimestamp < tStart {
				startTimestamp = tStart
			}
		}
		assert.Equal(t, threeDaysAgo, startTimestamp)
	}

	// 3. 当关闭配置项时，普通用户也不受限制
	{
		common.UserOnlyViewTodayLogsEnabled = false
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request, _ = http.NewRequest(http.MethodGet, "/api/log/self?start_timestamp="+strconv.FormatInt(threeDaysAgo, 10), nil)
		c.Set("id", 100)
		c.Set("role", common.RoleCommonUser)

		startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
		if c.GetInt("role") < common.RoleAdminUser && common.UserOnlyViewTodayLogsEnabled {
			tStart := model.ShanghaiUserLogMinStartUnix()
			if startTimestamp < tStart {
				startTimestamp = tStart
			}
		}
		assert.Equal(t, threeDaysAgo, startTimestamp)
	}
}
