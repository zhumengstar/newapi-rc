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
import type { Column, Table } from '@tanstack/react-table'
import { fireEvent, render, screen } from '@testing-library/react'
import { useMemo, useState } from 'react'
import { describe, expect, test, vi } from 'vitest'

import { DataTableViewOptions } from '../view-options'

function createColumn(
  id: string,
  label: string,
  canHide: boolean
): Column<unknown, unknown> {
  return {
    id,
    columnDef: {
      header: label,
    },
    getCanHide: () => canHide,
    getIsVisible: () => true,
    toggleVisibility: vi.fn(),
  } as unknown as Column<unknown, unknown>
}

function ControlledViewOptions({
  onColumnOrderChange,
}: {
  onColumnOrderChange: (columnOrder: string[]) => void
}) {
  const [columnOrder, setColumnOrder] = useState([
    'select',
    'name',
    'ratio',
    'actions',
  ])
  const columnsById = useMemo(
    () =>
      new Map(
        [
          createColumn('select', 'Select', false),
          createColumn('name', 'Name', true),
          createColumn('ratio', 'Ratio', true),
          createColumn('actions', 'Actions', false),
        ].map((column) => [column.id, column])
      ),
    []
  )
  const columns = columnOrder.map((columnId) => {
    const column = columnsById.get(columnId)
    if (!column) {
      throw new Error(`Missing test column: ${columnId}`)
    }
    return column
  })
  const table = {
    getAllLeafColumns: () => columns,
    setColumnOrder: (nextColumnOrder: string[]) => {
      onColumnOrderChange(nextColumnOrder)
      setColumnOrder(nextColumnOrder)
    },
    setColumnSizing: vi.fn(),
    setColumnVisibility: vi.fn(),
  } as unknown as Table<unknown>

  return <DataTableViewOptions table={table} enableColumnReordering />
}

describe('DataTableViewOptions', () => {
  test('moves only hideable columns while keeping fixed columns in place', async () => {
    const setColumnOrder = vi.fn()

    render(<ControlledViewOptions onColumnOrderChange={setColumnOrder} />)

    fireEvent.click(screen.getByRole('button', { name: 'View' }))
    fireEvent.click(
      await screen.findByRole('button', { name: 'Move Name down' })
    )

    expect(setColumnOrder).toHaveBeenCalledWith([
      'select',
      'ratio',
      'name',
      'actions',
    ])
    expect(
      screen
        .getAllByRole('menuitemcheckbox')
        .map((menuItem) => menuItem.textContent)
    ).toEqual(['Ratio', 'Name'])
    expect(
      screen.getByRole('button', { name: 'Move Name up' })
    ).not.toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Move Name down' })
    ).toBeDisabled()
  })

  test('provides a touch drag handle with keyboard reordering', async () => {
    const setColumnOrder = vi.fn()
    const columns = [
      createColumn('select', 'Select', false),
      createColumn('name', 'Name', true),
      createColumn('ratio', 'Ratio', true),
      createColumn('actions', 'Actions', false),
    ]
    const table = {
      getAllLeafColumns: () => columns,
      setColumnOrder,
    } as unknown as Table<unknown>

    render(<DataTableViewOptions table={table} enableColumnReordering />)

    fireEvent.click(screen.getByRole('button', { name: 'View' }))
    const dragHandle = await screen.findByRole('button', {
      name: 'Drag Name to reorder',
    })

    expect(dragHandle).toHaveClass('cursor-grab', 'touch-none')
    fireEvent.keyDown(dragHandle, { key: 'ArrowDown' })

    expect(setColumnOrder).toHaveBeenCalledWith([
      'select',
      'ratio',
      'name',
      'actions',
    ])
  })

  test('resets visibility, order, and sizing in one action', async () => {
    const columns = [
      createColumn('name', 'Name', true),
      createColumn('ratio', 'Ratio', true),
    ]
    const table = {
      getAllLeafColumns: () => columns,
      setColumnOrder: vi.fn(),
      setColumnSizing: vi.fn(),
      setColumnVisibility: vi.fn(),
    } as unknown as Table<unknown>

    render(<DataTableViewOptions table={table} enableColumnReordering />)

    fireEvent.click(screen.getByRole('button', { name: 'View' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Reset column layout' }))

    expect(table.setColumnVisibility).toHaveBeenCalledWith({
      name: true,
      ratio: true,
    })
    expect(table.setColumnOrder).toHaveBeenCalledWith([])
    expect(table.setColumnSizing).toHaveBeenCalledWith({})
  })
})
