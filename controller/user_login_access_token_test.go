package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLoginAccessTokenTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousPasswordLogin := common.PasswordLoginEnabled
	previousRedis := common.RedisEnabled
	previousSecret := common.SessionSecret

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Log{}, &model.TwoFA{}))
	model.DB, model.LOG_DB = db, db
	common.PasswordLoginEnabled = true
	common.RedisEnabled = false
	common.SessionSecret = "login-access-token-test-secret"

	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.PasswordLoginEnabled = previousPasswordLogin
		common.RedisEnabled = previousRedis
		common.SessionSecret = previousSecret
	})
	return db
}

func invokeLogin(t *testing.T, payload string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	Login(c)
	return recorder
}

func TestLoginFallsBackToValidAccessToken(t *testing.T) {
	db := setupLoginAccessTokenTest(t)
	accessToken := "personal-access-token"
	user := &model.User{
		Username: "token-login-user", Password: "not-a-bcrypt-hash",
		AccessToken: &accessToken, DisplayName: "Token Login User",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(user).Error)

	recorder := invokeLogin(t, `{"username":"token-login-user","password":"wrong-password","access_token":"Bearer personal-access-token"}`)
	assert.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			AccessToken string `json:"access_token"`
			User        struct {
				Username string `json:"username"`
			} `json:"user"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.NotEmpty(t, response.Data.AccessToken)
	assert.Equal(t, user.Username, response.Data.User.Username)

	var session model.UserSession
	require.NoError(t, db.Where("user_id = ?", user.Id).First(&session).Error)
	assert.Equal(t, "access_token", session.LoginMethod)
}

func TestLoginRejectsInvalidAccessTokenAfterPasswordFailure(t *testing.T) {
	db := setupLoginAccessTokenTest(t)
	accessToken := "personal-access-token"
	user := &model.User{
		Username: "token-login-user", Password: "not-a-bcrypt-hash",
		AccessToken: &accessToken, DisplayName: "Token Login User",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(user).Error)

	recorder := invokeLogin(t, `{"username":"token-login-user","password":"wrong-password","access_token":"invalid-token"}`)
	assert.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	var sessions int64
	require.NoError(t, db.Model(&model.UserSession{}).Count(&sessions).Error)
	assert.Zero(t, sessions)
}

func TestLoginFallsBackToDashboardSessionToken(t *testing.T) {
	db := setupLoginAccessTokenTest(t)
	user := &model.User{
		Username: "session-token-login-user", Password: "not-a-bcrypt-hash",
		DisplayName: "Session Token Login User",
		Role:        common.RoleCommonUser, Status: common.UserStatusEnabled,
		Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(user).Error)

	bundle, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "test")
	require.NoError(t, err)
	recorder := invokeLogin(t, `{"username":"session-token-login-user","password":"wrong-password","accessToken":"Bearer `+bundle.AccessToken+`"}`)
	assert.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.NotEmpty(t, response.Data.AccessToken)
}
