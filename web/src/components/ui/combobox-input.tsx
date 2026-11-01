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
import { Check, ChevronsUpDown } from 'lucide-react'
import * as React from 'react'
import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

export type ComboboxInputOption = {
  value: string
  label: string
  labelNode?: React.ReactNode
  icon?: React.ReactNode
  disabled?: boolean
  description?: React.ReactNode
}

interface ComboboxInputProps {
  options: readonly ComboboxInputOption[]
  value?: string
  onValueChange: (value: string) => void
  placeholder?: string
  emptyText?: string
  className?: string
  id?: string
  allowCustomValue?: boolean
  openOnFocus?: boolean
  suspended?: boolean
  onKeyDown?: React.KeyboardEventHandler<HTMLInputElement>
  renderOptionHoverPanel?: (
    option: ComboboxInputOption,
    closeDropdown: () => void
  ) => React.ReactNode
  'aria-label'?: string
  'aria-labelledby'?: string
  'aria-invalid'?: React.AriaAttributes['aria-invalid']
}

export function ComboboxInput({
  options,
  value = '',
  onValueChange,
  placeholder = 'Select or type...',
  emptyText = 'No option found.',
  className,
  id,
  allowCustomValue = false,
  openOnFocus = true,
  suspended = false,
  onKeyDown,
  renderOptionHoverPanel,
  'aria-label': ariaLabel,
  'aria-labelledby': ariaLabelledBy,
  'aria-invalid': ariaInvalid,
}: ComboboxInputProps) {
  const { t } = useTranslation()
  const listId = React.useId()
  const [open, setOpen] = React.useState(false)
  const [searchValue, setSearchValue] = React.useState('')
  const [searchChanged, setSearchChanged] = React.useState(false)
  const [highlightedIndex, setHighlightedIndex] = React.useState(-1)
  const [hoverSide, setHoverSide] = React.useState<'left' | 'right'>('left')
  const containerRef = React.useRef<HTMLDivElement>(null)
  const hoverPanelRef = React.useRef<HTMLDivElement>(null)
  const inputRef = React.useRef<HTMLInputElement>(null)
  const listRef = React.useRef<HTMLUListElement>(null)
  const pointerFocusRef = React.useRef(false)
  const pointerDownInsideRef = React.useRef(false)
  const isHoveringPanelRef = React.useRef(false)
  const hoverIntentTimerRef = React.useRef<number | null>(null)
  const highlightedValueRef = React.useRef<string | null>(null)
  const prevSuspendedRef = React.useRef(false)
  const lastResumedAtRef = React.useRef(0)
  const selectedOption = React.useMemo(
    () => (value ? options.find((option) => option.value === value) : undefined),
    [options, value]
  )
  const displayValue = open ? searchValue : (selectedOption?.label ?? value)

  const filteredOptions = React.useMemo(() => {
    if (!searchChanged || !searchValue.trim()) return options
    const search = searchValue.toLowerCase().trim()
    return options.filter(
      (option) =>
        option.label.toLowerCase().includes(search) ||
        option.value.toLowerCase().includes(search)
    )
  }, [options, searchValue, searchChanged])

  // Preserve highlighted option across background options refreshes; only reset on search text change
  React.useEffect(() => {
    setHighlightedIndex(-1)
    highlightedValueRef.current = null
  }, [searchValue])

  React.useEffect(() => {
    if (highlightedValueRef.current == null) return
    const nextIdx = filteredOptions.findIndex(
      (opt) => opt.value === highlightedValueRef.current
    )
    if (nextIdx >= 0 && nextIdx !== highlightedIndex) {
      setHighlightedIndex(nextIdx)
    }
  }, [filteredOptions, highlightedIndex])

  React.useEffect(() => {
    return () => {
      if (hoverIntentTimerRef.current !== null) {
        window.clearTimeout(hoverIntentTimerRef.current)
      }
    }
  }, [])

  React.useEffect(() => {
    if (!open || !renderOptionHoverPanel || !containerRef.current) return
    const rect = containerRef.current.getBoundingClientRect()
    const dropdownRight = rect.left + Math.max(rect.width, 460)
    const spaceRight = window.innerWidth - dropdownRight
    const spaceLeft = rect.left
    setHoverSide(spaceRight >= 390 || spaceRight >= spaceLeft ? 'right' : 'left')
  }, [open, renderOptionHoverPanel])

  // Restore open and highlighted option when returning from a suspended child dialog
  React.useEffect(() => {
    if (prevSuspendedRef.current && !suspended) {
      lastResumedAtRef.current = Date.now()
      setOpen(true)
      if (highlightedValueRef.current != null) {
        const idx = filteredOptions.findIndex(
          (opt) => opt.value === highlightedValueRef.current
        )
        if (idx >= 0) {
          setHighlightedIndex(idx)
        }
      }
    }
    prevSuspendedRef.current = Boolean(suspended)
  }, [suspended, filteredOptions])

  // Handle click outside to close
  React.useEffect(() => {
    if (!open || suspended) return

    const handleClickOutside = (e: MouseEvent) => {
      if (Date.now() - lastResumedAtRef.current < 200) {
        return
      }
      const targetEl = e.target as Element | null
      if (
        targetEl?.closest?.(
          '[data-slot="dialog-overlay"], [data-slot="dialog-content"], [role="dialog"]'
        )
      ) {
        return
      }
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setOpen(false)
        setSearchValue('')
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [open, suspended])

  const handleSelect = (selectedValue: string) => {
    onValueChange(selectedValue)
    setOpen(false)
    setSearchValue('')
    inputRef.current?.focus()
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (!open && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      e.preventDefault()
      setSearchValue(allowCustomValue ? value : '')
      setSearchChanged(false)
      setOpen(true)
      return
    }

    if (!open) return

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        setHighlightedIndex((prev) =>
          prev < filteredOptions.length - 1 ? prev + 1 : 0
        )
        break
      case 'ArrowUp':
        e.preventDefault()
        setHighlightedIndex((prev) =>
          prev > 0 ? prev - 1 : filteredOptions.length - 1
        )
        break
      case 'Enter':
        if (highlightedIndex >= 0 && filteredOptions[highlightedIndex]) {
          e.preventDefault()
          handleSelect(filteredOptions[highlightedIndex].value)
        } else if (allowCustomValue && searchValue.trim()) {
          e.preventDefault()
          handleSelect(searchValue.trim())
        } else {
          if (!onKeyDown) e.preventDefault()
          // No highlighted option, just close the dropdown and keep current value
          setOpen(false)
          setSearchValue('')
        }
        break
      case 'Escape':
        if (suspended) break
        e.preventDefault()
        e.stopPropagation()
        setOpen(false)
        setSearchValue('')
        break
    }
  }

  // Scroll highlighted item into view
  React.useEffect(() => {
    if (highlightedIndex < 0 || !listRef.current) return
    const item = listRef.current.children[highlightedIndex] as HTMLElement
    item?.scrollIntoView({ block: 'nearest' })
  }, [highlightedIndex])

  const showDropdown =
    open &&
    (filteredOptions.length > 0 || (allowCustomValue && searchValue.trim()))

  return (
    <div
      ref={containerRef}
      className='relative'
      onMouseDownCapture={() => {
        pointerDownInsideRef.current = true
        const resetPointerDown = () => {
          window.setTimeout(() => {
            pointerDownInsideRef.current = false
          }, 50)
        }
        window.addEventListener('mouseup', resetPointerDown, { once: true })
      }}
    >
      <Input
        ref={inputRef}
        id={id}
        type='text'
        role='combobox'
        aria-label={ariaLabel}
        aria-labelledby={ariaLabelledBy}
        aria-invalid={ariaInvalid}
        aria-expanded={!!showDropdown}
        aria-controls={
          showDropdown && filteredOptions.length > 0 ? listId : undefined
        }
        aria-activedescendant={
          showDropdown && highlightedIndex >= 0
            ? `${listId}-${highlightedIndex}`
            : undefined
        }
        aria-haspopup='listbox'
        aria-autocomplete='list'
        autoComplete='off'
        placeholder={placeholder}
        value={displayValue}
        onChange={(e) => {
          const nextValue = e.target.value
          setSearchValue(nextValue)
          setSearchChanged(true)
          if (allowCustomValue) {
            onValueChange(nextValue)
          }
          if (!open) setOpen(true)
        }}
        onPointerDown={() => {
          pointerFocusRef.current = true
          if (document.activeElement === inputRef.current && !open) {
            setSearchValue(allowCustomValue ? value : '')
            setSearchChanged(false)
            setOpen(true)
          }
        }}
        onFocus={() => {
          setSearchValue(allowCustomValue ? value : '')
          setSearchChanged(false)
          if (openOnFocus || pointerFocusRef.current) {
            setOpen(true)
          }
          pointerFocusRef.current = false
        }}
        onBlur={(e) => {
          if (
            suspended ||
            Date.now() - lastResumedAtRef.current < 200 ||
            pointerDownInsideRef.current ||
            isHoveringPanelRef.current
          ) {
            return
          }
          if (
            e.relatedTarget &&
            containerRef.current?.contains(e.relatedTarget as Node)
          ) {
            return
          }
          setOpen(false)
          setSearchValue('')
        }}
        onKeyDown={(event) => {
          handleKeyDown(event)
          if (!event.defaultPrevented) onKeyDown?.(event)
        }}
        className={cn('pr-9', className)}
      />
      <ChevronsUpDown className='pointer-events-none absolute top-1/2 right-3 size-4 shrink-0 -translate-y-1/2 opacity-50' />

      {showDropdown && (
        <div
          className={cn(
            'bg-popover text-popover-foreground absolute top-full mt-1 min-w-full w-max max-w-[min(680px,95vw)] rounded-md border shadow-md',
            suspended ? 'z-40 pointer-events-none' : 'z-100'
          )}
        >
          {filteredOptions.length > 0 ? (
            <ul
              ref={listRef}
              id={listId}
              role='listbox'
              className='max-h-[340px] overflow-y-auto p-1.5 space-y-1'
            >
              {filteredOptions.map((option, index) => {
                const isSelected =
                  value === option.value || (!value && !option.value)
                return (
                  <li
                    key={option.value}
                    id={`${listId}-${index}`}
                    role='option'
                    aria-selected={isSelected}
                    data-highlighted={index === highlightedIndex}
                    className={cn(
                      'relative flex cursor-pointer items-center justify-between gap-2.5 rounded-sm px-2 py-1 text-sm select-none',
                      index === highlightedIndex &&
                        'bg-accent/70 text-accent-foreground',
                      isSelected && 'font-medium'
                    )}
                    onMouseEnter={() => {
                      if (hoverIntentTimerRef.current !== null) {
                        window.clearTimeout(hoverIntentTimerRef.current)
                        hoverIntentTimerRef.current = null
                      }
                      const hasActiveHoverPanel =
                        Boolean(renderOptionHoverPanel) &&
                        highlightedIndex >= 0 &&
                        Boolean(filteredOptions[highlightedIndex]?.value)
                      const isPanelFocusedOrSelected =
                        Boolean(
                          hoverPanelRef.current &&
                            document.activeElement &&
                            hoverPanelRef.current.contains(document.activeElement)
                        ) ||
                        Boolean(
                          hoverPanelRef.current &&
                            window.getSelection()?.anchorNode &&
                            hoverPanelRef.current.contains(
                              window.getSelection()!.anchorNode
                            ) &&
                            window.getSelection()?.toString()
                        )
                      if (isPanelFocusedOrSelected) {
                        return
                      }
                      if (hasActiveHoverPanel && index !== highlightedIndex) {
                        hoverIntentTimerRef.current = window.setTimeout(() => {
                          if (!isHoveringPanelRef.current) {
                            highlightedValueRef.current = option.value
                            setHighlightedIndex(index)
                          }
                          hoverIntentTimerRef.current = null
                        }, 180)
                      } else {
                        highlightedValueRef.current = option.value
                        setHighlightedIndex(index)
                      }
                    }}
                    onMouseDown={(e) => {
                      e.preventDefault() // Prevent blur
                      handleSelect(option.value)
                    }}
                  >
                    {option.labelNode ? (
                      <>
                        <div
                          className={cn(
                            'border-primary flex size-4 shrink-0 items-center justify-center rounded-sm border',
                            isSelected
                              ? 'bg-primary text-primary-foreground'
                              : 'opacity-50 [&_svg]:invisible'
                          )}
                        >
                          <Check className='text-background size-3.5' />
                        </div>
                        <div className='w-full min-w-0 flex-1 leading-tight text-left'>
                          {option.labelNode}
                        </div>
                      </>
                    ) : (
                      <>
                        <div className='flex items-center gap-2 min-w-0 flex-1'>
                          <Check
                            className={cn(
                              'size-4 shrink-0',
                              isSelected ? 'opacity-100' : 'opacity-0'
                            )}
                          />
                          {option.icon && <span aria-hidden>{option.icon}</span>}
                          <span className='truncate font-mono text-xs'>{option.label}</span>
                        </div>
                        {option.description && (
                          <span
                            className={cn(
                              'shrink-0 text-xs font-mono',
                              option.description === '$0' ||
                                option.description === 'MPM $0 · RPM 0'
                                ? 'text-muted-foreground font-normal'
                                : 'font-medium text-emerald-600 dark:text-emerald-400'
                            )}
                          >
                            {option.description}
                          </span>
                        )}
                      </>
                    )}
                  </li>
                )
              })}
            </ul>
          ) : (
            <div className='px-2 py-6 text-center text-sm'>
              {t(emptyText)}
              {allowCustomValue && searchValue.trim() && (
                <div className='text-muted-foreground mt-1 text-xs'>
                  {t('Press Enter to use "{{value}}"', {
                    value: searchValue.trim(),
                  })}
                </div>
              )}
            </div>
          )}

          {renderOptionHoverPanel &&
            highlightedIndex >= 0 &&
            filteredOptions[highlightedIndex] &&
            (() => {
              const panel = renderOptionHoverPanel(
                filteredOptions[highlightedIndex],
                () => {
                  setOpen(false)
                  setSearchValue('')
                }
              )
              if (!panel) return null
              return (
                <div
                  ref={hoverPanelRef}
                  className={cn(
                    "bg-popover text-popover-foreground select-text absolute top-0 w-[456px] max-w-[92vw] rounded-xl border shadow-xl before:content-[''] before:absolute before:inset-y-0 before:w-3",
                    suspended ? 'z-40 pointer-events-none' : 'z-110',
                    hoverSide === 'right'
                      ? 'left-full ml-1.5 before:-left-3'
                      : 'right-full mr-1.5 before:-right-3'
                  )}
                  onMouseEnter={() => {
                    isHoveringPanelRef.current = true
                    if (hoverIntentTimerRef.current !== null) {
                      window.clearTimeout(hoverIntentTimerRef.current)
                      hoverIntentTimerRef.current = null
                    }
                  }}
                  onMouseLeave={() => {
                    isHoveringPanelRef.current = false
                  }}
                  onMouseDown={(e) => {
                    e.stopPropagation()
                  }}
                  onClick={(e) => {
                    e.stopPropagation()
                  }}
                >
                  {panel}
                </div>
              )
            })()}
        </div>
      )}
    </div>
  )
}
