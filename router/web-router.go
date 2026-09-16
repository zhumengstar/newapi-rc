package router

import (
	"embed"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS   embed.FS
	IndexPage []byte
}

func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatcher gin.HandlerFunc) {
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")
	canvasIndex, _ := assets.BuildFS.ReadFile("web/dist/canvas-app/index.html")

	router.NoRoute(
		pluginDispatcher,
		middleware.RouteTag("web"),
		gzip.Gzip(gzip.DefaultCompression),
		middleware.AccessTokenAudit(),
		middleware.GlobalWebRateLimit(),
		middleware.Cache(),
		func(c *gin.Context) {
			if c.Request.Method == http.MethodOptions {
				c.Status(http.StatusNoContent)
				c.Abort()
				return
			}
			reqPath := c.Request.URL.Path
			if reqPath == "/canvas-app" || reqPath == "/canvas-app/" {
				if len(canvasIndex) > 0 {
					c.Header("Cache-Control", "no-cache")
					c.Data(http.StatusOK, "text/html; charset=utf-8", canvasIndex)
					c.Abort()
					return
				}
			}
			if strings.HasSuffix(reqPath, ".js") || strings.HasSuffix(reqPath, ".mjs") {
				c.Header("Content-Type", "application/javascript; charset=utf-8")
			} else if strings.HasSuffix(reqPath, ".css") {
				c.Header("Content-Type", "text/css; charset=utf-8")
			}
		},
		static.Serve("/", frontendFS),
		func(c *gin.Context) {
			if c.Request.Method == http.MethodOptions {
				c.Status(http.StatusNoContent)
				c.Abort()
				return
			}
			if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
				controller.RelayNotFound(c)
				return
			}
			if strings.HasPrefix(c.Request.URL.Path, "/canvas-app") && len(canvasIndex) > 0 {
				c.Header("Cache-Control", "no-cache")
				c.Data(http.StatusOK, "text/html; charset=utf-8", canvasIndex)
				return
			}
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/html; charset=utf-8", assets.IndexPage)
		},
	)
}
