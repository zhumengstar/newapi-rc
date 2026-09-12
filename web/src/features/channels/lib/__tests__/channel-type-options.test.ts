/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'vitest'

import {
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_NEW_API,
  CHANNEL_TYPE_SUB2_API,
  CHANNEL_TYPE_TASK_PLUGIN,
  channelTypeOptionsForTaskPluginBind,
} from '../../constants'
import {
  getChannelSiteTypeLabel,
  getDetectedChannelSiteTypeLabel,
  extractTrailingContact,
  replaceTrailingContact,
} from '../channel-utils'

describe('channel type options for task plugin bind', () => {
  test('hides the task plugin type when the caller cannot bind', () => {
    const options = channelTypeOptionsForTaskPluginBind(false)

    expect(
      options.some((option) => option.value === CHANNEL_TYPE_TASK_PLUGIN)
    ).toBe(false)
  })

  test('shows the task plugin type when the caller can bind', () => {
    const options = channelTypeOptionsForTaskPluginBind(true)

    expect(options).toEqual(CHANNEL_TYPE_OPTIONS)
    expect(
      options.some((option) => option.value === CHANNEL_TYPE_TASK_PLUGIN)
    ).toBe(true)
  })
})

describe('channel name contact suffixes', () => {
  test('extracts an email, phone number, QQ number, or handle at the end of a name', () => {
    expect(extractTrailingContact('provider - sales@example.com')).toBe(
      'sales@example.com'
    )
    expect(extractTrailingContact('provider 13800138000')).toBe('13800138000')
    expect(extractTrailingContact('provider：12345678')).toBe('12345678')
    expect(extractTrailingContact('provider @support')).toBe('@support')
  })

  test('replaces only the trailing contact and can remove it', () => {
    expect(
      replaceTrailingContact('provider - old@example.com', 'new@example.com')
    ).toBe('provider - new@example.com')
    expect(replaceTrailingContact('provider - old@example.com', '')).toBe(
      'provider'
    )
  })
})

describe('channel site type labels', () => {
  test('identifies the NewAPI and Sub2API channel types', () => {
    expect(getChannelSiteTypeLabel(CHANNEL_TYPE_NEW_API)).toBe('NewAPI')
    expect(getChannelSiteTypeLabel(CHANNEL_TYPE_SUB2_API)).toBe('Sub2API')
  })

  test('does not infer a site family for unrelated providers', () => {
    expect(getChannelSiteTypeLabel(1)).toBeNull()
  })

  test('renders a detected site family independently of provider type', () => {
    expect(getDetectedChannelSiteTypeLabel('newapi')).toBe('NewAPI')
    expect(getDetectedChannelSiteTypeLabel('sub2api')).toBe('Sub2API')
    expect(getDetectedChannelSiteTypeLabel('unknown')).toBeNull()
  })
})
