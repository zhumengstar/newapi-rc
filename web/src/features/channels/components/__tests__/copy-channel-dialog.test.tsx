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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { createAppQueryClient } from '@/lib/query-client'
import * as channelApi from '../../api'
import { ChannelsProvider, useChannels } from '../channels-provider'
import { CopyChannelDialog } from '../dialogs/copy-channel-dialog'

vi.mock('../../api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api')>()
  return {
    ...actual,
    getGroups: vi.fn().mockResolvedValue({ success: true, data: ['default', 'vip'] }),
    getChannelGroupPriorities: vi.fn().mockResolvedValue({ success: true, data: { default: 1, vip: 11 } }),
    copyChannel: vi.fn().mockResolvedValue({ success: true, data: { id: 100 } }),
  }
})

function TestWrapper({ channelName = 'https://api.valxv.cn' }: { channelName?: string }) {
  const { setCurrentRow, setOpen } = useChannels()

  return (
    <div>
      <button
        onClick={() => {
          setCurrentRow({
            id: 42,
            name: channelName,
            type: 1,
            key: 'sk-test',
            status: 1,
            group: 'default',
            priority: 1,
            models: 'gpt-4',
          } as any)
          setOpen('copy')
        }}
      >
        Open Copy Dialog
      </button>
      <CopyChannelConsumer />
    </div>
  )
}

function CopyChannelConsumer() {
  const { open, setOpen } = useChannels()
  return (
    <CopyChannelDialog
      open={open === 'copy'}
      onOpenChange={(isOpen) => setOpen(isOpen ? 'copy' : null)}
    />
  )
}

describe('CopyChannelDialog', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    vi.clearAllMocks()
    queryClient = createAppQueryClient()
  })

  afterEach(() => {
    queryClient.clear()
  })

  it('pre-fills the exact channel name without any _copy suffix and allows directly overriding name', async () => {
    render(
      <QueryClientProvider client={queryClient}>
        <ChannelsProvider>
          <TestWrapper channelName='https://api.valxv.cn' />
        </ChannelsProvider>
      </QueryClientProvider>
    )

    // Open dialog
    fireEvent.click(screen.getByText('Open Copy Dialog'))

    // Channel name input should directly have original name, NOT _copy
    const input = (await screen.findByLabelText(/渠道名称|Channel Name/i)) as HTMLInputElement
    expect(input.value).toBe('https://api.valxv.cn')
    expect(input.value).not.toContain('_copy')

    // Directly override the channel name
    fireEvent.change(input, { target: { value: 'https://new-endpoint.cn' } })
    expect(input.value).toBe('https://new-endpoint.cn')

    // Submit copy
    const submitButton = screen.getByRole('button', { name: /复制渠道|Copy Channel/i })
    expect(submitButton).not.toBeDisabled()
    fireEvent.click(submitButton)

    await waitFor(() => {
      expect(channelApi.copyChannel).toHaveBeenCalledWith(
        42,
        expect.objectContaining({
          name: 'https://new-endpoint.cn',
          reset_balance: true,
          group: 'default',
        })
      )
    })
  })

  it('disables submit button when channel name is emptied', async () => {
    render(
      <QueryClientProvider client={queryClient}>
        <ChannelsProvider>
          <TestWrapper channelName='test-channel' />
        </ChannelsProvider>
      </QueryClientProvider>
    )

    fireEvent.click(screen.getByText('Open Copy Dialog'))

    const input = (await screen.findByLabelText(/渠道名称|Channel Name/i)) as HTMLInputElement
    fireEvent.change(input, { target: { value: '   ' } })

    const submitButton = screen.getByRole('button', { name: /复制渠道|Copy Channel/i })
    expect(submitButton).toBeDisabled()
  })
})
