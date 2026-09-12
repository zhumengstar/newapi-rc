package controller

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

const platformBalanceResponseLimit = 256 << 10

type platformBalanceResponse struct {
	status  int
	body    []byte
	cookies []*http.Cookie
}

func platformRequest(channel *model.Channel, method, path string, headers http.Header, payload any) (platformBalanceResponse, error) {
	base := strings.TrimRight(strings.TrimSpace(channel.GetBaseURL()), "/")
	if base == "" {
		return platformBalanceResponse{}, errors.New("balance base URL is empty")
	}
	var body io.Reader
	if payload != nil {
		encoded, err := common.Marshal(payload)
		if err != nil {
			return platformBalanceResponse{}, err
		}
		body = bytes.NewReader(encoded)
		if headers == nil {
			headers = make(http.Header)
		}
		headers.Set("Content-Type", "application/json")
	}
	req, err := http.NewRequest(method, base+path, body)
	if err != nil {
		return platformBalanceResponse{}, err
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	client, err := service.GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return platformBalanceResponse{}, err
	}
	// Keep the shared proxy client immutable; only this balance probe gets the
	// bounded timeout.
	clientCopy := *client
	clientCopy.Timeout = 20 * time.Second
	client = &clientCopy
	res, err := client.Do(req)
	if err != nil {
		return platformBalanceResponse{}, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, platformBalanceResponseLimit+1))
	if err != nil {
		return platformBalanceResponse{}, err
	}
	if len(data) > platformBalanceResponseLimit {
		return platformBalanceResponse{}, errors.New("balance response is too large")
	}
	return platformBalanceResponse{status: res.StatusCode, body: data, cookies: res.Cookies()}, nil
}

func platformTokenHeaders(token string) http.Header {
	h := make(http.Header)
	token = strings.TrimSpace(token)
	if strings.HasPrefix(strings.ToLower(token), "cookie:") {
		token = strings.TrimSpace(token[len("cookie:"):])
	}
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[len("bearer "):])
	}
	// Only an explicit Cookie prefix is treated as a cookie. Opaque bearer
	// tokens may legitimately contain '=' padding and must not be reclassified.
	if isPlatformCookie(token) {
		h.Set("Cookie", token)
	} else {
		h.Set("Authorization", "Bearer "+token)
	}
	h.Set("Accept", "application/json")
	return h
}

func isPlatformCookie(value string) bool {
	if strings.Contains(value, " ") || !strings.Contains(value, "=") {
		return false
	}
	if strings.Contains(value, ";") {
		return true
	}
	for _, name := range []string{"session", "sessionid", "new_api_session", "access_token"} {
		if strings.HasPrefix(value, name+"=") {
			return true
		}
	}
	return false
}

func platformAuthHeaders(token string, explicitUserID ...string) http.Header {
	h := platformTokenHeaders(token)
	userID := ""
	if len(explicitUserID) > 0 {
		userID = strings.TrimSpace(explicitUserID[0])
	}
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(token), "Bearer "), ".")
	if userID == "" && len(parts) == 3 {
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err == nil {
			var claims struct {
				ID  any `json:"id"`
				Sub any `json:"sub"`
			}
			if common.Unmarshal(payload, &claims) == nil {
				for _, value := range []any{claims.ID, claims.Sub} {
					switch typed := value.(type) {
					case string:
						userID = strings.TrimSpace(typed)
					case float64:
						if typed > 0 && typed == math.Trunc(typed) {
							userID = fmt.Sprintf("%.0f", typed)
						}
					}
					if userID != "" {
						break
					}
				}
			}
		}
	}
	if userID == "" {
		return h
	}
	for _, name := range []string{"New-API-User", "Veloera-User", "voapi-user", "User-id", "Rix-Api-User", "neo-api-user"} {
		h.Set(name, userID)
	}
	return h
}

func platformCookieHeader(cookies []*http.Cookie) string {
	values := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie != nil && cookie.Name != "" {
			values = append(values, cookie.Name+"="+cookie.Value)
		}
	}
	return strings.Join(values, "; ")
}

func extractPlatformToken(body []byte) string {
	var envelope struct {
		Token            string          `json:"token"`
		AccessToken      string          `json:"access_token"`
		AccessTokenCamel string          `json:"accessToken"`
		Data             json.RawMessage `json:"data"`
	}
	if common.Unmarshal(body, &envelope) != nil {
		return ""
	}
	values := []string{envelope.AccessToken, envelope.AccessTokenCamel, envelope.Token}
	if common.GetJsonType(envelope.Data) == "string" {
		var token string
		if common.Unmarshal(envelope.Data, &token) == nil {
			values = append(values, token)
		}
	} else if common.GetJsonType(envelope.Data) == "object" {
		var data struct {
			Token            string `json:"token"`
			AccessToken      string `json:"access_token"`
			AccessTokenCamel string `json:"accessToken"`
		}
		if common.Unmarshal(envelope.Data, &data) == nil {
			values = append(values, data.AccessToken, data.AccessTokenCamel, data.Token)
		}
	}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func platformLoginError(siteType string, response platformBalanceResponse) error {
	var envelope struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
		Detail  string `json:"detail"`
	}
	_ = common.Unmarshal(response.body, &envelope)
	message := strings.TrimSpace(envelope.Message)
	if message == "" {
		message = strings.TrimSpace(envelope.Error)
	}
	if message == "" {
		message = strings.TrimSpace(envelope.Detail)
	}
	if message == "" {
		message = "no token returned"
	}
	if len(message) > 180 {
		message = message[:180]
	}
	return fmt.Errorf("%s login failed (status %d): %s", siteType, response.status, message)
}

func extractPlatformUserID(body []byte) string {
	var envelope struct {
		ID   any `json:"id"`
		User struct {
			ID any `json:"id"`
		} `json:"user"`
		Data struct {
			ID   any `json:"id"`
			User struct {
				ID any `json:"id"`
			} `json:"user"`
		} `json:"data"`
	}
	if common.Unmarshal(body, &envelope) != nil {
		return ""
	}
	for _, value := range []any{envelope.ID, envelope.User.ID, envelope.Data.ID, envelope.Data.User.ID} {
		switch typed := value.(type) {
		case string:
			if userID := strings.TrimSpace(typed); userID != "" {
				return userID
			}
		case float64:
			if typed > 0 && typed == math.Trunc(typed) {
				return fmt.Sprintf("%.0f", typed)
			}
		}
	}
	return ""
}

func parsePlatformBalance(body []byte, siteType string) (float64, error) {
	type balanceNode struct {
		Quota   *float64     `json:"quota"`
		Balance *float64     `json:"balance"`
		Data    *balanceNode `json:"data"`
		User    *balanceNode `json:"user"`
	}
	var envelope balanceNode
	if err := common.Unmarshal(body, &envelope); err != nil {
		return 0, errors.New("invalid balance response")
	}
	nodes := []*balanceNode{&envelope, envelope.Data, envelope.User}
	if envelope.Data != nil {
		nodes = append(nodes, envelope.Data.Data, envelope.Data.User)
	}
	if envelope.User != nil {
		nodes = append(nodes, envelope.User.Data)
	}
	var balance *float64
	if siteType == "sub2api" {
		for _, node := range nodes {
			if node != nil && node.Balance != nil {
				balance = node.Balance
				break
			}
		}
	} else {
		for _, node := range nodes {
			if node != nil && node.Quota != nil {
				value := *node.Quota / 500000
				balance = &value
				break
			}
		}
	}
	if balance == nil {
		return 0, errors.New("balance field is missing")
	}
	if math.IsNaN(*balance) || math.IsInf(*balance, 0) {
		return 0, errors.New("invalid balance value")
	}
	return *balance, nil
}

func isChallengeResponse(body []byte) bool {
	text := strings.ToLower(string(body))
	for _, marker := range []string{"captcha", "verify", "challenge", "turnstile", "请完成验证", "验证码"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func fetchNewAPIBalance(channel *model.Channel, token, username, password string) (float64, error) {
	login := func() (string, string, error) {
		if username == "" || password == "" {
			return "", "", errors.New("NewAPI login credentials are not configured")
		}
		login, err := platformRequest(channel, http.MethodPost, "/api/user/login", http.Header{"X-Requested-With": []string{"XMLHttpRequest"}}, map[string]string{"username": username, "password": password})
		if err != nil {
			return "", "", err
		}
		if login.status >= 200 && login.status < 300 {
			if value := extractPlatformToken(login.body); value != "" {
				return value, extractPlatformUserID(login.body), nil
			}
			if cookie := platformCookieHeader(login.cookies); cookie != "" {
				return cookie, extractPlatformUserID(login.body), nil
			}
		}
		return "", "", platformLoginError("NewAPI", login)
	}
	userID := ""
	if token == "" {
		var loginErr error
		token, userID, loginErr = login()
		if loginErr != nil {
			return 0, loginErr
		}
	}
	requestBalance := func(value, valueUserID string) (float64, int, error) {
		if value == "" {
			return 0, 0, errors.New("NewAPI login/token unavailable")
		}
		response, err := platformRequest(channel, http.MethodGet, "/api/user/self", platformAuthHeaders(value, valueUserID), nil)
		if err != nil {
			return 0, 0, err
		}
		if response.status < 200 || response.status >= 300 {
			return 0, response.status, fmt.Errorf("NewAPI balance status: %d", response.status)
		}
		balance, err := parsePlatformBalance(response.body, "newapi")
		return balance, response.status, err
	}
	balance, status, err := requestBalance(token, userID)
	if err == nil {
		return balance, nil
	}
	// A configured API token may actually be a cookie value. Retry the common
	// cookie names before treating the credential as invalid.
	if status == http.StatusUnauthorized && !strings.Contains(token, "=") {
		for _, cookieName := range []string{"session", "token", "access_token"} {
			if retryBalance, _, retryErr := requestBalance(cookieName+"="+token, userID); retryErr == nil {
				return retryBalance, nil
			}
		}
	}
	// Prefer credentials for normal sites, but fall back to a manually entered
	// token when login is blocked by CAPTCHA or a challenge page.
	if username != "" && password != "" {
		if loginToken, loginUserID, loginErr := login(); loginErr == nil && loginToken != "" && loginToken != token {
			if retryBalance, _, retryErr := requestBalance(loginToken, loginUserID); retryErr == nil {
				return retryBalance, nil
			}
		}
	}
	return 0, err
}

func fetchSub2APIBalance(channel *model.Channel, token, username, password string) (float64, error) {
	if token == "" && username != "" && password != "" {
		login, err := platformRequest(channel, http.MethodPost, "/api/v1/auth/login", http.Header{}, map[string]string{"email": username, "password": password})
		if err != nil {
			return 0, err
		}
		if login.status >= 200 && login.status < 300 {
			token = extractPlatformToken(login.body)
		} else {
			return 0, platformLoginError("Sub2API", login)
		}
	}
	if token == "" {
		return 0, errors.New("Sub2API login succeeded but returned no access token")
	}
	response, err := platformRequest(channel, http.MethodGet, "/api/v1/auth/me", platformTokenHeaders(token), nil)
	if err != nil {
		return 0, err
	}
	if response.status < 200 || response.status >= 300 {
		return 0, fmt.Errorf("Sub2API balance status: %d", response.status)
	}
	return parsePlatformBalance(response.body, "sub2api")
}

func updatePlatformChannelBalance(channel *model.Channel) (float64, error) {
	channel.NormalizeBalanceSettings()
	token, username, password, credentialErr := channel.GetBalanceCredentialsWithError()
	if credentialErr != nil {
		return 0, credentialErr
	}
	configuredToken := token
	loginConfigured := username != "" && password != ""
	if loginConfigured {
		// Prefer a normal login; a token remains available as a CAPTCHA fallback.
		token = ""
	}
	siteType := ""
	if channel.SiteType != nil {
		siteType = strings.ToLower(strings.TrimSpace(*channel.SiteType))
	}
	if siteType == "" || siteType == "unknown" {
		if channel.Type == constant.ChannelTypeSub2API {
			siteType = "sub2api"
		} else {
			siteType = "newapi"
		}
	}
	var balance float64
	var err error
	if siteType == "sub2api" {
		balance, err = fetchSub2APIBalance(channel, token, username, password)
	} else {
		balance, err = fetchNewAPIBalance(channel, token, username, password)
	}
	// Login is preferred when configured. The dedicated balance access token is
	// retained as a CAPTCHA fallback; the inference channel key is deliberately
	// never used for site-account balance queries.
	if err != nil {
		fallbackToken := configuredToken
		if fallbackToken != "" {
			if siteType == "sub2api" {
				balance, err = fetchSub2APIBalance(channel, fallbackToken, "", "")
			} else {
				balance, err = fetchNewAPIBalance(channel, fallbackToken, "", "")
			}
		}
	}
	// Some local deployments provision site accounts with username == password.
	// Candidate usernames are opt-in through CHANNEL_BALANCE_LOGIN_CANDIDATES;
	// persist a candidate only after login and balance retrieval both succeed.
	if err != nil {
		for _, candidate := range strings.Split(os.Getenv("CHANNEL_BALANCE_LOGIN_CANDIDATES"), ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" || candidate == username {
				continue
			}
			var candidateBalance float64
			var candidateErr error
			if siteType == "sub2api" {
				candidateBalance, candidateErr = fetchSub2APIBalance(channel, "", candidate, candidate)
			} else {
				candidateBalance, candidateErr = fetchNewAPIBalance(channel, "", candidate, candidate)
			}
			if candidateErr != nil {
				continue
			}
			if setErr := channel.SetBalanceCredentials("", candidate, candidate); setErr == nil {
				_ = model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
					"balance_access_token": channel.BalanceAccessToken,
					"balance_username":     channel.BalanceUsername,
					"balance_password":     channel.BalancePassword,
				}).Error
				model.InitChannelCache()
			}
			err = nil
			balance = candidateBalance
			break
		}
	}
	if err == nil {
		channel.UpdateBalance(balance)
	}
	return balance, err
}
