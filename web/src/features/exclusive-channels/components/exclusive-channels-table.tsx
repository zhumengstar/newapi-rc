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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ArrowUpDown,
  Coins,
  DollarSign,
  ExternalLink,
  Filter,
  Layers,
  Search,
  Settings2,
  ShieldCheck,
  UserCheck,
  Users,
} from 'lucide-react'
import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { CHANNEL_TYPES } from '@/features/channels/constants'
import type { User } from '@/features/users/types'

import { updateChannelStatus } from '../api'
import type { ChannelIncomeItem, EnrichedChannel, GroupIncomeItem } from '../types'

interface ExclusiveChannelsTableProps {
  channels: EnrichedChannel[]
  allGroups: string[]
  userMap: Map<number, User>
  onOpenEditDialog: (channel: EnrichedChannel) => void
  onNavigateToChannelConfig: (channelId: number) => void
  highlightChannelId?: number | null
  incomeMap?: Record<number, ChannelIncomeItem>
  channelGroupIncomes?: Record<number, Record<string, GroupIncomeItem>>
}

function formatQuota(quota: number) {
  if (!quota || quota <= 0) return '0'
  if (quota >= 1000000000) return `${(quota / 1000000000).toFixed(2)}B`
  if (quota >= 1000000) return `${(quota / 1000000).toFixed(2)}M`
  if (quota >= 1000) return `${(quota / 1000).toFixed(1)}k`
  return `${quota}`
}

export function ExclusiveChannelsTable({
  channels,
  allGroups,
  userMap,
  onOpenEditDialog,
  onNavigateToChannelConfig,
  highlightChannelId,
  incomeMap,
  channelGroupIncomes,
}: ExclusiveChannelsTableProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  // 筛选状态
  const [searchKeyword, setSearchKeyword] = useState<string>(
    highlightChannelId ? String(highlightChannelId) : ''
  )
  const [selectedGroupFilter, setSelectedGroupFilter] = useState<string>('all')
  const [exclusiveTypeFilter, setExclusiveTypeFilter] = useState<'all' | 'exclusive' | 'public'>('all')
  const [statusFilter, setStatusFilter] = useState<'all' | '1' | '0'>('all')
  const [onlyWithIncome, setOnlyWithIncome] = useState<boolean>(false)
  const [sortBy, setSortBy] = useState<'default' | 'income_desc' | 'id_desc'>('income_desc')

  // 分页状态
  const [pageSize, setPageSize] = useState<number>(20)
  const [currentPage, setCurrentPage] = useState<number>(1)

  // 状态快速切换
  const statusMutation = useMutation({
    mutationFn: async ({ id, newStatus }: { id: number; newStatus: number }) => {
      await updateChannelStatus(id, newStatus)
    },
    onSuccess: () => {
      toast.success(t('Channel status updated', '渠道状态已更新'))
      queryClient.invalidateQueries({ queryKey: ['channels'] })
      queryClient.invalidateQueries({ queryKey: ['exclusive-channels'] })
    },
    onError: (err: any) => {
      toast.error(err?.message || t('Failed to update status', '状态更新失败'))
    },
  })

  // 过滤列表与排序
  const filteredChannels = useMemo(() => {
    const list = channels.filter((c) => {
      // 搜索关键词
      if (searchKeyword.trim()) {
        const kw = searchKeyword.trim().toLowerCase()
        const idMatch = String(c.id).includes(kw)
        const nameMatch = (c.name || '').toLowerCase().includes(kw)
        const groupMatch = c.groupsList.some((g) => g.toLowerCase().includes(kw))
        const userMatch = c.allowedUserIds.some((uid) => {
          if (String(uid).includes(kw)) return true
          const u = userMap.get(uid)
          return u && u.username.toLowerCase().includes(kw)
        })
        if (!idMatch && !nameMatch && !groupMatch && !userMatch) return false
      }

      // 专属渠道筛选
      if (exclusiveTypeFilter === 'exclusive' && !c.isExclusive) return false
      if (exclusiveTypeFilter === 'public' && c.isExclusive) return false

      // 分组筛选
      if (selectedGroupFilter !== 'all' && !c.groupsList.includes(selectedGroupFilter)) {
        return false
      }

      // 状态筛选
      if (statusFilter === '1' && c.status !== 1) return false
      if (statusFilter === '0' && c.status === 1) return false

      // 仅展示有今日收入
      if (onlyWithIncome) {
        const inc = incomeMap ? (incomeMap[c.id]?.today_income || 0) : 0
        if (inc <= 0) return false
      }

      return true
    })

    // 排序
    list.sort((a, b) => {
      if (sortBy === 'income_desc') {
        const incA = incomeMap ? (incomeMap[a.id]?.today_income || 0) : 0
        const incB = incomeMap ? (incomeMap[b.id]?.today_income || 0) : 0
        if (incB !== incA) {
          return incB - incA
        }
        return b.id - a.id
      }
      if (sortBy === 'id_desc') {
        return b.id - a.id
      }
      return a.id - b.id
    })

    return list
  }, [
    channels,
    searchKeyword,
    exclusiveTypeFilter,
    selectedGroupFilter,
    statusFilter,
    onlyWithIncome,
    sortBy,
    incomeMap,
    userMap,
  ])

  // 分页计算
  const totalItems = filteredChannels.length
  const totalPages = Math.ceil(totalItems / pageSize) || 1
  const paginatedChannels = useMemo(() => {
    const start = (currentPage - 1) * pageSize
    return filteredChannels.slice(start, start + pageSize)
  }, [filteredChannels, currentPage, pageSize])

  const handleToggleStatus = (c: EnrichedChannel) => {
    const nextStatus = c.status === 1 ? 0 : 1
    statusMutation.mutate({ id: c.id, newStatus: nextStatus })
  }

  return (
    <div className='space-y-4'>
      {/* 筛选与搜索工具条 */}
      <div className='flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-card p-3 shadow-2xs'>
        <div className='flex flex-wrap items-center gap-2.5 flex-1 min-w-[280px]'>
          <div className='relative w-full max-w-xs'>
            <Search className='absolute left-2.5 top-2.5 size-3.5 text-muted-foreground' />
            <Input
              placeholder={t('Search by channel, group or user...', '搜索渠道名、ID、分组或绑定用户...')}
              value={searchKeyword}
              onChange={(e) => {
                setSearchKeyword(e.target.value)
                setCurrentPage(1)
              }}
              className='h-8 pl-8 text-xs'
            />
          </div>

          {/* 属性过滤 */}
          <Select
            value={exclusiveTypeFilter}
            onValueChange={(v) => {
              if (v) {
                setExclusiveTypeFilter(v as 'all' | 'exclusive' | 'public')
                setCurrentPage(1)
              }
            }}
          >
            <SelectTrigger className='h-8 w-32 text-xs'>
              <ShieldCheck className='size-3 mr-1 text-muted-foreground' />
              <SelectValue>
                {exclusiveTypeFilter === 'all'
                  ? t('All Channels', '全部渠道')
                  : exclusiveTypeFilter === 'exclusive'
                    ? t('Exclusive Only', '仅专属渠道')
                    : t('Public Only', '仅公共渠道')}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='all' className='text-xs'>
                {t('All Channels', '全部渠道')}
              </SelectItem>
              <SelectItem value='exclusive' className='text-xs font-medium text-primary'>
                {t('Exclusive Only', '仅专属渠道')}
              </SelectItem>
              <SelectItem value='public' className='text-xs'>
                {t('Public Only', '仅公共渠道')}
              </SelectItem>
            </SelectContent>
          </Select>

          {/* 分组过滤 */}
          <Select
            value={selectedGroupFilter}
            onValueChange={(v) => {
              if (v) {
                setSelectedGroupFilter(v)
                setCurrentPage(1)
              }
            }}
          >
            <SelectTrigger className='h-8 w-36 text-xs'>
              <Layers className='size-3 mr-1 text-muted-foreground' />
              <SelectValue>
                {selectedGroupFilter === 'all' ? t('All Groups', '全部分组') : selectedGroupFilter}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='all' className='text-xs'>
                {t('All Groups', '全部分组')}
              </SelectItem>
              {allGroups.map((g) => (
                <SelectItem key={g} value={g} className='text-xs font-mono'>
                  {g}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* 状态过滤 */}
          <Select
            value={statusFilter}
            onValueChange={(v) => {
              if (v) {
                setStatusFilter(v as 'all' | '1' | '0')
                setCurrentPage(1)
              }
            }}
          >
            <SelectTrigger className='h-8 w-28 text-xs'>
              <Filter className='size-3 mr-1 text-muted-foreground' />
              <SelectValue>
                {statusFilter === 'all'
                  ? t('All Status', '全部状态')
                  : statusFilter === '1'
                    ? t('Enabled', '仅启用')
                    : t('Disabled', '仅禁用')}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='all' className='text-xs'>
                {t('All Status', '全部状态')}
              </SelectItem>
              <SelectItem value='1' className='text-xs text-emerald-600'>
                {t('Enabled', '仅启用')}
              </SelectItem>
              <SelectItem value='0' className='text-xs text-amber-600'>
                {t('Disabled', '仅禁用')}
              </SelectItem>
            </SelectContent>
          </Select>

          {/* 排序选择 */}
          <Select
            value={sortBy}
            onValueChange={(v) => {
              if (v) {
                setSortBy(v as 'default' | 'income_desc' | 'id_desc')
                setCurrentPage(1)
              }
            }}
          >
            <SelectTrigger className='h-8 w-44 text-xs'>
              <ArrowUpDown className='size-3 mr-1 text-muted-foreground' />
              <SelectValue>
                {sortBy === 'income_desc'
                  ? t('Today Revenue (High to Low)', '今日收入从高到低')
                  : sortBy === 'id_desc'
                    ? t('Channel ID (Newest)', '渠道 ID 倒序')
                    : t('Channel ID (Ascending)', '渠道 ID 正序')}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='income_desc' className='text-xs font-medium text-emerald-600 dark:text-emerald-400'>
                {t('Today Revenue (High to Low)', '今日收入从高到低')}
              </SelectItem>
              <SelectItem value='id_desc' className='text-xs'>
                {t('Channel ID (Newest)', '渠道 ID 倒序')}
              </SelectItem>
              <SelectItem value='default' className='text-xs'>
                {t('Channel ID (Ascending)', '渠道 ID 正序')}
              </SelectItem>
            </SelectContent>
          </Select>

          {/* 仅看今日有收入快捷筛选 */}
          <Button
            variant={onlyWithIncome ? 'default' : 'outline'}
            size='sm'
            onClick={() => {
              setOnlyWithIncome(!onlyWithIncome)
              setCurrentPage(1)
            }}
            className='h-8 text-xs'
          >
            <DollarSign className='size-3 mr-1' />
            {onlyWithIncome ? t('Showing with Revenue', '已筛选有收入') : t('Only with Revenue', '仅看有收入')}
          </Button>
        </div>

        <div className='flex items-center gap-2 text-xs text-muted-foreground'>
          <span>
            {t('Found', '共筛选出')} <strong className='text-foreground'>{totalItems}</strong> {t('channels', '个渠道')}
          </span>
          {(searchKeyword || selectedGroupFilter !== 'all' || exclusiveTypeFilter !== 'all' || statusFilter !== 'all' || onlyWithIncome || sortBy !== 'income_desc') && (
            <Button
              variant='ghost'
              size='sm'
              className='h-7 text-xs px-2'
              onClick={() => {
                setSearchKeyword('')
                setSelectedGroupFilter('all')
                setExclusiveTypeFilter('all')
                setStatusFilter('all')
                setOnlyWithIncome(false)
                setSortBy('income_desc')
                setCurrentPage(1)
              }}
            >
              {t('Reset Filters', '重置筛选')}
            </Button>
          )}
        </div>
      </div>

      {/* 表格主体 */}
      <div className='rounded-lg border bg-card shadow-xs overflow-hidden'>
        <Table>
          <TableHeader>
            <TableRow className='bg-muted/40 text-xs hover:bg-muted/40'>
              <TableHead className='w-16 font-semibold'>ID</TableHead>
              <TableHead className='min-w-[180px] font-semibold'>{t('Channel Name', '渠道名称')}</TableHead>
              <TableHead className='min-w-[130px] font-semibold'>
                <div className='flex items-center gap-1 text-emerald-700 dark:text-emerald-400'>
                  <DollarSign className='size-3.5' />
                  <span>{t('Today Revenue', '今日收入')}</span>
                </div>
              </TableHead>
              <TableHead className='w-28 font-semibold'>{t('Status', '状态')}</TableHead>
              <TableHead className='min-w-[160px] font-semibold'>{t('Applied Groups', '在哪些分组中使用')}</TableHead>
              <TableHead className='min-w-[200px] font-semibold'>{t('Dedicated Users', '绑定的专属客户')}</TableHead>
              <TableHead className='w-40 text-right font-semibold'>{t('Actions', '快捷操作')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {paginatedChannels.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className='h-32 text-center text-xs text-muted-foreground'>
                  <div className='flex flex-col items-center justify-center gap-1.5'>
                    <Users className='size-8 opacity-25' />
                    <span>{t('No channels found matching the criteria', '未找到符合条件的渠道')}</span>
                  </div>
                </TableCell>
              </TableRow>
            ) : (
              paginatedChannels.map((c) => {
                const isHighlighted = highlightChannelId && c.id === highlightChannelId
                const typeName = (CHANNEL_TYPES as any)[c.type] || t('Custom', '自定义')

                return (
                  <TableRow
                    key={c.id}
                    className={`transition-colors ${
                      isHighlighted
                        ? 'bg-primary/5 hover:bg-primary/10 border-l-2 border-l-primary'
                        : 'hover:bg-muted/30'
                    }`}
                  >
                    {/* ID */}
                    <TableCell className='font-mono text-xs text-muted-foreground'>
                      #{c.id}
                    </TableCell>

                    {/* 渠道名称与类型 */}
                    <TableCell>
                      <div className='flex flex-col gap-1'>
                        <div className='flex items-center gap-1.5'>
                          <span className='font-medium text-foreground text-sm leading-tight'>
                            {c.name}
                          </span>
                          {c.isExclusive && (
                            <Badge
                              variant='default'
                              className='bg-blue-600/90 text-white text-[10px] px-1.5 py-0 h-4 font-normal shadow-2xs'
                            >
                              <ShieldCheck className='size-2.5 mr-0.5' />
                              {t('Exclusive', '专属通道')}
                            </Badge>
                          )}
                        </div>
                        <div className='flex items-center gap-2 text-[11px] text-muted-foreground'>
                          <Badge variant='outline' className='text-[10px] px-1.5 py-0 h-4 font-normal'>
                            {typeName}
                          </Badge>
                          {c.models && (
                            <span className='truncate max-w-[200px]' title={c.models}>
                              {c.models.split(',').length} {t('models', '个模型')}
                            </span>
                          )}
                        </div>
                      </div>
                    </TableCell>

                    {/* 今日收入 */}
                    <TableCell>
                      {(() => {
                        const item = incomeMap ? incomeMap[c.id] : null
                        const income = item ? item.today_income : 0
                        const quota = item ? item.today_quota : 0
                        if (income > 0) {
                          return (
                            <div className='flex flex-col gap-0.5'>
                              <div className='flex items-baseline gap-1'>
                                <span className='font-mono text-xs font-bold text-emerald-600 dark:text-emerald-400'>
                                  ${income.toFixed(2)}
                                </span>
                                <span className='text-[10px] text-muted-foreground font-mono'>
                                  ≈¥{(income * 7.3).toFixed(2)}
                                </span>
                              </div>
                              <span className='text-[10px] text-muted-foreground font-mono'>
                                {formatQuota(quota)} quota
                              </span>
                            </div>
                          )
                        }
                        return <span className='text-xs text-muted-foreground/60 font-mono'>$0.00</span>
                      })()}
                    </TableCell>

                    {/* 状态开关 */}
                    <TableCell>
                      <div className='flex items-center gap-2'>
                        <Switch
                          checked={c.status === 1}
                          onCheckedChange={() => handleToggleStatus(c)}
                          disabled={statusMutation.isPending}
                        />
                        <span
                          className={`text-xs font-medium ${
                            c.status === 1 ? 'text-emerald-600 dark:text-emerald-400' : 'text-muted-foreground'
                          }`}
                        >
                          {c.status === 1 ? t('Enabled', '已启用') : t('Disabled', '已停用')}
                        </span>
                      </div>
                    </TableCell>

                    {/* 在哪些分组中使用 */}
                    <TableCell>
                      <div className='flex flex-wrap gap-1'>
                        {c.groupsList.length === 0 ? (
                          <Badge variant='outline' className='text-[10px] text-muted-foreground font-mono'>
                            default
                          </Badge>
                        ) : (
                          c.groupsList.map((g) => {
                            const gItem = channelGroupIncomes?.[c.id]?.[g]
                            const income = gItem?.today_income || 0
                            return (
                              <Badge
                                key={g}
                                variant={income > 0 ? 'default' : 'secondary'}
                                className={`cursor-pointer text-[10px] font-mono transition-colors ${
                                  income > 0
                                    ? 'bg-emerald-600/90 text-white hover:bg-emerald-700 shadow-2xs'
                                    : 'hover:bg-primary/20'
                                }`}
                                onClick={() => {
                                  setSelectedGroupFilter(g)
                                  setCurrentPage(1)
                                }}
                                title={
                                  income > 0
                                    ? `该渠道在 [${g}] 分组今日产生: $${income.toFixed(2)} (${formatQuota(gItem?.today_quota || 0)} quota)`
                                    : t('Click to filter channels in this group', '点击筛选该分组的渠道')
                                }
                              >
                                {g}
                                {income > 0 && <span className='ml-1 opacity-90 font-semibold'>${income.toFixed(2)}</span>}
                              </Badge>
                            )
                          })
                        )}
                      </div>
                    </TableCell>

                    {/* 绑定的专属客户 */}
                    <TableCell>
                      {!c.isExclusive ? (
                        <div className='inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs text-muted-foreground bg-muted/40 border border-dashed border-border/60'>
                          <Users className='size-3 opacity-60' />
                          <span>{t('Public (All users in group)', '公共渠道（所属分组全员可用）')}</span>
                        </div>
                      ) : (
                        <div className='flex flex-wrap gap-1'>
                          {c.allowedUserIds.map((uid) => {
                            const user = userMap.get(uid)
                            return (
                              <span
                                key={uid}
                                className='inline-flex items-center gap-1 rounded bg-blue-50/80 dark:bg-blue-950/40 border border-blue-200 dark:border-blue-900/60 px-1.5 py-0.5 text-xs text-blue-800 dark:text-blue-300 shadow-2xs'
                                title={
                                  user
                                    ? `${user.username} (#${user.id}) | 分组: ${user.group || 'default'}`
                                    : `客户 ID #${uid}`
                                }
                              >
                                <UserCheck className='size-3 text-blue-500' />
                                <span className='font-medium'>
                                  {user ? user.username : `ID #${uid}`}
                                </span>
                                {user && (
                                  <span className='text-[10px] opacity-75 font-mono'>
                                    #{user.id}
                                  </span>
                                )}
                              </span>
                            )
                          })}
                        </div>
                      )}
                    </TableCell>

                    {/* 快捷操作 */}
                    <TableCell className='text-right'>
                      <div className='flex items-center justify-end gap-1.5'>
                        <Button
                          variant='outline'
                          size='sm'
                          className='h-7 text-xs px-2.5 font-medium'
                          onClick={() => onOpenEditDialog(c)}
                        >
                          <Settings2 className='size-3.5 mr-1 text-primary' />
                          {t('Configure', '配置专属')}
                        </Button>
                        <Button
                          variant='ghost'
                          size='sm'
                          className='h-7 text-xs px-2 text-muted-foreground hover:text-foreground'
                          onClick={() => onNavigateToChannelConfig(c.id)}
                          title={t('Jump to full channel settings in Channels page', '跳转至渠道管理查看完整配置')}
                        >
                          <ExternalLink className='size-3.5' />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>

        {/* 分页控制栏 */}
        {totalItems > 0 && (
          <div className='flex flex-wrap items-center justify-between gap-3 border-t p-3 text-xs text-muted-foreground bg-muted/10'>
            <div className='flex items-center gap-2'>
              <span>{t('Rows per page', '每页条数')}:</span>
              <Select
                value={String(pageSize)}
                onValueChange={(v) => {
                  if (v) {
                    setPageSize(Number(v))
                    setCurrentPage(1)
                  }
                }}
              >
                <SelectTrigger className='h-7 w-16 text-xs'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='10'>10</SelectItem>
                  <SelectItem value='20'>20</SelectItem>
                  <SelectItem value='50'>50</SelectItem>
                  <SelectItem value='100'>100</SelectItem>
                </SelectContent>
              </Select>
              <span>
                {t('Showing', '显示')} {(currentPage - 1) * pageSize + 1} -{' '}
                {Math.min(currentPage * pageSize, totalItems)} {t('of', '共')} {totalItems}
              </span>
            </div>

            <div className='flex items-center gap-1.5'>
              <Button
                variant='outline'
                size='sm'
                className='h-7 text-xs px-2.5'
                disabled={currentPage <= 1}
                onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
              >
                {t('Previous', '上一页')}
              </Button>
              <span className='px-2 font-medium text-foreground'>
                {currentPage} / {totalPages}
              </span>
              <Button
                variant='outline'
                size='sm'
                className='h-7 text-xs px-2.5'
                disabled={currentPage >= totalPages}
                onClick={() => setCurrentPage((p) => Math.min(totalPages, p + 1))}
              >
                {t('Next', '下一页')}
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
