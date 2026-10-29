import { describe, expect, it } from 'vitest'

import type { PricingModel } from '@/features/pricing/types'
import type { UsageLog } from '../../data/schema'
import type { LogOtherData } from '../../types'
import {
  computeOriginalCostResult,
  findActualPricingModel,
  calculateTokensCostQuota,
} from '../original-cost'

describe('original-cost logic', () => {
  const mockPricingModels: PricingModel[] = [
    {
      id: 1,
      model_name: 'claude-opus-4-6',
      quota_type: 0,
      model_ratio: 2.5,
      completion_ratio: 5,
      cache_ratio: 1,
      create_cache_ratio: 1.25,
      enable_groups: ['cc反重力按量'],
      billing_mode: 'tiered_expr',
      billing_expr:
        'tier("base", p * 5 + c * 25 + cr * 5 + cc * 6.25 + cc1h * 10)',
    },
    {
      id: 2,
      model_name: 'claude-opus-4-6-c',
      quota_type: 1,
      model_price: 1,
      model_ratio: 0,
      completion_ratio: 0,
      enable_groups: ['cc反重力按次'],
    },
    {
      id: 3,
      model_name: 'gemini-2.5-flash',
      quota_type: 0,
      model_ratio: 0.15,
      completion_ratio: 8.333333333333,
      enable_groups: ['Gemini组'],
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("base", p * 0.3 + c * 2.5)',
    },
  ]

  it('calculates cost from actual model for per-call logs with prompt tokens', () => {
    // 114,070 prompt tokens at $5/M = $0.57035 = 285,175 quota
    const log: UsageLog = {
      id: 39698971,
      user_id: 1,
      created_at: 1793262987,
      type: 2,
      content: '',
      username: 'test',
      token_name: '反重力克',
      model_name: 'claude-opus-4-6-thinking-c',
      quota: 75000, // $0.15
      prompt_tokens: 114070,
      completion_tokens: 0,
      channel: 475,
      other: '',
    }
    const other: LogOtherData = {
      model_price: 0.15,
      is_model_mapped: true,
      upstream_model_name: 'claude-opus-4-6',
      group_ratio: 1,
    }

    const result = computeOriginalCostResult(log, other, mockPricingModels)
    expect(result.isCalculatedFromModel).toBe(true)
    expect(result.actualModelName).toBe('claude-opus-4-6')
    expect(result.originalQuota).toBe(285175)
  })

  it('calculates cost for per-call logs with both prompt and completion tokens', () => {
    // 150,484 prompt ($5/M) + 28,044 completion ($25/M)
    // 150,484 * 5 = 752,420
    // 28,044 * 25 = 701,100
    // total = 1,453,520 / 1,000,000 = $1.45352 -> 726,760 quota
    const log: UsageLog = {
      id: 39698972,
      user_id: 1,
      created_at: 1793262987,
      type: 2,
      content: '',
      username: 'bukun',
      token_name: '5555',
      model_name: 'claude-opus-4-6-c',
      quota: 75000,
      prompt_tokens: 150484,
      completion_tokens: 28044,
      channel: 464,
      other: '',
    }
    const other: LogOtherData = {
      model_price: 0.15,
      is_model_mapped: true,
      upstream_model_name: 'claude-opus-4-6',
      group_ratio: 1,
    }

    const result = computeOriginalCostResult(log, other, mockPricingModels)
    expect(result.isCalculatedFromModel).toBe(true)
    expect(result.actualModelName).toBe('claude-opus-4-6')
    expect(result.originalQuota).toBe(726760)
  })

  it('finds base model when model name has -c suffix even without upstream_model_name', () => {
    const log: UsageLog = {
      id: 1,
      user_id: 1,
      created_at: 1793262987,
      type: 2,
      content: '',
      username: 'test',
      token_name: 'test',
      model_name: 'claude-opus-4-6-c',
      quota: 75000,
      prompt_tokens: 100000,
      completion_tokens: 0,
      channel: 1,
      other: '',
    }
    const other: LogOtherData = {
      model_price: 0.15,
      group_ratio: 1,
    }

    const found = findActualPricingModel(log, other, mockPricingModels)
    expect(found?.model_name).toBe('claude-opus-4-6')

    const result = computeOriginalCostResult(log, other, mockPricingModels)
    expect(result.isCalculatedFromModel).toBe(true)
    // 100,000 * 5 / 1,000,000 = $0.50 -> 250,000 quota
    expect(result.originalQuota).toBe(250000)
  })

  it('handles regular ratio-discounted logs using fallback quota / ratio', () => {
    const log: UsageLog = {
      id: 2,
      user_id: 1,
      created_at: 1793262987,
      type: 2,
      content: '',
      username: 'test',
      token_name: 'test',
      model_name: 'gemini-2.5-flash',
      quota: 1135,
      prompt_tokens: 45151,
      completion_tokens: 257,
      channel: 412,
      other: '',
    }
    const other: LogOtherData = {
      group_ratio: 0.16,
    }

    const result = computeOriginalCostResult(log, other, mockPricingModels)
    expect(result.isCalculatedFromModel).toBe(false)
    // 1135 / 0.16 = 7093.75 -> 7093.75
    expect(Math.round(result.originalQuota)).toBe(7094)
  })

  it('returns 0 for error logs with type !== 2 or zero tokens/quota', () => {
    const errorLog: UsageLog = {
      id: 3,
      user_id: 1,
      created_at: 1793262987,
      type: 4,
      content: '',
      username: 'test',
      token_name: 'test',
      model_name: 'claude-opus-4-6-c',
      quota: 0,
      prompt_tokens: 0,
      completion_tokens: 0,
      channel: 475,
      other: '',
    }
    const result = computeOriginalCostResult(errorLog, null, mockPricingModels)
    expect(result.originalQuota).toBe(0)
  })
})
