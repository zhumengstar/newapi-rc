// ============================================================================
// New API 智能自愈热更新规则脚本 (AI Self-Healing Patch Script)
// 警告: 本文件是 AI 自愈引擎唯一允许实时更新的代码文件 (Single Locked Target File)
// 运行环境: 纯内存零权限 Sobek (Goja) JS 沙箱，无文件/网络/系统调用权限，单次限时 15ms
// ============================================================================

function autoHealRequest(body, ctx) {
  if (!body || typeof body !== "object") {
    return { modified: false, body: body, ruleId: "" };
  }

  var err = ((ctx && ctx.errorMsg) ? String(ctx.errorMsg) : "").toLowerCase();
  var protocol = ((ctx && ctx.protocol) ? String(ctx.protocol) : "").toLowerCase();
  var modified = false;
  var matchedRules = [];

  function markRule(id) {
    modified = true;
    for (var i = 0; i < matchedRules.length; i++) {
      if (matchedRules[i] === id) return;
    }
    matchedRules.push(id);
  }

  function deleteKeyRecursive(node, targetKey) {
    if (!node || typeof node !== "object") return false;
    var changed = false;
    if (Array.isArray(node)) {
      for (var i = 0; i < node.length; i++) {
        if (deleteKeyRecursive(node[i], targetKey)) changed = true;
      }
      return changed;
    }
    if (Object.prototype.hasOwnProperty.call(node, targetKey)) {
      delete node[targetKey];
      changed = true;
    }
    var keys = Object.keys(node);
    for (var k = 0; k < keys.length; k++) {
      if (deleteKeyRecursive(node[keys[k]], targetKey)) changed = true;
    }
    return changed;
  }

  // --------------------------------------------------------------------------
  // 规则 1: 自动提取上游报错中的未知字段名并递归剥离
  // 匹配: Unknown name "xxx" / Unrecognized field "xxx" / Additional properties ... ('xxx' was unexpected)
  // --------------------------------------------------------------------------
  var rawErr = (ctx && ctx.errorMsg) ? String(ctx.errorMsg) : "";
  var unknownMatch =
    rawErr.match(/Unknown name\s+["']([a-zA-Z0-9_.-]+)["']/i) ||
    rawErr.match(/Unrecognized field\s+["']([a-zA-Z0-9_.-]+)["']/i) ||
    rawErr.match(/Cannot find field\s+["']([a-zA-Z0-9_.-]+)["']/i) ||
    rawErr.match(/\(['"]([a-zA-Z0-9_.-]+)['"]\s+was unexpected\)/i) ||
    rawErr.match(/extra fields not permitted.*['"]([a-zA-Z0-9_.-]+)['"]/i);
  if (unknownMatch && unknownMatch[1]) {
    var badField = unknownMatch[1];
    // 保护核心根字段不被误删
    if (badField !== "contents" && badField !== "messages" && badField !== "parts" && badField !== "role" && badField !== "model") {
      if (deleteKeyRecursive(body, badField)) {
        markRule("strip_unknown_field:" + badField);
      }
    }
  }

  // --------------------------------------------------------------------------
  // 规则 2: Gemini contents 末尾 role="model" 预填充与连续 model(functionCall) 修复
  // --------------------------------------------------------------------------
  if (Array.isArray(body.contents) && body.contents.length > 0) {
    // 2.1 若报错涉及 thought signature 损坏或 invalid argument，剥离历史 thoughtSignature
    if (err.indexOf("thought signature") !== -1 || err.indexOf("thought_signature") !== -1 || err.indexOf("invalid argument") !== -1) {
      if (deleteKeyRecursive(body.contents, "thoughtSignature") || deleteKeyRecursive(body.contents, "thought_signature")) {
        markRule("gemini_strip_thought_signature");
      }
    }

    // 2.2 合并相邻的 model 回合，修复 function call turn comes immediately after a user turn
    var merged = [];
    for (var i = 0; i < body.contents.length; i++) {
      var cur = body.contents[i];
      if (!cur || typeof cur !== "object") continue;
      var role = String(cur.role || "").toLowerCase();
      if (role === "assistant") {
        cur.role = "model";
        role = "model";
        markRule("gemini_role_assistant_to_model");
      }
      if (role === "model" && merged.length > 0) {
        var prev = merged[merged.length - 1];
        if (prev && String(prev.role || "").toLowerCase() === "model") {
          var prevParts = Array.isArray(prev.parts) ? prev.parts : [];
          var curParts = Array.isArray(cur.parts) ? cur.parts : [];
          prev.parts = prevParts.concat(curParts);
          markRule("gemini_merge_consecutive_model_turns");
          continue;
        }
      }
      merged.push(cur);
    }
    body.contents = merged;

    // 2.3 确保最后一条消息不是孤立的 model 预填充
    if (body.contents.length > 0) {
      var last = body.contents[body.contents.length - 1];
      if (last && String(last.role || "").toLowerCase() === "model") {
        var parts = Array.isArray(last.parts) ? last.parts : [];
        var hasTool = false;
        var hasRealText = false;
        for (var p = 0; p < parts.length; p++) {
          var pt = parts[p];
          if (pt && (pt.functionCall || pt.function_call || pt.functionResponse || pt.function_response)) {
            hasTool = true;
          }
          if (pt && typeof pt.text === "string") {
            var t = pt.text.trim();
            if (t !== "" && t !== "." && t !== "..." && t !== "。") {
              hasRealText = true;
            }
          }
        }
        if (!hasTool) {
          if (!hasRealText && body.contents.length > 1) {
            body.contents.pop();
            markRule("gemini_pop_empty_trailing_model");
          } else {
            body.contents.push({
              role: "user",
              parts: [{ text: "Continue." }]
            });
            markRule("gemini_append_user_after_model_prefill");
          }
        }
      }
    }
  }

  // --------------------------------------------------------------------------
  // 规则 3: Gemini generationConfig 采样参数越界与冲突自愈
  // --------------------------------------------------------------------------
  var gc = body.generationConfig || body.generation_config;
  if (gc && typeof gc === "object") {
    if (typeof gc.temperature === "number") {
      if (gc.temperature > 2.0) {
        gc.temperature = 2.0;
        markRule("gemini_clamp_temperature_max");
      } else if (gc.temperature < 0) {
        gc.temperature = 0;
        markRule("gemini_clamp_temperature_min");
      }
    }
    if (typeof gc.topP === "number" && (gc.topP > 1.0 || gc.topP <= 0)) {
      if (gc.topP > 1.0) gc.topP = 1.0;
      else delete gc.topP;
      markRule("gemini_fix_top_p");
    }
    if (typeof gc.topK === "number" && gc.topK < 1) {
      delete gc.topK;
      markRule("gemini_delete_invalid_top_k");
    }
    if (err.indexOf("thinking") !== -1 || err.indexOf("budget") !== -1) {
      if (gc.thinkingConfig || gc.thinking_config) {
        delete gc.thinkingConfig;
        delete gc.thinking_config;
        markRule("gemini_strip_unsupported_thinking_config");
      }
    }
  }

  // --------------------------------------------------------------------------
  // 规则 4: Claude / OpenAI messages 历史 thinking 签名损坏与末尾 assistant 预填充修复
  // --------------------------------------------------------------------------
  if (Array.isArray(body.messages) && body.messages.length > 0) {
    if (err.indexOf("signature") !== -1 && err.indexOf("thinking") !== -1) {
      for (var m = 0; m < body.messages.length; m++) {
        var msg = body.messages[m];
        if (msg && Array.isArray(msg.content)) {
          var filtered = [];
          for (var b = 0; b < msg.content.length; b++) {
            var blk = msg.content[b];
            var bType = blk && blk.type ? String(blk.type).toLowerCase() : "";
            if (bType === "thinking" || bType === "redacted_thinking") {
              markRule("claude_strip_invalid_thinking_block");
              continue;
            }
            filtered.push(blk);
          }
          if (filtered.length === 0) {
            filtered.push({ type: "text", text: "." });
          }
          msg.content = filtered;
        }
      }
    }

    if (
      err.indexOf("prefill") !== -1 ||
      err.indexOf("end with a user") !== -1 ||
      err.indexOf("upstream service request failed") !== -1
    ) {
      while (
        body.messages.length > 1 &&
        body.messages[body.messages.length - 1] &&
        String(body.messages[body.messages.length - 1].role || "").toLowerCase() === "assistant"
      ) {
        body.messages.pop();
        markRule("messages_strip_trailing_assistant_prefill");
      }
    }
  }

  // === [AI_DYNAMIC_RULES_ANCHOR] ===

  return {
    modified: modified,
    body: body,
    ruleId: matchedRules.join(",")
  };
}
