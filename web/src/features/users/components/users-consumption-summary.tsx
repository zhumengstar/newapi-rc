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
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { formatQuotaWithCurrency } from '@/lib/currency'

import { getUserConsumptionStats } from '../api'

function formatStatQuota(quota: number | undefined): string {
  return formatQuotaWithCurrency(quota ?? 0, {
    digitsLarge: 2,
    digitsSmall: 6,
    abbreviate: false,
  })
}

export function UsersConsumptionSummary() {
  const { t } = useTranslation()

  const { data, isLoading } = useQuery({
    queryKey: ['user-consumption-stats'],
    queryFn: async () => {
      const res = await getUserConsumptionStats()
      if (!res.success) {
        throw new Error(res.message || 'Failed to fetch consumption stats')
      }
      return res.data
    },
    staleTime: 30000,
  })

  if (isLoading) {
    return (
      <div className='flex flex-col gap-2 rounded-lg border bg-card p-3 text-card-foreground shadow-sm'>
        <div className='flex flex-wrap items-center gap-6'>
          <Skeleton className='h-5 w-40' />
          <Skeleton className='h-5 w-40' />
          <Skeleton className='h-5 w-48' />
        </div>
        <div className='flex items-center gap-2 pt-1'>
          <Skeleton className='h-4 w-32' />
          <div className='flex gap-2 overflow-x-auto'>
            {Array.from({ length: 7 }).map((_, i) => (
              <Skeleton key={i} className='h-12 w-20 rounded-md' />
            ))}
          </div>
        </div>
      </div>
    )
  }

  if (!data) return null

  const {
    daily = [],
    today_quota = 0,
    total_quota = 0,
    balance_quota = 0,
  } = data

  return (
    <div className='flex flex-col gap-2 rounded-lg border bg-card p-3 text-card-foreground shadow-sm'>
      {/* Top metrics row */}
      <div className='flex flex-wrap items-center gap-x-6 gap-y-1.5 text-sm'>
        <div className='flex items-center gap-2'>
          <span className='text-muted-foreground whitespace-nowrap'>
            {t("Today's Consumption (Non-Admin)")}:
          </span>
          <span className='font-semibold tabular-nums text-blue-600 dark:text-blue-400'>
            {formatStatQuota(today_quota)}
          </span>
        </div>
        <div className='flex items-center gap-2'>
          <span className='text-muted-foreground whitespace-nowrap'>
            {t('Total Consumption (Non-Admin)')}:
          </span>
          <span className='font-semibold tabular-nums'>
            {formatStatQuota(total_quota)}
          </span>
        </div>
        <div className='flex items-center gap-2'>
          <span className='text-muted-foreground whitespace-nowrap'>
            {t('Total Balance (Active Non-Admin)')}:
          </span>
          <span className='font-semibold tabular-nums text-emerald-600 dark:text-emerald-400'>
            {formatStatQuota(balance_quota)}
          </span>
        </div>
      </div>

      {/* Recent 7 days grid */}
      <div className='flex flex-wrap items-center gap-2.5 pt-0.5 text-xs'>
        <span className='text-muted-foreground whitespace-nowrap font-medium'>
          {t('Last 7 Days Consumption (Non-Admin)')}:
        </span>
        <div className='flex max-w-full overflow-x-auto rounded-md border border-border bg-muted/40 p-0.5'>
          {daily.map((item, index) => {
            const dateLabel =
              item.date.length >= 10 ? item.date.slice(5) : item.date
            return (
              <div
                key={item.date}
                className={`flex min-w-[76px] flex-col px-2.5 py-1 text-center ${
                  index > 0 ? 'border-l border-border' : ''
                }`}
              >
                <span className='text-[11px] text-muted-foreground font-medium'>
                  {dateLabel}
                </span>
                <span className='font-medium tabular-nums text-foreground mt-0.5'>
                  {formatStatQuota(item.quota)}
                </span>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
