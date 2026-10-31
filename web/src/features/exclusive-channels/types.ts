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
import type { Channel } from '@/features/channels/types'
import type { User } from '@/features/users/types'

export interface ChannelExclusiveUpdateRequest {
  group?: string
  allowed_user_ids?: number[]
  status?: number
}

export interface EnrichedChannel extends Channel {
  allowedUserIds: number[]
  groupsList: string[]
  isExclusive: boolean
}

export type UserChannelBindingInfo = {
  user: User
  exclusiveChannels: EnrichedChannel[]
}

export interface GroupIncomeItem {
  group: string
  today_quota: number
  today_income: number
}

export interface SourceModelCostRule {
  group: string // '*' for all groups under this source, or specific group name
  models: string[]
  mode: 'per_call' | 'per_token'
  price?: number // used when mode === 'per_call' ($/call)
  input_price?: number // used when mode === 'per_token' ($/1M input tokens)
  output_price?: number // used when mode === 'per_token' ($/1M output tokens)
}

export interface UpstreamSourceDTO {
  base_url: string
  display_base_url: string
  wechat_name: string
  channel_count: number
  enabled_channel_count: number
  channel_ids: number[]
  channel_names: string[]
  groups: string[]
  group_channel_counts?: Record<string, number>
  group_models?: Record<string, string[]>
  group_customer_ratios?: Record<string, number>
  channel_ratio: number
  default_cost_ratio?: number
  group_cost_ratios?: Record<string, number>
  model_cost_rules?: SourceModelCostRule[]
  cost_tier: number
  models: string[]
  model_prices: Record<string, number>
  today_quota?: number
  today_income?: number
  group_incomes?: Record<string, GroupIncomeItem>
}

export interface ChannelIncomeItem {
  channel_id: number
  today_quota: number
  today_income: number
}

export interface ChannelTodayIncomeResponse {
  incomes: Record<number, ChannelIncomeItem>
  channel_group_incomes?: Record<number, Record<string, GroupIncomeItem>>
  total_quota: number
  total_income: number
  next_update_at: number
  interval_minutes: number
}

export interface UpdateUpstreamSourceRequest {
  base_url: string
  wechat_name?: string
  channel_ratio?: number
  default_cost_ratio?: number
  group_cost_ratios?: Record<string, number>
  model_cost_rules?: SourceModelCostRule[]
  group?: string
  apply_group_to_all_channels?: boolean
  cost_tier?: number
  model_prices?: Record<string, number>
}
