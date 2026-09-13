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
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { DataTableFacetedFilter } from '../faceted-filter'

describe('DataTableFacetedFilter', () => {
  test('clears the table value and runs the linked clear handler', () => {
    const setFilterValue = vi.fn()
    const onClear = vi.fn()
    const onOpenChange = vi.fn()
    const column = {
      getFacetedUniqueValues: () => new Map<string, number>(),
      getFilterValue: () => ['ccmax'],
      setFilterValue,
    } as unknown as Column<unknown, unknown>

    render(
      <DataTableFacetedFilter
        column={column}
        title='Group'
        options={[{ label: 'CCMAX', value: 'ccmax' }]}
        onClear={onClear}
        onOpenChange={onOpenChange}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: /Group/ }))
    fireEvent.click(screen.getByText('Clear filters'))

    expect(setFilterValue).toHaveBeenCalledWith(undefined)
    expect(onClear).toHaveBeenCalledOnce()
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  test('calls onOpenChange(false) when an option is selected in singleSelect mode', () => {
    const setFilterValue = vi.fn()
    const onOpenChange = vi.fn()
    const column = {
      getFacetedUniqueValues: () => new Map<string, number>(),
      getFilterValue: () => undefined,
      setFilterValue,
    } as unknown as Column<unknown, unknown>

    render(
      <DataTableFacetedFilter
        column={column}
        title='Status'
        open
        onOpenChange={onOpenChange}
        singleSelect
        options={[
          { label: 'Enabled', value: 'enabled' },
          { label: 'Disabled', value: 'disabled' },
        ]}
      />
    )

    fireEvent.click(screen.getByText('Enabled'))
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  test('renders grouped options with headings and clears on "all" selection', () => {
    const setFilterValue = vi.fn()
    const onClear = vi.fn()
    const onOpenChange = vi.fn()
    const column = {
      getFacetedUniqueValues: () => new Map<string, number>(),
      getFilterValue: () => ['family:Claude'],
      setFilterValue,
    } as unknown as Column<unknown, unknown>

    render(
      <DataTableFacetedFilter
        column={column}
        title='Type'
        open
        onOpenChange={onOpenChange}
        onClear={onClear}
        singleSelect
        options={[
          { label: 'All Types', value: 'all' },
          { label: 'Claude', value: 'family:Claude', group: 'Model Family' },
          { label: 'OpenAI', value: 'family:OpenAI', group: 'Model Family' },
          { label: 'Anthropic', value: 'protocol:14', group: 'Provider Protocol' },
        ]}
      />
    )

    // Heading groups are rendered
    expect(screen.getByText('Model Family')).toBeTruthy()
    expect(screen.getByText('Provider Protocol')).toBeTruthy()
    expect(screen.getAllByText('Claude').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('Anthropic')).toBeTruthy()

    // Selecting 'All Types' clears the filter
    fireEvent.click(screen.getByText('All Types'))
    expect(setFilterValue).toHaveBeenCalledWith(undefined)
    expect(onClear).toHaveBeenCalledOnce()
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})

describe('DataTableToolbar mutual exclusive filter dropdowns', () => {
  test('only allows opening one faceted filter at a time', async () => {
    const { DataTableToolbar } = await import('../toolbar')
    const mockTable = {
      getState: () => ({ columnFilters: [], globalFilter: '' }),
      getColumn: () => undefined,
      setColumnFilters: vi.fn(),
      setGlobalFilter: vi.fn(),
      resetColumnFilters: vi.fn(),
      getAllLeafColumns: () => [],
    } as unknown as import('@tanstack/react-table').Table<unknown>

    render(
      <DataTableToolbar
        table={mockTable}
        filters={[
          {
            columnId: 'status',
            title: 'Status',
            options: [
              { label: 'Status Option 1', value: 's1' },
              { label: 'Status Option 2', value: 's2' },
            ],
          },
          {
            columnId: 'type',
            title: 'Type',
            options: [
              { label: 'Type Option 1', value: 't1' },
              { label: 'Type Option 2', value: 't2' },
            ],
          },
        ]}
      />
    )

    // 初始状态：两个选择框的内容均未显示
    expect(screen.queryByText('Status Option 1')).toBeNull()
    expect(screen.queryByText('Type Option 1')).toBeNull()

    // 1. 打开 Status 选择栏
    fireEvent.click(screen.getByRole('button', { name: /Status/ }))
    expect(screen.getByText('Status Option 1')).toBeTruthy()
    expect(screen.queryByText('Type Option 1')).toBeNull()

    // 2. 打开 Type 选择栏：Status 必须自动关闭，只展示 Type
    fireEvent.click(screen.getByRole('button', { name: /Type/ }))
    expect(screen.getByText('Type Option 1')).toBeTruthy()
    expect(screen.queryByText('Status Option 1')).toBeNull()

    // 3. 再次点击 Type 按钮：Type 也关闭
    fireEvent.click(screen.getByRole('button', { name: /Type/ }))
    expect(screen.queryByText('Type Option 1')).toBeNull()
    expect(screen.queryByText('Status Option 1')).toBeNull()
  })
})
