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
  type ColumnDef,
  type ColumnFiltersState,
  type ColumnOrderState,
  type ColumnSizingState,
  type ExpandedState,
  type OnChangeFn,
  type PaginationState,
  type RowSelectionState,
  type SortingState,
  type TableOptions,
  type Updater,
  type VisibilityState,
  getCoreRowModel,
  getExpandedRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import * as React from 'react'

type DataTableFeatureOptions<TData> = Pick<
  TableOptions<TData>,
  | 'enableRowSelection'
  | 'getRowId'
  | 'getSubRows'
  | 'globalFilterFn'
  | 'autoResetPageIndex'
  | 'manualFiltering'
  | 'manualPagination'
  | 'manualSorting'
  | 'enableSorting'
  | 'enableColumnResizing'
>

type DataTableStateOptions = {
  initialSorting?: SortingState
  sorting?: SortingState
  onSortingChange?: OnChangeFn<SortingState>
  initialColumnOrder?: ColumnOrderState
  columnOrderStorageKey?: string | false
  columnOrder?: ColumnOrderState
  onColumnOrderChange?: OnChangeFn<ColumnOrderState>
  fixedColumnOrder?: {
    start?: readonly string[]
    end?: readonly string[]
  }
  initialColumnVisibility?: VisibilityState
  columnVisibilityStorageKey?: string | false
  columnVisibility?: VisibilityState
  onColumnVisibilityChange?: OnChangeFn<VisibilityState>
  initialColumnSizing?: ColumnSizingState
  columnSizingStorageKey?: string | false
  columnSizing?: ColumnSizingState
  onColumnSizingChange?: OnChangeFn<ColumnSizingState>
  initialRowSelection?: RowSelectionState
  rowSelection?: RowSelectionState
  onRowSelectionChange?: OnChangeFn<RowSelectionState>
  initialExpanded?: ExpandedState
  expanded?: ExpandedState
  onExpandedChange?: OnChangeFn<ExpandedState>
  columnFilters?: ColumnFiltersState
  onColumnFiltersChange?: OnChangeFn<ColumnFiltersState>
  globalFilter?: string
  onGlobalFilterChange?: OnChangeFn<string>
  initialPagination?: PaginationState
  pagination?: PaginationState
  onPaginationChange?: OnChangeFn<PaginationState>
}

type DataTableRowModelOptions = {
  withFilteredRowModel?: boolean
  withPaginationRowModel?: boolean
  withSortedRowModel?: boolean
  withFacetedRowModel?: boolean
  withExpandedRowModel?: boolean
}

type UseDataTableOptions<TData> = DataTableFeatureOptions<TData> &
  DataTableStateOptions &
  DataTableRowModelOptions & {
    data: TData[]
    columns: ColumnDef<TData, unknown>[]
    totalCount?: number
    pageCount?: number
    ensurePageInRange?: (pageCount: number) => void
  }

type ColumnSizingBounds = Record<
  string,
  {
    minSize?: number
    maxSize?: number
  }
>

type ColumnWithSizing<TData> = ColumnDef<TData, unknown> & {
  accessorKey?: string | number
  columns?: ColumnDef<TData, unknown>[]
}

const COLUMN_SIZING_PERSIST_DELAY_MS = 250
const EMPTY_COLUMN_ORDER: ColumnOrderState = []

function resolveUpdater<TValue>(
  updater: Updater<TValue>,
  previous: TValue
): TValue {
  return typeof updater === 'function'
    ? (updater as (old: TValue) => TValue)(previous)
    : updater
}

function useControllableTableState<TValue>(
  controlledValue: TValue | undefined,
  defaultValue: TValue,
  onChange: OnChangeFn<TValue> | undefined
): [TValue, OnChangeFn<TValue>] {
  const [uncontrolledValue, setUncontrolledValue] =
    React.useState<TValue>(defaultValue)

  const value = controlledValue ?? uncontrolledValue

  const setValue = React.useCallback<OnChangeFn<TValue>>(
    (updater) => {
      if (controlledValue === undefined) {
        setUncontrolledValue((previous) => resolveUpdater(updater, previous))
      }
      onChange?.(updater)
    },
    [controlledValue, onChange]
  )

  return [value, setValue]
}

function readColumnVisibility(storageKey: string | undefined): VisibilityState {
  if (!storageKey || typeof window === 'undefined') return {}

  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw) return {}

    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {}
    }

    return Object.entries(parsed).reduce<VisibilityState>(
      (visibility, [key, value]) => {
        if (typeof value === 'boolean') {
          visibility[key] = value
        }
        return visibility
      },
      {}
    )
  } catch {
    return {}
  }
}

function getColumnId<TData>(column: ColumnDef<TData, unknown>) {
  const columnWithSizing = column as ColumnWithSizing<TData>

  if (typeof columnWithSizing.id === 'string') {
    return columnWithSizing.id
  }

  if (typeof columnWithSizing.accessorKey === 'string') {
    return columnWithSizing.accessorKey.replaceAll('.', '_')
  }

  if (typeof columnWithSizing.accessorKey === 'number') {
    return String(columnWithSizing.accessorKey)
  }

  return undefined
}

function getLeafColumnIds<TData>(
  columns: ColumnDef<TData, unknown>[]
): ColumnOrderState {
  const ids: string[] = []

  for (const column of columns) {
    const columnWithSizing = column as ColumnWithSizing<TData>
    if (Array.isArray(columnWithSizing.columns)) {
      ids.push(...getLeafColumnIds(columnWithSizing.columns))
      continue
    }

    const columnId = getColumnId(column)
    if (columnId) {
      ids.push(columnId)
    }
  }

  return [...new Set(ids)]
}

function getKnownColumnIds(
  columnIds: readonly string[],
  requestedIds: readonly string[]
): string[] {
  const knownIds = new Set(columnIds)
  const result: string[] = []
  const seenIds = new Set<string>()

  for (const id of requestedIds) {
    if (!knownIds.has(id) || seenIds.has(id)) {
      continue
    }

    seenIds.add(id)
    result.push(id)
  }

  return result
}

function normalizeColumnOrder(
  columnOrder: readonly string[] | undefined,
  columnIds: readonly string[],
  fixedColumnOrder: DataTableStateOptions['fixedColumnOrder']
): ColumnOrderState {
  const fixedStart = getKnownColumnIds(
    columnIds,
    fixedColumnOrder?.start ?? EMPTY_COLUMN_ORDER
  )
  const fixedStartIds = new Set(fixedStart)
  const fixedEnd = getKnownColumnIds(
    columnIds,
    (fixedColumnOrder?.end ?? EMPTY_COLUMN_ORDER).filter(
      (id) => !fixedStartIds.has(id)
    )
  )
  const fixedColumnIds = new Set([...fixedStart, ...fixedEnd])
  const requestedIds = getKnownColumnIds(
    columnIds,
    (columnOrder ?? EMPTY_COLUMN_ORDER).filter((id) => !fixedColumnIds.has(id))
  )
  const requestedIdSet = new Set(requestedIds)
  const remainingIds = columnIds.filter(
    (id) => !fixedColumnIds.has(id) && !requestedIdSet.has(id)
  )

  return [...fixedStart, ...requestedIds, ...remainingIds, ...fixedEnd]
}

function readColumnOrder(
  storageKey: string | undefined
): ColumnOrderState | undefined {
  if (!storageKey || typeof window === 'undefined') return undefined

  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw) return undefined

    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed)) return undefined

    return parsed.filter((id): id is string => typeof id === 'string')
  } catch {
    return undefined
  }
}

function buildColumnSizingBounds<TData>(
  columns: ColumnDef<TData, unknown>[]
): ColumnSizingBounds {
  return columns.reduce<ColumnSizingBounds>((bounds, column) => {
    const columnWithSizing = column as ColumnWithSizing<TData>
    const columnId = getColumnId(column)

    if (columnId) {
      const minSize =
        typeof columnWithSizing.minSize === 'number' &&
        Number.isFinite(columnWithSizing.minSize)
          ? columnWithSizing.minSize
          : undefined
      const maxSize =
        typeof columnWithSizing.maxSize === 'number' &&
        Number.isFinite(columnWithSizing.maxSize)
          ? columnWithSizing.maxSize
          : undefined

      if (minSize !== undefined || maxSize !== undefined) {
        bounds[columnId] = { minSize, maxSize }
      }
    }

    if (Array.isArray(columnWithSizing.columns)) {
      Object.assign(bounds, buildColumnSizingBounds(columnWithSizing.columns))
    }

    return bounds
  }, {})
}

function getBoundedColumnSize(
  columnId: string,
  value: unknown,
  bounds: ColumnSizingBounds
) {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) {
    return undefined
  }

  const columnBounds = bounds[columnId]
  let size = value

  if (columnBounds?.minSize !== undefined && size < columnBounds.minSize) {
    size = columnBounds.minSize
  }

  if (columnBounds?.maxSize !== undefined && size > columnBounds.maxSize) {
    size = columnBounds.maxSize
  }

  return size > 0 ? size : undefined
}

function readColumnSizing(
  storageKey: string | undefined,
  bounds: ColumnSizingBounds
): ColumnSizingState {
  if (!storageKey || typeof window === 'undefined') return {}

  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw) return {}

    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {}
    }

    return Object.entries(parsed).reduce<ColumnSizingState>(
      (sizing, [key, value]) => {
        const boundedSize = getBoundedColumnSize(key, value, bounds)

        if (boundedSize !== undefined) {
          sizing[key] = boundedSize
        }
        return sizing
      },
      {}
    )
  } catch {
    return {}
  }
}

export function useDataTable<TData>(options: UseDataTableOptions<TData>) {
  const {
    data,
    columns,
    totalCount,
    pageCount: explicitPageCount,
    ensurePageInRange,
    manualFiltering,
    manualPagination,
    manualSorting,
    initialSorting = [],
    initialColumnOrder = EMPTY_COLUMN_ORDER,
    initialColumnVisibility = {},
    initialColumnSizing = {},
    initialRowSelection = {},
    initialExpanded = {},
    initialPagination = { pageIndex: 0, pageSize: 20 },
    withFilteredRowModel = !manualFiltering,
    withPaginationRowModel = !manualPagination,
    withSortedRowModel = !manualSorting && !manualPagination,
    withFacetedRowModel = !manualFiltering,
    withExpandedRowModel = false,
  } = options

  const columnVisibilityStorageKey =
    typeof options.columnVisibilityStorageKey === 'string'
      ? options.columnVisibilityStorageKey
      : undefined
  const columnOrderStorageKey =
    typeof options.columnOrderStorageKey === 'string'
      ? options.columnOrderStorageKey
      : undefined
  const columnSizingStorageKey =
    typeof options.columnSizingStorageKey === 'string'
      ? options.columnSizingStorageKey
      : undefined
  const fixedColumnOrderStart =
    options.fixedColumnOrder?.start ?? EMPTY_COLUMN_ORDER
  const fixedColumnOrderEnd =
    options.fixedColumnOrder?.end ?? EMPTY_COLUMN_ORDER
  const fixedColumnOrder = React.useMemo(
    () => ({
      start: fixedColumnOrderStart,
      end: fixedColumnOrderEnd,
    }),
    [fixedColumnOrderEnd, fixedColumnOrderStart]
  )
  const leafColumnIds = React.useMemo(
    () => getLeafColumnIds(columns),
    [columns]
  )
  const storedColumnOrder = React.useMemo(
    () => readColumnOrder(columnOrderStorageKey),
    [columnOrderStorageKey]
  )
  const resolvedInitialColumnOrder = React.useMemo(
    () =>
      normalizeColumnOrder(
        storedColumnOrder ?? initialColumnOrder,
        leafColumnIds,
        fixedColumnOrder
      ),
    [fixedColumnOrder, initialColumnOrder, leafColumnIds, storedColumnOrder]
  )
  const resolvedInitialColumnVisibility = React.useMemo(
    () => ({
      ...initialColumnVisibility,
      ...readColumnVisibility(columnVisibilityStorageKey),
    }),
    [columnVisibilityStorageKey, initialColumnVisibility]
  )
  const columnSizingBounds = React.useMemo(
    () => buildColumnSizingBounds(columns),
    [columns]
  )
  const resolvedInitialColumnSizing = React.useMemo(
    () => ({
      ...initialColumnSizing,
      ...readColumnSizing(columnSizingStorageKey, columnSizingBounds),
    }),
    [columnSizingBounds, columnSizingStorageKey, initialColumnSizing]
  )

  const [sorting, onSortingChange] = useControllableTableState(
    options.sorting,
    initialSorting,
    options.onSortingChange
  )
  const [unresolvedColumnOrder, setUnresolvedColumnOrder] =
    useControllableTableState(
      options.columnOrder,
      resolvedInitialColumnOrder,
      options.onColumnOrderChange
    )
  const columnOrder = React.useMemo(
    () =>
      normalizeColumnOrder(
        unresolvedColumnOrder,
        leafColumnIds,
        fixedColumnOrder
      ),
    [fixedColumnOrder, leafColumnIds, unresolvedColumnOrder]
  )
  const onColumnOrderChange = React.useCallback<OnChangeFn<ColumnOrderState>>(
    (updater) => {
      const nextColumnOrder = normalizeColumnOrder(
        resolveUpdater(updater, columnOrder),
        leafColumnIds,
        fixedColumnOrder
      )
      setUnresolvedColumnOrder(nextColumnOrder)
    },
    [columnOrder, fixedColumnOrder, leafColumnIds, setUnresolvedColumnOrder]
  )
  const [columnVisibility, onColumnVisibilityChange] =
    useControllableTableState(
      options.columnVisibility,
      resolvedInitialColumnVisibility,
      options.onColumnVisibilityChange
    )
  const [columnSizing, onColumnSizingChange] = useControllableTableState(
    options.columnSizing,
    resolvedInitialColumnSizing,
    options.onColumnSizingChange
  )
  const latestColumnSizingRef = React.useRef(columnSizing)
  latestColumnSizingRef.current = columnSizing
  const hydratedColumnVisibilityStorageKeyRef = React.useRef(
    columnVisibilityStorageKey
  )
  const hydratedColumnOrderStorageKeyRef = React.useRef(columnOrderStorageKey)
  const hydratedColumnSizingStorageKeyRef = React.useRef(columnSizingStorageKey)
  const skipNextColumnVisibilityPersistRef = React.useRef(false)
  const skipNextColumnOrderPersistRef = React.useRef(false)
  const skipNextColumnSizingPersistRef = React.useRef(false)
  const columnSizingPersistTimerRef = React.useRef<number | undefined>(
    undefined
  )
  const [rowSelection, onRowSelectionChange] = useControllableTableState(
    options.rowSelection,
    initialRowSelection,
    options.onRowSelectionChange
  )
  const [expanded, onExpandedChange] = useControllableTableState(
    options.expanded,
    initialExpanded,
    options.onExpandedChange
  )
  const [pagination, onPaginationChange] = useControllableTableState(
    options.pagination,
    initialPagination,
    options.onPaginationChange
  )

  const resolvedPageCount =
    explicitPageCount ??
    (totalCount !== undefined
      ? Math.ceil(totalCount / pagination.pageSize)
      : undefined)
  const resolvedEnableSorting =
    options.enableSorting ??
    (!manualPagination ||
      Boolean(options.sorting) ||
      Boolean(options.onSortingChange))

  const table = useReactTable({
    data,
    columns,
    rowCount: totalCount,
    pageCount: resolvedPageCount,
    state: {
      sorting,
      columnOrder,
      columnVisibility,
      columnSizing,
      rowSelection,
      expanded,
      columnFilters: options.columnFilters,
      globalFilter: options.globalFilter,
      pagination,
    },
    enableRowSelection: options.enableRowSelection,
    enableSorting: resolvedEnableSorting,
    getRowId: options.getRowId,
    getSubRows: options.getSubRows,
    globalFilterFn: options.globalFilterFn,
    autoResetPageIndex: options.autoResetPageIndex,
    manualFiltering,
    manualPagination,
    manualSorting,
    enableColumnResizing: options.enableColumnResizing,
    columnResizeMode: 'onChange',
    onSortingChange,
    onColumnOrderChange,
    onColumnVisibilityChange,
    onColumnSizingChange,
    onRowSelectionChange,
    onExpandedChange,
    onColumnFiltersChange: options.onColumnFiltersChange,
    onGlobalFilterChange: options.onGlobalFilterChange,
    onPaginationChange,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: withFilteredRowModel
      ? getFilteredRowModel()
      : undefined,
    getPaginationRowModel: withPaginationRowModel
      ? getPaginationRowModel()
      : undefined,
    getSortedRowModel: withSortedRowModel ? getSortedRowModel() : undefined,
    getFacetedRowModel: withFacetedRowModel ? getFacetedRowModel() : undefined,
    getFacetedUniqueValues: withFacetedRowModel
      ? getFacetedUniqueValues()
      : undefined,
    getExpandedRowModel: withExpandedRowModel
      ? getExpandedRowModel()
      : undefined,
  })

  const actualPageCount = table.getPageCount()
  React.useEffect(() => {
    ensurePageInRange?.(actualPageCount)
  }, [actualPageCount, ensurePageInRange])

  React.useEffect(() => {
    if (
      options.columnOrder !== undefined ||
      columnOrderStorageKey === hydratedColumnOrderStorageKeyRef.current
    ) {
      return
    }

    hydratedColumnOrderStorageKeyRef.current = columnOrderStorageKey
    skipNextColumnOrderPersistRef.current = true
    setUnresolvedColumnOrder(() => resolvedInitialColumnOrder)
  }, [
    columnOrderStorageKey,
    options.columnOrder,
    resolvedInitialColumnOrder,
    setUnresolvedColumnOrder,
  ])

  React.useEffect(() => {
    if (
      options.columnVisibility !== undefined ||
      columnVisibilityStorageKey ===
        hydratedColumnVisibilityStorageKeyRef.current
    ) {
      return
    }

    hydratedColumnVisibilityStorageKeyRef.current = columnVisibilityStorageKey
    skipNextColumnVisibilityPersistRef.current = true
    onColumnVisibilityChange(() => resolvedInitialColumnVisibility)
  }, [
    columnVisibilityStorageKey,
    onColumnVisibilityChange,
    options.columnVisibility,
    resolvedInitialColumnVisibility,
  ])

  React.useEffect(() => {
    if (
      options.columnSizing !== undefined ||
      columnSizingStorageKey === hydratedColumnSizingStorageKeyRef.current
    ) {
      return
    }

    hydratedColumnSizingStorageKeyRef.current = columnSizingStorageKey
    skipNextColumnSizingPersistRef.current = true
    onColumnSizingChange(() => resolvedInitialColumnSizing)
  }, [
    columnSizingStorageKey,
    onColumnSizingChange,
    options.columnSizing,
    resolvedInitialColumnSizing,
  ])

  React.useEffect(() => {
    if (!columnOrderStorageKey || typeof window === 'undefined') return

    if (skipNextColumnOrderPersistRef.current) {
      skipNextColumnOrderPersistRef.current = false
      return
    }

    try {
      window.localStorage.setItem(
        columnOrderStorageKey,
        JSON.stringify(columnOrder)
      )
    } catch {
      // Storage can be unavailable in private mode; table controls still work.
    }
  }, [columnOrder, columnOrderStorageKey])

  React.useEffect(() => {
    if (!columnVisibilityStorageKey || typeof window === 'undefined') return

    if (skipNextColumnVisibilityPersistRef.current) {
      skipNextColumnVisibilityPersistRef.current = false
      return
    }

    try {
      window.localStorage.setItem(
        columnVisibilityStorageKey,
        JSON.stringify(columnVisibility)
      )
    } catch {
      // Storage can be unavailable in private mode; table controls still work.
    }
  }, [columnVisibility, columnVisibilityStorageKey])

  React.useEffect(() => {
    if (!columnSizingStorageKey || typeof window === 'undefined') return

    if (skipNextColumnSizingPersistRef.current) {
      skipNextColumnSizingPersistRef.current = false
      return
    }

    if (columnSizingPersistTimerRef.current !== undefined) {
      window.clearTimeout(columnSizingPersistTimerRef.current)
    }

    columnSizingPersistTimerRef.current = window.setTimeout(() => {
      try {
        window.localStorage.setItem(
          columnSizingStorageKey,
          JSON.stringify(columnSizing)
        )
      } catch {
        // Storage can be unavailable in private mode; table controls still work.
      } finally {
        columnSizingPersistTimerRef.current = undefined
      }
    }, COLUMN_SIZING_PERSIST_DELAY_MS)

    return () => {
      if (columnSizingPersistTimerRef.current !== undefined) {
        window.clearTimeout(columnSizingPersistTimerRef.current)
        columnSizingPersistTimerRef.current = undefined
      }
    }
  }, [columnSizing, columnSizingStorageKey])

  // Column resizing is debounced while dragging. Persist the most recent size
  // on unmount as well, otherwise leaving the page within the debounce window
  // drops the user's final resize.
  React.useEffect(() => {
    if (!columnSizingStorageKey || typeof window === 'undefined') return

    return () => {
      try {
        window.localStorage.setItem(
          columnSizingStorageKey,
          JSON.stringify(latestColumnSizingRef.current)
        )
      } catch {
        // Storage can be unavailable in private mode; table controls still work.
      }
    }
  }, [columnSizingStorageKey])

  return {
    table,
  }
}
