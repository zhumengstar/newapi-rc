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
import { getRouteApi } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { formatLogQuota } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import { getLogStats, getUserLogStats } from '../api'
import { DEFAULT_LOG_STATS } from '../constants'
import { buildApiParams, getDefaultTimeRange } from '../lib/utils'
import { useLogsViewScope, useUsageLogsContext } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

function StatBadge(props: {
  label: string
  value: string | number
  accent: string
  cardClass?: string
  valueClass?: string
  title?: string
}) {
  return (
    <span
      title={props.title}
      className={cn(
        'inline-flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-xs shadow-2xs transition-colors',
        props.cardClass ?? 'border-border/60 bg-muted/25'
      )}
    >
      <span className={cn('size-1.5 shrink-0 rounded-full', props.accent)} />
      <span className='text-muted-foreground font-medium'>{props.label}</span>
      <span
        className={cn(
          'font-mono font-semibold tabular-nums',
          props.valueClass ?? 'text-foreground/90'
        )}
      >
        {props.value}
      </span>
    </span>
  )
}

export function CommonLogsStats() {
  const { t } = useTranslation()
  const { isAdminView: isAdmin } = useLogsViewScope()
  const searchParams = route.useSearch()
  const { sensitiveVisible, autoRefresh, refreshInterval, refreshTrigger } =
    useUsageLogsContext()

  const { data: stats, isLoading } = useQuery({
    queryKey: ['usage-logs-stats', isAdmin, searchParams, refreshTrigger],
    queryFn: async () => {
      let effectiveSearchParams = searchParams
      const nowMs = Date.now()
      const endParam = searchParams.endTime ? Number(searchParams.endTime) : 0
      const isHistorical = endParam > 0 && endParam < nowMs - 60 * 1000
      if (!isHistorical) {
        const defaultRange = getDefaultTimeRange()
        effectiveSearchParams = {
          ...searchParams,
          startTime: searchParams.startTime ?? defaultRange.start.getTime(),
          endTime: nowMs + 3600 * 1000,
        }
      }

      const params = buildApiParams({
        page: 1,
        pageSize: 1,
        searchParams: effectiveSearchParams,
        columnFilters: [],
        isAdmin,
      })

      const result = isAdmin
        ? requireServerSuccess(await getLogStats(params))
        : requireServerSuccess(await getUserLogStats(params))

      return result.success
        ? result.data || DEFAULT_LOG_STATS
        : DEFAULT_LOG_STATS
    },
    refetchInterval: autoRefresh ? refreshInterval : false,
    refetchIntervalInBackground: false,
    placeholderData: (previousData) => previousData,
  })

  if (isLoading) {
    return (
      <div className='flex items-center gap-2'>
        <Skeleton className='h-7 w-[150px] rounded-md' />
        <Skeleton className='h-7 w-[100px] rounded-md' />
        <Skeleton className='h-7 w-[120px] rounded-md' />
        {isAdmin && <Skeleton className='h-7 w-[130px] rounded-md' />}
      </div>
    )
  }

  return (
    <div className='flex flex-wrap items-center gap-1.5'>
      <StatBadge
        label={t('Usage')}
        value={sensitiveVisible ? formatLogQuota(stats?.quota || 0) : '••••'}
        accent='bg-sky-500'
        cardClass='border-sky-500/20 bg-sky-500/[0.06]'
        valueClass='text-sky-700 dark:text-sky-300'
      />
      <StatBadge
        label={t('RPM')}
        value={stats?.rpm || 0}
        accent='bg-rose-500'
        cardClass='border-rose-500/20 bg-rose-500/[0.06]'
        valueClass='text-rose-700 dark:text-rose-300'
      />
      <StatBadge
        label={t('TPM')}
        value={stats?.tpm || 0}
        accent='bg-indigo-500'
        cardClass='border-indigo-500/20 bg-indigo-500/[0.06]'
        valueClass='text-indigo-700 dark:text-indigo-300'
      />
      {isAdmin && (
        <StatBadge
          label={t('MPM')}
          value={
            sensitiveVisible
              ? formatLogQuota(stats?.mpm ?? 0)
              : '••••'
          }
          accent='bg-emerald-500'
          cardClass='border-emerald-500/20 bg-emerald-500/[0.06]'
          valueClass='text-emerald-700 dark:text-emerald-300'
          title={t('Current Minute Spend (MPM)') || '最近一分钟消耗金额 (MPM)'}
        />
      )}
    </div>
  )
}
