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
import { useQuery } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import {
  ExternalLink,
  Layers,
  Loader2,
  RefreshCw,
  ShieldCheck,
  UserCheck,
} from 'lucide-react'
import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { getChannels } from '@/features/channels/api'
import type { Channel } from '@/features/channels/types'
import { getGroups, getUser } from '@/features/users/api'
import type { User } from '@/features/users/types'

import { ExclusiveChannelsTable } from './components/exclusive-channels-table'
import { ExclusiveSummaryCards } from './components/exclusive-summary-cards'
import { QuickExclusiveDialog } from './components/quick-exclusive-dialog'
import { UserExclusiveView } from './components/user-exclusive-view'
import type { EnrichedChannel } from './types'

export function ExclusiveChannels() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  // 尝试读取 URL 中的 channel_id 查询参数
  const searchParams = useSearch({ strict: false }) as Record<string, any>
  const targetChannelId = searchParams?.channel_id
    ? Number(searchParams.channel_id)
    : searchParams?.search
      ? Number(searchParams.search)
      : null

  // 弹窗状态
  const [editingChannel, setEditingChannel] = useState<EnrichedChannel | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<'channels' | 'users'>('channels')

  // 拉取所有渠道列表
  const {
    data: channelsRawData,
    isLoading: isLoadingChannels,
    refetch: refetchChannels,
    isRefetching,
  } = useQuery({
    queryKey: ['exclusive-channels-list'],
    queryFn: async () => {
      const res = await getChannels({ page_size: 1000 })
      return res?.data?.items || []
    },
    staleTime: 60 * 1000,
  })

  // 拉取全量分组列表
  const { data: groupsData } = useQuery({
    queryKey: ['exclusive-channel-groups'],
    queryFn: async () => {
      const res = await getGroups()
      return res?.data || []
    },
    staleTime: 5 * 60 * 1000,
  })

  const allGroups = groupsData || ['default']

  // 将渠道丰富化
  const enrichedChannels: EnrichedChannel[] = useMemo(() => {
    if (!channelsRawData) return []
    return channelsRawData.map((c: Channel) => {
      let allowedUserIds: number[] = []
      if (c.settings) {
        try {
          const parsed = JSON.parse(c.settings)
          if (Array.isArray(parsed.allowed_user_ids)) {
            allowedUserIds = parsed.allowed_user_ids
              .map(Number)
              .filter((n: number) => Number.isFinite(n) && n > 0)
          }
        } catch {
          // ignore error
        }
      }

      const groupsList = (c.group || 'default')
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)

      return {
        ...c,
        allowedUserIds,
        groupsList,
        isExclusive: allowedUserIds.length > 0,
      }
    })
  }, [channelsRawData])

  // 收集所有专属渠道涉及的客户 ID
  const allBoundUserIds = useMemo(() => {
    const ids = new Set<number>()
    for (const c of enrichedChannels) {
      for (const uid of c.allowedUserIds) {
        ids.add(uid)
      }
    }
    return Array.from(ids)
  }, [enrichedChannels])

  // 批量拉取已绑定的客户信息
  const { data: boundUsersList } = useQuery({
    queryKey: ['exclusive_all_bound_users', allBoundUserIds.sort().join(',')],
    queryFn: async () => {
      if (allBoundUserIds.length === 0) return []
      const results = await Promise.all(
        allBoundUserIds.map(async (id) => {
          try {
            const res = await getUser(id)
            return res?.data || null
          } catch {
            return null
          }
        })
      )
      return results.filter(Boolean) as User[]
    },
    enabled: allBoundUserIds.length > 0,
    staleTime: 5 * 60 * 1000,
  })

  // 缓存用户 Map
  const userMap = useMemo(() => {
    const map = new Map<number, User>()
    if (boundUsersList) {
      for (const u of boundUsersList) {
        map.set(u.id, u)
      }
    }
    return map
  }, [boundUsersList])

  const handleOpenEdit = (channel: EnrichedChannel) => {
    setEditingChannel(channel)
    setDialogOpen(true)
  }

  // 双向联动：跳转到原渠道管理界面
  const handleNavigateToChannelConfig = (channelId: number) => {
    navigate({
      to: '/channels',
      search: {
        filter: String(channelId),
      },
    })
  }

  return (
    <SectionPageLayout fixedContent>
      {/* 顶部标题栏与操作 */}
      <SectionPageLayout.Title>
        <div className='flex items-center gap-2'>
          <ShieldCheck className='size-5 text-primary' />
          <span>{t('Exclusive Channels', '专属渠道管理')}</span>
        </div>
      </SectionPageLayout.Title>

      <SectionPageLayout.Actions>
        <div className='flex items-center gap-2'>
          <Button
            variant='outline'
            size='sm'
            onClick={() => refetchChannels()}
            disabled={isLoadingChannels || isRefetching}
            className='h-8 text-xs'
          >
            <RefreshCw
              className={`size-3.5 mr-1.5 ${isRefetching ? 'animate-spin' : ''}`}
            />
            {t('Refresh', '刷新数据')}
          </Button>

          {/* 联动按钮：前往标准渠道管理 */}
          <Button
            variant='secondary'
            size='sm'
            onClick={() => navigate({ to: '/channels' })}
            className='h-8 text-xs'
          >
            <ExternalLink className='size-3.5 mr-1.5' />
            {t('All Channels Configuration', '标准渠道管理')}
          </Button>
        </div>
      </SectionPageLayout.Actions>

      {/* 主体内容 */}
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-4 overflow-y-auto pb-6'>
          {/* 统计指标卡片 */}
          <ExclusiveSummaryCards channels={enrichedChannels} />

          {/* 视图切换 Tabs */}
          <Tabs
            value={activeTab}
            onValueChange={(v) => setActiveTab(v as 'channels' | 'users')}
            className='w-full'
          >
            <div className='flex items-center justify-between pb-2 border-b'>
              <TabsList className='h-8'>
                <TabsTrigger value='channels' className='text-xs px-3 gap-1.5'>
                  <Layers className='size-3.5' />
                  <span>{t('Channel Perspective', '渠道与分组视角')}</span>
                </TabsTrigger>
                <TabsTrigger value='users' className='text-xs px-3 gap-1.5'>
                  <UserCheck className='size-3.5' />
                  <span>{t('Client Dedicated Perspective', '客户专属透视')}</span>
                </TabsTrigger>
              </TabsList>
            </div>

            {isLoadingChannels ? (
              <div className='flex flex-col items-center justify-center py-20 text-xs text-muted-foreground'>
                <Loader2 className='size-8 animate-spin text-primary opacity-80 mb-2' />
                <span>{t('Loading channel and routing data...', '正在加载渠道与分组数据...')}</span>
              </div>
            ) : (
              <>
                <TabsContent value='channels' className='mt-3 space-y-4'>
                  <ExclusiveChannelsTable
                    channels={enrichedChannels}
                    allGroups={allGroups}
                    userMap={userMap}
                    onOpenEditDialog={handleOpenEdit}
                    onNavigateToChannelConfig={handleNavigateToChannelConfig}
                    highlightChannelId={Number.isFinite(targetChannelId) ? targetChannelId : null}
                  />
                </TabsContent>

                <TabsContent value='users' className='mt-3 space-y-4'>
                  <UserExclusiveView
                    channels={enrichedChannels}
                    userMap={userMap}
                    onOpenEditDialog={handleOpenEdit}
                    onNavigateToChannelConfig={handleNavigateToChannelConfig}
                  />
                </TabsContent>
              </>
            )}
          </Tabs>
        </div>
      </SectionPageLayout.Content>

      {/* 快捷配置弹窗 */}
      <QuickExclusiveDialog
        channel={editingChannel}
        allGroups={allGroups}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onNavigateToChannelConfig={handleNavigateToChannelConfig}
      />
    </SectionPageLayout>
  )
}
export default ExclusiveChannels
