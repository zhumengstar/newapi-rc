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
  ArrowDown01Icon,
  ArrowUp01Icon,
  Drag01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { RotateCcw } from 'lucide-react'
import type { Column, Table } from '@tanstack/react-table'
import { Reorder, useDragControls } from 'motion/react'
import * as React from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

type DataTableViewOptionsProps<TData> = {
  table: Table<TData>
  enableColumnReordering?: boolean
}

type DataTableColumnOptionProps<TData> = {
  column: Column<TData, unknown>
  index: number
  count: number
  enableColumnReordering: boolean
  onMove: (columnId: string, direction: -1 | 1) => void
}

function DataTableColumnOption<TData>({
  column,
  index,
  count,
  enableColumnReordering,
  onMove,
}: DataTableColumnOptionProps<TData>) {
  const { t } = useTranslation()
  const dragControls = useDragControls()
  const columnLabel =
    typeof column.columnDef.header === 'string'
      ? column.columnDef.header
      : String(column.columnDef.meta?.label ?? column.id)
  const dragLabel = t('Drag {{group}} to reorder', {
    group: columnLabel,
  })
  const moveUpLabel = t('Move {{group}} up', {
    group: columnLabel,
  })
  const moveDownLabel = t('Move {{group}} down', {
    group: columnLabel,
  })

  const handleDragStart = (event: React.PointerEvent<HTMLButtonElement>) => {
    dragControls.start(event)
  }

  const handleDragKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>) => {
    let direction: -1 | 1
    if (event.key === 'ArrowUp') {
      direction = -1
    } else if (event.key === 'ArrowDown') {
      direction = 1
    } else {
      return
    }

    event.preventDefault()
    event.stopPropagation()
    onMove(column.id, direction)
  }

  return (
    <Reorder.Item
      as='div'
      value={column.id}
      drag={enableColumnReordering ? 'y' : false}
      dragListener={false}
      dragControls={dragControls}
      className='bg-popover flex items-center gap-0.5'
    >
      <DropdownMenuCheckboxItem
        className={cn(
          'min-w-0 flex-1 capitalize',
          enableColumnReordering && 'pr-2'
        )}
        checked={column.getIsVisible()}
        onCheckedChange={(value) => column.toggleVisibility(!!value)}
      >
        <span className='truncate'>{columnLabel}</span>
      </DropdownMenuCheckboxItem>
      {enableColumnReordering && (
        <div className='flex shrink-0 items-center'>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-xs'
                  className='text-muted-foreground cursor-grab touch-none active:cursor-grabbing'
                  aria-label={dragLabel}
                  onPointerDown={handleDragStart}
                  onKeyDown={handleDragKeyDown}
                >
                  <HugeiconsIcon
                    icon={Drag01Icon}
                    strokeWidth={2}
                    aria-hidden='true'
                  />
                </Button>
              }
            />
            <TooltipContent>{dragLabel}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-xs'
                  aria-label={moveUpLabel}
                  disabled={index === 0}
                  onClick={() => onMove(column.id, -1)}
                >
                  <HugeiconsIcon
                    icon={ArrowUp01Icon}
                    strokeWidth={2}
                    aria-hidden='true'
                  />
                </Button>
              }
            />
            <TooltipContent>{moveUpLabel}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-xs'
                  aria-label={moveDownLabel}
                  disabled={index === count - 1}
                  onClick={() => onMove(column.id, 1)}
                >
                  <HugeiconsIcon
                    icon={ArrowDown01Icon}
                    strokeWidth={2}
                    aria-hidden='true'
                  />
                </Button>
              }
            />
            <TooltipContent>{moveDownLabel}</TooltipContent>
          </Tooltip>
        </div>
      )}
    </Reorder.Item>
  )
}

export function DataTableViewOptions<TData>({
  table,
  enableColumnReordering = false,
}: DataTableViewOptionsProps<TData>) {
  const { t } = useTranslation()
  const hideableColumns = table
    .getAllLeafColumns()
    .filter((column) => column.getCanHide())
  const movableColumnIds = hideableColumns.map((column) => column.id)

  const applyMovableColumnOrder = React.useCallback(
    (nextMovableColumnIds: string[]) => {
      const allLeafColumns = table.getAllLeafColumns()
      const currentMovableColumnIds = allLeafColumns
        .filter((column) => column.getCanHide())
        .map((column) => column.id)
      const movableColumnIdSet = new Set(currentMovableColumnIds)

      if (
        nextMovableColumnIds.length !== currentMovableColumnIds.length ||
        new Set(nextMovableColumnIds).size !== currentMovableColumnIds.length ||
        nextMovableColumnIds.some(
          (columnId) => !movableColumnIdSet.has(columnId)
        )
      ) {
        return
      }

      let nextMovableIndex = 0
      table.setColumnOrder(
        allLeafColumns.map((column) => {
          if (!movableColumnIdSet.has(column.id)) {
            return column.id
          }

          const nextColumnId = nextMovableColumnIds[nextMovableIndex]
          nextMovableIndex += 1
          return nextColumnId
        })
      )
    },
    [table]
  )

  const moveColumn = React.useCallback(
    (columnId: string, direction: -1 | 1) => {
      const allLeafColumns = table.getAllLeafColumns()
      const movableColumnIds = allLeafColumns
        .filter((column) => column.getCanHide())
        .map((column) => column.id)
      const currentIndex = movableColumnIds.indexOf(columnId)
      const targetIndex = currentIndex + direction

      if (
        currentIndex === -1 ||
        targetIndex < 0 ||
        targetIndex >= movableColumnIds.length
      ) {
        return
      }

      const nextMovableColumnIds = [...movableColumnIds]
      ;[nextMovableColumnIds[currentIndex], nextMovableColumnIds[targetIndex]] =
        [nextMovableColumnIds[targetIndex], nextMovableColumnIds[currentIndex]]
      applyMovableColumnOrder(nextMovableColumnIds)
    },
    [applyMovableColumnOrder, table]
  )

  const resetColumnLayout = React.useCallback(() => {
    // Persist explicit visibility so reset survives initial hidden columns.
    const visibility = Object.fromEntries(
      table
        .getAllLeafColumns()
        .filter(column => column.getCanHide())
        .map(column => [column.id, true])
    )
    table.setColumnVisibility(visibility)
    table.setColumnOrder([])
    table.setColumnSizing({})
  }, [table])

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        render={
          <Button
            variant='outline'
            className='shrink-0'
            aria-label={t('View')}
          />
        }
      >
        {t('View')}
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align='end'
        className={cn(enableColumnReordering ? 'w-[224px]' : 'w-[150px]')}
      >
        <DropdownMenuGroup>
          <DropdownMenuLabel>{t('Toggle columns')}</DropdownMenuLabel>
          <Reorder.Group
            as='div'
            axis='y'
            values={movableColumnIds}
            onReorder={applyMovableColumnOrder}
          >
            {hideableColumns.map((column, index) => (
              <DataTableColumnOption
                key={column.id}
                column={column}
                index={index}
                count={hideableColumns.length}
                enableColumnReordering={enableColumnReordering}
                onMove={moveColumn}
              />
            ))}
          </Reorder.Group>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={resetColumnLayout}>
          <RotateCcw className='size-4' aria-hidden='true' />
          {t('Reset column layout')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
