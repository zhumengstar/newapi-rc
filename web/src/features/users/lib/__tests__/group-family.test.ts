import { describe, expect, it } from 'vitest'
import {
  inferGroupFamily,
  sortGroupsByFamilyAndRatio,
} from '../group-family'

describe('group-family', () => {
  it('correctly classifies real production groups into model families', () => {
    // Claude family
    expect(inferGroupFamily('CCMAX').id).toBe('claude')
    expect(inferGroupFamily('cc反重力按量').id).toBe('claude')
    expect(inferGroupFamily('aswb专用分组外接').id).toBe('claude')
    expect(inferGroupFamily('aswb专用分组外接-不可蒸馏').id).toBe('claude')
    expect(inferGroupFamily('高缓kiro99').id).toBe('claude')

    // OpenAI / GPT family
    expect(inferGroupFamily('GPTPro-对接池').id).toBe('openai')
    expect(inferGroupFamily('gpt特惠组').id).toBe('openai')
    expect(inferGroupFamily('GPT-纯PRO池').id).toBe('openai')
    expect(inferGroupFamily('GPT对接组').id).toBe('openai')
    expect(inferGroupFamily('GPTPro-企业级').id).toBe('openai')
    expect(inferGroupFamily('GPTPro-低价池').id).toBe('openai')
    expect(
      inferGroupFamily('自用模型组', '', ['gpt-5.6-sol', 'gpt-5.6-terra']).id
    ).toBe('openai')

    // Gemini / Google family (even if channel accidentally mapped claude model)
    expect(inferGroupFamily('Gemini组', 'Gemini组', ['claude-sonnet-4-6', 'gemini-3.1-pro']).id).toBe('gemini')
    expect(inferGroupFamily('香蕉渠道组').id).toBe('gemini')
    expect(inferGroupFamily('大香蕉对接组').id).toBe('gemini')
    expect(inferGroupFamily('小香蕉对接组').id).toBe('gemini')

    // Grok / xAI family
    expect(inferGroupFamily('grok分组').id).toBe('grok')
    expect(inferGroupFamily('supergrok组').id).toBe('grok')

    // Image & Media
    expect(inferGroupFamily('AzImage2').id).toBe('image_media')
    expect(inferGroupFamily('生成图片组').id).toBe('image_media')
    expect(inferGroupFamily('Seedance', '', ['sd-2.0-720-900']).id).toBe(
      'image_media'
    )
  })

  it('sorts groups by model family first and ratio descending second', () => {
    const rawGroups = [
      '香蕉渠道组',
      'CCMAX',
      'GPTPro-对接池',
      'gpt特惠组',
      'GPT-纯PRO池',
      '生成图片组',
      '高缓kiro99',
      'GPT对接组',
      'supergrok组',
      'cc反重力按量',
      '大香蕉对接组',
      'GPTPro-企业级',
      '小香蕉对接组',
      '自用模型组',
      'AzImage2',
      'GPTPro-低价池',
      'aswb专用分组外接',
      'aswb专用分组外接-不可蒸馏',
      'grok分组',
      'Gemini组',
      'Seedance',
    ]

    const groupMeta: Record<string, { ratio: number; desc?: string; models?: string[] }> = {
      '香蕉渠道组': { ratio: 1.9, desc: '香蕉as渠道组-外接的渠道，自行测试' },
      CCMAX: { ratio: 0.95, desc: 'CCMAX-外接的渠道，自行测试' },
      'GPTPro-对接池': { ratio: 0.12, desc: '先Pro后Plus-外接的渠道，自行测试' },
      gpt特惠组: { ratio: 0.09, desc: 'gpt特惠组-外接的混合渠道，自行测试' },
      'GPT-纯PRO池': { ratio: 4, desc: 'GPT-纯PRO池' },
      生成图片组: { ratio: 1.4, desc: '官渠生成图片组' },
      高缓kiro99: { ratio: 0.1, desc: '高缓kiro99-外接的渠道，自行测试' },
      GPT对接组: { ratio: 0.08, desc: 'GPT对接组' },
      supergrok组: { ratio: 0.15, desc: 'supergrok组' },
      cc反重力按量: { ratio: 0.28, desc: 'cc反重力按量' },
      大香蕉对接组: { ratio: 0.8, desc: '大香蕉对接组' },
      'GPTPro-企业级': { ratio: 0.14, desc: 'GPTPro-企业级-外接的渠道，自行测试' },
      小香蕉对接组: { ratio: 0.8, desc: '小香蕉对接组' },
      自用模型组: { ratio: 0.1, desc: '自用模型组', models: ['gpt-5.6-sol', 'gpt-5.6-terra'] },
      AzImage2: { ratio: 1.8, desc: 'AzImage2' },
      'GPTPro-低价池': { ratio: 0.11, desc: 'GPTPro-低价池' },
      aswb专用分组外接: { ratio: 3.5, desc: 'aswb专用分组外接-外接的渠道，自行测试' },
      'aswb专用分组外接-不可蒸馏': { ratio: 3.3, desc: 'aswb专用分组外接-不可蒸馏' },
      grok分组: { ratio: 0.04, desc: 'grok分组-外接的渠道，自行测试' },
      Gemini组: { ratio: 0.15, desc: 'Gemini组', models: ['claude-sonnet-4-6', 'gemini-3.1-pro'] },
      Seedance: { ratio: 1, desc: 'Seedance', models: ['sd-2.0-720-900'] },
    }

    const sorted = sortGroupsByFamilyAndRatio(rawGroups, { groupMeta })

    // Claude family should come first (5 groups), ratio descending
    const claudeSubset = sorted.slice(0, 5)
    expect(claudeSubset).toEqual([
      'aswb专用分组外接', // 3.5
      'aswb专用分组外接-不可蒸馏', // 3.3
      'CCMAX', // 0.95
      'cc反重力按量', // 0.28
      '高缓kiro99', // 0.1
    ])

    // OpenAI family comes second (7 groups), ratio descending
    const openaiSubset = sorted.slice(5, 12)
    expect(openaiSubset).toEqual([
      'GPT-纯PRO池', // 4.0
      'GPTPro-企业级', // 0.14
      'GPTPro-对接池', // 0.12
      'GPTPro-低价池', // 0.11
      '自用模型组', // 0.1
      'gpt特惠组', // 0.09
      'GPT对接组', // 0.08
    ])

    // Gemini family comes third (4 groups), ratio descending
    const geminiSubset = sorted.slice(12, 16)
    expect(geminiSubset).toEqual([
      '香蕉渠道组', // 1.9
      '大香蕉对接组', // 0.8
      '小香蕉对接组', // 0.8
      'Gemini组', // 0.15
    ])

    // Grok comes fourth (2 groups), ratio descending
    const grokSubset = sorted.slice(16, 18)
    expect(grokSubset).toEqual([
      'supergrok组', // 0.15
      'grok分组', // 0.04
    ])

    // Image media comes fifth (3 groups), ratio descending
    const imageSubset = sorted.slice(18, 21)
    expect(imageSubset).toEqual([
      'AzImage2', // 1.8
      '生成图片组', // 1.4
      'Seedance', // 1.0
    ])
  })

  it('respects user custom ratios when sorting', () => {
    const raw = ['aswb专用分组外接', 'CCMAX', '高缓kiro99']
    const groupMeta = {
      aswb专用分组外接: { ratio: 3.5 },
      CCMAX: { ratio: 0.95 },
      高缓kiro99: { ratio: 0.1 },
    }
    const userGroupRatios = {
      高缓kiro99: 10.0,
    }
    const sorted = sortGroupsByFamilyAndRatio(raw, { groupMeta, userGroupRatios })
    expect(sorted[0]).toBe('高缓kiro99')
    expect(sorted[1]).toBe('aswb专用分组外接')
    expect(sorted[2]).toBe('CCMAX')
  })
})
