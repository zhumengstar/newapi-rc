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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { render, renderHook, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import type { Channel } from '../../types'
import {
  ChannelRatioCell,
  NameCell,
  SiteBalanceCell,
  SiteTypeCell,
  useChannelsColumns,
} from '../channels-columns'

const updateChannel = vi.hoisted(() => vi.fn())
const updateChannelBalance = vi.hoisted(() => vi.fn())
const setCurrentRow = vi.hoisted(() => vi.fn())
const setOpen = vi.hoisted(() => vi.fn())

vi.mock('../../api', () => ({
  getCodexUsage: vi.fn(),
  updateChannel,
  updateChannelBalance,
}))

vi.mock('@/features/pricing/hooks', () => ({
  usePricingData: () => ({ groupRatio: {} }),
}))

vi.mock('@/components/provider-badge', () => ({
  ProviderBadge: () => null,
}))

vi.mock('../channels-provider', () => ({
  useChannels: () => ({
    sensitiveVisible: true,
    setCurrentRow,
    setOpen,
  }),
}))

const channels: Channel[] = []

function useChannelsTableForTest() {
  const columns = useChannelsColumns({ enableSelection: false })
  return useReactTable({
    data: channels,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
}

test('allows sorting the used quota column', () => {
  const { result } = renderHook(useChannelsTableForTest)
  const column = result.current.getColumn('used_quota')

  expect(column).toBeDefined()
  expect(column?.getCanSort()).toBe(true)
})

test('allows sorting the channel ratio column', () => {
  const { result } = renderHook(useChannelsTableForTest)
  const column = result.current.getColumn('channel_ratio')

  expect(column).toBeDefined()
  expect(column?.getCanSort()).toBe(true)
})

test('opens balance settings without querying upstream when site balance is clicked', async () => {
  const user = userEvent.setup()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const channel = {
    id: 12,
    type: 1,
    name: 'Example channel',
    balance: 12.5,
    balance_updated_time: 1,
  } as Channel
  setCurrentRow.mockClear()
  setOpen.mockClear()
  updateChannelBalance.mockClear()

  render(
    <QueryClientProvider client={queryClient}>
      <SiteBalanceCell channel={channel} />
    </QueryClientProvider>
  )

  await user.click(screen.getByText('$12.5'))

  expect(setCurrentRow).toHaveBeenCalledWith(channel)
  expect(setOpen).toHaveBeenCalledWith('balance-query')
  expect(updateChannelBalance).not.toHaveBeenCalled()
})

test('edits a channel ratio by clicking its value', async () => {
  const user = userEvent.setup()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  updateChannel.mockResolvedValue({ success: true })

  render(
    <QueryClientProvider client={queryClient}>
      <ChannelRatioCell channel={{ id: 7, channel_ratio: 0.07 } as Channel} />
    </QueryClientProvider>
  )

  await user.click(screen.getByRole('button', { name: 'Edit Channel Ratio' }))
  const input = screen.getByRole('textbox', { name: 'Channel Ratio' })
  await user.clear(input)
  await user.type(input, '0.125{Enter}')

  await waitFor(() =>
    expect(updateChannel).toHaveBeenCalledWith(7, { channel_ratio: 0.125 })
  )
  expect(
    await screen.findByRole('button', { name: 'Edit Channel Ratio' })
  ).toHaveTextContent('0.125x')
})

test('edits a channel name by clicking its value', async () => {
  const user = userEvent.setup()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  updateChannel.mockResolvedValue({ success: true })
  const channel = { id: 11, name: 'Original name' } as Channel

  render(
    <QueryClientProvider client={queryClient}>
      <NameCell channel={channel} sensitiveVisible />
    </QueryClientProvider>
  )

  await user.click(screen.getByRole('button', { name: 'Edit Name' }))
  const input = screen.getByRole('textbox', { name: 'Name' })
  await user.clear(input)
  await user.type(input, 'Updated name{Enter}')

  await waitFor(() =>
    expect(updateChannel).toHaveBeenCalledWith(11, { name: 'Updated name' })
  )
})

test('shows a compact site type label and saves the selected value', async () => {
  const user = userEvent.setup()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  updateChannel.mockClear()
  updateChannel.mockResolvedValue({ success: true })

  render(
    <QueryClientProvider client={queryClient}>
      <SiteTypeCell
        channel={{ id: 13, type: 1, site_type: 'newapi' } as Channel}
      />
    </QueryClientProvider>
  )

  expect(
    screen.getByRole('button', { name: 'Edit Site Type' })
  ).toHaveTextContent('NewAPI')
  expect(screen.queryByRole('combobox')).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Edit Site Type' }))
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Site Type' }),
    'sub2api'
  )

  await waitFor(() =>
    expect(updateChannel).toHaveBeenCalledWith(13, { site_type: 'sub2api' })
  )
})

test('clears a channel ratio when the editor is emptied', async () => {
  const user = userEvent.setup()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  updateChannel.mockResolvedValue({ success: true })

  render(
    <QueryClientProvider client={queryClient}>
      <ChannelRatioCell channel={{ id: 8, channel_ratio: 0.25 } as Channel} />
    </QueryClientProvider>
  )

  await user.click(screen.getByRole('button', { name: 'Edit Channel Ratio' }))
  const input = screen.getByRole('textbox', { name: 'Channel Ratio' })
  await user.clear(input)
  await user.tab()

  await waitFor(() =>
    expect(updateChannel).toHaveBeenCalledWith(8, { channel_ratio: null })
  )
  expect(
    await screen.findByRole('button', { name: 'Edit Channel Ratio' })
  ).toHaveTextContent('-')
})

test('escape cancels a channel ratio edit without saving', async () => {
  const user = userEvent.setup()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  updateChannel.mockClear()

  render(
    <QueryClientProvider client={queryClient}>
      <ChannelRatioCell channel={{ id: 9, channel_ratio: 0.5 } as Channel} />
    </QueryClientProvider>
  )

  await user.click(screen.getByRole('button', { name: 'Edit Channel Ratio' }))
  const input = screen.getByRole('textbox', { name: 'Channel Ratio' })
  await user.clear(input)
  await user.type(input, '0.75')
  await user.keyboard('{Escape}')

  expect(updateChannel).not.toHaveBeenCalled()
  expect(
    screen.getByRole('button', { name: 'Edit Channel Ratio' })
  ).toHaveTextContent('0.5x')
})

test('a new edit after escape still saves on blur', async () => {
  const user = userEvent.setup()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  updateChannel.mockClear()
  updateChannel.mockResolvedValue({ success: true })

  render(
    <QueryClientProvider client={queryClient}>
      <ChannelRatioCell channel={{ id: 10, channel_ratio: 0.5 } as Channel} />
    </QueryClientProvider>
  )

  await user.click(screen.getByRole('button', { name: 'Edit Channel Ratio' }))
  await user.keyboard('{Escape}')
  await user.click(screen.getByRole('button', { name: 'Edit Channel Ratio' }))
  const input = screen.getByRole('textbox', { name: 'Channel Ratio' })
  await user.clear(input)
  await user.type(input, '0.75')
  await user.tab()

  await waitFor(() =>
    expect(updateChannel).toHaveBeenCalledWith(10, { channel_ratio: 0.75 })
  )
})
