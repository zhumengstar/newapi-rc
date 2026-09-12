package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestFetchNewAPIBalancePrefersLogin(t *testing.T) {
	loginCalled, tokenSeen := false, ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/user/login" {
			loginCalled = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"access_token":"login-token"}}`))
			return
		}
		if r.URL.Path == "/api/user/self" {
			tokenSeen = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"quota":1250000,"used_quota":500000}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	base := server.URL
	channel := &model.Channel{BaseURL: &base}
	balance, err := fetchNewAPIBalance(channel, "", "user", "pass")
	require.NoError(t, err)
	require.Equal(t, 2.5, balance)
	require.True(t, loginCalled)
	require.Equal(t, "Bearer login-token", tokenSeen)
}

func TestFetchNewAPIBalanceReportsLoginMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/user/login" {
			_, _ = w.Write([]byte(`{"success":false,"message":"password is incorrect"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	base := server.URL
	_, err := fetchNewAPIBalance(&model.Channel{BaseURL: &base}, "", "user", "pass")
	require.EqualError(t, err, "NewAPI login failed (status 200): password is incorrect")
}

func TestExtractPlatformTokenAcceptsStringData(t *testing.T) {
	require.Equal(t, "string-token", extractPlatformToken([]byte(`{"success":true,"data":"string-token"}`)))
}

func TestFetchSub2APIBalanceWithToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer configured", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"code":0,"data":{"balance":3.75}}`))
	}))
	defer server.Close()
	base := server.URL
	balance, err := fetchSub2APIBalance(&model.Channel{BaseURL: &base}, "configured", "", "")
	require.NoError(t, err)
	require.Equal(t, 3.75, balance)
}

func TestFetchNewAPIBalanceRetriesCookieAndUserHeaders(t *testing.T) {
	requests := 0
	token := "eyJhbGciOiJub25lIn0.eyJpZCI6NDJ9.signature"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/self" {
			http.NotFound(w, r)
			return
		}
		requests++
		if requests == 1 {
			require.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Cookie") != "session="+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		require.Equal(t, "42", r.Header.Get("New-API-User"))
		_, _ = w.Write([]byte(`{"data":{"quota":500000}}`))
	}))
	defer server.Close()
	base := server.URL
	// The token payload contains an id claim used by NewAPI-compatible deployments.
	balance, err := fetchNewAPIBalance(&model.Channel{BaseURL: &base}, token, "", "")
	require.NoError(t, err)
	require.Equal(t, 1.0, balance)
	require.Equal(t, 2, requests)
}

func TestFetchNewAPIBalanceUsesLoginResponseUserIDForOpaqueToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			_, _ = w.Write([]byte(`{"data":{"access_token":"opaque-token","user":{"id":27}}}`))
		case "/api/user/self":
			require.Equal(t, "Bearer opaque-token", r.Header.Get("Authorization"))
			require.Equal(t, "27", r.Header.Get("New-API-User"))
			_, _ = w.Write([]byte(`{"data":{"quota":750000}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base := server.URL
	balance, err := fetchNewAPIBalance(&model.Channel{BaseURL: &base}, "", "user", "pass")
	require.NoError(t, err)
	require.Equal(t, 1.5, balance)
}

func TestParseSub2APIBalanceFromNestedUser(t *testing.T) {
	balance, err := parsePlatformBalance([]byte(`{"data":{"user":{"balance":6.25}}}`), "sub2api")
	require.NoError(t, err)
	require.Equal(t, 6.25, balance)
}

func TestParseSub2APIBalanceAllowsNegativeAccountBalance(t *testing.T) {
	balance, err := parsePlatformBalance([]byte(`{"data":{"balance":-0.25}}`), "sub2api")
	require.NoError(t, err)
	require.Equal(t, -0.25, balance)
}

func TestParseNewAPIBalanceAllowsNegativeQuota(t *testing.T) {
	balance, err := parsePlatformBalance([]byte(`{"data":{"quota":-125000}}`), "newapi")
	require.NoError(t, err)
	require.Equal(t, -0.25, balance)
}

func TestParsePlatformBalanceRejectsMissingField(t *testing.T) {
	_, err := parsePlatformBalance([]byte(`{"success":false,"message":"unauthorized"}`), "newapi")
	require.Error(t, err)
}

func TestUpdatePlatformChannelBalanceDoesNotUseChannelKey(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	baseURL := server.URL
	siteType := channelPlatformNewAPI
	channel := &model.Channel{
		BaseURL:  &baseURL,
		SiteType: &siteType,
		Key:      "inference-key-must-not-be-used",
	}

	_, err := updatePlatformChannelBalance(channel)
	require.Error(t, err)
	require.Zero(t, requests)
}

func TestChannelBalanceSecretRoundTrip(t *testing.T) {
	previous := common.CryptoSecret
	common.CryptoSecret = "test-secret"
	defer func() { common.CryptoSecret = previous }()
	channel := &model.Channel{}
	require.NoError(t, channel.SetBalanceCredentials("token", "user", "password"))
	token, username, password := channel.GetBalanceCredentials()
	require.Equal(t, "token", token)
	require.Equal(t, "user", username)
	require.Equal(t, "password", password)
	require.NotEqual(t, "token", channel.BalanceAccessToken)
}
