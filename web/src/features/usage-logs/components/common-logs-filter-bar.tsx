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
import { useQueryClient, useIsFetching, useQuery } from '@tanstack/react-query'
import { useNavigate, getRouteApi } from '@tanstack/react-router'
import type { Table } from '@tanstack/react-table'
import { Eye, EyeOff } from 'lucide-react'
import { useState, useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { cn } from '@/lib/utils'
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
import { getGroups, getTodayConsumptionUsers } from '@/features/users/api'
import { getRankedModels } from '@/features/models/api'
import { getChannelApiAddresses } from '@/features/channels/api'
import { getChannelSources } from '@/features/exclusive-channels/api'
import { QuickSourceDialog } from '@/features/exclusive-channels/components/quick-source-dialog'
import type { UpstreamSourceDTO } from '@/features/exclusive-channels/types'
import { formatLogQuota } from '@/lib/format'
import { useMediaQuery } from '@/hooks'
import { getUserGroups } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import { LOG_TYPE_ALL_VALUE, LOG_TYPE_FILTERS } from '../constants'
import { buildSearchParams } from '../lib/filter'
import { getDefaultTimeRange } from '../lib/utils'
import type { CommonLogFilters } from '../types'
import { ApiAddressHoverPanel } from './api-address-hover-panel'
import { CommonLogsStats } from './common-logs-stats'
import { CompactDateTimeRangePicker } from './compact-date-time-range-picker'
import {
  LogsFilterField,
  LogsFilterInput,
  LogsFilterToolbar,
} from './logs-filter-toolbar'
import { useLogsViewScope, useUsageLogsContext } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

const STATUS_CODE_OPTIONS = [
  { value: 'all', label: 'All Status Codes' },
  { value: '400', label: '400' },
  { value: '401', label: '401' },
  { value: '403', label: '403' },
  { value: '404', label: '404' },
  { value: '429', label: '429' },
  { value: '500', label: '500' },
  { value: '502', label: '502' },
  { value: '503', label: '503' },
  { value: '504', label: '504' },
] as const

type LogTypeValue = (typeof LOG_TYPE_FILTERS)[number]['value']
const logTypeValueSet = new Set<string>(
  LOG_TYPE_FILTERS.map((type) => type.value)
)

type CommonLogDraft = {
  sourceKey: string
  filters: CommonLogFilters
  logType: LogTypeValue
}

function isLogTypeValue(value: string): value is LogTypeValue {
  return logTypeValueSet.has(value)
}

function getLogTypeValue(value: unknown): LogTypeValue {
  return Array.isArray(value) &&
    value.length === 1 &&
    typeof value[0] === 'string' &&
    isLogTypeValue(value[0])
    ? value[0]
    : LOG_TYPE_ALL_VALUE
}

function buildSearchSourceKey(values: {
  startTime?: unknown
  endTime?: unknown
  channel?: unknown
  model?: unknown
  token?: unknown
  group?: unknown
  username?: unknown
  requestId?: unknown
  upstreamRequestId?: unknown
  statusCode?: unknown
  baseUrl?: unknown
  type?: unknown
}) {
  return [
    values.startTime,
    values.endTime,
    values.channel,
    values.model,
    values.token,
    values.group,
    values.username,
    values.requestId,
    values.upstreamRequestId,
    values.statusCode,
    values.baseUrl,
    Array.isArray(values.type) ? values.type.join(',') : values.type,
  ]
    .map((value) => String(value ?? ''))
    .join('\u001f')
}

interface CommonLogsFilterBarProps<TData> {
  table: Table<TData>
}

const CARD_VISUAL_PALETTES = [
  {
    rowBg: 'bg-sky-500/[0.04] border-sky-500/15',
    badgeBg:
      'bg-sky-500/15 text-sky-700 dark:text-sky-300 border-sky-500/30',
    dot: 'bg-sky-500',
  },
  {
    rowBg: 'bg-violet-500/[0.04] border-violet-500/15',
    badgeBg:
      'bg-violet-500/15 text-violet-700 dark:text-violet-300 border-violet-500/30',
    dot: 'bg-violet-500',
  },
  {
    rowBg: 'bg-amber-500/[0.04] border-amber-500/15',
    badgeBg:
      'bg-amber-500/15 text-amber-700 dark:text-amber-300 border-amber-500/30',
    dot: 'bg-amber-500',
  },
  {
    rowBg: 'bg-emerald-500/[0.04] border-emerald-500/15',
    badgeBg:
      'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300 border-emerald-500/30',
    dot: 'bg-emerald-500',
  },
  {
    rowBg: 'bg-indigo-500/[0.04] border-indigo-500/15',
    badgeBg:
      'bg-indigo-500/15 text-indigo-700 dark:text-indigo-300 border-indigo-500/30',
    dot: 'bg-indigo-500',
  },
] as const

function getFilterCardVisualStyle(text: string, hintText?: string) {
  const combined = `${text} ${hintText ?? ''}`.toLowerCase()
  if (
    combined.includes('cc') ||
    combined.includes('claude') ||
    combined.includes('anthropic') ||
    combined.includes('反重力') ||
    combined.includes('apiloop') ||
    combined.includes('mysandbox')
  ) {
    return CARD_VISUAL_PALETTES[1]
  }
  if (
    combined.includes('gemini') ||
    combined.includes('terrakinc') ||
    combined.includes('18331') ||
    combined.includes('18333')
  ) {
    return CARD_VISUAL_PALETTES[0]
  }
  if (
    combined.includes('香蕉') ||
    combined.includes('banana') ||
    combined.includes('nano') ||
    combined.includes('aijinmi')
  ) {
    return CARD_VISUAL_PALETTES[2]
  }
  if (
    combined.includes('gpt') ||
    combined.includes('openai') ||
    combined.includes('codex') ||
    combined.includes('o1') ||
    combined.includes('o3')
  ) {
    return CARD_VISUAL_PALETTES[3]
  }
  let hash = 0
  for (let i = 0; i < text.length; i++) {
    hash = (hash * 31 + text.charCodeAt(i)) >>> 0
  }
  return CARD_VISUAL_PALETTES[hash % CARD_VISUAL_PALETTES.length]
}

export function CommonLogsFilterBar<TData>(
  props: CommonLogsFilterBarProps<TData>
) {
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const searchParams = route.useSearch()
  const { isAdminView: isAdmin } = useLogsViewScope()
  const {
    sensitiveVisible,
    setSensitiveVisible,
    autoRefresh,
    refreshInterval,
    triggerRefresh,
  } = useUsageLogsContext()
  const fetchingLogs = useIsFetching({ queryKey: ['logs'] })
  const { data: adminGroups } = useQuery({
    queryKey: ['groups'],
    queryFn: async () => requireServerSuccess(await getGroups()),
    enabled: isAdmin,
    refetchInterval: autoRefresh ? refreshInterval : 5000,
    staleTime: 2000,
  })
  const { data: userGroups } = useQuery({
    queryKey: ['user-groups'],
    queryFn: async () => requireServerSuccess(await getUserGroups()),
    enabled: !isAdmin,
  })
  const groupOptions = useMemo(() => {
    const groups = isAdmin
      ? (adminGroups?.data ?? [])
      : Object.keys(userGroups?.data ?? {})
    const rates = adminGroups?.rates ?? {}
    const quotas =
      (adminGroups as { quotas?: Record<string, number> } | undefined)
        ?.quotas ?? {}
    const ratios =
      (adminGroups as { ratios?: Record<string, number> } | undefined)
        ?.ratios ?? {}
    const items = groups
      .filter((group) => group !== 'auto')
      .map((group) => {
        const rate = rates[group]
        const mpm = rate?.mpm ?? 0
        const rpm = rate?.rpm ?? 0
        const isLive = mpm > 0 || rpm > 0
        const todayQuota = quotas[group] ?? 0
        const todayText = todayQuota > 0 ? formatLogQuota(todayQuota) : '$0'
        const rawRatio = isAdmin
          ? ratios[group]
          : userGroups?.data?.[group]?.ratio
        const ratio =
          typeof rawRatio === 'number' && Number.isFinite(rawRatio)
            ? rawRatio
            : null
        const style = getFilterCardVisualStyle(group)
        return {
          label: group,
          value: group,
          labelNode: (
            <div
              className={cn(
                'flex w-full items-center justify-between gap-2 rounded-md border px-2 py-1 transition-colors',
                style.rowBg
              )}
            >
              <span
                title={group}
                className={cn(
                  'inline-flex w-[140px] shrink-0 items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-semibold whitespace-nowrap shadow-2xs',
                  style.badgeBg
                )}
              >
                <span
                  className={cn('size-1.5 shrink-0 rounded-full', style.dot)}
                />
                <span className='truncate'>{group}</span>
              </span>
              {isAdmin && (
                <div className='flex shrink-0 items-center gap-1.5'>
                  <span
                    className={cn(
                      'inline-flex w-[158px] shrink-0 items-center justify-between gap-1 rounded-md border px-2 py-0.5 font-mono text-[11px] tabular-nums whitespace-nowrap',
                      isLive
                        ? 'border-indigo-500/25 bg-indigo-500/12 font-medium text-indigo-700 dark:text-indigo-300'
                        : 'border-border/60 bg-background/70 text-muted-foreground'
                    )}
                  >
                    <span>MPM {formatLogQuota(mpm)}</span>
                    <span className='opacity-40'>·</span>
                    <span>RPM {rpm}</span>
                  </span>
                  <span
                    className={cn(
                      'inline-flex w-[116px] shrink-0 items-center justify-between gap-1 rounded-md border px-2 py-0.5 font-mono text-[11px] tabular-nums whitespace-nowrap',
                      todayQuota > 0
                        ? 'border-emerald-500/25 bg-emerald-500/12 font-semibold text-emerald-700 dark:text-emerald-300'
                        : 'border-border/60 bg-background/70 text-muted-foreground'
                    )}
                  >
                    <span className='font-sans text-[10px] font-medium opacity-80'>
                      {t('Today', '今天')}
                    </span>
                    <span>{todayText}</span>
                  </span>
                  <span className='inline-flex w-[80px] shrink-0 items-center justify-between gap-1 rounded-md border border-amber-500/25 bg-amber-500/12 px-2 py-0.5 font-mono text-[11px] font-semibold text-amber-700 dark:text-amber-300 tabular-nums whitespace-nowrap'>
                    <span className='font-sans text-[10px] font-medium opacity-80'>
                      {t('RatioLabel', '倍率')}
                    </span>
                    <span>{ratio !== null ? `${ratio}x` : '1x'}</span>
                  </span>
                </div>
              )}
            </div>
          ),
        }
      })
    return [
      {
        value: '',
        label: t('All Groups', '全部分组'),
        labelNode: (
          <div className='flex w-full items-center justify-between gap-2 rounded-md border border-primary/20 bg-primary/[0.05] px-2.5 py-1'>
            <span className='text-xs font-semibold text-foreground'>
              {t('All Groups', '全部分组')}
            </span>
            <span className='rounded bg-primary/10 px-1.5 py-0.5 font-mono text-[10px] font-medium text-primary'>
              {items.length} {t('Groups', '分组')}
            </span>
          </div>
        ),
      },
      ...items,
    ]
  }, [isAdmin, adminGroups, userGroups, t])

  const { data: rankedModelsData } = useQuery({
    queryKey: ['ranked-models'],
    queryFn: async () => requireServerSuccess(await getRankedModels()),
    staleTime: 60 * 1000,
  })

  const modelOptions = useMemo(() => {
    const list = rankedModelsData?.data ?? []
    const items = list.map((m) => {
      const style = getFilterCardVisualStyle(m)
      const lower = m.toLowerCase()
      const familyLabel =
        lower.includes('claude') || lower.includes('cc')
          ? 'Claude'
          : lower.includes('gemini')
            ? 'Gemini'
            : lower.includes('gpt') ||
                lower.includes('o1') ||
                lower.includes('o3') ||
                lower.includes('codex')
              ? 'OpenAI'
              : lower.includes('banana') ||
                  lower.includes('imagen') ||
                  lower.includes('flux')
                ? 'Image'
                : 'Model'
      return {
        label: m,
        value: m,
        labelNode: (
          <div
            className={cn(
              'flex w-full items-center justify-between gap-2 rounded-md border px-2 py-1 transition-colors',
              style.rowBg
            )}
          >
            <span
              title={m}
              className={cn(
                'inline-flex w-[235px] shrink-0 items-center gap-1.5 rounded-md border px-2 py-0.5 font-mono text-xs font-semibold whitespace-nowrap shadow-2xs',
                style.badgeBg
              )}
            >
              <span
                className={cn('size-1.5 shrink-0 rounded-full', style.dot)}
              />
              <span className='truncate'>{m}</span>
            </span>
            <span className='inline-flex w-[64px] shrink-0 items-center justify-center rounded-md border border-border/60 bg-background/80 px-1.5 py-0.5 font-mono text-[10px] font-medium text-muted-foreground whitespace-nowrap'>
              {familyLabel}
            </span>
          </div>
        ),
      }
    })
    return [
      {
        value: '',
        label: t('All Models', '全部模型'),
        labelNode: (
          <div className='flex w-full items-center justify-between gap-2 rounded-md border border-primary/20 bg-primary/[0.05] px-2.5 py-1'>
            <span className='text-xs font-semibold text-foreground'>
              {t('All Models', '全部模型')}
            </span>
            <span className='rounded bg-primary/10 px-1.5 py-0.5 font-mono text-[10px] font-medium text-primary'>
              {items.length} {t('Models', '个模型')}
            </span>
          </div>
        ),
      },
      ...items,
    ]
  }, [rankedModelsData, t])

  const { data: userCandidatesData } = useQuery({
    queryKey: ['today-consumption-users'],
    queryFn: async () => requireServerSuccess(await getTodayConsumptionUsers()),
    enabled: isAdmin,
    staleTime: 15 * 1000,
  })

  const userOptions = useMemo(() => {
    const users = userCandidatesData?.data ?? []
    const items = users.map((u) => {
      const todayQuota = u.today_consumed_quota || 0
      const dailyQuota = Math.max(u.daily_consumed_quota || 0, todayQuota)
      const totalQuota = u.total_consumed_quota || 0
      const remainQuota = u.remain_quota || 0
      const isLowBalance =
        (dailyQuota > 0 && remainQuota < dailyQuota) ||
        (remainQuota <= 0 && totalQuota > 0)
      const todayText = todayQuota > 0 ? formatLogQuota(todayQuota) : '$0'
      const dailyText = dailyQuota > 0 ? formatLogQuota(dailyQuota) : '$0'
      const totalText = totalQuota > 0 ? formatLogQuota(totalQuota) : '$0'
      const remainText = remainQuota > 0 ? formatLogQuota(remainQuota) : '$0'
      const style = getFilterCardVisualStyle(u.username, u.group)
      return {
        value: u.username,
        label: u.username,
        labelNode: (
          <div
            className={cn(
              'flex w-full items-center justify-between gap-2 rounded-md border px-2 py-1 transition-colors',
              isLowBalance
                ? 'border-rose-500/35 bg-rose-500/[0.06]'
                : style.rowBg
            )}
          >
            <span
              title={u.username}
              className={cn(
                'inline-flex w-[140px] shrink-0 items-center gap-1.5 rounded-md border px-2 py-0.5 font-mono text-xs font-semibold whitespace-nowrap shadow-2xs',
                isLowBalance
                  ? 'border-rose-500/40 bg-rose-500/15 text-rose-700 dark:text-rose-300'
                  : style.badgeBg
              )}
            >
              <span
                className={cn(
                  'size-1.5 shrink-0 rounded-full',
                  isLowBalance ? 'bg-rose-500 animate-pulse' : style.dot
                )}
              />
              <span className='truncate'>{u.username}</span>
            </span>

            <div className='flex shrink-0 items-center gap-1.5'>
              <span
                className={cn(
                  'inline-flex w-[116px] shrink-0 items-center justify-between gap-1 rounded-md border px-2 py-0.5 font-mono text-[11px] tabular-nums whitespace-nowrap',
                  todayQuota > 0
                    ? 'border-emerald-500/25 bg-emerald-500/12 font-semibold text-emerald-700 dark:text-emerald-300'
                    : 'border-border/60 bg-background/70 text-muted-foreground'
                )}
              >
                <span className='font-sans text-[10px] font-medium opacity-80'>
                  {t('Today', '今天')}
                </span>
                <span>{todayText}</span>
              </span>

              <span
                title={
                  isLowBalance
                    ? `余额不足一天消耗（近1天消耗: ${dailyText}，当前余额: ${remainText}）`
                    : `当前余额: ${remainText}`
                }
                className={cn(
                  'inline-flex w-[134px] shrink-0 items-center justify-between gap-1 rounded-md border px-2 py-0.5 font-mono text-[11px] tabular-nums whitespace-nowrap',
                  isLowBalance
                    ? 'border-rose-500/50 bg-rose-500/15 font-bold text-rose-600 ring-1 ring-rose-500/25 dark:border-rose-500/50 dark:bg-rose-500/20 dark:text-rose-300'
                    : remainQuota > 0
                      ? 'border-amber-500/25 bg-amber-500/12 font-semibold text-amber-700 dark:text-amber-300'
                      : 'border-border/60 bg-background/70 text-muted-foreground'
                )}
              >
                <span className='inline-flex items-center gap-0.5 font-sans text-[10px] font-medium opacity-90'>
                  {isLowBalance && (
                    <span className='size-1.5 rounded-full bg-rose-500 shrink-0' />
                  )}
                  {t('BalanceLabel', '余额')}
                </span>
                <span>{remainText}</span>
              </span>

              <span
                className={cn(
                  'inline-flex w-[138px] shrink-0 items-center justify-between gap-1 rounded-md border px-2 py-0.5 font-mono text-[11px] tabular-nums whitespace-nowrap',
                  totalQuota > 0
                    ? 'border-indigo-500/25 bg-indigo-500/12 font-semibold text-indigo-700 dark:text-indigo-300'
                    : 'border-border/60 bg-background/70 text-muted-foreground'
                )}
              >
                <span className='font-sans text-[10px] font-medium opacity-80'>
                  {t('TotalLabel', '总计')}
                </span>
                <span>{totalText}</span>
              </span>
            </div>
          </div>
        ),
      }
    })
    return [
      {
        value: '',
        label: t('All Users', '全部用户'),
        labelNode: (
          <div className='flex w-full items-center justify-between gap-2 rounded-md border border-primary/20 bg-primary/[0.05] px-2.5 py-1'>
            <span className='text-xs font-semibold text-foreground'>
              {t('All Users', '全部用户')}
            </span>
            <span className='rounded bg-primary/10 px-1.5 py-0.5 font-mono text-[10px] font-medium text-primary'>
              {items.length} {t('Users', '个用户')}
            </span>
          </div>
        ),
      },
      ...items,
    ]
  }, [userCandidatesData, t])


  const searchState = useMemo<CommonLogDraft>(() => {
    const { start, end } = getDefaultTimeRange()
    const sourceValues = {
      startTime: searchParams.startTime,
      endTime: searchParams.endTime,
      channel: searchParams.channel,
      model: searchParams.model,
      token: searchParams.token,
      group: searchParams.group,
      username: searchParams.username,
      requestId: searchParams.requestId,
      upstreamRequestId: searchParams.upstreamRequestId,
      statusCode: searchParams.statusCode,
      baseUrl: searchParams.baseUrl,
      type: searchParams.type,
    }
    const filters: CommonLogFilters = {
      startTime: searchParams.startTime
        ? new Date(searchParams.startTime)
        : start,
      endTime: searchParams.endTime ? new Date(searchParams.endTime) : end,
      channel: searchParams.channel || undefined,
      model: searchParams.model || undefined,
      token: searchParams.token || undefined,
      group: searchParams.group || undefined,
      username: searchParams.username || undefined,
      requestId: searchParams.requestId || undefined,
      upstreamRequestId: searchParams.upstreamRequestId || undefined,
      statusCode: searchParams.statusCode || undefined,
      baseUrl: searchParams.baseUrl || undefined,
    }
    return {
      sourceKey: buildSearchSourceKey(sourceValues),
      filters,
      logType: getLogTypeValue(searchParams.type),
    }
  }, [
    searchParams.startTime,
    searchParams.endTime,
    searchParams.channel,
    searchParams.model,
    searchParams.token,
    searchParams.group,
    searchParams.username,
    searchParams.requestId,
    searchParams.upstreamRequestId,
    searchParams.statusCode,
    searchParams.type,
  ])
  const [draft, setDraft] = useState<CommonLogDraft>(() => searchState)
  const activeDraft =
    draft.sourceKey === searchState.sourceKey ? draft : searchState
  const filters = activeDraft.filters
  const logType = activeDraft.logType

  const startSec = filters.startTime
    ? Math.floor(filters.startTime.getTime() / 1000)
    : undefined
  const endSec = filters.endTime
    ? Math.floor(filters.endTime.getTime() / 1000)
    : undefined

  const { data: apiAddressesData } = useQuery({
    queryKey: [
      'channel-api-addresses',
      filters.group,
      filters.model,
      startSec,
      endSec,
    ],
    queryFn: async () => {
      try {
        const res = await getChannelApiAddresses(
          filters.group,
          filters.model,
          startSec,
          endSec
        )
        return res?.data ?? []
      } catch {
        return []
      }
    },
    enabled: isAdmin,
    staleTime: 30 * 1000,
  })

  const { data: channelSourcesData, isLoading: channelSourcesLoading } =
    useQuery({
      queryKey: ['channel-sources-list'],
      queryFn: async () => {
        try {
          const res = await getChannelSources()
          if (Array.isArray(res)) return res
          return (res as unknown as { data?: UpstreamSourceDTO[] })?.data ?? []
        } catch {
          return []
        }
      },
      enabled: isAdmin,
      staleTime: 60 * 1000,
    })

  const sourceByBaseUrl = useMemo(() => {
    const map = new Map<string, UpstreamSourceDTO>()
    for (const s of channelSourcesData ?? []) {
      const norm = s.base_url.trim().toLowerCase().replace(/\/+$/, '')
      if (norm) map.set(norm, s)
      map.set(s.base_url, s)
      if (s.display_base_url) {
        map.set(s.display_base_url, s)
        const dispNorm = s.display_base_url
          .trim()
          .toLowerCase()
          .replace(/\/+$/, '')
        if (dispNorm) map.set(dispNorm, s)
      }
    }
    return map
  }, [channelSourcesData])

  const [editingSource, setEditingSource] = useState<UpstreamSourceDTO | null>(
    null
  )
  const [sourceDialogOpen, setSourceDialogOpen] = useState(false)

  const apiAddressOptions = useMemo(() => {
    const list = apiAddressesData ?? []
    const items = list.map((item) => {
      const norm = item.base_url.trim().toLowerCase().replace(/\/+$/, '')
      const src =
        sourceByBaseUrl.get(norm) || sourceByBaseUrl.get(item.base_url)
      const todayQuota =
        item.today_quota && item.today_quota > 0
          ? item.today_quota
          : item.period_quota > 0
            ? item.period_quota
            : (src?.today_quota ?? 0)
      const totalQuota = item.used_quota || 0
      const todayText = todayQuota > 0 ? formatLogQuota(todayQuota) : '$0'
      const totalText = totalQuota > 0 ? formatLogQuota(totalQuota) : '$0'
      const primaryGroup = src?.groups?.[0]?.group
      const style = getFilterCardVisualStyle(item.base_url, primaryGroup)
      return {
        value: item.base_url,
        label: item.base_url,
        todayQuota,
        totalQuota,
        labelNode: (
          <div
            className={cn(
              'flex w-full items-center justify-between gap-2 rounded-md border px-2 py-1 transition-colors',
              style.rowBg
            )}
          >
            <span
              title={item.base_url}
              className={cn(
                'inline-flex w-[215px] shrink-0 items-center gap-1.5 rounded-md border px-2 py-0.5 font-mono text-xs font-semibold whitespace-nowrap shadow-2xs',
                style.badgeBg
              )}
            >
              <span
                className={cn('size-1.5 shrink-0 rounded-full', style.dot)}
              />
              <span className='truncate'>{item.base_url}</span>
            </span>

            <div className='flex shrink-0 items-center gap-1.5'>
              <span
                className={cn(
                  'inline-flex w-[128px] shrink-0 items-center justify-between gap-1 rounded-md border px-2 py-0.5 font-mono text-[11px] tabular-nums whitespace-nowrap',
                  todayQuota > 0
                    ? 'border-emerald-500/25 bg-emerald-500/12 font-semibold text-emerald-700 dark:text-emerald-300'
                    : 'border-border/60 bg-background/70 text-muted-foreground'
                )}
              >
                <span className='font-sans text-[10px] font-medium opacity-80'>
                  {t('TodayLabel', '当天')}
                </span>
                <span>{todayText}</span>
              </span>

              <span
                className={cn(
                  'inline-flex w-[138px] shrink-0 items-center justify-between gap-1 rounded-md border px-2 py-0.5 font-mono text-[11px] tabular-nums whitespace-nowrap',
                  totalQuota > 0
                    ? 'border-indigo-500/25 bg-indigo-500/12 font-semibold text-indigo-700 dark:text-indigo-300'
                    : 'border-border/60 bg-background/70 text-muted-foreground'
                )}
              >
                <span className='font-sans text-[10px] font-medium opacity-80'>
                  {t('TotalLabel', '总计')}
                </span>
                <span>{totalText}</span>
              </span>
            </div>
          </div>
        ),
      }
    })
    items.sort((a, b) => {
      if (b.todayQuota !== a.todayQuota) return b.todayQuota - a.todayQuota
      if (b.totalQuota !== a.totalQuota) return b.totalQuota - a.totalQuota
      return a.value.localeCompare(b.value)
    })
    return [
      {
        value: '',
        label: t('All API Addresses', '全部 API 地址'),
        labelNode: (
          <div className='flex w-full items-center justify-between gap-2 rounded-md border border-primary/20 bg-primary/[0.05] px-2.5 py-1'>
            <span className='text-xs font-semibold text-foreground'>
              {t('All API Addresses', '全部 API 地址')}
            </span>
            <span className='rounded bg-primary/10 px-1.5 py-0.5 font-mono text-[10px] font-medium text-primary'>
              {items.length} {t('Addresses', '个地址')}
            </span>
          </div>
        ),
      },
      ...items,
    ]
  }, [apiAddressesData, sourceByBaseUrl, t])

  const handleChange = useCallback(
    (field: keyof CommonLogFilters, value: Date | string | undefined) => {
      setDraft((current) => {
        const base =
          current.sourceKey === searchState.sourceKey ? current : searchState
        return {
          sourceKey: searchState.sourceKey,
          filters: { ...base.filters, [field]: value },
          logType: base.logType,
        }
      })
    },
    [searchState]
  )

  const handleApply = useCallback(
    (
      nextFilters: CommonLogFilters = filters,
      nextLogType: LogTypeValue = logType
    ) => {
      const filterParams = buildSearchParams(nextFilters, 'common')
      const nextSearch = {
        ...filterParams,
        type: [nextLogType],
        page: 1,
      }
      navigate({
        to: '/usage-logs/$section',
        params: { section: 'common' },
        search: nextSearch,
      })
      triggerRefresh()
      void queryClient.invalidateQueries({ queryKey: ['groups'] })
      void queryClient.invalidateQueries({ queryKey: ['today-consumption-users'] })
      void queryClient.invalidateQueries({ queryKey: ['channel-api-addresses'] })
    },
    [filters, logType, navigate, queryClient, triggerRefresh]
  )

  const handleReset = useCallback(() => {
    const { start, end } = getDefaultTimeRange()
    const resetFilters: CommonLogFilters = {
      startTime: start,
      endTime: end,
      channel: undefined,
      model: undefined,
      token: undefined,
      group: undefined,
      username: undefined,
      requestId: undefined,
      upstreamRequestId: undefined,
      statusCode: undefined,
      baseUrl: undefined,
    }
    const resetSearch = {
      page: 1,
      type: undefined,
      startTime: undefined,
      endTime: undefined,
      channel: undefined,
      model: undefined,
      token: undefined,
      group: undefined,
      username: undefined,
      requestId: undefined,
      upstreamRequestId: undefined,
      statusCode: undefined,
      baseUrl: undefined,
    }
    setDraft({
      sourceKey: buildSearchSourceKey({}),
      filters: resetFilters,
      logType: LOG_TYPE_ALL_VALUE,
    })

    navigate({
      to: '/usage-logs/$section',
      params: { section: 'common' },
      search: resetSearch,
    })
    queryClient.invalidateQueries({ queryKey: ['logs'] })
    queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
  }, [navigate, queryClient])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter') handleApply()
    },
    [handleApply]
  )

  const hasExpandedFilters =
    !!filters.token ||
    !!filters.username ||
    !!filters.channel ||
    !!filters.baseUrl ||
    !!filters.requestId ||
    !!filters.upstreamRequestId

  const hasTypeFilter = logType !== LOG_TYPE_ALL_VALUE
  const isTimeCustom = useMemo(() => {
    if (searchParams.startTime != null || searchParams.endTime != null) return true
    const { start, end } = getDefaultTimeRange()
    if (filters.startTime && Math.abs(filters.startTime.getTime() - start.getTime()) > 60 * 1000) {
      return true
    }
    if (filters.endTime && Math.abs(filters.endTime.getTime() - end.getTime()) > 60 * 1000) {
      return true
    }
    return false
  }, [searchParams.startTime, searchParams.endTime, filters.startTime, filters.endTime])
  const hasAdditionalFilters =
    !!filters.model ||
    !!filters.group ||
    !!filters.statusCode ||
    hasTypeFilter ||
    hasExpandedFilters ||
    isTimeCustom

  const expandedFilterCount = [
    filters.token,
    isAdmin ? filters.username : undefined,
    isAdmin ? filters.channel : undefined,
    isAdmin ? filters.baseUrl : undefined,
    filters.requestId,
    filters.upstreamRequestId,
  ].filter(Boolean).length
  const sensitiveInputClass = sensitiveVisible
    ? undefined
    : '[-webkit-text-security:disc]'
  const logTypeItems = useMemo(
    () =>
      LOG_TYPE_FILTERS.map((type) => ({
        value: type.value,
        label: t(type.label),
        deprecated: type.deprecated,
      })),
    [t]
  )
  const selectedLogType = logTypeItems.find((type) => type.value === logType)
  const deprecatedTypeDescription = t(
    'Only used to find historical logs. New records are available in Audit Logs.'
  )

  const statsBar = <CommonLogsStats />
  const sensitiveToggle = (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            onClick={() => setSensitiveVisible(!sensitiveVisible)}
            aria-label={sensitiveVisible ? t('Hide') : t('Show')}
            className='text-muted-foreground hover:text-foreground size-7 max-sm:size-11'
          />
        }
      >
        {sensitiveVisible ? <Eye /> : <EyeOff />}
      </TooltipTrigger>
      <TooltipContent>
        {sensitiveVisible ? t('Hide') : t('Show')}
      </TooltipContent>
    </Tooltip>
  )

  const dateRangeFilter = (
    <LogsFilterField wide className='shrink-0'>
      <CompactDateTimeRangePicker
        start={filters.startTime}
        end={filters.endTime}
        onChange={({ start, end }) => {
          handleChange('startTime', start)
          handleChange('endTime', end)
          handleApply({ ...filters, startTime: start, endTime: end }, logType)
        }}
      />
    </LogsFilterField>
  )
  const modelFilter = (
    <LogsFilterField flex className='min-w-[100px]'>
      <Combobox
        options={modelOptions}
        allowCustomValue
        aria-label={t('Model Name')}
        emptyText={t('No model found.')}
        placeholder={t('Model Name')}
        className='h-8 min-w-0 text-sm leading-5'
        value={filters.model || ''}
        onValueChange={(value) => {
          const modelValue = value || undefined
          handleChange('model', modelValue)
          handleApply({ ...filters, model: modelValue }, logType)
        }}
        onKeyDown={handleKeyDown}
      />
    </LogsFilterField>
  )
  const groupFilter = (
    <LogsFilterField flex className={cn('min-w-[85px]', sensitiveInputClass)}>
      <Combobox
        options={groupOptions}
        allowCustomValue
        aria-label={t('Group')}
        emptyText={t('No group found.')}
        placeholder={t('Group')}
        className='h-8 min-w-0 text-sm leading-5'
        value={filters.group || ''}
        onValueChange={(value) => {
          const groupValue = value || undefined
          handleChange('group', groupValue)
          handleApply({ ...filters, group: groupValue }, logType)
        }}
        onKeyDown={handleKeyDown}
      />
    </LogsFilterField>
  )
  const typeFilter = (
    <LogsFilterField flex className='min-w-[85px]'>
      <Select
        items={logTypeItems}
        value={logType}
        onValueChange={(value) => {
          const nextLogType =
            value !== null && isLogTypeValue(value) ? value : LOG_TYPE_ALL_VALUE
          setDraft((current) => {
            const base =
              current.sourceKey === searchState.sourceKey
                ? current
                : searchState
            return {
              sourceKey: searchState.sourceKey,
              filters: base.filters,
              logType: nextLogType,
            }
          })
          handleApply(filters, nextLogType)
        }}
      >
        <SelectTrigger
          aria-label={t('Type')}
          aria-description={
            selectedLogType?.deprecated ? deprecatedTypeDescription : undefined
          }
        >
          <SelectValue className='min-w-0'>
            <span className='truncate'>
              {selectedLogType?.label ?? t('All Types')}
            </span>
            {selectedLogType?.deprecated && (
              <Badge
                variant='secondary'
                className='h-4 px-1.5 text-[10px] font-normal'
                title={deprecatedTypeDescription}
              >
                {t('Deprecated')}
              </Badge>
            )}
          </SelectValue>
        </SelectTrigger>
        <SelectContent
          alignItemWithTrigger={false}
          className='max-w-[calc(100vw-2rem)] min-w-52'
        >
          <SelectGroup>
            {LOG_TYPE_FILTERS.map((type) => (
              <SelectItem
                key={type.value}
                value={type.value}
                className='[&_[data-slot=select-item-text]]:items-center'
                aria-description={
                  type.deprecated ? deprecatedTypeDescription : undefined
                }
              >
                {t(type.label)}
                {type.deprecated && (
                  <Badge
                    variant='secondary'
                    className='h-4 px-1.5 text-[10px] font-normal'
                    title={deprecatedTypeDescription}
                  >
                    {t('Deprecated')}
                  </Badge>
                )}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </LogsFilterField>
  )
  const apiAddressFilter = isAdmin ? (
    <LogsFilterField flex className='min-w-[160px] max-w-[260px]'>
      <Combobox
        options={apiAddressOptions}
        allowCustomValue
        aria-label={t('API Address', 'API 地址')}
        emptyText={t('No API address found.', '未找到 API 地址')}
        placeholder={t('API Address', 'API 地址')}
        className='h-8 min-w-0 text-sm leading-5 font-mono'
        value={filters.baseUrl || ''}
        onValueChange={(value) => {
          const val = value || undefined
          handleChange('baseUrl', val)
          handleApply({ ...filters, baseUrl: val }, logType)
        }}
        onKeyDown={handleKeyDown}
        renderOptionHoverPanel={(option, closeDropdown) => {
          if (!option.value) return null
          const norm = option.value.trim().toLowerCase().replace(/\/+$/, '')
          const source =
            sourceByBaseUrl.get(norm) || sourceByBaseUrl.get(option.value)
          return (
            <ApiAddressHoverPanel
              baseUrl={option.value}
              source={source}
              onOpenFullConfig={(src) => {
                closeDropdown()
                setEditingSource(src)
                setSourceDialogOpen(true)
              }}
            />
          )
        }}
      />
    </LogsFilterField>
  ) : null

  const expandedFilters = (
    <>
      <LogsFilterField flex className={cn('min-w-[85px]', sensitiveInputClass)}>
        <LogsFilterInput
          placeholder={t('Token Name')}
          className={sensitiveInputClass}
          value={filters.token || ''}
          onChange={(e) => handleChange('token', e.target.value)}
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
      {isAdmin && (
        <LogsFilterField flex className={cn('min-w-[120px]', sensitiveInputClass)}>
          <Combobox
            options={userOptions}
            allowCustomValue
            aria-label={t('Username')}
            emptyText={t('No user found.')}
            placeholder={t('Username')}
            className={cn('h-8 min-w-0 text-sm leading-5', sensitiveInputClass)}
            value={filters.username || ''}
            onValueChange={(value) => {
              const userVal = value || undefined
              handleChange('username', userVal)
              handleApply({ ...filters, username: userVal }, logType)
            }}
            onKeyDown={handleKeyDown}
          />
        </LogsFilterField>
      )}
      {isAdmin && (
        <LogsFilterField flex className='min-w-[75px]'>
          <LogsFilterInput
            placeholder={t('Channel ID')}
            value={filters.channel || ''}
            onChange={(e) => handleChange('channel', e.target.value)}
            onKeyDown={handleKeyDown}
          />
        </LogsFilterField>
      )}
      <LogsFilterField flex className='min-w-[80px]'>
        <LogsFilterInput
          placeholder={t('Request ID')}
          value={filters.requestId || ''}
          onChange={(e) => handleChange('requestId', e.target.value)}
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
      <LogsFilterField flex className='min-w-[90px]'>
        <LogsFilterInput
          placeholder={t('Upstream Request ID')}
          value={filters.upstreamRequestId || ''}
          onChange={(e) => handleChange('upstreamRequestId', e.target.value)}
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
    </>
  )

  const statusCodeItems = useMemo(
    () =>
      STATUS_CODE_OPTIONS.map((item) => ({
        value: item.value,
        label:
          item.value === 'all'
            ? t('Status Code', '状态码')
            : item.label,
      })),
    [t]
  )
  const currentStatusCode = filters.statusCode || 'all'
  const selectedStatusCode = statusCodeItems.find(
    (item) => item.value === currentStatusCode
  )

  const statusCodeFilter = (
    <Select
      items={statusCodeItems}
      value={currentStatusCode}
      onValueChange={(value) => {
        const nextCode =
          value !== null && value !== 'all' ? value : undefined
        handleChange('statusCode', nextCode)
        handleApply({ ...filters, statusCode: nextCode }, logType)
      }}
    >
      <SelectTrigger
        size='default'
        aria-label={t('Status Code', '状态码')}
        className='h-8 min-w-[95px] shrink-0 text-xs sm:text-sm'
      >
        <SelectValue className='min-w-0'>
          <span className='truncate'>
            {selectedStatusCode?.label ?? t('Status Code', '状态码')}
          </span>
        </SelectValue>
      </SelectTrigger>
      <SelectContent
        alignItemWithTrigger={false}
        className='max-w-[calc(100vw-2rem)] min-w-32'
      >
        <SelectGroup>
          {statusCodeItems.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.value === 'all'
                ? t('All Status Codes', '全部状态码')
                : item.label}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  )

  return (
    <>
      <LogsFilterToolbar
        table={props.table}
        compactMobile
        stats={statsBar}
        actionStart={sensitiveToggle}
        actionEnd={statusCodeFilter}
        primaryFilters={
          <>
            {dateRangeFilter}
            {modelFilter}
            {apiAddressFilter}
            {groupFilter}
            {typeFilter}
            {expandedFilters}
          </>
        }
        mobilePinnedFilters={dateRangeFilter}
        mobileFilters={
          <>
            {modelFilter}
            {apiAddressFilter}
            {groupFilter}
            {typeFilter}
            {expandedFilters}
            <div className='flex items-center justify-between gap-2 pt-1'>
              <span className='text-muted-foreground text-sm'>{t('Status Code', '状态码')}</span>
              {statusCodeFilter}
            </div>
          </>
        }
        mobileFilterCount={
          [filters.model, filters.group, hasTypeFilter, !!filters.statusCode].filter(Boolean).length +
          expandedFilterCount
        }
        hasActiveFilters={hasAdditionalFilters}
        onSearch={() => handleApply()}
        searchLoading={fetchingLogs > 0}
        onReset={handleReset}
      />
      {isAdmin && (
        <QuickSourceDialog
          source={editingSource}
          open={sourceDialogOpen}
          onOpenChange={setSourceDialogOpen}
        />
      )}
    </>
  )
}
