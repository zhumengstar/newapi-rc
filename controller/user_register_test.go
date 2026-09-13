package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterIPLimitEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_ = i18n.Init()

	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	oldRegisterEnabled := common.RegisterEnabled
	oldPasswordRegisterEnabled := common.PasswordRegisterEnabled
	oldMaxUsers := common.MaxUsersPerIP

	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.MaxUsersPerIP = 2

	t.Cleanup(func() {
		common.RegisterEnabled = oldRegisterEnabled
		common.PasswordRegisterEnabled = oldPasswordRegisterEnabled
		common.MaxUsersPerIP = oldMaxUsers
	})

	router := gin.New()
	require.NoError(t, middleware.ConfigureTrustedProxies(router))
	router.POST("/api/user/register", Register)

	registerUser := func(username, password, clientIP string) (int, map[string]any) {
		payload, _ := json.Marshal(map[string]string{
			"username": username,
			"password": password,
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/user/register", bytes.NewReader(payload))
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept-Language", "zh-CN")
		if clientIP != "" {
			req.Header.Set("X-Forwarded-For", clientIP)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}

	ipA := "203.0.113.195"

	// 1. First user registration from ipA
	code, resp := registerUser("test_user_1", "Password12345", ipA)
	require.Equal(t, http.StatusOK, code)
	require.True(t, resp["success"].(bool), "First registration must succeed")

	// 2. Second user registration from ipA
	code, resp = registerUser("test_user_2", "Password12345", ipA)
	require.Equal(t, http.StatusOK, code)
	require.True(t, resp["success"].(bool), "Second registration must succeed")

	// 3. Third user registration from ipA must be rejected
	code, resp = registerUser("test_user_3", "Password12345", ipA)
	require.Equal(t, http.StatusOK, code)
	require.False(t, resp["success"].(bool), "Third registration from same IP must fail")
	require.Contains(t, resp["message"].(string), "该 IP 注册账号数量已达上限")

	// 4. Registration from a different IP must succeed
	ipB := "203.0.113.196"
	code, resp = registerUser("test_user_4", "Password12345", ipB)
	require.Equal(t, http.StatusOK, code)
	require.True(t, resp["success"].(bool), "Registration from different IP must succeed")
}
