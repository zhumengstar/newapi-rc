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
}: ExclusiveSummaryCardsProps) {
  const { t } = useTranslation()

  const totalChannels = channels.length
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

  // 涉及分组数
  const groupSet = new Set<string>()
  for (const c of exclusiveChannels) {
    for (const g of c.groupsList) {
      groupSet.add(g)
    }
  }
  const uniqueGroupsCount = groupSet.size

  // 专属渠道健康度
  const enabledExclusive = exclusiveChannels.filter((c) => c.status === 1).length
  const disabledExclusive = exclusiveCount - enabledExclusive

  const totalIncome = todayIncomeData?.total_income || 0
  const totalQuota = todayIncomeData?.total_quota || 0

  return (
    <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-5'>
      {/* 今日全量总收入卡片 */}
      <Card size='sm' className='relative overflow-hidden border shadow-xs bg-gradient-to-br from-emerald-500/5 via-background to-background border-emerald-500/20'>
        <CardContent className='flex items-center justify-between p-4'>
          <div className='space-y-1 min-w-0'>
            <div className='flex items-center gap-1.5'>
              <p className='text-xs font-semibold text-emerald-800 dark:text-emerald-300'>
                {t('Today Revenue', '今日渠道总收入')}
              </p>
              {onRefreshIncome && (
                <Button
                  variant='ghost'
                  size='icon'
                  onClick={onRefreshIncome}
                  disabled={isRefreshingIncome}
                  className='size-5 text-emerald-600 dark:text-emerald-400 hover:bg-emerald-500/10'
                  title={t('Refresh today revenue', '强制刷新今日收入缓存')}
                >
                  <RefreshCw className={`size-3 ${isRefreshingIncome ? 'animate-spin' : ''}`} />
                </Button>
              )}
            </div>
            <div className='flex items-baseline gap-1.5'>
              <span className='text-2xl font-black tracking-tight text-emerald-600 dark:text-emerald-400'>
                ${totalIncome.toFixed(2)}
              </span>
              <span className='text-[11px] font-medium text-emerald-700/80 dark:text-emerald-400/80'>
                ≈ ¥{(totalIncome * 7.3).toFixed(2)}
              </span>
            </div>
            <div className='flex items-center gap-1.5 text-[10px] text-muted-foreground'>
              <span>额度 {formatQuota(totalQuota)}</span>
              <span>·</span>
              <span className='inline-flex items-center gap-0.5 text-muted-foreground' title={t('Updated every 30 minutes', '数据每30分钟自动统计更新一次')}>
                <Clock className='size-2.5' />
                30m刷新
              </span>
            </div>
          </div>
          <div className='flex size-10 shrink-0 items-center justify-center rounded-lg bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 shadow-2xs'>
            <BadgeDollarSign className='size-5' />
          </div>
        </CardContent>
      </Card>

      <Card size='sm' className='relative overflow-hidden border shadow-xs'>
        <CardContent className='flex items-center justify-between p-4'>
          <div className='space-y-1'>
            <p className='text-xs font-medium text-muted-foreground'>
              {t('Exclusive Channels', '专属渠道总数')}
            </p>
            <div className='flex items-baseline gap-2'>
              <span className='text-2xl font-bold tracking-tight'>{exclusiveCount}</span>
              <span className='text-xs text-muted-foreground'>
                / {totalChannels} {t('Total Channels', '总渠道')}
              </span>
            </div>
            <p className='text-[11px] text-muted-foreground'>
              {exclusiveCount > 0
                ? t('Bound to dedicated users', '已绑定专属用户')
                : t('All channels are public', '全部为公共全员可用')}
            </p>
          </div>
          <div className='flex size-10 items-center justify-center rounded-lg bg-primary/10 text-primary'>
            <ShieldCheck className='size-5' />
          </div>
        </CardContent>
      </Card>

      <Card size='sm' className='relative overflow-hidden border shadow-xs'>
        <CardContent className='flex items-center justify-between p-4'>
          <div className='space-y-1'>
            <p className='text-xs font-medium text-muted-foreground'>
              {t('Dedicated Users', '专属关联用户')}
            </p>
            <div className='flex items-baseline gap-2'>
              <span className='text-2xl font-bold tracking-tight'>{uniqueUsersCount}</span>
              <span className='text-xs text-muted-foreground'>
                {t('Users configured', '位专享客户')}
              </span>
            </div>
            <p className='text-[11px] text-muted-foreground'>
              {t('Enjoy dedicated routing', '享有专属通道隔离')}
            </p>
          </div>
          <div className='flex size-10 items-center justify-center rounded-lg bg-blue-500/10 text-blue-600 dark:text-blue-400'>
            <Users className='size-5' />
          </div>
        </CardContent>
      </Card>

      <Card size='sm' className='relative overflow-hidden border shadow-xs'>
        <CardContent className='flex items-center justify-between p-4'>
          <div className='space-y-1'>
            <p className='text-xs font-medium text-muted-foreground'>
              {t('Covered Groups', '涉及分组范围')}
            </p>
            <div className='flex items-baseline gap-2'>
              <span className='text-2xl font-bold tracking-tight'>{uniqueGroupsCount}</span>
              <span className='text-xs text-muted-foreground'>
                {t('Active groups', '个业务分组')}
              </span>
            </div>
            <p className='text-[11px] text-muted-foreground'>
              {t('Applied in channel groups', '专属通道所在的分组')}
            </p>
          </div>
          <div className='flex size-10 items-center justify-center rounded-lg bg-purple-500/10 text-purple-600 dark:text-purple-400'>
            <Layers className='size-5' />
          </div>
        </CardContent>
      </Card>

      <Card size='sm' className='relative overflow-hidden border shadow-xs'>
        <CardContent className='flex items-center justify-between p-4'>
          <div className='space-y-1'>
            <p className='text-xs font-medium text-muted-foreground'>
              {t('Exclusive Health', '专属通道在线健康度')}
            </p>
            <div className='flex items-baseline gap-2'>
              <span className='text-2xl font-bold tracking-tight text-emerald-600 dark:text-emerald-400'>
                {enabledExclusive}
              </span>
              {disabledExclusive > 0 ? (
                <span className='inline-flex items-center gap-1 text-xs font-medium text-amber-600 dark:text-amber-400'>
                  <AlertTriangle className='size-3' />
                  {disabledExclusive} {t('Disabled', '异常停用')}
                </span>
              ) : (
                <span className='inline-flex items-center gap-1 text-xs text-muted-foreground'>
                  <CheckCircle2 className='size-3 text-emerald-500' />
                  {t('All normal', '全量在线')}
                </span>
              )}
            </div>
            <p className='text-[11px] text-muted-foreground'>
              {disabledExclusive > 0
                ? t('Risk of user traffic drop!', '注意避免客户请求断流')
                : t('Continuous zero downtime', '专属流量稳定保障')}
            </p>
          </div>
          <div className='flex size-10 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'>
            <Radio className='size-5' />
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
