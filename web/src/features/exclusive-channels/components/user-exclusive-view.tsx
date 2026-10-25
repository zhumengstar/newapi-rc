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
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  ExternalLink,
  Search,
  Settings2,
  ShieldCheck,
  UserCheck,
} from 'lucide-react'
import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { CHANNEL_TYPES } from '@/features/channels/constants'
import type { User } from '@/features/users/types'

import type { EnrichedChannel } from '../types'

interface UserExclusiveViewProps {
  channels: EnrichedChannel[]
  userMap: Map<number, User>
  onOpenEditDialog: (channel: EnrichedChannel) => void
  onNavigateToChannelConfig: (channelId: number) => void
}

export function UserExclusiveView({
  channels,
  userMap,
  onOpenEditDialog,
  onNavigateToChannelConfig,
}: UserExclusiveViewProps) {
  const { t } = useTranslation()
  const [userSearchTerm, setUserSearchTerm] = useState('')

  // 聚合并组织所有有专属绑定的用户列表
  const userBindings = useMemo(() => {
    const userToChannels = new Map<number, EnrichedChannel[]>()

    for (const c of channels) {
      if (c.isExclusive) {
        for (const uid of c.allowedUserIds) {
          if (!userToChannels.has(uid)) {
            userToChannels.set(uid, [])
          }
          userToChannels.get(uid)!.push(c)
        }
      }
    }

    const list: {
      userId: number
      user?: User
      exclusiveChannels: EnrichedChannel[]
    }[] = []

    for (const [uid, userChannels] of userToChannels.entries()) {
      list.push({
        userId: uid,
        user: userMap.get(uid),
        exclusiveChannels: userChannels,
      })
    }

    // 按渠道数降序排列
    return list.sort((a, b) => b.exclusiveChannels.length - a.exclusiveChannels.length)
  }, [channels, userMap])

  // 搜索过滤
  const filteredUsers = useMemo(() => {
    if (!userSearchTerm.trim()) return userBindings
    const kw = userSearchTerm.trim().toLowerCase()
    return userBindings.filter((item) => {
      if (String(item.userId).includes(kw)) return true
      if (item.user) {
        if (item.user.username.toLowerCase().includes(kw)) return true
        if (item.user.display_name?.toLowerCase().includes(kw)) return true
        if (item.user.group?.toLowerCase().includes(kw)) return true
      }
      return false
    })
  }, [userBindings, userSearchTerm])

  return (
    <div className='space-y-4'>
      {/* 搜索工具栏 */}
      <div className='flex items-center justify-between gap-3 rounded-lg border bg-card p-3 shadow-2xs'>
        <div className='relative w-full max-w-sm'>
          <Search className='absolute left-2.5 top-2.5 size-3.5 text-muted-foreground' />
          <Input
            placeholder={t('Search bound users by username, ID or group...', '搜索专属客户用户名、ID 或分组...')}
            value={userSearchTerm}
            onChange={(e) => setUserSearchTerm(e.target.value)}
            className='h-8 pl-8 text-xs'
          />
        </div>
        <div className='text-xs text-muted-foreground'>
          {t('Total exclusive clients', '共有')} <strong className='text-foreground'>{userBindings.length}</strong>{' '}
          {t('clients configured with dedicated channels', '位客户配置了专属渠道')}
        </div>
      </div>

      {/* 用户专属渠道卡片列表 */}
      {filteredUsers.length === 0 ? (
        <div className='flex flex-col items-center justify-center rounded-lg border border-dashed py-12 text-center text-xs text-muted-foreground bg-muted/10'>
          <UserCheck className='size-8 opacity-30 mb-2' />
          <p className='font-medium text-foreground'>{t('No dedicated users found', '暂无专属客户数据')}</p>
          <p className='mt-1 max-w-sm'>
            {userSearchTerm
              ? t('No users match your search criteria.', '未找到符合条件的客户')
              : t('Configure exclusive channels for users in the Channel View to see them here.', '在渠道视角为客户绑定专属通道后将在此处汇总透视。')}
          </p>
        </div>
      ) : (
        <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
          {filteredUsers.map(({ userId, user, exclusiveChannels }) => {
            const enabledCount = exclusiveChannels.filter((c) => c.status === 1).length
            const disabledCount = exclusiveChannels.length - enabledCount
            const hasRisk = enabledCount === 0 && exclusiveChannels.length > 0

            return (
              <Card
                key={userId}
                className={`overflow-hidden border shadow-xs transition-shadow hover:shadow-sm ${
                  hasRisk ? 'border-destructive/40 bg-destructive/5' : ''
                }`}
              >
                <CardContent className='p-4 space-y-3'>
                  {/* 用户信息标头 */}
                  <div className='flex items-start justify-between gap-2 border-b pb-3'>
                    <div className='space-y-1 min-w-0'>
                      <div className='flex items-center gap-2'>
                        <span className='font-semibold text-foreground text-sm truncate'>
                          {user ? user.username : `ID #${userId}`}
                        </span>
                        <Badge variant='outline' className='font-mono text-[10px] px-1 py-0'>
                          #{userId}
                        </Badge>
                        {user?.display_name && (
                          <span className='text-xs text-muted-foreground truncate'>
                            ({user.display_name})
                          </span>
                        )}
                      </div>
                      <div className='flex items-center gap-2 text-xs text-muted-foreground'>
                        <span>{t('User Groups', '所属分组')}:</span>
                        <div className='flex flex-wrap gap-1'>
                          {(user?.group || 'default').split(',').map((g) => (
                            <Badge key={g} variant='secondary' className='text-[10px] font-mono px-1 py-0'>
                              {g.trim()}
                            </Badge>
                          ))}
                        </div>
                      </div>
                    </div>

                    {/* 状态徽章 */}
                    <div>
                      {hasRisk ? (
                        <Badge variant='destructive' className='text-xs gap-1 py-0.5'>
                          <AlertCircle className='size-3' />
                          {t('Traffic at Risk!', '存在断流风险')}
                        </Badge>
                      ) : disabledCount > 0 ? (
                        <Badge variant='outline' className='text-xs gap-1 border-amber-500/50 text-amber-600 dark:text-amber-400'>
                          <AlertTriangle className='size-3' />
                          {disabledCount} {t('Disabled', '个停用')}
                        </Badge>
                      ) : (
                        <Badge variant='outline' className='text-xs gap-1 border-emerald-500/50 text-emerald-600 dark:text-emerald-400'>
                          <CheckCircle2 className='size-3' />
                          {t('All channels healthy', '专属通道正常')}
                        </Badge>
                      )}
                    </div>
                  </div>

                  {/* 专属通道列表 */}
                  <div className='space-y-1.5'>
                    <div className='flex items-center justify-between text-xs text-muted-foreground'>
                      <span className='font-medium text-foreground flex items-center gap-1'>
                        <ShieldCheck className='size-3.5 text-primary' />
                        {t('Dedicated Channels', '专享渠道列表')} ({exclusiveChannels.length})
                      </span>
                      <span className='text-[11px]'>
                        {enabledCount} {t('online', '在线可用')} / {exclusiveChannels.length} {t('total', '总数')}
                      </span>
                    </div>

                    <div className='space-y-1.5 max-h-48 overflow-y-auto pr-1'>
                      {exclusiveChannels.map((c) => {
                        const typeName = (CHANNEL_TYPES as any)[c.type] || t('Custom', '自定义')
                        return (
                          <div
                            key={c.id}
                            className={`flex items-center justify-between rounded-md border p-2 text-xs transition-colors ${
                              c.status === 1
                                ? 'bg-background hover:bg-muted/30'
                                : 'bg-muted/40 opacity-75'
                            }`}
                          >
                            <div className='space-y-0.5 min-w-0 flex-1 mr-2'>
                              <div className='flex items-center gap-1.5'>
                                <span className='font-medium text-foreground truncate'>
                                  {c.name}
                                </span>
                                <span className='font-mono text-[10px] text-muted-foreground'>
                                  #{c.id}
                                </span>
                                <Badge variant='outline' className='text-[9px] px-1 py-0 h-3.5'>
                                  {typeName}
                                </Badge>
                              </div>
                              <div className='flex items-center gap-1 text-[11px] text-muted-foreground'>
                                <span>{t('Groups', '所属分组')}:</span>
                                <span className='font-mono truncate max-w-[150px]'>{c.group}</span>
                              </div>
                            </div>

                            <div className='flex items-center gap-1.5 shrink-0'>
                              <Badge
                                variant={c.status === 1 ? 'default' : 'secondary'}
                                className={`text-[10px] px-1.5 py-0 h-4 ${
                                  c.status === 1
                                    ? 'bg-emerald-600/90 text-white'
                                    : 'bg-muted text-muted-foreground'
                                }`}
                              >
                                {c.status === 1 ? t('Online', '在线') : t('Offline', '停用')}
                              </Badge>

                              <Button
                                variant='ghost'
                                size='sm'
                                className='h-6 w-6 p-0 text-muted-foreground hover:text-foreground'
                                onClick={() => onOpenEditDialog(c)}
                                title={t('Configure Channel', '配置渠道')}
                              >
                                <Settings2 className='size-3' />
                              </Button>

                              <Button
                                variant='ghost'
                                size='sm'
                                className='h-6 w-6 p-0 text-muted-foreground hover:text-foreground'
                                onClick={() => onNavigateToChannelConfig(c.id)}
                                title={t('Open full channel config', '打开完整配置')}
                              >
                                <ExternalLink className='size-3' />
                              </Button>
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}
    </div>
  )
}
