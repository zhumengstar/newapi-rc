export interface ModelFamilyConfig {
  id: string
  name: string
  nameEn: string
  order: number
  badgeClassName: string
  color: string
}

export const MODEL_FAMILIES: Record<string, ModelFamilyConfig> = {
  claude: {
    id: 'claude',
    name: 'Claude / Anthropic',
    nameEn: 'Claude / Anthropic',
    order: 1,
    badgeClassName:
      'border-purple-200 text-purple-700 bg-purple-50 dark:border-purple-900/50 dark:text-purple-300 dark:bg-purple-950/40',
    color: '#8b5cf6',
  },
  openai: {
    id: 'openai',
    name: 'OpenAI / GPT',
    nameEn: 'OpenAI / GPT',
    order: 2,
    badgeClassName:
      'border-emerald-200 text-emerald-700 bg-emerald-50 dark:border-emerald-900/50 dark:text-emerald-300 dark:bg-emerald-950/40',
    color: '#10b981',
  },
  gemini: {
    id: 'gemini',
    name: 'Gemini / Google',
    nameEn: 'Gemini / Google',
    order: 3,
    badgeClassName:
      'border-blue-200 text-blue-700 bg-blue-50 dark:border-blue-900/50 dark:text-blue-300 dark:bg-blue-950/40',
    color: '#3b82f6',
  },
  grok: {
    id: 'grok',
    name: 'Grok / xAI',
    nameEn: 'Grok / xAI',
    order: 4,
    badgeClassName:
      'border-rose-200 text-rose-700 bg-rose-50 dark:border-rose-900/50 dark:text-rose-300 dark:bg-rose-950/40',
    color: '#f43f5e',
  },
  deepseek: {
    id: 'deepseek',
    name: 'DeepSeek',
    nameEn: 'DeepSeek',
    order: 5,
    badgeClassName:
      'border-sky-200 text-sky-700 bg-sky-50 dark:border-sky-900/50 dark:text-sky-300 dark:bg-sky-950/40',
    color: '#0ea5e9',
  },
  glm: {
    id: 'glm',
    name: 'GLM / 智谱',
    nameEn: 'GLM / Zhipu',
    order: 6,
    badgeClassName:
      'border-cyan-200 text-cyan-700 bg-cyan-50 dark:border-cyan-900/50 dark:text-cyan-300 dark:bg-cyan-950/40',
    color: '#06b6d4',
  },
  qwen: {
    id: 'qwen',
    name: 'Qwen / 通义',
    nameEn: 'Qwen / Tongyi',
    order: 7,
    badgeClassName:
      'border-amber-200 text-amber-700 bg-amber-50 dark:border-amber-900/50 dark:text-amber-300 dark:bg-amber-950/40',
    color: '#f59e0b',
  },
  image_media: {
    id: 'image_media',
    name: '图像与多媒体',
    nameEn: 'Image & Media',
    order: 8,
    badgeClassName:
      'border-pink-200 text-pink-700 bg-pink-50 dark:border-pink-900/50 dark:text-pink-300 dark:bg-pink-950/40',
    color: '#ec4899',
  },
  other: {
    id: 'other',
    name: '通用与自定义',
    nameEn: 'General & Custom',
    order: 99,
    badgeClassName:
      'border-slate-200 text-slate-700 bg-slate-50 dark:border-slate-800 dark:text-slate-300 dark:bg-slate-900/40',
    color: '#64748b',
  },
}

/**
 * 智能三阶推断分组所属的模型家族：
 * 1. 组名关键词（最高优先级）
 * 2. 关联渠道模型投票（次优先级）
 * 3. 描述字段（兜底）
 */
export function inferGroupFamily(
  groupName: string,
  desc?: string,
  models: string[] = []
): ModelFamilyConfig {
  const gLower = (groupName || '').toLowerCase().trim()
  const dLower = (desc || '').toLowerCase().trim()
  const modelsLower = (models || []).map((m) => (m || '').toLowerCase().trim())

  // 第一优先级：根据组名 (groupName) 显式关键词直接判定
  // 1.1 Gemini（包括国内常见的香蕉渠道组）
  if (
    gLower.includes('gemini') ||
    gLower.includes('香蕉') ||
    gLower.includes('banana') ||
    gLower.includes('google') ||
    gLower.includes('gemma')
  ) {
    return MODEL_FAMILIES.gemini
  }

  // 1.2 Claude
  if (
    gLower.includes('claude') ||
    gLower.includes('anthropic') ||
    gLower.includes('aswb') ||
    gLower.includes('kiro') ||
    /\b(cc|ccmax)\b/i.test(groupName) ||
    gLower.startsWith('cc')
  ) {
    return MODEL_FAMILIES.claude
  }

  // 1.3 OpenAI / GPT
  if (
    gLower.includes('gpt') ||
    gLower.includes('openai') ||
    gLower.includes('codex') ||
    gLower.includes('chatgpt') ||
    /\b(o1|o3|o4)\b/i.test(groupName)
  ) {
    return MODEL_FAMILIES.openai
  }

  // 1.4 Grok
  if (gLower.includes('grok') || gLower.includes('xai')) {
    return MODEL_FAMILIES.grok
  }

  // 1.5 DeepSeek
  if (
    gLower.includes('deepseek') ||
    gLower.includes('deep-seek') ||
    gLower.includes('深度求索')
  ) {
    return MODEL_FAMILIES.deepseek
  }

  // 1.6 GLM
  if (
    gLower.includes('glm') ||
    gLower.includes('zhipu') ||
    gLower.includes('智谱')
  ) {
    return MODEL_FAMILIES.glm
  }

  // 1.7 Qwen
  if (
    gLower.includes('qwen') ||
    gLower.includes('tongyi') ||
    gLower.includes('通义') ||
    gLower.includes('千问')
  ) {
    return MODEL_FAMILIES.qwen
  }

  // 1.8 图像/多模态
  if (
    gLower.includes('image') ||
    gLower.includes('img') ||
    gLower.includes('图片') ||
    gLower.includes('图像') ||
    gLower.includes('绘图') ||
    gLower.includes('画') ||
    gLower.includes('sd') ||
    gLower.includes('seedance') ||
    gLower.includes('flux') ||
    gLower.includes('midjourney') ||
    gLower.includes('video') ||
    gLower.includes('视频')
  ) {
    return MODEL_FAMILIES.image_media
  }

  // 第二优先级：如果组名没有直接命中，根据关联模型数量投票判定
  if (modelsLower.length > 0) {
    let claudeCount = 0
    let openaiCount = 0
    let geminiCount = 0
    let grokCount = 0
    let deepseekCount = 0
    let glmCount = 0
    let qwenCount = 0
    let imageCount = 0

    for (const m of modelsLower) {
      if (/^claude/i.test(m) || m.includes('anthropic')) {
        claudeCount++
      } else if (/^(gpt|o1|o3|o4|codex|text-embedding|chatgpt)/i.test(m)) {
        openaiCount++
      } else if (/^(gemini|gemma)/i.test(m)) {
        geminiCount++
      } else if (/^grok/i.test(m)) {
        grokCount++
      } else if (/^deepseek/i.test(m)) {
        deepseekCount++
      } else if (/^(glm|chatglm|cogview)/i.test(m)) {
        glmCount++
      } else if (/^(qwen|tongyi)/i.test(m)) {
        qwenCount++
      } else if (
        /^(sd|flux|midjourney|mj|dall-e|imagen|image|kling|sora|runway)/i.test(m) ||
        m.includes('image')
      ) {
        imageCount++
      }
    }

    const scores = [
      { fam: MODEL_FAMILIES.claude, count: claudeCount },
      { fam: MODEL_FAMILIES.openai, count: openaiCount },
      { fam: MODEL_FAMILIES.gemini, count: geminiCount },
      { fam: MODEL_FAMILIES.grok, count: grokCount },
      { fam: MODEL_FAMILIES.deepseek, count: deepseekCount },
      { fam: MODEL_FAMILIES.glm, count: glmCount },
      { fam: MODEL_FAMILIES.qwen, count: qwenCount },
      { fam: MODEL_FAMILIES.image_media, count: imageCount },
    ]
    scores.sort((a, b) => b.count - a.count)
    if (scores[0].count > 0) {
      return scores[0].fam
    }
  }

  // 第三优先级：根据描述 desc 字段兜底
  if (dLower) {
    if (dLower.includes('claude') || dLower.includes('anthropic') || dLower.includes('aswb'))
      return MODEL_FAMILIES.claude
    if (dLower.includes('gpt') || dLower.includes('openai') || dLower.includes('codex'))
      return MODEL_FAMILIES.openai
    if (dLower.includes('gemini') || dLower.includes('google') || dLower.includes('香蕉'))
      return MODEL_FAMILIES.gemini
    if (dLower.includes('grok') || dLower.includes('xai')) return MODEL_FAMILIES.grok
    if (dLower.includes('deepseek')) return MODEL_FAMILIES.deepseek
    if (dLower.includes('glm') || dLower.includes('智谱')) return MODEL_FAMILIES.glm
    if (dLower.includes('qwen') || dLower.includes('通义')) return MODEL_FAMILIES.qwen
    if (dLower.includes('image') || dLower.includes('图片')) return MODEL_FAMILIES.image_media
  }

  return MODEL_FAMILIES.other
}

export interface GroupSortOptions {
  userGroupRatios?: Record<string, number>
  groupMeta?: Record<string, { ratio?: number; desc?: string; models?: string[] }>
}

/**
 * 获取分组在当前上下文中的有效倍率
 */
export function getEffectiveGroupRatio(
  group: string,
  userGroupRatios?: Record<string, number>,
  groupMeta?: Record<string, { ratio?: number }>
): number {
  const custom = userGroupRatios?.[group]
  if (custom !== undefined && custom !== null && Number.isFinite(Number(custom))) {
    return Number(custom)
  }
  const def = groupMeta?.[group]?.ratio
  if (def !== undefined && def !== null && Number.isFinite(Number(def))) {
    return Number(def)
  }
  return 1
}

/**
 * 按照模型家族排序，然后在同一家族内按照倍率从高到低降序排序
 */
export function sortGroupsByFamilyAndRatio(
  groups: string[],
  options: GroupSortOptions = {}
): string[] {
  const { userGroupRatios = {}, groupMeta = {} } = options

  return [...groups].sort((a, b) => {
    const metaA = groupMeta[a]
    const metaB = groupMeta[b]

    const famA = inferGroupFamily(a, metaA?.desc, metaA?.models)
    const famB = inferGroupFamily(b, metaB?.desc, metaB?.models)

    // 1. 先按模型家族优先级排序 (order 升序: Claude -> OpenAI -> Gemini -> ...)
    if (famA.order !== famB.order) {
      return famA.order - famB.order
    }

    // 2. 同一家族内，按有效倍率从高到低降序排序
    const ratioA = getEffectiveGroupRatio(a, userGroupRatios, groupMeta)
    const ratioB = getEffectiveGroupRatio(b, userGroupRatios, groupMeta)
    if (ratioA !== ratioB) {
      return ratioB - ratioA
    }

    // 3. 倍率相同时，按组名自然字母排序
    return a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' })
  })
}
