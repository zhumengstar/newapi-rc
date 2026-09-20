package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetGeneratedImageAsset(t *testing.T) {
	gin.SetMode(gin.TestMode)

	date := "20260920"
	filename := "17d63f73631d4f5c8b037fcbf9eb94f1.png"

	targetPath := service.GeneratedImageAssetFilePath(date, filename)
	require.NoError(t, os.MkdirAll(filepath.Dir(targetPath), 0755))
	dummyContent := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	require.NoError(t, os.WriteFile(targetPath, dummyContent, 0644))
	t.Cleanup(func() {
		_ = os.Remove(targetPath)
	})

	router := gin.New()
	router.GET("/api/log/generated-images/:date/:filename", GetGeneratedImageAsset)

	// Case 1: Browser <img> tag request (No Authorization header, anonymous) -> 200 OK with Cache-Control
	req := httptest.NewRequest(http.MethodGet, "/api/log/generated-images/"+date+"/"+filename, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "public, max-age=86400", w.Header().Get("Cache-Control"))
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Equal(t, dummyContent, w.Body.Bytes())

	// Case 2: Download mode -> 200 OK with Content-Disposition attachment
	reqDownload := httptest.NewRequest(http.MethodGet, "/api/log/generated-images/"+date+"/"+filename+"?download=1", nil)
	wDownload := httptest.NewRecorder()
	router.ServeHTTP(wDownload, reqDownload)
	assert.Equal(t, http.StatusOK, wDownload.Code)
	assert.Contains(t, wDownload.Header().Get("Content-Disposition"), "attachment")

	// Case 3: Invalid date pattern -> 404
	reqBadDate := httptest.NewRequest(http.MethodGet, "/api/log/generated-images/invalid-date/"+filename, nil)
	wBadDate := httptest.NewRecorder()
	router.ServeHTTP(wBadDate, reqBadDate)
	assert.Equal(t, http.StatusNotFound, wBadDate.Code)

	// Case 4: Invalid filename pattern / Path traversal -> 404
	reqBadFile := httptest.NewRequest(http.MethodGet, "/api/log/generated-images/"+date+"/../secret.txt", nil)
	wBadFile := httptest.NewRecorder()
	router.ServeHTTP(wBadFile, reqBadFile)
	assert.Equal(t, http.StatusNotFound, wBadFile.Code)

	// Case 5: Non-existent file -> 404
	reqNotFound := httptest.NewRequest(http.MethodGet, "/api/log/generated-images/"+date+"/00000000000000000000000000000000.png", nil)
	wNotFound := httptest.NewRecorder()
	router.ServeHTTP(wNotFound, reqNotFound)
	assert.Equal(t, http.StatusNotFound, wNotFound.Code)
}
