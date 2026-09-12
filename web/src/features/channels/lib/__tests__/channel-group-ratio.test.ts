/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import { sortGroupsByModelAndRatio, sortGroupsByRatio } from '../channel-utils'

describe('channel group pricing ratios', () => {
  test('sorts configured groups by public ratio descending', () => {
    expect(
      sortGroupsByRatio(['low', 'high', 'free', 'unconfigured'], {
        low: 0.5,
        high: 2,
        free: 0,
      })
    ).toEqual(['high', 'low', 'free', 'unconfigured'])
  })

  test('does not mutate the channel group list', () => {
    const groups = ['default', 'vip']
    expect(sortGroupsByRatio(groups, { default: 1, vip: 2 })).toEqual([
      'vip',
      'default',
    ])
    expect(groups).toEqual(['default', 'vip'])
  })

  test('uses enabled models before group-name fallback to keep families together', () => {
    const groups = [
      'CCMAX',
      'gpt-discount',
      'banana',
      'high-buffer',
      'GPTPro-low',
      'custom-high',
    ]
    const groupModels = new Map<string, string[]>([
      ['CCMAX', ['claude-3-7-sonnet']],
      ['gpt-discount', ['gpt-4.1']],
      ['banana', ['gemini-2.5-pro']],
      ['high-buffer', ['claude-3-5-haiku']],
    ])

    expect(
      sortGroupsByModelAndRatio(
        groups,
        {
          CCMAX: 0.95,
          'gpt-discount': 0.09,
          banana: 0.15,
          'high-buffer': 0.2,
          'GPTPro-low': 0.12,
          'custom-high': 0.99,
        },
        groupModels
      )
    ).toEqual([
      'GPTPro-low',
      'gpt-discount',
      'CCMAX',
      'high-buffer',
      'banana',
      'custom-high',
    ])
  })

  test('keeps mixed-model groups after single-family groups', () => {
    expect(
      sortGroupsByModelAndRatio(
        ['mixed', 'gpt'],
        { mixed: 10, gpt: 0.1 },
        new Map([
          ['mixed', ['gpt-4.1', 'claude-3-7-sonnet']],
          ['gpt', ['gpt-4.1']],
        ])
      )
    ).toEqual(['gpt', 'mixed'])
  })

  test('uses a two-thirds model majority as the group family', () => {
    expect(
      sortGroupsByModelAndRatio(
        ['mostly-gpt', 'claude'],
        { 'mostly-gpt': 0.1, claude: 1 },
        new Map([
          ['mostly-gpt', ['gpt-4.1', 'gpt-5', 'claude-3-7-sonnet']],
          ['claude', ['claude-3-7-sonnet']],
        ])
      )
    ).toEqual(['mostly-gpt', 'claude'])
  })

  test('orders unrecognized groups by public ratio after model families', () => {
    expect(
      sortGroupsByModelAndRatio(['custom-low', 'GPTPro', 'custom-high'], {
        'custom-low': 0.1,
        GPTPro: 0.2,
        'custom-high': 0.9,
      })
    ).toEqual(['GPTPro', 'custom-high', 'custom-low'])
  })
})
