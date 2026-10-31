package common

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/gin-gonic/gin"
	"github.com/grafana/sobek"
)

// InternalAIChannelResolver 由上层 relay 包注入项目自身渠道池查询能力，避免包循环引用
type InternalAIChannelResolver func(group, modelName string) (baseURL, apiKey string, ok bool)

var internalAIChannelResolver atomic.Value

func RegisterInternalAIChannelResolver(fn InternalAIChannelResolver) {
	if fn != nil {
		internalAIChannelResolver.Store(fn)
	}
}

//go:embed ai_auto_heal_patch.js
var embeddedHealScriptSeed string

// 物理硬编码锁定唯一允许 AI 更新的代码文件路径（严禁修改项目任何其他文件）
// 生产容器内 /data 挂载自宿主机 /opt/new-api/data，实现蓝绿双容器共享与永久持久化
const (
	lockedProdHealScriptPath     = "/data/ai_auto_heal_patch.js"
	lockedFallbackHealScriptPath = "/tmp/ai_auto_heal_patch.js"
	healScriptAnchorMarker       = "// === [AI_DYNAMIC_RULES_ANCHOR] ==="
	sandboxExecTimeout           = 15 * time.Millisecond
	aiEvolutionCooldown          = 10 * time.Minute
	aiEvolutionHTTPTimeout       = 12 * time.Second
)

type compiledHealScript struct {
	source  string
	program *sobek.Program
	modTime time.Time
}

type aiEvolutionTask struct {
	protocol string
	model    string
	errMsg   string
	rawBytes []byte
}

var (
	activeHealScript    atomic.Pointer[compiledHealScript]
	healInitOnce        sync.Once
	healFileWriteMu     sync.Mutex
	evolutionCooldownMu sync.Map // fingerprint -> time.Time
	evolutionQueue      = make(chan aiEvolutionTask, 4)
	digitNormRegex      = regexp.MustCompile(`\b\d+\b`)
	idNormRegex         = regexp.MustCompile(`(?i)(request[_ -]?id|id|req_[a-z0-9_]+)[:= ]*[a-zA-Z0-9_-]+`)
)

func initAISelfHealer() {
	healInitOnce.Do(func() {
		source := embeddedHealScriptSeed
		var mtime time.Time
		targetPath := resolveLockedHealScriptPath()
		if data, err := os.ReadFile(targetPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
			if strings.Contains(string(data), "function autoHealRequest") && strings.Contains(string(data), healScriptAnchorMarker) {
				source = string(data)
				if st, sErr := os.Stat(targetPath); sErr == nil {
					mtime = st.ModTime()
				}
			}
		} else {
			_ = writeLockedHealScriptFile(source)
			if st, sErr := os.Stat(targetPath); sErr == nil {
				mtime = st.ModTime()
			}
		}

		if prog, err := sobek.Compile("ai_auto_heal_patch.js", source, false); err == nil {
			activeHealScript.Store(&compiledHealScript{
				source:  source,
				program: prog,
				modTime: mtime,
			})
		}

		go backgroundEvolutionWorker()
		go backgroundScriptHotReloadWatcher()
	})
}

// resolveLockedHealScriptPath 仅返回硬编码白名单中的唯一自愈脚本路径
func resolveLockedHealScriptPath() string {
	if st, err := os.Stat("/data"); err == nil && st.IsDir() {
		return lockedProdHealScriptPath
	}
	return lockedFallbackHealScriptPath
}

// writeLockedHealScriptFile 物理锁定只允许写入唯一的 ai_auto_heal_patch.js 文件，绝不触碰任何其他文件
func writeLockedHealScriptFile(newSource string) error {
	healFileWriteMu.Lock()
	defer healFileWriteMu.Unlock()

	targetPath := resolveLockedHealScriptPath()
	if filepath.Base(targetPath) != "ai_auto_heal_patch.js" {
		return errors.New("security violation: target file is not ai_auto_heal_patch.js")
	}

	if oldBytes, err := os.ReadFile(targetPath); err == nil && len(oldBytes) > 0 {
		_ = os.WriteFile(targetPath+".bak", oldBytes, 0644)
	}

	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(newSource), 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, targetPath)
}

func backgroundScriptHotReloadWatcher() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		targetPath := resolveLockedHealScriptPath()
		st, err := os.Stat(targetPath)
		if err != nil {
			continue
		}
		cur := activeHealScript.Load()
		if cur != nil && !st.ModTime().After(cur.modTime) {
			continue
		}
		data, err := os.ReadFile(targetPath)
		if err != nil || len(data) == 0 {
			continue
		}
		src := string(data)
		if err := validateHealScriptSource(src); err != nil {
			continue
		}
		prog, err := sobek.Compile("ai_auto_heal_patch.js", src, false)
		if err != nil {
			continue
		}
		if err := runHealScriptRegressionSuite(prog); err != nil {
			continue
		}
		activeHealScript.Store(&compiledHealScript{
			source:  src,
			program: prog,
			modTime: st.ModTime(),
		})
	}
}

// IsCandidateForAISelfHeal 严格过滤：仅对未知参数/协议/格式/Schema 报错返回 true，
// 对 429 限流、502/503/504 过载或网络故障、401/403 鉴权、敏感词拦截直接返回 false（0 开销）
func IsCandidateForAISelfHeal(errStr string) bool {
	if len(errStr) == 0 {
		return false
	}
	lower := strings.ToLower(errStr)

	// 排除非参数类错误（限流、过载、网络断开、配额、鉴权、内容审查）
	nonParamKeywords := []string{
		"429", "rate limit", "rpm", "tpm", "too many", "quota", "冷却", "cooldown",
		"overloaded", "unavailable", "bad gateway", "gateway timeout", "502", "503", "504",
		"do request failed", "context canceled", "client_gone", "eof", "connection reset",
		"unauthorized", "invalid api key", "permission denied", "401", "403",
		"sensitive_words_detected", "prohibited_content", "image_safety", "排队等待", "cpu 负载",
		"候选均请求失败",
	}
	for _, kw := range nonParamKeywords {
		if strings.Contains(lower, kw) {
			return false
		}
	}

	// 匹配参数、字段、Schema、角色、签名或协议格式报错
	paramKeywords := []string{
		"400", "422", "invalid", "unknown name", "cannot find field", "unrecognized",
		"unsupported", "required", "must be", "must end with", "ends with a user",
		"corrupted", "malformed", "schema", "enum", "thinking", "signature",
		"function call turn", "prefill", "extra fields", "unexpected",
		"upstream service request failed",
	}
	for _, kw := range paramKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// TryAISelfHealRequest 在请求遇到候选参数报错时调用：
// 1. 先用内存预编译的 ai_auto_heal_patch.js 同步极速自愈 (< 0.2ms)；
// 2. 若现有脚本尚未覆盖该新型报错，绝不阻塞当前请求，立即投递后台异步任务由项目自身 AI 能力更新 ai_auto_heal_patch.js。
func TryAISelfHealRequest(c *gin.Context, info *RelayInfo, rawBytes []byte, errStr string) ([]byte, string, bool) {
	if len(rawBytes) == 0 || !IsCandidateForAISelfHeal(errStr) {
		return rawBytes, "", false
	}
	if c != nil && c.GetBool("ai_self_heal_retried") {
		return rawBytes, "", false
	}

	initAISelfHealer()

	protocol := "openai"
	modelName := ""
	if info != nil {
		modelName = info.UpstreamModelName
		if modelName == "" {
			modelName = info.OriginModelName
		}
		if common.IsAPITypeGeminiCompatible(info.ApiType) {
			protocol = "gemini"
		} else if common.IsAPITypeClaudeCompatible(info.ApiType) {
			protocol = "claude"
		}
	}

	compiled := activeHealScript.Load()
	if compiled != nil && compiled.program != nil {
		healedBytes, ruleID, ok := executeHealProgramInSandbox(compiled.program, rawBytes, protocol, modelName, errStr)
		if ok && len(healedBytes) > 0 {
			if c != nil {
				c.Set("ai_self_heal_retried", true)
			}
			return healedBytes, ruleID, true
		}
	}

	// 现有脚本未命中该新型报错：异步投递给项目自身 AI 进行代码热更新演进（0ms 阻塞当前请求）
	enqueueAsyncAIEvolution(protocol, modelName, errStr, rawBytes)
	return rawBytes, "", false
}

func executeHealProgramInSandbox(prog *sobek.Program, rawBytes []byte, protocol, modelName, errStr string) (healedBytes []byte, ruleID string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			healedBytes = nil
			ruleID = ""
			ok = false
		}
	}()

	// 包体积保护：超过 512KB（如含超大 Base64 图片）直接跳过 JS 沙箱，确保零内存抖动
	if len(rawBytes) == 0 || len(rawBytes) > 512*1024 {
		return nil, "", false
	}

	rawStr := string(rawBytes)
	hadContents := strings.Contains(rawStr, `"contents"`)
	hadMessages := strings.Contains(rawStr, `"messages"`)

	vm := sobek.New()
	timer := time.AfterFunc(sandboxExecTimeout, func() {
		vm.Interrupt("ai_self_heal_sandbox_timeout")
	})
	defer timer.Stop()

	if _, err := vm.RunProgram(prog); err != nil {
		return nil, "", false
	}

	_ = vm.Set("__raw_json_input", rawStr)
	_ = vm.Set("__heal_ctx", map[string]any{
		"protocol": protocol,
		"model":    modelName,
		"errorMsg": errStr,
	})

	runnerScript := `
		(function() {
			var parsed = JSON.parse(__raw_json_input);
			var res = autoHealRequest(parsed, __heal_ctx);
			if (!res || !res.modified || !res.body) {
				return null;
			}
			if (Array.isArray(res.body.contents) && res.body.contents.length === 0) {
				return null;
			}
			if (Array.isArray(res.body.messages) && res.body.messages.length === 0) {
				return null;
			}
			return JSON.stringify({
				ruleId: String(res.ruleId || "ai_js_auto_heal"),
				bodyJson: JSON.stringify(res.body)
			});
		})()
	`
	resVal, err := vm.RunString(runnerScript)
	if err != nil || resVal == nil || sobek.IsUndefined(resVal) || sobek.IsNull(resVal) {
		return nil, "", false
	}

	resJSON := resVal.String()
	if resJSON == "" || resJSON == "null" {
		return nil, "", false
	}

	var wrapper struct {
		RuleID   string `json:"ruleId"`
		BodyJSON string `json:"bodyJson"`
	}
	if err := json.Unmarshal([]byte(resJSON), &wrapper); err != nil || len(wrapper.BodyJSON) == 0 {
		return nil, "", false
	}

	// 结构完整性校验：严禁把原有的 contents 或 messages 字段删掉
	if hadContents && !strings.Contains(wrapper.BodyJSON, `"contents"`) {
		return nil, "", false
	}
	if hadMessages && !strings.Contains(wrapper.BodyJSON, `"messages"`) {
		return nil, "", false
	}

	return []byte(wrapper.BodyJSON), wrapper.RuleID, true
}

func normalizeErrorFingerprint(protocol, errStr string) string {
	s := strings.ToLower(strings.TrimSpace(errStr))
	s = idNormRegex.ReplaceAllString(s, "$1:*")
	s = digitNormRegex.ReplaceAllString(s, "*")
	if len(s) > 180 {
		s = s[:180]
	}
	return protocol + "|" + s
}

func enqueueAsyncAIEvolution(protocol, modelName, errStr string, rawBytes []byte) {
	fp := normalizeErrorFingerprint(protocol, errStr)
	now := time.Now()
	if prev, loaded := evolutionCooldownMu.Load(fp); loaded {
		if lastTime, ok := prev.(time.Time); ok && now.Sub(lastTime) < aiEvolutionCooldown {
			return
		}
	}
	evolutionCooldownMu.Store(fp, now)

	cloned := make([]byte, len(rawBytes))
	copy(cloned, rawBytes)

	select {
	case evolutionQueue <- aiEvolutionTask{
		protocol: protocol,
		model:    modelName,
		errMsg:   errStr,
		rawBytes: cloned,
	}:
	default:
		// 队列已满则直接丢弃，绝不阻塞业务请求
	}
}

func backgroundEvolutionWorker() {
	for task := range evolutionQueue {
		func(t aiEvolutionTask) {
			defer func() {
				if r := recover(); r != nil {
					logger.LogError(context.Background(), fmt.Sprintf("[AI-SelfHeal] background evolution panic recovered: %v", r))
				}
			}()
			evolveHealScriptWithInternalAI(t)
		}(task)
		// 速率保护：每次演进间隔至少 5 秒，避免频繁请求内部 AI
		time.Sleep(5 * time.Second)
	}
}

// ExtractJSONSkeleton 提取脱敏且极简的 JSON 结构骨架（剥离长文本和 Base64，总大小控制在 1KB 以内）
func ExtractJSONSkeleton(rawBytes []byte) string {
	var root any
	if err := common.UnmarshalJsonStr(string(rawBytes), &root); err != nil {
		return "{}"
	}
	skel := buildSkeletonNode(root, 0)
	out, err := common.Marshal(skel)
	if err != nil {
		return "{}"
	}
	if len(out) > 1500 {
		return string(out[:1500]) + "...(truncated)"
	}
	return string(out)
}

func buildSkeletonNode(node any, depth int) any {
	if depth > 6 {
		return "<max_depth>"
	}
	switch v := node.(type) {
	case map[string]any:
		res := make(map[string]any, len(v))
		for k, val := range v {
			if k == "role" || k == "type" || k == "name" || k == "category" || k == "threshold" || k == "mimeType" || k == "mime_type" {
				res[k] = val
				continue
			}
			res[k] = buildSkeletonNode(val, depth+1)
		}
		return res
	case []any:
		limit := len(v)
		if limit > 4 {
			// 保留首尾元素，精准体现首尾角色与结构
			return []any{
				buildSkeletonNode(v[0], depth+1),
				fmt.Sprintf("<...%d_items_omitted...>", len(v)-2),
				buildSkeletonNode(v[len(v)-1], depth+1),
			}
		}
		arr := make([]any, 0, limit)
		for i := 0; i < limit; i++ {
			arr = append(arr, buildSkeletonNode(v[i], depth+1))
		}
		return arr
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" || trimmed == "." || trimmed == "..." {
			return v
		}
		if len(v) > 48 {
			return fmt.Sprintf("<str:len=%d>", len(v))
		}
		return "<str>"
	default:
		return v
	}
}

// evolveHealScriptWithInternalAI 调用项目自身的内部 AI 渠道生成增量修复分支，经五重校验通过后仅更新 ai_auto_heal_patch.js
func evolveHealScriptWithInternalAI(task aiEvolutionTask) {
	cur := activeHealScript.Load()
	if cur == nil {
		return
	}
	// 如果当前脚本已经能修复该任务，无需重复演进
	if _, _, alreadyOk := executeHealProgramInSandbox(cur.program, task.rawBytes, task.protocol, task.model, task.errMsg); alreadyOk {
		return
	}

	skeleton := ExtractJSONSkeleton(task.rawBytes)
	prompt := fmt.Sprintf(`You are the built-in AI Self-Healing Engine for an enterprise LLM API Gateway.
An upstream channel returned a parameter/schema/format error that is not yet handled by our JavaScript self-healing script (ai_auto_heal_patch.js).

[Target Protocol]: %s
[Target Model]: %s
[Upstream Error Message]:
%s

[Desensitized Request JSON Skeleton]:
%s

[Current ai_auto_heal_patch.js Source]:
%s

Task:
Write a concise ES5 JavaScript "if (...)" self-healing block to insert immediately BEFORE the marker "%s" inside autoHealRequest(body, ctx) so that this exact error is automatically healed on retry.
Strict Rules:
1. Return ONLY a JSON object with two keys: {"ruleId": "short_snake_case_name", "jsSnippet": "if (...) { ... markRule('...'); }"}
2. Pure ES5 JavaScript only. Do NOT use eval, Function, require, import, or external APIs.
3. Never empty out body.contents or body.messages. Call markRule("your_rule_id") when you modify body.
4. Keep the snippet under 25 lines and guard it with specific error keyword checks on "err" or structure checks on "body".`,
		task.protocol, task.model, task.errMsg, skeleton, cur.source, healScriptAnchorMarker)

	aiReply, err := callInternalGatewayAI(prompt)
	if err != nil || strings.TrimSpace(aiReply) == "" {
		return
	}

	snippet, ruleID, err := parseAISnippetResponse(aiReply)
	if err != nil || snippet == "" {
		return
	}

	// 将 AI 生成的分支插入到唯一锚点 healScriptAnchorMarker 之前
	latest := activeHealScript.Load()
	if latest == nil || !strings.Contains(latest.source, healScriptAnchorMarker) {
		return
	}
	timestampComment := fmt.Sprintf("  // [AI-Auto-Evolved Rule: %s | %s]\n", ruleID, time.Now().Format(time.RFC3339))
	replacement := timestampComment + "  " + strings.TrimSpace(snippet) + "\n\n  " + healScriptAnchorMarker
	newSource := strings.Replace(latest.source, healScriptAnchorMarker, replacement, 1)

	// 五重安全护栏校验
	if err := validateHealScriptSource(newSource); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("[AI-SelfHeal] rejected AI snippet (%s): source validation failed: %v", ruleID, err))
		return
	}
	newProg, err := sobek.Compile("ai_auto_heal_patch.js", newSource, false)
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("[AI-SelfHeal] rejected AI snippet (%s): compile failed: %v", ruleID, err))
		return
	}
	if err := runHealScriptRegressionSuite(newProg); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("[AI-SelfHeal] rejected AI snippet (%s): regression suite failed: %v", ruleID, err))
		return
	}
	// 验证新编译的代码确实能修复当前报错的请求样本
	healedSample, _, healedOk := executeHealProgramInSandbox(newProg, task.rawBytes, task.protocol, task.model, task.errMsg)
	if !healedOk || len(healedSample) == 0 {
		logger.LogWarn(context.Background(), fmt.Sprintf("[AI-SelfHeal] rejected AI snippet (%s): did not heal failing sample", ruleID))
		return
	}

	// 校验全部通过：原子写入唯一锁定的 ai_auto_heal_patch.js 文件并热更新内存字节码
	if err := writeLockedHealScriptFile(newSource); err != nil {
		logger.LogError(context.Background(), fmt.Sprintf("[AI-SelfHeal] write locked script failed: %v", err))
		return
	}
	var mtime time.Time
	if st, sErr := os.Stat(resolveLockedHealScriptPath()); sErr == nil {
		mtime = st.ModTime()
	}
	activeHealScript.Store(&compiledHealScript{
		source:  newSource,
		program: newProg,
		modTime: mtime,
	})
	logger.LogInfo(context.Background(), fmt.Sprintf("[AI-SelfHeal] evolved and hot-reloaded ai_auto_heal_patch.js with new rule: %s", ruleID))
}

// callInternalGatewayAI 使用项目自身的渠道缓存池选取可用高速渠道（如 gemini-3-flash / gemini-2.5-flash，严禁调用 gemini-2.5-pro）
func callInternalGatewayAI(prompt string) (string, error) {
	resolverAny := internalAIChannelResolver.Load()
	resolver, ok := resolverAny.(InternalAIChannelResolver)
	if !ok || resolver == nil {
		return "", errors.New("internal AI channel resolver not registered")
	}

	candidateModels := []string{"gemini-3-flash", "gemini-2.5-flash", "gemini-3.5-flash-lite"}
	candidateGroups := []string{"Gemini组", "default", "小香蕉对接组"}

	var baseURL, apiKey, selectedModel string
	for _, mName := range candidateModels {
		for _, grp := range candidateGroups {
			if bURL, key, found := resolver(grp, mName); found && bURL != "" && key != "" {
				baseURL = strings.TrimRight(bURL, "/")
				apiKey = key
				selectedModel = mName
				break
			}
		}
		if baseURL != "" {
			break
		}
	}
	if baseURL == "" || apiKey == "" {
		return "", errors.New("no internal AI channel available")
	}

	reqURL := fmt.Sprintf("%s/v1beta/models/%s:generateContent", baseURL, selectedModel)

	payload := map[string]any{
		"contents": []any{
			map[string]any{
				"role":  "user",
				"parts": []any{map[string]any{"text": prompt}},
			},
		},
		"generationConfig": map[string]any{
			"temperature":      0.1,
			"responseMimeType": "application/json",
		},
	}
	bodyBytes, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), aiEvolutionHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := (&http.Client{Timeout: aiEvolutionHTTPTimeout}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("internal AI status %d", resp.StatusCode)
	}
	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", err
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBytes, &geminiResp); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, cand := range geminiResp.Candidates {
		for _, p := range cand.Content.Parts {
			sb.WriteString(p.Text)
		}
	}
	return sb.String(), nil
}

func parseAISnippetResponse(raw string) (snippet string, ruleID string, err error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var parsed struct {
		RuleID    string `json:"ruleId"`
		JSSnippet string `json:"jsSnippet"`
	}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(parsed.JSSnippet) == "" {
		return "", "", errors.New("empty jsSnippet")
	}
	rID := strings.TrimSpace(parsed.RuleID)
	if rID == "" {
		rID = "ai_evolved_rule"
	}
	return parsed.JSSnippet, rID, nil
}

func validateHealScriptSource(src string) error {
	if !strings.Contains(src, "function autoHealRequest(body, ctx)") {
		return errors.New("missing function autoHealRequest(body, ctx)")
	}
	if !strings.Contains(src, healScriptAnchorMarker) {
		return errors.New("missing anchor marker")
	}
	forbiddenTokens := []string{"eval(", "Function(", "require(", "import(", "process.", "globalThis", "while(true)", "for(;;)"}
	for _, tok := range forbiddenTokens {
		if strings.Contains(src, tok) {
			return fmt.Errorf("forbidden token detected: %s", tok)
		}
	}
	return nil
}

func runHealScriptRegressionSuite(prog *sobek.Program) error {
	// 基准测试 1: 正常请求绝不能被误修改
	normalGemini := []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"generationConfig":{"temperature":0.7}}`)
	if _, _, mod := executeHealProgramInSandbox(prog, normalGemini, "gemini", "gemini-4-argon", "status_code=400, some unrelated error"); mod {
		return errors.New("regression failed: normal gemini request was unexpectedly modified")
	}

	// 基准测试 2: 末尾 model 预填充必须被修复并保留前置内容
	prefillGemini := []byte(`{"contents":[{"role":"user","parts":[{"text":"write story"}]},{"role":"model","parts":[{"text":"Sure:"}]}]}`)
	healed, _, mod := executeHealProgramInSandbox(prog, prefillGemini, "gemini", "gemini-4-argon", "upstream service request failed")
	if !mod || !strings.Contains(string(healed), `"Continue."`) {
		return errors.New("regression failed: trailing model prefill was not healed")
	}

	// 基准测试 3: Unknown name 字段剥离必须正常工作
	unknownFieldJSON := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"customBadField":123}`)
	healedUnk, _, modUnk := executeHealProgramInSandbox(prog, unknownFieldJSON, "gemini", "gemini-4-argon", `Unknown name "customBadField" at 'request'`)
	if !modUnk || strings.Contains(string(healedUnk), "customBadField") {
		return errors.New("regression failed: unknown field was not stripped")
	}

	return nil
}
