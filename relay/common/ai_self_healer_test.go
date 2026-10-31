package common

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/grafana/sobek"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAISelfHealer_SandboxAndEvolutionValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_ = os.Remove(lockedFallbackHealScriptPath)
	_ = os.Remove(lockedFallbackHealScriptPath + ".bak")
	defer func() {
		_ = os.Remove(lockedFallbackHealScriptPath)
		_ = os.Remove(lockedFallbackHealScriptPath + ".bak")
	}()

	// 1. 验证严格过滤器：绝不拦截 429/502/503/网络/敏感词错误
	assert.False(t, IsCandidateForAISelfHeal("status_code=429, Too many pending requests"))
	assert.False(t, IsCandidateForAISelfHeal("status_code=503, The model is temporarily overloaded"))
	assert.False(t, IsCandidateForAISelfHeal("status_code=500, sensitive_words_detected"))
	assert.False(t, IsCandidateForAISelfHeal("status_code=500, upstream error: do request failed"))

	// 验证候选参数/协议错误可正常识别
	assert.True(t, IsCandidateForAISelfHeal(`status_code=400, Unknown name "fooBar" at 'request'`))
	assert.True(t, IsCandidateForAISelfHeal(`status_code=500, upstream service request failed`))
	assert.True(t, IsCandidateForAISelfHeal(`status_code=400, Invalid signature in thinking block`))

	// 2. 验证 Sobek 内存沙箱同步自愈未知字段 (Unknown name "fooBar")
	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ApiType:           constant.APITypeGemini,
			UpstreamModelName: "gemini-4-argon",
		},
	}
	rawJSON := []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"fooBar":"unsupported_val"}`)
	healedBytes, ruleID, ok := TryAISelfHealRequest(nil, info, rawJSON, `status_code=400, Unknown name "fooBar" at 'request'`)
	require.True(t, ok)
	assert.Contains(t, ruleID, "strip_unknown_field:fooBar")
	assert.NotContains(t, string(healedBytes), "fooBar")
	assert.Contains(t, string(healedBytes), `"hello"`)

	// 3. 验证安全铁笼拒绝包含危险 Token 的脚本
	badSource := strings.Replace(embeddedHealScriptSeed, healScriptAnchorMarker, "eval('1+1');\n"+healScriptAnchorMarker, 1)
	assert.Error(t, validateHealScriptSource(badSource))

	// 4. 验证模拟 AI 动态演进注入新规则分支、回归测试通过并热编译生效
	aiSnippet := `if (err.indexOf("custom_ frequency_limit") !== -1 && body.customFreq) { delete body.customFreq; markRule("ai_strip_custom_freq"); }`
	evolvedSource := strings.Replace(
		embeddedHealScriptSeed,
		healScriptAnchorMarker,
		"  "+aiSnippet+"\n\n  "+healScriptAnchorMarker,
		1,
	)
	require.NoError(t, validateHealScriptSource(evolvedSource))
	evolvedProg, err := sobek.Compile("ai_auto_heal_patch.js", evolvedSource, false)
	require.NoError(t, err)
	require.NoError(t, runHealScriptRegressionSuite(evolvedProg))

	sampleWithCustomFreq := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"customFreq":999}`)
	healedSample, evolvedRuleID, evolvedOk := executeHealProgramInSandbox(
		evolvedProg,
		sampleWithCustomFreq,
		"gemini",
		"gemini-4-argon",
		"status_code=400, custom_ frequency_limit is not supported",
	)
	require.True(t, evolvedOk)
	assert.Equal(t, "ai_strip_custom_freq", evolvedRuleID)
	assert.NotContains(t, string(healedSample), "customFreq")

	// 5. 端到端测试：通过项目自身 AI 能力 (RegisterInternalAIChannelResolver) 自动演进更新唯一脚本文件并实时生效
	mockAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"{\"ruleId\":\"ai_fix_novel_vendor_flag\",\"jsSnippet\":\"if (err.indexOf(\\\"novel_vendor_flag_unsupported\\\") !== -1 && body.novelVendorFlag !== undefined) { delete body.novelVendorFlag; markRule(\\\"ai_fix_novel_vendor_flag\\\"); }\"}"}]}}]}`)
	}))
	defer mockAIServer.Close()

	RegisterInternalAIChannelResolver(func(group, modelName string) (string, string, bool) {
		return mockAIServer.URL, "mock-internal-key", true
	})

	novelPayload := []byte(`{"contents":[{"role":"user","parts":[{"text":"test novel error"}]}],"novelVendorFlag":true}`)
	novelErr := "status_code=400, novel_vendor_flag_unsupported by upstream"

	// 演进前：尚未覆盖该新型错误
	evolveHealScriptWithInternalAI(aiEvolutionTask{
		protocol: "gemini",
		model:    "gemini-4-argon",
		errMsg:   novelErr,
		rawBytes: novelPayload,
	})

	// 演进后：内存 AST 与唯一文件已实时更新，再次调用 TryAISelfHealRequest 直接命中新规则！
	healedAfterEvolve, ruleAfterEvolve, okAfterEvolve := TryAISelfHealRequest(nil, info, novelPayload, novelErr)
	require.True(t, okAfterEvolve)
	assert.Equal(t, "ai_fix_novel_vendor_flag", ruleAfterEvolve)
	assert.NotContains(t, string(healedAfterEvolve), "novelVendorFlag")
}
