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
import type { Column } from '@tanstack/react-table'
import { Check as CheckIcon, PlusCircle as PlusCircledIcon } from 'lucide-react'
import * as React from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'

export type FacetedFilterOption = {
  label: string
  value: string
  icon?: React.ComponentType<{ className?: string }>
  iconNode?: React.ReactNode
  count?: number
  group?: string
}

type DataTableFacetedFilterProps<TData, TValue> = {
  column?: Column<TData, TValue>
  title?: string
  options: FacetedFilterOption[]
  renderOptionActions?: (option: FacetedFilterOption) => React.ReactNode
  /** Enable single select mode (only one option can be selected at a time) */
  singleSelect?: boolean
  /** Invoked after this filter is cleared. */
  onClear?: () => void
  /** Enables mouse drag-and-drop reordering of options. */
  onOptionReorder?: (sourceValue: string, targetValue: string) => void
  /** Controlled open state for the popover */
  open?: boolean
  /** Callback fired when the open state changes */
  onOpenChange?: (open: boolean) => void
}

function DataTableFacetedFilterInner<TData, TValue>({
  column,
  title,
  options,
  singleSelect = false,
  onClear,
  renderOptionActions,
  onOptionReorder,
  open: controlledOpen,
  onOpenChange: controlledOnOpenChange,
}: DataTableFacetedFilterProps<TData, TValue>) {
  const { t } = useTranslation()
  const facets = column?.getFacetedUniqueValues()
  const filterValue = column?.getFilterValue() as string[] | undefined
  const selectedValues = React.useMemo(() => {
    const raw = new Set(filterValue)
    raw.delete('all')
    return raw
  }, [filterValue])
  const draggedOption = React.useRef<string | null>(null)

  const [uncontrolledOpen, setUncontrolledOpen] = React.useState(false)
  const isControlled = controlledOpen !== undefined
  const open = isControlled ? controlledOpen : uncontrolledOpen
  const onOpenChange = React.useCallback(
    (nextOpen: boolean) => {
      if (!isControlled) {
        setUncontrolledOpen(nextOpen)
      }
      controlledOnOpenChange?.(nextOpen)
    },
    [isControlled, controlledOnOpenChange]
  )

  const handleOptionSelect = (optionValue: string) => {
    if (optionValue === 'all') {
      column?.setFilterValue(undefined)
      onClear?.()
      if (singleSelect) {
        onOpenChange(false)
      }
      return
    }

    const nextSelectedValues = getNextSelectedValues(
      selectedValues,
      optionValue,
      singleSelect
    )

    column?.setFilterValue(
      nextSelectedValues.length ? nextSelectedValues : undefined
    )
    if (singleSelect) {
      onOpenChange(false)
    }
  }

  const handleClear = () => {
    column?.setFilterValue(undefined)
    onClear?.()
    onOpenChange(false)
  }

  const groupedOptions = React.useMemo(() => {
    const hasAnyGroup = options.some((opt) => Boolean(opt.group))
    if (!hasAnyGroup) {
      return [{ group: undefined, items: options }]
    }
    const groups: { group?: string; items: FacetedFilterOption[] }[] = []
    for (const option of options) {
      const current = groups[groups.length - 1]
      if (current && current.group === option.group) {
        current.items.push(option)
      } else {
        groups.push({ group: option.group, items: [option] })
      }
    }
    return groups
  }, [options])

  const renderItem = (option: FacetedFilterOption) => {
    const isSelected =
      selectedValues.has(option.value) ||
      (option.value === 'all' && selectedValues.size === 0)
    const facetCount = facets?.get(option.value)
    let icon: React.ReactNode = null
    if (option.iconNode) {
      icon = (
        <span className='text-muted-foreground flex size-4 items-center justify-center'>
          {option.iconNode}
        </span>
      )
    } else if (option.icon) {
      icon = (
        <option.icon className='text-muted-foreground size-4' />
      )
    }

    let count: React.ReactNode = null
    if (typeof option.count === 'number') {
      count = (
        <span className='text-muted-foreground ms-auto flex h-4 min-w-4 items-center justify-center font-mono text-xs'>
          {option.count}
        </span>
      )
    } else if (facetCount) {
      count = (
        <span className='ms-auto flex h-4 w-4 items-center justify-center font-mono text-xs'>
          {facetCount}
        </span>
      )
    }

    return (
      <CommandItem
        key={option.value}
        draggable={Boolean(onOptionReorder) && option.value !== 'all'}
        onDragStart={() => {
          draggedOption.current = option.value
        }}
        onDragOver={(event) => {
          if (onOptionReorder && draggedOption.current) {
            event.preventDefault()
          }
        }}
        onDrop={(event) => {
          event.preventDefault()
          if (onOptionReorder && draggedOption.current) {
            onOptionReorder(draggedOption.current, option.value)
          }
          draggedOption.current = null
        }}
        onDragEnd={() => {
          draggedOption.current = null
        }}
        className={cn(
          'min-w-0',
          renderOptionActions && '[&>svg:last-child]:hidden'
        )}
        onSelect={() => handleOptionSelect(option.value)}
      >
        <div
          className={cn(
            'border-primary flex size-4 items-center justify-center rounded-sm border',
            isSelected
              ? 'bg-primary text-primary-foreground'
              : 'opacity-50 [&_svg]:invisible'
          )}
        >
          <CheckIcon className={cn('text-background h-4 w-4')} />
        </div>
        {icon}
        <span
          className={cn(
            'min-w-0 truncate whitespace-nowrap',
            renderOptionActions
              ? 'max-w-[min(280px,calc(100vw-7rem))] flex-none'
              : 'flex-1'
          )}
          title={t(option.label)}
        >
          {t(option.label)}
        </span>
        {count}
        {renderOptionActions ? (
          <span className='ms-auto flex shrink-0 items-center gap-1'>
            {renderOptionActions(option)}
          </span>
        ) : null}
      </CommandItem>
    )
  }

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger
        render={
          <Button variant='outline' size='sm' className='h-8 border-dashed' />
        }
      >
        <PlusCircledIcon className='size-4' />
        {title}
        {selectedValues?.size > 0 && (
          <>
            <Separator orientation='vertical' className='mx-2 h-4' />
            <Badge
              variant='secondary'
              className='rounded-sm px-1 font-normal lg:hidden'
            >
              {selectedValues.size}
            </Badge>
            <div className='hidden space-x-1 lg:flex'>
              {selectedValues.size > 2 ? (
                <Badge
                  variant='secondary'
                  className='rounded-sm px-1 font-normal'
                >
                  {selectedValues.size} {t('selected')}
                </Badge>
              ) : (
                options
                  .filter((option) => selectedValues.has(option.value))
                  .map((option) => (
                    <Badge
                      variant='secondary'
                      key={option.value}
                      className='rounded-sm px-1 font-normal'
                    >
                      {t(option.label)}
                    </Badge>
                  ))
              )}
            </div>
          </>
        )}
      </PopoverTrigger>
      {open && (
        <PopoverContent
          className='w-max min-w-[260px] max-w-[min(420px,calc(100vw-2rem))] p-0'
          align='start'
        >
        <Command>
          <CommandInput placeholder={title} />
          <CommandList>
            <CommandEmpty>{t('No results found.')}</CommandEmpty>
            {groupedOptions.map((g, idx) => (
              <React.Fragment key={g.group ?? `group-${idx}`}>
                {idx > 0 && <CommandSeparator />}
                <CommandGroup heading={g.group ? t(g.group) : undefined}>
                  {g.items.map(renderItem)}
                </CommandGroup>
              </React.Fragment>
            ))}
            {selectedValues.size > 0 && (
              <>
                <CommandSeparator />
                <CommandGroup>
                  <CommandItem
                    onSelect={handleClear}
                    className='justify-center text-center'
                  >
                    {t('Clear filters')}
                  </CommandItem>
                </CommandGroup>
              </>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
      )}
    </Popover>
  )
}

export const DataTableFacetedFilter = DataTableFacetedFilterInner

function getNextSelectedValues(
  selectedValues: Set<string>,
  optionValue: string,
  singleSelect: boolean
): string[] {
  if (singleSelect) {
    return selectedValues.has(optionValue) ? [] : [optionValue]
  }

  const nextSelectedValues = new Set(selectedValues)
  if (nextSelectedValues.has(optionValue)) {
    nextSelectedValues.delete(optionValue)
  } else {
    nextSelectedValues.add(optionValue)
  }

  return [...nextSelectedValues]
}
