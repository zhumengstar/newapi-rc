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
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ExternalLink,
  Loader2,
  Plus,
  Search,
  Trash2,
  User as UserIcon,
  Users,
} from 'lucide-react'
import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { searchUsers, getUser } from '@/features/users/api'
import type { User } from '@/features/users/types'

import { updateChannelExclusive, updateChannelStatus } from '../api'
import type { EnrichedChannel } from '../types'

interface QuickExclusiveDialogProps {
  channel: EnrichedChannel | null
  allGroups: string[]
  open: boolean
  onOpenChange: (open: boolean) => void
  onNavigateToChannelConfig?: (channelId: number) => void
}

export function QuickExclusiveDialog({
  channel,
  allGroups,
  open,
  onOpenChange,
  onNavigateToChannelConfig,
}: QuickExclusiveDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [status, setStatus] = useState<number>(1)
  const [selectedGroups, setSelectedGroups] = useState<string[]>([])
  const [customGroupInput, setCustomGroupInput] = useState('')
  const [allowedUserIds, setAllowedUserIds] = useState<number[]>([])

  // 用户搜索状态
  const [userSearchKeyword, setUserSearchKeyword] = useState('')
  const [userPool, setUserPool] = useState<Map<number, User>>(new Map())

  // 当弹窗打开或 channel 改变时初始化
  useEffect(() => {
    if (channel && open) {
      setStatus(channel.status)
      setSelectedGroups(channel.groupsList.length > 0 ? channel.groupsList : ['default'])
      setAllowedUserIds(channel.allowedUserIds || [])
      setCustomGroupInput('')
      setUserSearchKeyword('')
    }
  }, [channel, open])

  // 拉取已绑定但本地尚未缓存的用户信息
  const missingUserIds = allowedUserIds.filter((id) => !userPool.has(id))
  useQuery({
    queryKey: ['exclusive_bound_users', missingUserIds.join(',')],
    queryFn: async () => {
      if (missingUserIds.length === 0) return []
      const results = await Promise.all(
        missingUserIds.map(async (id) => {
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
    enabled: missingUserIds.length > 0 && open,
  })

  // 搜索用户
  const { data: searchedUsersData, isFetching: isSearchingUsers } = useQuery({
    queryKey: ['exclusive_user_search', userSearchKeyword],
    queryFn: async () => {
      if (!userSearchKeyword.trim()) return []
      try {
        const res = await searchUsers({
          keyword: userSearchKeyword.trim(),
          page_size: 10,
        })
        return res?.data?.items || []
      } catch {
        return []
      }
    },
    enabled: userSearchKeyword.trim().length > 0 && open,
  })

  // 缓存搜索到的用户
  useEffect(() => {
    if (searchedUsersData && searchedUsersData.length > 0) {
      setUserPool((prev) => {
        const next = new Map(prev)
        for (const u of searchedUsersData) {
          next.set(u.id, u)
        }
        return next
      })
    }
  }, [searchedUsersData])

  const toggleGroup = (grp: string) => {
    setSelectedGroups((prev) => {
      if (prev.includes(grp)) {
        if (prev.length <= 1) {
          toast.warning(t('Channel must have at least one group', '渠道必须至少保留一个分组'))
          return prev
        }
        return prev.filter((g) => g !== grp)
      } else {
        return [...prev, grp]
      }
    })
  }

  const handleAddCustomGroup = () => {
    const trimmed = customGroupInput.trim()
    if (!trimmed) return
    if (!selectedGroups.includes(trimmed)) {
      setSelectedGroups((prev) => [...prev, trimmed])
    }
    setCustomGroupInput('')
  }

  const handleAddUser = (user: User) => {
    if (!allowedUserIds.includes(user.id)) {
      setAllowedUserIds((prev) => [...prev, user.id])
      setUserPool((prev) => new Map(prev).set(user.id, user))
    }
    setUserSearchKeyword('')
  }

  const handleRemoveUser = (userId: number) => {
    setAllowedUserIds((prev) => prev.filter((id) => id !== userId))
  }

  const updateMutation = useMutation({
    mutationFn: async () => {
      if (!channel) return
      const groupStr = selectedGroups.join(',')

      // 如果修改了 status
      if (status !== channel.status) {
        await updateChannelStatus(channel.id, status)
      }

      // 更新专属用户与分组
      await updateChannelExclusive(channel.id, {
        group: groupStr,
        allowed_user_ids: allowedUserIds,
        status,
      })
    },
    onSuccess: () => {
      toast.success(t('Exclusive configuration updated', '专属配置已更新生效'))
      queryClient.invalidateQueries({ queryKey: ['channels'] })
      queryClient.invalidateQueries({ queryKey: ['exclusive-channels'] })
      onOpenChange(false)
    },
    onError: (err: any) => {
      toast.error(err?.message || t('Failed to update configuration', '更新配置失败'))
    },
  })

  if (!channel) return null

  // 合并系统全部可用分组选项
  const displayGroups = Array.from(new Set([...allGroups, ...selectedGroups])).sort()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-xl'>
        <DialogHeader>
          <div className='flex items-center justify-between pr-6'>
            <DialogTitle className='flex items-center gap-2 text-base font-semibold'>
              <span>{t('Manage Channel Routing & Users', '管理渠道分组与专属客户')}</span>
            </DialogTitle>
          </div>
          <DialogDescription className='text-xs text-muted-foreground'>
            {t('Channel', '渠道')}: <span className='font-mono font-medium text-foreground'>{channel.name}</span>{' '}
            (#{channel.id})
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-5 py-2'>
          {/* 渠道启用状态 */}
          <div className='flex items-center justify-between rounded-lg border p-3 bg-muted/20'>
            <div className='space-y-0.5'>
              <Label className='text-sm font-medium'>
                {t('Channel Status', '渠道启用状态')}
              </Label>
              <p className='text-xs text-muted-foreground'>
                {status === 1
                  ? t('Channel is online and ready for traffic', '渠道正常启用，接收调度请求')
                  : t('Channel is disabled and will be bypassed', '渠道已停用，系统调度自动旁路')}
              </p>
            </div>
            <div className='flex items-center gap-2'>
              <Badge variant={status === 1 ? 'default' : 'secondary'} className='text-xs'>
                {status === 1 ? t('Enabled', '已启用') : t('Disabled', '已停用')}
              </Badge>
              <Switch
                checked={status === 1}
                onCheckedChange={(checked) => setStatus(checked ? 1 : 0)}
              />
            </div>
          </div>

          {/* 分组配置（哪些分组使用了本渠道） */}
          <div className='space-y-2.5'>
            <div className='flex items-center justify-between'>
              <Label className='text-sm font-medium'>
                {t('Applied Groups', '在哪些分组中使用')} ({selectedGroups.length})
              </Label>
              <span className='text-[11px] text-muted-foreground'>
                {t('Users in these groups can route to this channel', '具备以下分组的客户可请求本渠道')}
              </span>
            </div>

            <div className='flex flex-wrap gap-1.5 p-3 rounded-lg border bg-background min-h-16'>
              {displayGroups.map((grp) => {
                const isSelected = selectedGroups.includes(grp)
                return (
                  <Badge
                    key={grp}
                    variant={isSelected ? 'default' : 'outline'}
                    className={`cursor-pointer transition-all ${
                      isSelected
                        ? 'bg-primary text-primary-foreground hover:bg-primary/90'
                        : 'text-muted-foreground hover:bg-muted'
                    }`}
                    onClick={() => toggleGroup(grp)}
                  >
                    {grp}
                    {isSelected && <span className='ml-1 text-[10px]'>✓</span>}
                  </Badge>
                )
              })}
            </div>

            {/* 新增自定义分组输入 */}
            <div className='flex items-center gap-2'>
              <Input
                placeholder={t('Add other custom group...', '添加其他分组名...')}
                value={customGroupInput}
                onChange={(e) => setCustomGroupInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    handleAddCustomGroup()
                  }
                }}
                className='h-8 text-xs'
              />
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='h-8 shrink-0'
                onClick={handleAddCustomGroup}
                disabled={!customGroupInput.trim()}
              >
                <Plus className='size-3 mr-1' />
                {t('Add', '添加')}
              </Button>
            </div>
          </div>

          {/* 专属用户配置 */}
          <div className='space-y-2.5'>
            <div className='flex items-center justify-between'>
              <Label className='text-sm font-medium flex items-center gap-1.5'>
                <Users className='size-4 text-primary' />
                {t('Exclusive Dedicated Users', '绑定的专属客户')} ({allowedUserIds.length})
              </Label>
              <span className='text-[11px] text-muted-foreground'>
                {allowedUserIds.length === 0
                  ? t('Public to all users in group', '未配置用户时：全员共享（公共通道）')
                  : t('Exclusive: only specified users can access', '专属受限：仅指定客户可请求')}
              </span>
            </div>

            {/* 当前绑定的专属用户列表 */}
            <div className='space-y-1.5 rounded-lg border p-3 bg-background min-h-20 max-h-48 overflow-y-auto'>
              {allowedUserIds.length === 0 ? (
                <div className='flex flex-col items-center justify-center py-4 text-center text-xs text-muted-foreground'>
                  <Users className='size-6 opacity-30 mb-1' />
                  <span>{t('Currently a public channel for all users', '当前为公共渠道，所属分组内所有客户均可使用')}</span>
                  <span className='text-[10px] opacity-70 mt-0.5'>
                    {t('Add users below to make this channel exclusively accessible to them', '如需专门给大客户独享，请在下方搜索并添加客户')}
                  </span>
                </div>
              ) : (
                <div className='flex flex-wrap gap-1.5'>
                  {allowedUserIds.map((uid) => {
                    const user = userPool.get(uid)
                    return (
                      <span
                        key={uid}
                        className='inline-flex items-center gap-1.5 rounded-md border bg-muted/60 px-2 py-1 text-xs shadow-2xs'
                      >
                        <UserIcon className='size-3 text-muted-foreground' />
                        <span className='font-medium text-foreground'>
                          {user ? user.username : `ID #${uid}`}
                        </span>
                        {user && (
                          <span className='text-[10px] text-muted-foreground font-mono'>
                            #{user.id}
                          </span>
                        )}
                        {user?.display_name && (
                          <span className='text-[11px] text-muted-foreground truncate max-w-[80px]'>
                            ({user.display_name})
                          </span>
                        )}
                        <button
                          type='button'
                          onClick={() => handleRemoveUser(uid)}
                          className='ml-0.5 rounded p-0.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive'
                          title={t('Remove exclusive binding', '移除专属绑定')}
                        >
                          <Trash2 className='size-3' />
                        </button>
                      </span>
                    )
                  })}
                </div>
              )}
            </div>

            {/* 用户搜索与添加 */}
            <div className='relative'>
              <div className='relative'>
                <Search className='absolute left-2.5 top-2.5 size-3.5 text-muted-foreground' />
                <Input
                  placeholder={t('Search users by username or ID to add...', '输入用户名、邮箱或用户ID搜索添加专属客户...')}
                  value={userSearchKeyword}
                  onChange={(e) => setUserSearchKeyword(e.target.value)}
                  className='h-8 pl-8 text-xs'
                />
                {isSearchingUsers && (
                  <Loader2 className='absolute right-2.5 top-2.5 size-3.5 animate-spin text-muted-foreground' />
                )}
              </div>

              {/* 搜索结果浮层 */}
              {userSearchKeyword.trim().length > 0 && searchedUsersData && (
                <div className='absolute z-50 mt-1 max-h-48 w-full overflow-y-auto rounded-md border bg-popover p-1 shadow-md'>
                  {searchedUsersData.length === 0 ? (
                    <div className='py-3 text-center text-xs text-muted-foreground'>
                      {t('No matching users found', '未搜索到匹配的用户')}
                    </div>
                  ) : (
                    searchedUsersData.map((u) => {
                      const isAlreadyBound = allowedUserIds.includes(u.id)
                      return (
                        <div
                          key={u.id}
                          className='flex items-center justify-between rounded px-2 py-1.5 text-xs hover:bg-accent cursor-pointer'
                          onClick={() => {
                            if (!isAlreadyBound) {
                              handleAddUser(u)
                            }
                          }}
                        >
                          <div className='flex items-center gap-1.5 min-w-0'>
                            <span className='font-medium text-foreground truncate'>
                              {u.username}
                            </span>
                            <span className='text-[10px] font-mono text-muted-foreground bg-muted px-1 rounded'>
                              #{u.id}
                            </span>
                            {u.display_name && (
                              <span className='text-[11px] text-muted-foreground truncate max-w-[120px]'>
                                ({u.display_name})
                              </span>
                            )}
                            <span className='text-[10px] text-muted-foreground'>
                              [{u.group || 'default'}]
                            </span>
                          </div>
                          <div>
                            {isAlreadyBound ? (
                              <Badge variant='outline' className='text-[10px]'>
                                {t('Already added', '已添加')}
                              </Badge>
                            ) : (
                              <Button variant='ghost' size='sm' className='h-6 px-2 text-xs'>
                                <Plus className='size-3 mr-1' />
                                {t('Add', '添加')}
                              </Button>
                            )}
                          </div>
                        </div>
                      )
                    })
                  )}
                </div>
              )}
            </div>

            <p className='text-[11px] text-muted-foreground'>
              * {t('Note: Bound users must belong to at least one of this channel\'s groups.', '提示：绑定的专属客户必须已被分配到该渠道所属的至少一个分组内。')}
            </p>
          </div>
        </div>

        <DialogFooter className='flex items-center justify-between sm:justify-between pt-2 border-t'>
          <div>
            {onNavigateToChannelConfig && (
              <Button
                type='button'
                variant='ghost'
                size='sm'
                className='text-xs text-muted-foreground hover:text-foreground'
                onClick={() => onNavigateToChannelConfig(channel.id)}
              >
                <ExternalLink className='size-3.5 mr-1' />
                {t('Open Full Technical Config', '打开完整技术配置 ↗')}
              </Button>
            )}
          </div>
          <div className='flex items-center gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => onOpenChange(false)}
            >
              {t('Cancel', '取消')}
            </Button>
            <Button
              type='button'
              size='sm'
              disabled={updateMutation.isPending}
              onClick={() => updateMutation.mutate()}
            >
              {updateMutation.isPending && (
                <Loader2 className='size-3.5 mr-1.5 animate-spin' />
              )}
              {t('Save Configuration', '保存专属配置')}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
