package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func CanvasTokenNameAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := c.GetInt("id")
		tokenName := strings.TrimSpace(c.GetHeader("X-NewAPI-Token-Name"))
		if tokenName == "" {
			tokenName = strings.TrimSpace(c.GetHeader("X-New-Api-Token-Name"))
		}
		if tokenName == "" {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "missing X-NewAPI-Token-Name header")
			return
		}

		token, err := model.GetUsableTokenByName(userId, tokenName)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				abortWithOpenAiMessage(c, http.StatusUnauthorized, "token name is invalid or unavailable")
			} else {
				common.SysLog("CanvasTokenNameAuth GetUsableTokenByName error: " + err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, common.TranslateMessage(c, i18n.MsgDatabaseError))
			}
			return
		}

		c.Request.Header.Set("Authorization", "Bearer "+token.GetFullKey())
		c.Next()
	}
}
