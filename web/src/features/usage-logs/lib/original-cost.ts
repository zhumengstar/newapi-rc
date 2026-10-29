import {
  parseTiersFromExpr,
  splitBillingExprAndRequestRules,
  type ParsedTier,
} from '@/features/pricing/lib/billing-expr'
import type { PricingModel } from '@/features/pricing/types'

import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'
import { isPerCallBilling } from './utils'

export interface OriginalCostResult {
  /**
   * Calculated quota representing the original model value
   * 500,000 quota = $1.00 USD
   */
  originalQuota: number
  /**
   * Resolved actual model name used for calculation
   */
  actualModelName?: string
  /**
   * Whether this cost was calculated using actual model pricing
   */
  isCalculatedFromModel: boolean
}

export function getGroupRatio(other: LogOtherData | null): number | null {
  const userGroupRatio = other?.user_group_ratio
  if (
    userGroupRatio != null &&
    userGroupRatio !== -1 &&
    Number.isFinite(userGroupRatio)
  ) {
    return userGroupRatio
  }

  const groupRatio = other?.group_ratio
  if (groupRatio != null && groupRatio !== 1 && Number.isFinite(groupRatio)) {
    return groupRatio
  }

  return null
}

export function getFallbackOriginalQuota(
  log: UsageLog,
  other: LogOtherData | null
): number {
  const quota = other?.fee_quota ?? log.quota
  if (!quota || quota <= 0) return 0
  const ratio = getGroupRatio(other)
  if (ratio != null && ratio > 0) {
    return quota / ratio
  }
  return quota
}

/**
 * Find the most suitable PricingModel with token-based pricing for the given log
 */
export function findActualPricingModel(
  log: UsageLog,
  other: LogOtherData | null,
  pricingModels: PricingModel[]
): PricingModel | null {
  if (!pricingModels || pricingModels.length === 0) return null

  const candidates: string[] = []

  if (other?.upstream_model_name && typeof other.upstream_model_name === 'string') {
    candidates.push(other.upstream_model_name.trim())
  }

  if (log.model_name) {
    candidates.push(log.model_name.trim())
  }

  const derivedCandidates: string[] = []
  for (const name of candidates) {
    if (name.endsWith('-c')) {
      derivedCandidates.push(name.slice(0, -2))
    }
    if (name.includes('-thinking-c')) {
      derivedCandidates.push(name.replace('-thinking-c', '-thinking'))
      derivedCandidates.push(name.replace('-thinking-c', ''))
    }
    if (name.endsWith('-thinking')) {
      derivedCandidates.push(name.slice(0, -9))
    }
    if (name.startsWith('a/')) {
      derivedCandidates.push(name.slice(2))
    }
    if (name.includes('.0')) {
      derivedCandidates.push(name.replace('.0', ''))
    }
  }

  const allCandidates = Array.from(
    new Set([...candidates, ...derivedCandidates])
  ).filter(Boolean)

  // Pass 1: find a model with real token-based pricing (quota_type === 0 or model_ratio > 0 or billing_expr)
  for (const cand of allCandidates) {
    const candLower = cand.toLowerCase()
    const match = pricingModels.find(
      (m) =>
        m.model_name.toLowerCase() === candLower &&
        (m.quota_type === 0 ||
          m.model_ratio > 0 ||
          (Boolean(m.billing_expr) && m.billing_expr?.trim() !== ''))
    )
    if (match) return match
  }

  // Pass 2: any model matching by name
  for (const cand of allCandidates) {
    const candLower = cand.toLowerCase()
    const match = pricingModels.find(
      (m) => m.model_name.toLowerCase() === candLower
    )
    if (match) return match
  }

  return null
}

/**
 * Calculate quota cost based on token counts and PricingModel unit prices
 */
export function calculateTokensCostQuota(
  log: UsageLog,
  other: LogOtherData | null,
  model: PricingModel
): number | null {
  const promptTokens = log.prompt_tokens || 0
  const completionTokens = log.completion_tokens || 0
  const cacheReadTokens = other?.cache_tokens || 0
  const cacheWrite5m = other?.cache_creation_tokens_5m || 0
  const cacheWrite1h = other?.cache_creation_tokens_1h || 0
  const cacheWriteTokens =
    cacheWrite5m > 0 || cacheWrite1h > 0
      ? cacheWrite5m + cacheWrite1h
      : other?.cache_creation_tokens || 0

  if (
    promptTokens === 0 &&
    completionTokens === 0 &&
    cacheReadTokens === 0 &&
    cacheWriteTokens === 0
  ) {
    return null
  }

  let inputPrice = 0 // $/M
  let outputPrice = 0 // $/M
  let cacheReadPrice = 0 // $/M
  let cacheCreatePrice = 0 // $/M
  let cacheCreate1hPrice = 0 // $/M

  if (model.billing_expr && model.billing_expr.trim() !== '') {
    try {
      const split = splitBillingExprAndRequestRules(model.billing_expr)
      const tiers = parseTiersFromExpr(split.billingExpr)
      if (tiers && tiers.length > 0) {
        let matchedTier: ParsedTier | undefined
        if (other?.matched_tier) {
          matchedTier = tiers.find(
            (t) =>
              t.label?.toLowerCase() === other.matched_tier?.toLowerCase()
          )
        }
        if (!matchedTier) {
          matchedTier =
            tiers.find((t) => t.label === 'base' || t.label === 'standard') ||
            tiers[0]
        }
        if (matchedTier) {
          inputPrice = Number(matchedTier.inputPrice ?? 0)
          outputPrice = Number(
            matchedTier.outputPrice ??
              inputPrice * (model.completion_ratio || 1)
          )
          cacheReadPrice = Number(
            matchedTier.cacheReadPrice ??
              inputPrice * (model.cache_ratio ?? 0.5)
          )
          cacheCreatePrice = Number(
            matchedTier.cacheCreatePrice ??
              inputPrice * (model.create_cache_ratio ?? 1.25)
          )
          cacheCreate1hPrice = Number(
            matchedTier.cacheCreate1hPrice ??
              cacheCreatePrice * 2.0
          )
        }
      }
    } catch {
      // Ignore parse failure, fall back to model_ratio below
    }
  }

  if (inputPrice === 0 && model.model_ratio > 0) {
    inputPrice = model.model_ratio * 2.0
    outputPrice = inputPrice * (model.completion_ratio || 1)
    cacheReadPrice = inputPrice * (model.cache_ratio ?? 0.5)
    cacheCreatePrice = inputPrice * (model.create_cache_ratio ?? 1.25)
    cacheCreate1hPrice = cacheCreatePrice * 2.0
  }

  if (inputPrice === 0 && outputPrice === 0) {
    return null
  }

  const effectiveCacheCreatePrice =
    cacheWrite1h > 0 && cacheCreate1hPrice > 0
      ? (cacheWrite5m * cacheCreatePrice + cacheWrite1h * cacheCreate1hPrice)
      : cacheWriteTokens * cacheCreatePrice

  const totalUSD =
    (promptTokens * inputPrice +
      completionTokens * outputPrice +
      cacheReadTokens * cacheReadPrice +
      effectiveCacheCreatePrice) /
    1_000_000

  // 1 USD = 500,000 quota
  return Math.round(totalUSD * 500_000)
}

/**
 * Compute the original cost result for a usage log row
 */
export function computeOriginalCostResult(
  log: UsageLog,
  other: LogOtherData | null,
  pricingModels?: PricingModel[]
): OriginalCostResult {
  // If not consumption or error log with 0 quota & 0 tokens, return 0
  if (log.type !== 2) {
    return {
      originalQuota: 0,
      isCalculatedFromModel: false,
    }
  }

  const hasTokens =
    (log.prompt_tokens || 0) > 0 ||
    (log.completion_tokens || 0) > 0 ||
    (other?.cache_tokens || 0) > 0 ||
    (other?.cache_creation_tokens || 0) > 0

  const isPerCall =
    isPerCallBilling(other?.model_price) || (other?.model_price ?? 0) > 0

  // Case 1: Per-call billing with tokens -> calculate from actual model
  if (isPerCall && hasTokens && pricingModels && pricingModels.length > 0) {
    const actualModel = findActualPricingModel(log, other, pricingModels)
    if (actualModel) {
      const calculatedQuota = calculateTokensCostQuota(log, other, actualModel)
      if (calculatedQuota != null && calculatedQuota > 0) {
        return {
          originalQuota: calculatedQuota,
          actualModelName: actualModel.model_name,
          isCalculatedFromModel: true,
        }
      }
    }
  }

  // Case 2: Model is explicitly mapped to an upstream model with tokens
  if (
    other?.is_model_mapped &&
    other?.upstream_model_name &&
    hasTokens &&
    pricingModels &&
    pricingModels.length > 0
  ) {
    const actualModel = findActualPricingModel(log, other, pricingModels)
    if (actualModel && actualModel.model_name !== log.model_name) {
      const calculatedQuota = calculateTokensCostQuota(log, other, actualModel)
      if (calculatedQuota != null && calculatedQuota > 0) {
        return {
          originalQuota: calculatedQuota,
          actualModelName: actualModel.model_name,
          isCalculatedFromModel: true,
        }
      }
    }
  }

  // Case 3: Regular usage or fallback -> quota / ratio
  const fallback = getFallbackOriginalQuota(log, other)
  return {
    originalQuota: fallback,
    isCalculatedFromModel: false,
  }
}
