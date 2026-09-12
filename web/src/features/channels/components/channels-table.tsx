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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import type {
  ColumnFiltersState,
  OnChangeFn,
  SortingState,
  Row,
} from '@tanstack/react-table'
import { Eye, EyeOff, Power, PowerOff, ScanLine } from 'lucide-react'
import { useState, useMemo, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
  useDebouncedColumnFilter,
  useDataTable,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { usePricingData } from '@/features/pricing/hooks'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { getLobeIcon } from '@/lib/lobe-icon'

import {
  detectAllChannelTypes,
  getChannelGroups,
  getChannels,
  searchChannels,
  updateChannelGroupAdaptiveEnabled,
  updateChannelGroupPriorityOrder,
} from '../api'
import {
  DEFAULT_PAGE_SIZE,
  CHANNEL_STATUS,
  CHANNEL_STATUS_OPTIONS,
  CHANNEL_TYPE_OPTIONS,
} from '../constants'
import {
  channelsQueryKeys,
  aggregateChannelsByTag,
  getChannelTableRowId,
  isTagAggregateRow,
  getChannelTypeIcon,
  getChannelTypeLabel,
  sortGroupsByModelAndRatio,
} from '../lib'
import type { Channel, ChannelSortBy } from '../types'
import { ChannelCard } from './channel-card'
import { useChannelsColumns } from './channels-columns'
import { useChannels } from './channels-provider'
import { DataTableBulkActions } from './data-table-bulk-actions'

const route = getRouteApi('/_authenticated/channels/')
const CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY = 'channels:column-visibility'
// v2 places the independently managed site type column next to the channel
// name. The versioned key migrates users who still have the pre-site-type
// column order persisted in localStorage.
const CHANNELS_COLUMN_ORDER_STORAGE_KEY = 'channels:column-order:v2'
const CHANNELS_COLUMN_SIZING_STORAGE_KEY = 'channels:column-sizing'
const CHANNELS_VIEW_MODE_STORAGE_KEY = 'channels:view-mode'
const CHANNELS_STATUS_FILTER_STORAGE_KEY = 'channel-status-filter'
const CHANNELS_TYPE_FILTER_STORAGE_KEY = 'channel-type-filter'
const CHANNELS_GROUP_FILTER_STORAGE_KEY = 'channel-group-filter'
const CHANNELS_GROUP_ORDER_STORAGE_KEY = 'channel-group-order'

function getStoredChannelArrayFilter(key: string): string[] | undefined {
  try {
    const stored = localStorage.getItem(key)
    if (stored === null) return undefined

    const parsed: unknown = JSON.parse(stored)
    return Array.isArray(parsed)
      ? parsed.filter((value): value is string => typeof value === 'string')
      : []
  } catch {
    return []
  }
}

function setStoredChannelArrayFilter(key: string, value: string[]) {
  try {
    if (value.length === 0) {
      localStorage.removeItem(key)
      return
    }
    localStorage.setItem(key, JSON.stringify(value))
  } catch {
    // Ignore storage failures, such as private browsing restrictions.
  }
}

function clearStoredChannelFilter(key: string) {
  try {
    localStorage.removeItem(key)
  } catch {
    // Ignore storage failures, such as private browsing restrictions.
  }
}

const CHANNEL_SORTABLE_COLUMNS = new Set<ChannelSortBy>([
  'id',
  'name',
  'site_type',
  'type',
  'priority',
  'balance',
  'channel_ratio',
  'used_quota',
  'response_time',
  'test_time',
])

function isDisabledChannelRow(channel: Channel) {
  return (
    !isTagAggregateRow(channel) && channel.status !== CHANNEL_STATUS.ENABLED
  )
}

export function ChannelsTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const {
    enableTagMode,
    idSort,
    groupSort,
    batchMode,
    groupOrderSort,
    sensitiveVisible,
    setSensitiveVisible,
  } = useChannels()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const { groupRatio, models } = usePricingData()

  const groupModels = useMemo(() => {
    const modelNamesByGroup = new Map<string, string[]>()
    for (const model of models) {
      for (const group of model.enable_groups) {
        const groupName = group.trim()
        if (!groupName || groupName === 'auto') {
          continue
        }

        const existingModelNames = modelNamesByGroup.get(groupName)
        if (existingModelNames) {
          if (!existingModelNames.includes(model.model_name)) {
            existingModelNames.push(model.model_name)
          }
          continue
        }
        modelNamesByGroup.set(groupName, [model.model_name])
      }
    }
    return modelNamesByGroup
  }, [models])

  // Table state
  const [sorting, setSorting] = useState<SortingState>([
    { id: 'type', desc: false },
  ])

  // URL state management
  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: {
      defaultPage: 1,
      defaultPageSize: isMobile ? 10 : DEFAULT_PAGE_SIZE,
    },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      {
        columnId: 'status',
        searchKey: 'status',
        type: 'array',
        deserialize: (value) => {
          if (value !== undefined) return value
          const stored = localStorage.getItem(
            CHANNELS_STATUS_FILTER_STORAGE_KEY
          )
          return stored === 'enabled' || stored === 'disabled' ? [stored] : []
        },
      },
      {
        columnId: 'type',
        searchKey: 'type',
        type: 'array',
        deserialize: (value) =>
          value !== undefined
            ? value
            : getStoredChannelArrayFilter(CHANNELS_TYPE_FILTER_STORAGE_KEY),
      },
      {
        columnId: 'group',
        searchKey: 'group',
        type: 'array',
        deserialize: (value) =>
          value !== undefined
            ? value
            : getStoredChannelArrayFilter(CHANNELS_GROUP_FILTER_STORAGE_KEY),
      },
      { columnId: 'model', searchKey: 'model', type: 'string' },
    ],
  })

  const handleColumnFiltersChange: OnChangeFn<ColumnFiltersState> = (
    updater
  ) => {
    onColumnFiltersChange((previous) => {
      const next = typeof updater === 'function' ? updater(previous) : updater
      const status = next.find((f) => f.id === 'status')?.value as
        | string[]
        | undefined
      localStorage.setItem(
        CHANNELS_STATUS_FILTER_STORAGE_KEY,
        status?.[0] ?? 'all'
      )
      const type = next.find((f) => f.id === 'type')?.value
      setStoredChannelArrayFilter(
        CHANNELS_TYPE_FILTER_STORAGE_KEY,
        Array.isArray(type)
          ? type.filter((value): value is string => typeof value === 'string')
          : []
      )
      const group = next.find((f) => f.id === 'group')?.value
      setStoredChannelArrayFilter(
        CHANNELS_GROUP_FILTER_STORAGE_KEY,
        Array.isArray(group)
          ? group.filter((value): value is string => typeof value === 'string')
          : []
      )
      return next
    })
  }

  // Extract filters from column filters
  const statusFilter =
    (columnFilters.find((f) => f.id === 'status')?.value as string[]) || []
  const typeFilter = useMemo(
    () => (columnFilters.find((f) => f.id === 'type')?.value as string[]) || [],
    [columnFilters]
  )
  const groupFilter =
    (columnFilters.find((f) => f.id === 'group')?.value as string[]) || []
  const {
    value: modelFilter,
    inputValue: modelFilterInput,
    onChange: onModelFilterInputChange,
    onCompositionStart: onModelFilterCompositionStart,
    onCompositionEnd: onModelFilterCompositionEnd,
    resetInput: resetModelFilterInput,
  } = useDebouncedColumnFilter({
    columnFilters,
    columnId: 'model',
    onColumnFiltersChange,
  })

  // Determine whether to use search or regular list API
  const shouldSearch = Boolean(globalFilter?.trim() || modelFilter.trim())

  const sortParams = useMemo(() => {
    const activeSort = sorting[0]
    if (
      !activeSort ||
      !CHANNEL_SORTABLE_COLUMNS.has(activeSort.id as ChannelSortBy)
    ) {
      return {}
    }

    return {
      sort_by: activeSort.id as ChannelSortBy,
      sort_order: activeSort.desc ? 'desc' : 'asc',
    } as const
  }, [sorting])

  const handleSortingChange: OnChangeFn<SortingState> = (updater) => {
    setSorting((previous) => {
      const next = typeof updater === 'function' ? updater(previous) : updater
      if (pagination.pageIndex > 0) {
        onPaginationChange({ ...pagination, pageIndex: 0 })
      }
      return next
    })
  }

  // The group filter order is also the canonical order used by the table when
  // group sorting is enabled. Keep both views driven by the same memoized list.
  const { data: groupsData } = useQuery({
    queryKey: ['channel-groups'],
    queryFn: getChannelGroups,
  })

  const groupOptions = useMemo(
    () =>
      (groupsData?.data || []).map((g) => ({
        label: g,
        value: g,
      })),
    [groupsData]
  )

  const [groupOrderOverride, setGroupOrderOverride] = useState<string[]>(
    () => getStoredChannelArrayFilter(CHANNELS_GROUP_ORDER_STORAGE_KEY) ?? []
  )
  const groupOrder = useMemo(
    () => {
      const naturalOrder = sortGroupsByModelAndRatio(
        groupOptions.map((option) => option.value),
        groupRatio,
        groupModels
      )
      const known = new Set(naturalOrder)
      const persisted = groupOrderOverride.filter((group) => known.has(group))
      return [...persisted, ...naturalOrder.filter((group) => !persisted.includes(group))]
    },
    [groupModels, groupOptions, groupOrderOverride, groupRatio]
  )

  const groupPriorityMutation = useMutation({
    mutationFn: updateChannelGroupPriorityOrder,
    onSuccess: (_, order) => {
      lastAppliedGroupPriorityOrder.current = order.join(',')
      queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
    },
    onError: (error, order) => {
      if (order) pendingGroupPriorityOrder.current = ''
      toast.error(error instanceof Error ? error.message : t('Failed to update channel'))
    },
  })

  // Applying group-order sorting also normalizes persisted priorities once per
  // order. This keeps existing channels consistent even when the user only
  // enables the toggle and does not drag a row first.
  const lastAppliedGroupPriorityOrder = useRef('')
  const pendingGroupPriorityOrder = useRef('')
  useEffect(() => {
    if (!groupOrderSort || groupOrder.length === 0) return
    const orderKey = groupOrder.join(',')
    if (
      lastAppliedGroupPriorityOrder.current === orderKey ||
      pendingGroupPriorityOrder.current === orderKey
    ) return
    pendingGroupPriorityOrder.current = orderKey
    groupPriorityMutation.mutate(groupOrder)
  }, [groupOrder, groupOrderSort, groupPriorityMutation])

  const moveGroup = (source: string, target: string) => {
    if (source === target || target === 'all' || source === 'all') return
    const next = [...groupOrder]
    const sourceIndex = next.indexOf(source)
    const targetIndex = next.indexOf(target)
    if (sourceIndex < 0 || targetIndex < 0) return
    next.splice(sourceIndex, 1)
    next.splice(next.indexOf(target), 0, source)
    setGroupOrderOverride(next)
    setStoredChannelArrayFilter(CHANNELS_GROUP_ORDER_STORAGE_KEY, next)
  }

  // Fetch channels data
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  const { data, isLoading, isFetching } = useQuery({
    queryKey: channelsQueryKeys.list({
      keyword: globalFilter,
      model: modelFilter,
      group:
        groupFilter.length > 0 && !groupFilter.includes('all')
          ? groupFilter[0]
          : undefined,
      status:
        statusFilter.length > 0 && !statusFilter.includes('all')
          ? statusFilter[0]
          : undefined,
      type:
        typeFilter.length > 0 && !typeFilter.includes('all')
          ? Number(typeFilter[0])
          : undefined,
      tag_mode: enableTagMode,
      id_sort: groupSort || groupOrderSort ? false : idSort,
      ...(groupSort ? { sort_by: 'priority' as const, sort_order: 'desc' as const } : {}),
      ...(groupOrderSort ? { group_order: groupOrder.join(',') } : {}),
      ...(groupSort || groupOrderSort ? {} : sortParams),
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
    }),
    queryFn: async () => {
      if (shouldSearch) {
        return searchChannels({
          keyword: globalFilter,
          model: modelFilter,
          group:
            groupFilter.length > 0 && !groupFilter.includes('all')
              ? groupFilter[0]
              : undefined,
          status:
            statusFilter.length > 0 && !statusFilter.includes('all')
              ? statusFilter[0]
              : undefined,
          type:
            typeFilter.length > 0 && !typeFilter.includes('all')
              ? Number(typeFilter[0])
              : undefined,
          tag_mode: enableTagMode,
          id_sort: groupSort || groupOrderSort ? false : idSort,
          ...(groupSort ? { sort_by: 'priority' as const, sort_order: 'desc' as const } : {}),
          ...(groupOrderSort ? { group_order: groupOrder.join(',') } : {}),
          ...(groupSort || groupOrderSort ? {} : sortParams),
          p: pagination.pageIndex + 1,
          page_size: pagination.pageSize,
        })
      } else {
        return getChannels({
          group:
            groupFilter.length > 0 && !groupFilter.includes('all')
              ? groupFilter[0]
              : undefined,
          status:
            statusFilter.length > 0 && !statusFilter.includes('all')
              ? statusFilter[0]
              : undefined,
          type:
            typeFilter.length > 0 && !typeFilter.includes('all')
              ? Number(typeFilter[0])
              : undefined,
          tag_mode: enableTagMode,
          id_sort: groupSort || groupOrderSort ? false : idSort,
          ...(groupSort ? { sort_by: 'priority' as const, sort_order: 'desc' as const } : {}),
          ...(groupOrderSort ? { group_order: groupOrder.join(',') } : {}),
          ...(groupSort || groupOrderSort ? {} : sortParams),
          p: pagination.pageIndex + 1,
          page_size: pagination.pageSize,
        })
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const detectSiteTypesMutation = useMutation({
    mutationFn: (ids: number[]) => detectAllChannelTypes(ids, true),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() }),
  })

  // Apply tag aggregation if tag mode is enabled
  const channels = useMemo(() => {
    const rawChannels = data?.data?.items || []

    if (enableTagMode && rawChannels.length > 0) {
      return aggregateChannelsByTag(rawChannels)
    }

    return rawChannels
  }, [data, enableTagMode])

  const totalCount = data?.data?.total || 0
  const typeCounts = data?.data?.type_counts

  // Columns configuration
  const columns = useChannelsColumns({ enableSelection: batchMode })

  // React Table instance
  const { table } = useDataTable({
    data: channels,
    columns,
    totalCount,
    sorting,
    initialColumnVisibility: {
      models: false,
      tag: false,
    },
    columnVisibilityStorageKey: CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY,
    columnOrderStorageKey: CHANNELS_COLUMN_ORDER_STORAGE_KEY,
    fixedColumnOrder: {
      start: batchMode ? ['select'] : [],
      end: ['actions'],
    },
    columnSizingStorageKey: isMobile
      ? false
      : CHANNELS_COLUMN_SIZING_STORAGE_KEY,
    columnFilters,
    pagination,
    globalFilter,
    enableRowSelection: batchMode
      ? (row: Row<Channel>) => !isTagAggregateRow(row.original)
      : false,
    onSortingChange: handleSortingChange,
    onColumnFiltersChange: handleColumnFiltersChange,
    onPaginationChange,
    onGlobalFilterChange,
    getRowId: getChannelTableRowId,
    getSubRows: (row: Channel & { children?: Channel[] }) => row.children,
    manualPagination: true,
    manualSorting: true,
    manualFiltering: true,
    withExpandedRowModel: true,
    enableColumnResizing: !isMobile,
    ensurePageInRange,
  })

  useEffect(() => {
    if (!batchMode) {
      table.resetRowSelection()
    }
  }, [batchMode, table])

  // Prepare filter options from existing channel types only.
  const typeFilterOptions = useMemo(() => {
    const counts = typeCounts || {}
    const typeIds = Object.entries(counts)
      .map(([type, count]) => ({
        type: Number(type),
        count: Number(count) || 0,
      }))
      .filter((item) => item.type > 0 && item.count > 0)
      .sort((a, b) => {
        const orderA = CHANNEL_TYPE_OPTIONS.findIndex(
          (option) => option.value === a.type
        )
        const orderB = CHANNEL_TYPE_OPTIONS.findIndex(
          (option) => option.value === b.type
        )
        // Keep the filter order aligned with the channel type order used by
        // the channel form; unknown types remain at the end.
        return (
          (orderA < 0 ? Number.MAX_SAFE_INTEGER : orderA) -
          (orderB < 0 ? Number.MAX_SAFE_INTEGER : orderB)
        )
      })

    const selectedType = typeFilter.find((value) => value !== 'all')
    if (selectedType) {
      const selectedTypeId = Number(selectedType)
      const alreadyIncluded = typeIds.some(
        (item) => item.type === selectedTypeId
      )
      if (selectedTypeId > 0 && !alreadyIncluded) {
        typeIds.push({
          type: selectedTypeId,
          count: Number(counts[selectedType]) || 0,
        })
      }
    }

    const totalTypes = Object.values(counts).reduce(
      (sum, count) => sum + (Number(count) || 0),
      0
    )

    return [
      {
        label: 'All Types',
        value: 'all',
        count: totalTypes,
      },
      ...typeIds.map((item) => {
        const iconName = getChannelTypeIcon(item.type)
        return {
          label: getChannelTypeLabel(item.type),
          value: String(item.type),
          count: item.count,
          iconNode: getLobeIcon(`${iconName}.Color`, 16),
        }
      }),
    ]
  }, [typeCounts, typeFilter])

  const sortedGroupNames = groupOrder

  const groupAdaptiveMutation = useMutation({
    mutationFn: ({ group, enabled }: { group: string; enabled: boolean }) =>
      updateChannelGroupAdaptiveEnabled(group, enabled),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() }),
    onError: (error) =>
      toast.error(
        error instanceof Error ? error.message : t('Failed to update channel')
      ),
  })

  const groupAdaptiveState = useMemo(() => {
    const state = new Map<string, boolean>()
    for (const channel of data?.data?.items || []) {
      for (const group of channel.group?.split(',').map((item) => item.trim()) || []) {
        if (!group) continue
        state.set(group, (state.get(group) ?? true) && channel.adaptive_enabled)
      }
    }
    return state
  }, [data])

  const groupFilterOptions = useMemo(() => {
    const ratioForGroup = (group: string) => {
      const ratio = groupRatio[group]
      return typeof ratio === 'number' && Number.isFinite(ratio) ? ratio : null
    }

    return [
      { label: t('All Groups'), value: 'all' },
      ...sortedGroupNames.map((group) => {
        const ratio = ratioForGroup(group)
        const label = sensitiveVisible ? group : '••••'
        return {
          value: group,
          label:
            ratio === null
              ? label
              : `${label} · ${t('Ratio: {{value}}', { value: `${ratio}x` })}`,
        }
      }),
    ]
  }, [groupRatio, sensitiveVisible, sortedGroupNames, t])

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No Channels Found')}
      emptyDescription={t(
        'No channels available. Create your first channel to get started.'
      )}
      skeletonKeyPrefix='channel-skeleton'
      enableCardView
      viewModeStorageKey={CHANNELS_VIEW_MODE_STORAGE_KEY}
      renderCard={(row, { isSelected }) => (
        <ChannelCard
          row={row}
          isSelected={isSelected}
          groupRatio={groupRatio}
        />
      )}
      cardGridClassName='grid grid-cols-1 gap-3 sm:gap-4 lg:grid-cols-3'
      applyHeaderSize
      toolbarProps={{
        enableColumnReordering: true,
        searchPlaceholder: t('Filter by name, ID, or key...'),
        searchDebounceMs: 500,
        onReset: () => {
          resetModelFilterInput()
        },
        additionalSearch: (
          <Input
            placeholder={t('Filter by model...')}
            value={modelFilterInput}
            onChange={onModelFilterInputChange}
            onCompositionStart={onModelFilterCompositionStart}
            onCompositionEnd={onModelFilterCompositionEnd}
            className='w-full sm:w-[150px] lg:w-[180px]'
          />
        ),
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            options: [...CHANNEL_STATUS_OPTIONS],
            singleSelect: true,
            onClear: () =>
              clearStoredChannelFilter(CHANNELS_STATUS_FILTER_STORAGE_KEY),
          },
          {
            columnId: 'type',
            title: t('Type'),
            options: typeFilterOptions,
            singleSelect: true,
            onClear: () =>
              clearStoredChannelFilter(CHANNELS_TYPE_FILTER_STORAGE_KEY),
          },
          {
            columnId: 'group',
            title: t('Group'),
            options: groupFilterOptions,
            singleSelect: true,
            renderOptionActions: (option) => {
              if (option.value === 'all') return null
              const enabled = groupAdaptiveState.get(option.value) ?? false
              return (
                <Button
                  variant='ghost'
                  size='icon'
                  className='size-7 shrink-0 text-muted-foreground hover:text-foreground'
                  disabled={groupAdaptiveMutation.isPending}
                  title={t(enabled ? 'Disable' : 'Enable')}
                  aria-label={t(enabled ? 'Disable' : 'Enable')}
                  onPointerDown={(event) => event.stopPropagation()}
                  onClick={(event) => {
                    event.stopPropagation()
                    event.preventDefault()
                    groupAdaptiveMutation.mutate({
                      group: option.value,
                      enabled: !enabled,
                    })
                  }}
                >
                  {enabled ? <PowerOff className='size-3.5' /> : <Power className='size-3.5' />}
                </Button>
              )
            },
            onOptionReorder: moveGroup,
            onClear: () =>
              clearStoredChannelFilter(CHANNELS_GROUP_FILTER_STORAGE_KEY),
          },
        ],
        preActions: (
          <>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant='ghost'
                    size='icon'
                    onClick={() => {
                      const ids = (data?.data?.items || [])
                        .filter(
                          (channel) =>
                            !channel.site_type || channel.site_type === 'unknown'
                        )
                        .map((channel) => channel.id)
                      if (ids.length > 0) {
                        detectSiteTypesMutation.mutate(ids)
                      }
                    }}
                    disabled={
                      detectSiteTypesMutation.isPending ||
                      !(data?.data?.items || []).some(
                        (channel) =>
                          !channel.site_type || channel.site_type === 'unknown'
                      )
                    }
                    aria-label={t('Detect Site Types')}
                    className='text-muted-foreground hover:text-foreground size-8'
                  />
                }
              >
                <ScanLine />
              </TooltipTrigger>
              <TooltipContent>{t('Detect Site Types')}</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant='ghost'
                    size='icon'
                    onClick={() => setSensitiveVisible(!sensitiveVisible)}
                    aria-label={sensitiveVisible ? t('Hide') : t('Show')}
                    className='text-muted-foreground hover:text-foreground size-8'
                  />
                }
              >
                {sensitiveVisible ? <Eye /> : <EyeOff />}
              </TooltipTrigger>
              <TooltipContent>
                {sensitiveVisible ? t('Hide') : t('Show')}
              </TooltipContent>
            </Tooltip>
          </>
        ),
      }}
      getRowClassName={(row, { isMobile }) => {
        if (!isDisabledChannelRow(row.original)) {
          return undefined
        }
        if (isMobile) {
          return DISABLED_ROW_MOBILE
        }
        return DISABLED_ROW_DESKTOP
      }}
      bulkActions={batchMode ? <DataTableBulkActions table={table} /> : null}
    />
  )
}
