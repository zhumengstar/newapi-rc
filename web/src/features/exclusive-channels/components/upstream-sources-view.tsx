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
import {
  ArrowUpDown,
  BadgeDollarSign,
  Clock,
  Coins,
  Copy,
  DollarSign,
  ExternalLink,
  Filter,
  Globe,
  Layers,
  Loader2,
  MessageSquare,
  Plus,
  RefreshCw,
  Search,
  Settings2,
  ShieldCheck,
} from 'lucide-react'
import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { getChannelSources } from '../api'
import type { UpstreamSourceDTO } from '../types'
import { QuickSourceDialog } from './quick-source-dialog'

interface UpstreamSourcesViewProps {
  allGroups: string[]
  onNavigateToChannelsWithFilter?: (filterText: string) => void
}

function formatQuota(quota: number) {
  if (!quota || quota <= 0) return '0'
  if (quota >= 1000000000) return `${(quota / 1000000000).toFixed(2)}B`
  if (quota >= 1000000) return `${(quota / 1000000).toFixed(2)}M`
  if (quota >= 1000) return `${(quota / 1000).toFixed(1)}k`
  return `${quota}`
}

export function UpstreamSourcesView({
  allGroups,
  onNavigateToChannelsWithFilter,
}: UpstreamSourcesViewProps) {
  const { t } = useTranslation()

  // 筛选与排序状态
  const [searchKeyword, setSearchKeyword] = useState('')
  const [wechatFilter, setWechatFilter] = useState<'all' | 'set' | 'unset'>('all')
  const [selectedGroupFilter, setSelectedGroupFilter] = useState('all')
  const [sortBy, setSortBy] = useState<'income_desc' | 'channels_desc' | 'default'>('income_desc')
  const [onlyWithIncome, setOnlyWithIncome] = useState(false)

  // 弹窗状态
  const [selectedSource, setSelectedSource] = useState<UpstreamSourceDTO | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)

  // 拉取来源数据
  const {
    data: sourcesData,
    isLoading,
    refetch,
    isRefetching,
  } = useQuery({
    queryKey: ['channel-sources-list'],
    queryFn: async () => {
      const data = await getChannelSources()
      return data || []
    },
    staleTime: 30 * 1000,
  })

  const sources = sourcesData || []

  // 复制地址
  const handleCopy = (text: string) => {
    navigator.clipboard.writeText(text)
    toast.success(t('Copied to clipboard', '已复制地址到剪贴板'))
  }

  // 过滤与排序数据
  const filteredSources = useMemo(() => {
    const list = sources.filter((s) => {
      if (searchKeyword.trim()) {
        const kw = searchKeyword.trim().toLowerCase()
        const urlMatch = s.base_url.toLowerCase().includes(kw) || s.display_base_url.toLowerCase().includes(kw)
        const wechatMatch = (s.wechat_name || '').toLowerCase().includes(kw)
        const groupMatch = s.groups.some((g) => g.toLowerCase().includes(kw))
        const modelMatch = s.models.some((m) => m.toLowerCase().includes(kw))
        if (!urlMatch && !wechatMatch && !groupMatch && !modelMatch) return false
      }

      if (wechatFilter === 'set' && !s.wechat_name) return false
      if (wechatFilter === 'unset' && s.wechat_name) return false

      if (selectedGroupFilter !== 'all' && !s.groups.includes(selectedGroupFilter)) {
        return false
      }

      if (onlyWithIncome && (!s.today_income || s.today_income <= 0)) {
        return false
      }

      return true
    })

    list.sort((a, b) => {
      if (sortBy === 'income_desc') {
        const incA = a.today_income || 0
        const incB = b.today_income || 0
        if (incB !== incA) {
          return incB - incA
        }
        return b.channel_count - a.channel_count
      }
      if (sortBy === 'channels_desc') {
        return b.channel_count - a.channel_count
      }
      return 0
    })

    return list
  }, [sources, searchKeyword, wechatFilter, selectedGroupFilter, onlyWithIncome, sortBy])

  // KPI 统计
  const totalSourcesCount = sources.length
  const wechatConfiguredCount = sources.filter((s) => !!s.wechat_name).length
  const totalAggregatedChannels = sources.reduce((sum, s) => sum + s.channel_count, 0)
  const totalEnabledChannels = sources.reduce((sum, s) => sum + s.enabled_channel_count, 0)
  const totalTodayIncome = sources.reduce((sum, s) => sum + (s.today_income || 0), 0)
  const totalTodayQuota = sources.reduce((sum, s) => sum + (s.today_quota || 0), 0)

  // 统计已设定单价的模型去重数
  const pricedModelSet = new Set<string>()
  for (const s of sources) {
    for (const mName of Object.keys(s.model_prices || {})) {
      pricedModelSet.add(mName)
    }
  }

  return (
    <div className='space-y-3'>
      {/* 搜索与筛选工具条 */}
      <div className='flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-card p-2.5 shadow-2xs'>
        <div className='flex flex-wrap items-center gap-2.5 flex-1 min-w-[280px]'>
          <div className='relative w-full max-w-xs'>
            <Search className='absolute left-2.5 top-2.5 size-3.5 text-muted-foreground' />
            <Input
              placeholder={t('Search address, WeChat name, group or model...', '搜索来源地址、微信名、分组或模型...')}
              value={searchKeyword}
              onChange={(e) => setSearchKeyword(e.target.value)}
              className='h-8 pl-8 text-xs'
            />
          </div>

          {/* 微信标记筛选 */}
          <Select
            value={wechatFilter}
            onValueChange={(v) => {
              if (v) setWechatFilter(v as 'all' | 'set' | 'unset')
            }}
          >
            <SelectTrigger className='h-8 w-36 text-xs'>
              <MessageSquare className='size-3 mr-1 text-muted-foreground' />
              <SelectValue>
                {wechatFilter === 'all'
                  ? t('All Suppliers', '全部微信标记')
                  : wechatFilter === 'set'
                    ? t('With WeChat Name', '已标微信名称')
                    : t('Without WeChat', '未标微信名称')}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='all' className='text-xs'>
                {t('All Suppliers', '全部微信标记')}
              </SelectItem>
              <SelectItem value='set' className='text-xs text-emerald-600'>
                {t('With WeChat Name', '已标微信名称')}
              </SelectItem>
              <SelectItem value='unset' className='text-xs text-amber-600'>
                {t('Without WeChat', '未标微信名称')}
              </SelectItem>
            </SelectContent>
          </Select>

          {/* 分组筛选 */}
          <Select
            value={selectedGroupFilter}
            onValueChange={(v) => {
              if (v) setSelectedGroupFilter(v)
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

          {/* 排序选择 */}
          <Select
            value={sortBy}
            onValueChange={(v) => {
              if (v) setSortBy(v as 'income_desc' | 'channels_desc' | 'default')
            }}
          >
            <SelectTrigger className='h-8 w-44 text-xs'>
              <ArrowUpDown className='size-3 mr-1 text-muted-foreground' />
              <SelectValue>
                {sortBy === 'income_desc'
                  ? t('Today Revenue (High to Low)', '今日收入从高到低')
                  : sortBy === 'channels_desc'
                    ? t('Channel Count (Most)', '下属渠道数最多')
                    : t('Default Order', '默认排序')}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='income_desc' className='text-xs font-medium text-emerald-600 dark:text-emerald-400'>
                {t('Today Revenue (High to Low)', '今日收入从高到低')}
              </SelectItem>
              <SelectItem value='channels_desc' className='text-xs'>
                {t('Channel Count (Most)', '下属渠道数最多')}
              </SelectItem>
              <SelectItem value='default' className='text-xs'>
                {t('Default Order', '默认排序')}
              </SelectItem>
            </SelectContent>
          </Select>

          {/* 仅看今日有收入 */}
          <Button
            variant={onlyWithIncome ? 'default' : 'outline'}
            size='sm'
            onClick={() => setOnlyWithIncome(!onlyWithIncome)}
            className='h-8 text-xs'
          >
            <DollarSign className='size-3 mr-1' />
            {onlyWithIncome ? t('Showing with Revenue', '已筛选有收入') : t('Only with Revenue', '仅看有收入')}
          </Button>

          <Button
            variant='outline'
            size='sm'
            onClick={() => refetch()}
            disabled={isLoading || isRefetching}
            className='h-8 text-xs'
          >
            <RefreshCw className={`size-3.5 mr-1.5 ${isRefetching ? 'animate-spin' : ''}`} />
            {t('Refresh', '刷新')}
          </Button>
        </div>

        <div className='flex items-center gap-2 text-xs text-muted-foreground'>
          <span>
            {t('Found', '共筛选出')} <strong className='text-foreground'>{filteredSources.length}</strong>{' '}
            {t('unique upstream sources', '个独立供应商来源地址')}
          </span>
          {(searchKeyword || wechatFilter !== 'all' || selectedGroupFilter !== 'all' || onlyWithIncome || sortBy !== 'income_desc') && (
            <Button
              variant='ghost'
              size='sm'
              className='h-7 text-xs px-2'
              onClick={() => {
                setSearchKeyword('')
                setWechatFilter('all')
                setSelectedGroupFilter('all')
                setOnlyWithIncome(false)
                setSortBy('income_desc')
              }}
            >
              {t('Reset Filters', '重置筛选')}
            </Button>
          )}
        </div>
      </div>

      {/* 来源主表格 */}
      <div className='rounded-lg border bg-card shadow-xs overflow-hidden'>
        <Table>
          <TableHeader>
            <TableRow className='bg-muted/40 text-xs hover:bg-muted/40'>
              <TableHead className='min-w-[200px] font-semibold'>
                {t('Unique Upstream Address (Base URL)', '唯一请求地址 (Base URL)')}
              </TableHead>
              <TableHead className='min-w-[140px] font-semibold'>
                {t('Source WeChat Name', '来源微信名称')}
              </TableHead>
              <TableHead className='min-w-[130px] font-semibold'>
                <div className='flex items-center gap-1 text-emerald-700 dark:text-emerald-400'>
                  <DollarSign className='size-3.5' />
                  <span>{t('Today Total Revenue', '今日总收入')}</span>
                </div>
              </TableHead>
              <TableHead className='min-w-[260px] font-semibold'>
                <div className='flex items-center gap-1 text-blue-700 dark:text-blue-400'>
                  <Layers className='size-3.5' />
                  <span>{t('Groups & Revenue Breakdown', '所属分组与今日收入对应')}</span>
                </div>
              </TableHead>
              <TableHead className='w-28 font-semibold'>
                {t('Channel / Group Ratio', '分组/渠道倍率')}
              </TableHead>
              <TableHead className='min-w-[180px] font-semibold'>
                {t('Model Per-Request Pricing', '模型单请求价格 (按次)')}
              </TableHead>
              <TableHead className='w-28 text-right font-semibold'>
                {t('Actions', '操作')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow>
                <TableCell colSpan={7} className='h-32 text-center text-xs text-muted-foreground'>
                  <div className='flex flex-col items-center justify-center gap-1.5'>
                    <Loader2 className='size-6 animate-spin text-primary opacity-80' />
                    <span>{t('Loading upstream sources...', '正在汇聚各来源地址数据...')}</span>
                  </div>
                </TableCell>
              </TableRow>
            ) : filteredSources.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className='h-32 text-center text-xs text-muted-foreground'>
                  <div className='flex flex-col items-center justify-center gap-1.5'>
                    <Globe className='size-8 opacity-25' />
                    <span>{t('No upstream sources found matching criteria', '未找到符合条件的来源地址')}</span>
                  </div>
                </TableCell>
              </TableRow>
            ) : (
              filteredSources.map((source) => {
                const pricedModels = Object.entries(source.model_prices || {}).filter(
                  ([_, p]) => typeof p === 'number' && p >= 0
                )

                return (
                  <TableRow key={source.base_url} className='hover:bg-muted/30 transition-colors'>
                    {/* 唯一地址 Base URL */}
                    <TableCell>
                      <div className='space-y-1 min-w-0'>
                        <div className='flex items-center gap-1.5'>
                          <Globe className='size-3.5 text-blue-500 shrink-0' />
                          <span
                            className='font-mono text-xs font-semibold text-foreground truncate max-w-[280px]'
                            title={source.display_base_url}
                          >
                            {source.display_base_url}
                          </span>
                          {source.base_url !== 'default' && (
                            <button
                              type='button'
                              onClick={() => handleCopy(source.display_base_url)}
                              className='rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground'
                              title={t('Copy Address', '复制地址')}
                            >
                              <Copy className='size-3' />
                            </button>
                          )}
                        </div>
                        <div className='flex items-center gap-1.5 text-[11px] text-muted-foreground'>
                          <span>{source.models.length} {t('models supported', '个模型')}</span>
                          {source.cost_tier > 0 && (
                            <Badge variant='outline' className='text-[9px] px-1 py-0 h-3.5'>
                              Tier {source.cost_tier}
                            </Badge>
                          )}
                        </div>
                      </div>
                    </TableCell>

                    {/* 来源微信名称 */}
                    <TableCell>
                      {source.wechat_name ? (
                        <div
                          className='inline-flex items-center gap-1.5 rounded-md border bg-emerald-50/70 dark:bg-emerald-950/30 border-emerald-200 dark:border-emerald-900/60 px-2 py-1 text-xs text-emerald-800 dark:text-emerald-300 font-medium cursor-pointer shadow-2xs hover:bg-emerald-100/70 transition-colors'
                          onClick={() => {
                            setSelectedSource(source)
                            setDialogOpen(true)
                          }}
                          title={t('Click to edit WeChat supplier name', '点击修改此来源微信名称')}
                        >
                          <MessageSquare className='size-3 text-emerald-600' />
                          <span className='truncate max-w-[140px]'>{source.wechat_name}</span>
                        </div>
                      ) : (
                        <Button
                          variant='ghost'
                          size='sm'
                          className='h-7 text-xs text-muted-foreground border border-dashed border-border/80 px-2 hover:border-emerald-500 hover:text-emerald-600'
                          onClick={() => {
                            setSelectedSource(source)
                            setDialogOpen(true)
                          }}
                        >
                          <Plus className='size-3 mr-1' />
                          {t('Set WeChat Name', '设置微信名称')}
                        </Button>
                      )}
                    </TableCell>

                    {/* 今日总收入 */}
                    <TableCell>
                      {source.today_income && source.today_income > 0 ? (
                        <div className='flex flex-col gap-0.5'>
                          <div className='flex items-baseline gap-1'>
                            <span className='font-mono text-sm font-black text-emerald-600 dark:text-emerald-400'>
                              ${source.today_income.toFixed(2)}
                            </span>
                            <span className='text-[10px] text-muted-foreground font-mono'>
                              ≈¥{(source.today_income * 7.3).toFixed(2)}
                            </span>
                          </div>
                          <span className='text-[10px] text-muted-foreground font-mono'>
                            总 {formatQuota(source.today_quota || 0)} quota
                          </span>
                        </div>
                      ) : (
                        <span className='text-xs text-muted-foreground/60 font-mono'>$0.00</span>
                      )}
                    </TableCell>

                    {/* 所属分组与今日收入对应 */}
                    <TableCell>
                      <div className='flex flex-col gap-1.5 py-1 min-w-[240px]'>
                        {(() => {
                          const displayGroupSet = new Set<string>(source.groups || [])
                          if (source.group_incomes) {
                            for (const g of Object.keys(source.group_incomes)) {
                              displayGroupSet.add(g)
                            }
                          }
                          const groups = Array.from(displayGroupSet).sort((a, b) => {
                            const incA = source.group_incomes?.[a]?.today_income || 0
                            const incB = source.group_incomes?.[b]?.today_income || 0
                            if (incB !== incA) return incB - incA
                            return a.localeCompare(b)
                          })

                          if (groups.length === 0) {
                            return (
                              <div className='flex items-center justify-between gap-2 rounded border bg-muted/30 px-2 py-0.5 text-xs font-mono'>
                                <span className='text-muted-foreground'>default</span>
                                <span className='text-muted-foreground font-mono text-[11px]'>$0.00</span>
                              </div>
                            )
                          }

                          return groups.map((g) => {
                            const gIncome = source.group_incomes ? source.group_incomes[g] : null
                            const income = gIncome ? gIncome.today_income : 0
                            const quota = gIncome ? gIncome.today_quota : 0
                            const hasIncome = income > 0
                            const grpCostRatio =
                              source.group_cost_ratios?.[g] ??
                              (source.default_cost_ratio && source.default_cost_ratio > 0
                                ? source.default_cost_ratio
                                : undefined)

                            return (
                              <div
                                key={g}
                                className={`flex items-center justify-between gap-2 rounded-md border px-2 py-1 text-xs transition-colors ${
                                  hasIncome
                                    ? 'bg-emerald-50/80 dark:bg-emerald-950/30 border-emerald-300/80 dark:border-emerald-800/60 shadow-2xs'
                                    : 'bg-muted/30 border-border/60 text-muted-foreground'
                                }`}
                              >
                                <div className='flex items-center gap-1.5 min-w-0'>
                                  <span
                                    className={`font-mono font-medium truncate max-w-[130px] cursor-pointer hover:underline ${
                                      hasIncome ? 'text-emerald-900 dark:text-emerald-200 font-semibold' : ''
                                    }`}
                                    onClick={() => setSelectedGroupFilter(g)}
                                    title={t('Filter this group', `点击按分组筛选: ${g}`)}
                                  >
                                    {g}
                                  </span>
                                  {grpCostRatio !== undefined && (
                                    <Badge
                                      variant='outline'
                                      className='h-4 px-1 text-[10px] font-mono border-indigo-300/80 text-indigo-700 bg-indigo-50/80 dark:border-indigo-800 dark:text-indigo-300 dark:bg-indigo-950/40 shrink-0'
                                    >
                                      成本 {grpCostRatio}x
                                    </Badge>
                                  )}
                                </div>
                                <div className='flex items-baseline gap-1 shrink-0'>
                                  {hasIncome ? (
                                    <>
                                      <span className='font-mono font-bold text-emerald-600 dark:text-emerald-400'>
                                        ${income.toFixed(2)}
                                      </span>
                                      <span className='text-[10px] text-muted-foreground font-mono'>
                                        ({formatQuota(quota)})
                                      </span>
                                    </>
                                  ) : (
                                    <span className='text-[11px] text-muted-foreground/50 font-mono'>
                                      $0.00
                                    </span>
                                  )}
                                </div>
                              </div>
                            )
                          })
                        })()}
                      </div>
                    </TableCell>

                    {/* 渠道/分组倍率 */}
                    <TableCell>
                      {(() => {
                        const groupRatioEntries = Object.entries(
                          source.group_cost_ratios || {}
                        )
                        if (groupRatioEntries.length > 0) {
                          return (
                            <div className='flex flex-col gap-1'>
                              <Badge
                                variant='outline'
                                className='font-mono text-[11px] border-indigo-300 text-indigo-700 bg-indigo-50 dark:border-indigo-800 dark:text-indigo-300 dark:bg-indigo-950/40 w-fit'
                              >
                                <Coins className='size-3 mr-1' />
                                {groupRatioEntries.length} 个分组已设
                              </Badge>
                              {source.default_cost_ratio &&
                                source.default_cost_ratio > 0 ? (
                                <span className='text-[10px] text-muted-foreground font-mono'>
                                  兜底 {source.default_cost_ratio}x
                                </span>
                              ) : null}
                            </div>
                          )
                        }
                        if (source.channel_ratio > 0) {
                          return (
                            <Badge
                              variant='default'
                              className='font-mono text-xs bg-amber-600/90 text-white shadow-2xs'
                            >
                              <Coins className='size-3 mr-1' />
                              {source.channel_ratio}x
                            </Badge>
                          )
                        }
                        return (
                          <span className='text-xs text-muted-foreground font-mono'>
                            未设成本倍率
                          </span>
                        )
                      })()}
                    </TableCell>

                    {/* 模型成本单价 */}
                    <TableCell>
                      {(() => {
                        const rules = source.model_cost_rules || []
                        if (rules.length > 0) {
                          return (
                            <div className='flex flex-col gap-1 max-w-[240px]'>
                              {rules.slice(0, 3).map((r, idx) => (
                                <Badge
                                  key={idx}
                                  variant='outline'
                                  className='text-[10px] font-mono border-amber-400/50 text-amber-800 dark:text-amber-300 bg-amber-50/70 dark:bg-amber-950/30 justify-start truncate'
                                  title={`${r.group === '*' ? '全部分组' : r.group}: ${r.models.join(', ')}`}
                                >
                                  {r.group !== '*' ? `[${r.group}] ` : ''}
                                  {r.models.length === 1
                                    ? r.models[0]
                                    : `${r.models[0]} 等${r.models.length}个`}
                                  :{' '}
                                  {r.mode === 'per_token'
                                    ? `入$${r.input_price ?? 0}/出$${r.output_price ?? 0}/M`
                                    : `$${r.price ?? 0}/次`}
                                </Badge>
                              ))}
                              {rules.length > 3 && (
                                <Badge variant='outline' className='text-[10px] w-fit'>
                                  +{rules.length - 3} 条规则
                                </Badge>
                              )}
                            </div>
                          )
                        }
                        if (pricedModels.length === 0) {
                          return (
                            <span className='text-xs text-muted-foreground italic'>
                              {t('By Group Cost Ratio', '按分组成本倍率计费')}
                            </span>
                          )
                        }
                        return (
                          <div className='flex flex-wrap gap-1'>
                            {pricedModels.slice(0, 3).map(([mName, p]) => (
                              <Badge
                                key={mName}
                                variant='outline'
                                className='text-[10px] font-mono border-emerald-500/40 text-emerald-700 dark:text-emerald-400 bg-emerald-500/5'
                              >
                                {mName}: ${p.toFixed(3)}/次
                              </Badge>
                            ))}
                            {pricedModels.length > 3 && (
                              <Badge variant='outline' className='text-[10px]'>
                                +{pricedModels.length - 3}
                              </Badge>
                            )}
                          </div>
                        )
                      })()}
                    </TableCell>

                    {/* 快捷操作 */}
                    <TableCell className='text-right'>
                      <div className='flex items-center justify-end gap-1.5'>
                        <Button
                          variant='outline'
                          size='sm'
                          className='h-7 text-xs px-2.5 font-medium'
                          onClick={() => {
                            setSelectedSource(source)
                            setDialogOpen(true)
                          }}
                        >
                          <Settings2 className='size-3.5 mr-1 text-primary' />
                          {t('Cost & WeChat', '成本/微信')}
                        </Button>

                        {onNavigateToChannelsWithFilter && (
                          <Button
                            variant='ghost'
                            size='sm'
                            className='h-7 text-xs px-2 text-muted-foreground hover:text-foreground'
                            onClick={() => onNavigateToChannelsWithFilter(source.display_base_url)}
                            title={t('View related channels in channels page', '查看该地址下的所有底层渠道')}
                          >
                            <ExternalLink className='size-3.5' />
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>

      {/* 来源编辑弹窗 */}
      <QuickSourceDialog
        source={selectedSource}
        allGroups={allGroups}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
      />
    </div>
  )
}
