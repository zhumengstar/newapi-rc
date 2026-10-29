package relay

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompatibleHandler_CrossProtocolPassThroughProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Gemini channel with PassThroughBodyEnabled converts OpenAI request to contents", func(t *testing.T) {
		receivedBodyChan := make(chan []byte, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			receivedBodyChan <- body
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, `{"error":{"message":"mock upstream error"}}`)
		}))
		defer server.Close()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		openaiPayload := `{"model":"gemini-4-argon","messages":[{"role":"user","content":"hello"}]}`
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(openaiPayload))
		c.Request.Header.Set("Content-Type", "application/json")

		common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeGemini)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
		common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
			PassThroughBodyEnabled: true, // Enabled, but must NOT bypass conversion for Gemini channel!
		})

		openaiReq, err := helper.GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
		require.NoError(t, err)

		info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, openaiReq, nil)
		require.NoError(t, err)

		_ = TextHelper(c, info)

		select {
		case bodyBytes := <-receivedBodyChan:
			var parsed map[string]any
			err := json.Unmarshal(bodyBytes, &parsed)
			require.NoError(t, err)

			// Upstream must receive "contents", NOT raw "messages"!
			assert.Contains(t, parsed, "contents", "Gemini upstream MUST receive 'contents'")
			assert.NotContains(t, parsed, "messages", "Gemini upstream MUST NOT receive raw OpenAI 'messages'")
		default:
			t.Fatal("upstream server did not receive request")
		}
	})

	t.Run("OpenAI channel with PassThroughBodyEnabled preserves raw body pass-through", func(t *testing.T) {
		receivedBodyChan := make(chan []byte, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			receivedBodyChan <- body
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, `{"error":{"message":"mock upstream error"}}`)
		}))
		defer server.Close()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		openaiPayload := `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"custom_vendor_param":"xyz"}`
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(openaiPayload))
		c.Request.Header.Set("Content-Type", "application/json")

		common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
		common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
			PassThroughBodyEnabled: true,
		})

		openaiReq, err := helper.GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
		require.NoError(t, err)

		info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, openaiReq, nil)
		require.NoError(t, err)

		_ = TextHelper(c, info)

		select {
		case bodyBytes := <-receivedBodyChan:
			var parsed map[string]any
			err := json.Unmarshal(bodyBytes, &parsed)
			require.NoError(t, err)

			// OpenAI upstream must receive raw pass-through body including custom params
			assert.Contains(t, parsed, "messages")
			assert.Equal(t, "xyz", parsed["custom_vendor_param"])
		default:
			t.Fatal("upstream server did not receive request")
		}
	})
}

