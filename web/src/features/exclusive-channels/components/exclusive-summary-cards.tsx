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
  Layers,
  BadgeDollarSign,
  RefreshCw,
  Globe,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
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

  const totalIncome = todayIncomeData?.total_income || 0
  const totalQuota = todayIncomeData?.total_quota || 0

  return (
    <div className='flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-card/60 px-3.5 py-2 text-xs shadow-2xs'>
      <div className='flex flex-wrap items-center gap-4 sm:gap-6'>
        {/* 1. 今日全量总收入 */}
        <div className='flex items-center gap-2'>
          <div className='flex size-6 shrink-0 items-center justify-center rounded-md bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'>
            <BadgeDollarSign className='size-3.5' />
          </div>
          <div className='flex items-baseline gap-1.5'>
            <span className='text-xs text-muted-foreground font-medium'>{t('Today Revenue', '今日总收入')}:</span>
            <span className='font-mono font-black text-emerald-600 dark:text-emerald-400 text-sm'>
              ${totalIncome.toFixed(2)}
            </span>
            <span className='text-[11px] text-muted-foreground font-mono'>
              (≈¥{(totalIncome * 7.3).toFixed(2)} · {formatQuota(totalQuota)})
            </span>
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
        </div>

        <div className='h-3.5 w-px bg-border hidden sm:block' />

        {/* 2. 供应商来源与微信标记 */}
        <div className='flex items-center gap-2'>
          <div className='flex size-6 shrink-0 items-center justify-center rounded-md bg-blue-500/10 text-blue-600 dark:text-blue-400'>
            <Globe className='size-3.5' />
          </div>
          <div className='flex items-baseline gap-1.5'>
            <span className='text-xs text-muted-foreground font-medium'>{t('Upstream Sources', '聚合来源')}:</span>
            <span className='font-mono font-bold text-foreground'>
              {typeof totalSourcesCount === 'number' ? totalSourcesCount : '--'}
            </span>
            <span className='text-muted-foreground text-[11px]'>个地址</span>
            <span className='text-[10px] text-emerald-700 dark:text-emerald-300 bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-800/50 px-1 rounded ml-0.5'>
              {typeof wechatConfiguredCount === 'number' ? wechatConfiguredCount : 0} 微信标记
            </span>
          </div>
        </div>

        <div className='h-3.5 w-px bg-border hidden sm:block' />

        {/* 3. 全局渠道在线率与专属通道 */}
        <div className='flex items-center gap-2'>
          <div className='flex size-6 shrink-0 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'>
            <CheckCircle2 className='size-3.5' />
          </div>
          <div className='flex items-baseline gap-1.5'>
            <span className='text-xs text-muted-foreground font-medium'>{t('Channels Online', '渠道在线')}:</span>
            <span className='font-mono font-bold text-emerald-600 dark:text-emerald-400'>
              {enabledChannels} / {totalChannels}
            </span>
            <span className='text-[11px] text-muted-foreground'>
              ({exclusiveCount} 专属 / {uniqueUsersCount} 客户)
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}
