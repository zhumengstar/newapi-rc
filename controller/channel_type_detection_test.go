package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectChannelPlatformSub2API(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/me" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"UNAUTHORIZED","message":"Authorization header is required"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	result, err := detectChannelPlatform(context.Background(), server.Client(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, channelPlatformSub2API, result.Type)
	assert.Equal(t, "high", result.Confidence)
}

func TestDetectChannelPlatformNewAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/user/self" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":1}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	result, err := detectChannelPlatform(context.Background(), server.Client(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, channelPlatformNewAPI, result.Type)
	assert.Equal(t, "high", result.Confidence)
}

func TestDetectChannelPlatformNewAPIFromStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/status" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"system_name":"Example","version":"1.0.0"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	result, err := detectChannelPlatform(context.Background(), server.Client(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, channelPlatformNewAPI, result.Type)
	assert.Equal(t, "high", result.Confidence)
}

func TestDetectChannelPlatformSub2APILoggedOutVariant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/me" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false,"message":"未登录"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	result, err := detectChannelPlatform(context.Background(), server.Client(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, channelPlatformSub2API, result.Type)
	assert.Equal(t, "high", result.Confidence)
}

func TestDetectChannelPlatformDoesNotMisclassifyGenericUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	result, err := detectChannelPlatform(context.Background(), server.Client(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, channelPlatformUnknown, result.Type)
	assert.Equal(t, "none", result.Confidence)
}

func TestNormalizeChannelDetectionURL(t *testing.T) {
	_, err := normalizeChannelDetectionURL("ftp://example.com")
	require.Error(t, err)
	_, err = normalizeChannelDetectionURL("https://example.com/path?token=secret")
	require.Error(t, err)
	baseURL, err := normalizeChannelDetectionURL(" https://example.com/path/// ")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/path", baseURL)
}
