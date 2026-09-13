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
import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import { useOptionalUsageLogsContext } from './usage-logs-provider'

const INTERVAL_OPTIONS = [
  { value: 'off', seconds: 0 },
  { value: '3000', seconds: 3 },
  { value: '5000', seconds: 5 },
  { value: '10000', seconds: 10 },
  { value: '30000', seconds: 30 },
  { value: '60000', seconds: 60 },
] as const

interface AutoRefreshControlProps {
  className?: string
  showManualButton?: boolean
}

function AutoRefreshControlInner({
  className,
  showManualButton = true,
}: AutoRefreshControlProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const context = useOptionalUsageLogsContext()
  if (!context) return null
  const {
    autoRefresh,
    setAutoRefresh,
    refreshInterval,
    setRefreshInterval,
    triggerRefresh,
  } = context

  const isFetchingLogs = useIsFetching({ queryKey: ['logs'] }) > 0

  const handleManualRefresh = useCallback(() => {
    triggerRefresh()
    void queryClient.invalidateQueries({ queryKey: ['logs'] })
    void queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
  }, [queryClient, triggerRefresh])

  const currentValue = useMemo(() => {
    return autoRefresh ? String(refreshInterval) : 'off'
  }, [autoRefresh, refreshInterval])

  const handleValueChange = useCallback(
    (value: string | null) => {
      if (!value || value === 'off') {
        setAutoRefresh(false)
        return
      }
      const intervalMs = Number(value)
      if (Number.isFinite(intervalMs) && intervalMs >= 1000) {
        setRefreshInterval(intervalMs)
        setAutoRefresh(true)
      }
    },
    [setAutoRefresh, setRefreshInterval]
  )

  const activeSeconds = Math.round(refreshInterval / 1000)

  const selectItems = useMemo(
    () =>
      INTERVAL_OPTIONS.map((opt) => ({
        value: opt.value,
        label:
          opt.value === 'off'
            ? t('Off')
            : t('every {{seconds}}s', { seconds: opt.seconds }),
      })),
    [t]
  )

  return (
    <div className={cn('inline-flex items-center gap-1.5', className)}>
      {showManualButton && (
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='h-8 px-2 text-xs text-muted-foreground hover:text-foreground'
                onClick={handleManualRefresh}
                disabled={isFetchingLogs}
                aria-label={t('Refresh')}
              >
                <RefreshCw
                  className={cn('size-3.5', isFetchingLogs && 'animate-spin')}
                />
                <span className='hidden sm:inline'>{t('Refresh')}</span>
              </Button>
            }
          />
          <TooltipContent>{t('Refresh now')}</TooltipContent>
        </Tooltip>
      )}

      <Select
        items={selectItems}
        value={currentValue}
        onValueChange={handleValueChange}
      >
        <SelectTrigger
          className={cn(
            'h-8 gap-1.5 px-2 text-xs font-medium transition-colors',
            autoRefresh
              ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-700 hover:bg-emerald-500/15 dark:text-emerald-400 dark:bg-emerald-950/30'
              : 'text-muted-foreground hover:text-foreground'
          )}
          aria-label={t('Auto refresh')}
        >
          <span
            className={cn(
              'size-1.5 shrink-0 rounded-full',
              autoRefresh
                ? 'bg-emerald-500 animate-pulse'
                : 'bg-muted-foreground/40'
            )}
            aria-hidden='true'
          />
          <SelectValue className='min-w-0'>
            {autoRefresh
              ? t('Auto {{seconds}}s', { seconds: activeSeconds })
              : t('Auto refresh')}
          </SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false} className='min-w-36'>
          <SelectGroup>
            {INTERVAL_OPTIONS.map((opt) => (
              <SelectItem key={opt.value} value={opt.value} className='text-xs'>
                <div className='flex items-center gap-2'>
                  <span
                    className={cn(
                      'size-1.5 rounded-full',
                      opt.value === 'off'
                        ? 'bg-muted-foreground/40'
                        : 'bg-emerald-500'
                    )}
                  />
                  <span>
                    {opt.value === 'off'
                      ? t('Off')
                      : t('every {{seconds}}s', { seconds: opt.seconds })}
                  </span>
                </div>
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  )
}

export function AutoRefreshControl(props: AutoRefreshControlProps) {
  const context = useOptionalUsageLogsContext()
  if (!context) return null
  return <AutoRefreshControlInner {...props} />
}
