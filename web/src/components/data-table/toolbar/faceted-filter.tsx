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
  labelNode?: React.ReactNode
  keywords?: string[]
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
  /** Place selected options at the top. Defaults to true. */
  selectedFirst?: boolean
  /** Extra controls rendered in the filter header bar (e.g. sort toggles). */
  headerExtra?: React.ReactNode
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
  selectedFirst = true,
  headerExtra,
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

  // 记录下拉弹窗打开瞬间的已选中集合，用于将选中的选项默认置顶，
  // 同时保证在弹窗打开期间选项位置保持稳定，避免点击时页面跳动
  const [pinnedSelectedValues, setPinnedSelectedValues] = React.useState<Set<string>>(() => {
    const raw = new Set(filterValue)
    raw.delete('all')
    return raw
  })
  const prevOpenRef = React.useRef(open)

  React.useEffect(() => {
    // 仅在弹窗由关闭切换到打开的一瞬间更新置顶集合
    if (open && !prevOpenRef.current) {
      const raw = new Set(filterValue)
      raw.delete('all')
      setPinnedSelectedValues(raw)
    }
    prevOpenRef.current = open
  }, [open])

  const sortedOptions = React.useMemo(() => {
    if (!selectedFirst || singleSelect || pinnedSelectedValues.size === 0) {
      return options
    }
    const selected: FacetedFilterOption[] = []
    const unselected: FacetedFilterOption[] = []

    for (const opt of options) {
      if (opt.value !== 'all' && pinnedSelectedValues.has(opt.value)) {
        selected.push(opt)
      } else {
        unselected.push(opt)
      }
    }
    return [...selected, ...unselected]
  }, [options, pinnedSelectedValues, selectedFirst, singleSelect])

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

  const [searchQuery, setSearchQuery] = React.useState('')

  const nonAllOptions = React.useMemo(() => {
    return sortedOptions.filter((opt) => opt.value !== 'all')
  }, [sortedOptions])

  const filteredOptions = React.useMemo(() => {
    const q = searchQuery.trim().toLowerCase()
    if (!q) return nonAllOptions
    return nonAllOptions.filter((opt) => {
      const labelMatch = opt.label?.toLowerCase().includes(q)
      const valMatch = opt.value?.toLowerCase().includes(q)
      const kwMatch = opt.keywords?.some((kw) => kw.toLowerCase().includes(q))
      return labelMatch || valMatch || Boolean(kwMatch)
    })
  }, [nonAllOptions, searchQuery])

  const allFilteredSelected =
    filteredOptions.length > 0 &&
    filteredOptions.every((opt) => selectedValues.has(opt.value))

  const handleToggleSelectFiltered = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    const nextSelected = new Set(selectedValues)
    if (allFilteredSelected) {
      filteredOptions.forEach((opt) => nextSelected.delete(opt.value))
    } else {
      filteredOptions.forEach((opt) => nextSelected.add(opt.value))
    }
    const result = Array.from(nextSelected)
    column?.setFilterValue(result.length ? result : undefined)
  }

  const groupedOptions = React.useMemo(() => {
    const hasAnyGroup = sortedOptions.some((opt) => Boolean(opt.group))
    if (!hasAnyGroup) {
      return [{ group: undefined, items: sortedOptions }]
    }
    const groups: { group?: string; items: FacetedFilterOption[] }[] = []
    for (const option of sortedOptions) {
      const current = groups[groups.length - 1]
      if (current && current.group === option.group) {
        current.items.push(option)
      } else {
        groups.push({ group: option.group, items: [option] })
      }
    }
    return groups
  }, [sortedOptions])

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
        keywords={option.keywords}
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
            'border-primary flex size-4 shrink-0 items-center justify-center rounded-sm border',
            isSelected
              ? 'bg-primary text-primary-foreground'
              : 'opacity-50 [&_svg]:invisible'
          )}
        >
          <CheckIcon className={cn('text-background h-4 w-4')} />
        </div>
        {icon}
        {option.labelNode ? (
          <div
            className={cn(
              'min-w-0 flex-1 leading-tight text-left',
              renderOptionActions && 'pe-1'
            )}
          >
            {option.labelNode}
          </div>
        ) : (
          <span
            className={cn(
              'min-w-0 flex-1 break-all whitespace-normal leading-tight text-left',
              renderOptionActions && 'pe-2'
            )}
            title={t(option.label)}
          >
            {t(option.label)}
          </span>
        )}
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
          className='w-max min-w-[340px] max-w-[min(680px,calc(100vw-2rem))] p-0'
          align='start'
        >
        <Command>
          <CommandInput
            placeholder={title}
            value={searchQuery}
            onValueChange={setSearchQuery}
          />
          {!singleSelect && (
            <div className='flex items-center justify-between border-b px-2.5 py-1.5 text-xs text-muted-foreground bg-muted/30'>
              <span className='truncate'>
                {searchQuery.trim()
                  ? `${t('Matched')}: ${filteredOptions.length}`
                  : `${t('Total')}: ${nonAllOptions.length}`}
                {selectedValues.size > 0 && (
                  <span className='ms-1 text-primary font-medium'>
                    ({t('Selected')}: {selectedValues.size})
                  </span>
                )}
              </span>
              <div className='flex items-center gap-1 shrink-0'>
                {headerExtra}
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  className='h-6 px-1.5 text-xs hover:text-foreground'
                  disabled={filteredOptions.length === 0}
                  onClick={handleToggleSelectFiltered}
                >
                  {allFilteredSelected
                    ? t('Deselect All')
                    : searchQuery.trim()
                    ? t('Select Matches')
                    : t('Select All')}
                </Button>
                {selectedValues.size > 0 && (
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    className='h-6 px-1.5 text-xs hover:text-foreground'
                    onClick={(e) => {
                      e.preventDefault()
                      e.stopPropagation()
                      handleClear()
                    }}
                  >
                    {t('Clear')}
                  </Button>
                )}
              </div>
            </div>
          )}
          <CommandList className='max-h-96'>
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
