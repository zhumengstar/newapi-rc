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
import { type Column } from '@tanstack/react-table'
import {
  ArrowDown as ArrowDownIcon,
  ArrowUp as ArrowUpIcon,
  ChevronDown as ChevronDownIcon,
  ChevronsUpDown as CaretSortIcon,
  EyeOff as EyeNoneIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

type DataTableColumnHeaderProps<TData, TValue> =
  React.HTMLAttributes<HTMLDivElement> & {
    column: Column<TData, TValue>
    title: React.ReactNode
  }

export function DataTableColumnHeader<TData, TValue>({
  column,
  title,
  className,
}: DataTableColumnHeaderProps<TData, TValue>) {
  const { t } = useTranslation()
  if (!column.getCanSort()) {
    return <div className={cn(className)}>{title}</div>
  }

  const sortDescFirst = Boolean(column.columnDef.sortDescFirst)
  const isSorted = column.getIsSorted()

  const handleToggleSort = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (!isSorted) {
      column.toggleSorting(sortDescFirst)
    } else if (sortDescFirst) {
      if (isSorted === 'desc') {
        column.toggleSorting(false)
      } else {
        column.clearSorting()
      }
    } else {
      if (isSorted === 'asc') {
        column.toggleSorting(true)
      } else {
        column.clearSorting()
      }
    }
  }

  return (
    <div className={cn('flex items-center space-x-0.5', className)}>
      <Button
        variant='ghost'
        size='sm'
        className='-ms-3 h-8 px-2 font-medium hover:bg-accent'
        onClick={handleToggleSort}
        title={t('Click to sort')}
      >
        <span>{title}</span>
        {isSorted === 'desc' ? (
          <ArrowDownIcon className='ms-1.5 h-4 w-4 text-primary' />
        ) : isSorted === 'asc' ? (
          <ArrowUpIcon className='ms-1.5 h-4 w-4 text-primary' />
        ) : (
          <CaretSortIcon className='ms-1.5 h-4 w-4 text-muted-foreground/70' />
        )}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant='ghost'
              size='sm'
              className='data-popup-open:bg-accent h-8 w-4 p-0 opacity-40 hover:opacity-100 hover:bg-accent'
              aria-label={t('Sort options')}
            />
          }
        >
          <ChevronDownIcon className='h-3 w-3 text-muted-foreground' />
        </DropdownMenuTrigger>
        <DropdownMenuContent align='start'>
          {sortDescFirst ? (
            <>
              <DropdownMenuItem onClick={() => column.toggleSorting(true)}>
                <ArrowDownIcon className='text-muted-foreground/70 size-3.5' />
                {t('Desc')}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => column.toggleSorting(false)}>
                <ArrowUpIcon className='text-muted-foreground/70 size-3.5' />
                {t('Asc')}
              </DropdownMenuItem>
            </>
          ) : (
            <>
              <DropdownMenuItem onClick={() => column.toggleSorting(false)}>
                <ArrowUpIcon className='text-muted-foreground/70 size-3.5' />
                {t('Asc')}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => column.toggleSorting(true)}>
                <ArrowDownIcon className='text-muted-foreground/70 size-3.5' />
                {t('Desc')}
              </DropdownMenuItem>
            </>
          )}
          {Boolean(isSorted) && (
            <DropdownMenuItem onClick={() => column.clearSorting()}>
              <CaretSortIcon className='text-muted-foreground/70 size-3.5' />
              {t('Reset')}
            </DropdownMenuItem>
          )}
          {column.getCanHide() && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => column.toggleVisibility(false)}>
                <EyeNoneIcon className='text-muted-foreground/70 size-3.5' />
                {t('Hide')}
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
