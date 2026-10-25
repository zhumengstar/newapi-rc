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
  CheckCircle2,
  AlertTriangle,
  Users,
  Layers,
  ShieldCheck,
  Radio,
  BadgeDollarSign,
  RefreshCw,
  Clock,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import type { ChannelTodayIncomeResponse, EnrichedChannel } from '../types'

interface ExclusiveSummaryCardsProps {
  channels: EnrichedChannel[]
  todayIncomeData?: ChannelTodayIncomeResponse | null
  isRefreshingIncome?: boolean
  onRefreshIncome?: () => void
  totalSourcesCount?: number
  wechatConfiguredCount?: number
}

function formatQuota(quota: number) {
  if (!quota || quota <= 0) return '0'
  if (quota >= 1000000000) return `${(quota / 1000000000).toFixed(2)}B`
  if (quota >= 1000000) return `${(quota / 1000000).toFixed(2)}M`
  if (quota >= 1000) return `${(quota / 1000).toFixed(1)}k`
  return `${quota}`
}

export function ExclusiveSummaryCards({
  channels,
  todayIncomeData,
  isRefreshingIncome = false,
  onRefreshIncome,
  totalSourcesCount,
  wechatConfiguredCount,
}: ExclusiveSummaryCardsProps) {
  const { t } = useTranslation()

  const totalChannels = channels.length
  const enabledChannels = channels.filter((c) => c.status === 1).length
  const exclusiveChannels = channels.filter((c) => c.isExclusive)
  const exclusiveCount = exclusiveChannels.length

  // 去重统计专属用户
  const exclusiveUserSet = new Set<number>()
  for (const c of exclusiveChannels) {
    for (const uid of c.allowedUserIds) {
      exclusiveUserSet.add(uid)
    }
  }
  const uniqueUsersCount = exclusiveUserSet.size

  // 专属渠道健康度
  const enabledExclusive = exclusiveChannels.filter((c) => c.status === 1).length
  const disabledExclusive = exclusiveCount - enabledExclusive

  const totalIncome = todayIncomeData?.total_income || 0
  const totalQuota = todayIncomeData?.total_quota || 0

  return (
    <div className='grid grid-cols-2 gap-2.5 sm:grid-cols-3 lg:grid-cols-5'>
      {/* 1. 今日全量总收入卡片 (小巧紧凑) */}
      <Card size='sm' className='relative overflow-hidden border shadow-2xs bg-gradient-to-br from-emerald-500/5 via-background to-background border-emerald-500/20 py-2'>
        <CardContent className='flex items-center justify-between p-2.5'>
          <div className='space-y-0.5 min-w-0'>
            <div className='flex items-center gap-1'>
              <p className='text-[11px] font-semibold text-emerald-800 dark:text-emerald-300'>
                {t('Today Revenue', '今日总收入')}
              </p>
              {onRefreshIncome && (
                <Button
                  variant='ghost'
                  size='icon'
                  onClick={onRefreshIncome}
                  disabled={isRefreshingIncome}
                  className='size-4 text-emerald-600 dark:text-emerald-400 hover:bg-emerald-500/10'
                  title={t('Refresh today revenue', '强制刷新今日收入缓存')}
                >
                  <RefreshCw className={`size-2.5 ${isRefreshingIncome ? 'animate-spin' : ''}`} />
                </Button>
              )}
            </div>
            <div className='flex items-baseline gap-1'>
              <span className='text-lg font-black tracking-tight text-emerald-600 dark:text-emerald-400'>
                ${totalIncome.toFixed(2)}
              </span>
              <span className='text-[10px] font-medium text-emerald-700/80 dark:text-emerald-400/80'>
                ≈¥{(totalIncome * 7.3).toFixed(2)}
              </span>
            </div>
            <p className='text-[10px] text-muted-foreground truncate'>
              {formatQuota(totalQuota)} quota · 30m刷新
            </p>
          </div>
          <div className='flex size-7 shrink-0 items-center justify-center rounded-md bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'>
            <BadgeDollarSign className='size-3.5' />
          </div>
        </CardContent>
      </Card>

      {/* 2. 专属通道与客户 */}
      <Card size='sm' className='relative overflow-hidden border shadow-2xs py-2'>
        <CardContent className='flex items-center justify-between p-2.5'>
          <div className='space-y-0.5 min-w-0'>
            <p className='text-[11px] font-medium text-muted-foreground'>
              {t('Exclusive Channels', '专属渠道 / 客户')}
            </p>
            <div className='flex items-baseline gap-1'>
              <span className='text-lg font-bold tracking-tight'>{exclusiveCount}</span>
              <span className='text-[10px] text-muted-foreground'>
                / {totalChannels} 渠道
              </span>
            </div>
            <p className='text-[10px] text-muted-foreground truncate'>
              服务 {uniqueUsersCount} 位专享客户
            </p>
          </div>
          <div className='flex size-7 shrink-0 items-center justify-center rounded-md bg-blue-500/10 text-blue-600 dark:text-blue-400'>
            <ShieldCheck className='size-3.5' />
          </div>
        </CardContent>
      </Card>

      {/* 3. 专属通道健康度 */}
      <Card size='sm' className='relative overflow-hidden border shadow-2xs py-2'>
        <CardContent className='flex items-center justify-between p-2.5'>
          <div className='space-y-0.5 min-w-0'>
            <p className='text-[11px] font-medium text-muted-foreground'>
              {t('Exclusive Health', '专属通道健康度')}
            </p>
            <div className='flex items-baseline gap-1.5'>
              <span className='text-lg font-bold tracking-tight text-emerald-600 dark:text-emerald-400'>
                {enabledExclusive} 在线
              </span>
              {disabledExclusive > 0 && (
                <span className='text-[10px] font-semibold text-amber-600 dark:text-amber-400'>
                  {disabledExclusive} 停用
                </span>
              )}
            </div>
            <p className='text-[10px] text-muted-foreground truncate'>
              {disabledExclusive > 0 ? '注意避免客户请求断流' : '专属流量稳定保障'}
            </p>
          </div>
          <div className='flex size-7 shrink-0 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'>
            <Radio className='size-3.5' />
          </div>
        </CardContent>
      </Card>

      {/* 4. 地址来源总数与微信标记 */}
      <Card size='sm' className='relative overflow-hidden border shadow-2xs py-2'>
        <CardContent className='flex items-center justify-between p-2.5'>
          <div className='space-y-0.5 min-w-0'>
            <p className='text-[11px] font-medium text-muted-foreground'>
              {t('Unique Upstream Sources', '聚合来源 / 微信')}
            </p>
            <div className='flex items-baseline gap-1'>
              <span className='text-lg font-bold tracking-tight'>
                {typeof totalSourcesCount === 'number' ? totalSourcesCount : '--'}
              </span>
              <span className='text-[10px] text-muted-foreground'>个地址</span>
            </div>
            <p className='text-[10px] text-muted-foreground truncate'>
              已标记 {typeof wechatConfiguredCount === 'number' ? wechatConfiguredCount : 0} 个微信供应商
            </p>
          </div>
          <div className='flex size-7 shrink-0 items-center justify-center rounded-md bg-purple-500/10 text-purple-600 dark:text-purple-400'>
            <Layers className='size-3.5' />
          </div>
        </CardContent>
      </Card>

      {/* 5. 全渠道在线调度 */}
      <Card size='sm' className='relative overflow-hidden border shadow-2xs py-2'>
        <CardContent className='flex items-center justify-between p-2.5'>
          <div className='space-y-0.5 min-w-0'>
            <p className='text-[11px] font-medium text-muted-foreground'>
              {t('Aggregated Channels', '下属渠道在线率')}
            </p>
            <div className='flex items-baseline gap-1'>
              <span className='text-lg font-bold tracking-tight text-emerald-600 dark:text-emerald-400'>
                {enabledChannels}
              </span>
              <span className='text-[10px] text-muted-foreground'>
                / {totalChannels} 在线
              </span>
            </div>
            <p className='text-[10px] text-muted-foreground truncate'>
              按地址唯一聚合与成本调度
            </p>
          </div>
          <div className='flex size-7 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary'>
            <CheckCircle2 className='size-3.5' />
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
