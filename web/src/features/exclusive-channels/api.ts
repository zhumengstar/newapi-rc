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
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  ChannelExclusiveUpdateRequest,
  UpstreamSourceDTO,
  UpdateUpstreamSourceRequest,
  ChannelTodayIncomeResponse,
} from './types'

export async function getChannelTodayIncome(
  force?: boolean
): Promise<ChannelTodayIncomeResponse> {
  const response = await api.get<{
    success: boolean
    message?: string
    data: ChannelTodayIncomeResponse
  }>('/api/channel/today_income', {
    params: force ? { force: 'true' } : undefined,
  })
  return requireServerSuccess(response.data).data
}

export async function getChannelSources(): Promise<UpstreamSourceDTO[]> {
  const response = await api.get<{
    success: boolean
    message?: string
    data: UpstreamSourceDTO[]
  }>('/api/channel/sources')
  return requireServerSuccess(response.data).data
}

export async function updateChannelSource(
  data: UpdateUpstreamSourceRequest
): Promise<any> {
  const response = await api.put<{
    success: boolean
    message?: string
    data: any
  }>('/api/channel/sources', data)
  return requireServerSuccess(response.data).data
}

export async function updateChannelExclusive(
  id: number,
  data: ChannelExclusiveUpdateRequest
): Promise<Channel> {
  const response = await api.put<{
    success: boolean
    message?: string
    data: Channel
  }>(`/api/channel/${id}/exclusive`, data)
  return requireServerSuccess(response.data).data
}

export async function updateChannelStatus(
  id: number,
  status: number
): Promise<boolean> {
  const response = await api.post<{
    success: boolean
    message?: string
    data: boolean
  }>(`/api/channel/${id}/status`, { status })
  return requireServerSuccess(response.data).data
}
