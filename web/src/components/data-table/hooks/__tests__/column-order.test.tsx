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
import type { ColumnDef } from '@tanstack/react-table'
import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import { useDataTable } from '../use-data-table'

type Row = {
  name: string
  ratio: string
  balance: string
}

const data: Row[] = [{ name: 'Channel A', ratio: '1x', balance: '100' }]
const columns: ColumnDef<Row, unknown>[] = [
  {
    id: 'select',
    header: 'Select',
    enableHiding: false,
    cell: () => null,
  },
  {
    accessorKey: 'name',
    header: 'Name',
  },
  {
    accessorKey: 'ratio',
    header: 'Ratio',
  },
  {
    accessorKey: 'balance',
    header: 'Balance',
  },
  {
    id: 'actions',
    header: 'Actions',
    enableHiding: false,
    cell: () => null,
  },
]

const originalLocalStorageDescriptor = Object.getOwnPropertyDescriptor(
  window,
  'localStorage'
)

function createStorage(): Storage {
  const values = new Map<string, string>()

  return {
    get length() {
      return values.size
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => {
      values.delete(key)
    },
    setItem: (key, value) => {
      values.set(key, value)
    },
  }
}

function useTestTable(storageKey: string) {
  return useDataTable({
    data,
    columns,
    columnOrderStorageKey: storageKey,
    fixedColumnOrder: {
      start: ['select'],
      end: ['actions'],
    },
  })
}

describe('useDataTable column order', () => {
  beforeEach(() => {
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      value: createStorage(),
    })
  })

  afterEach(() => {
    window.localStorage.clear()
    if (originalLocalStorageDescriptor) {
      Object.defineProperty(
        window,
        'localStorage',
        originalLocalStorageDescriptor
      )
      return
    }

    Reflect.deleteProperty(window, 'localStorage')
  })

  test('filters saved invalid ids, appends new columns, and preserves fixed columns', async () => {
    const storageKey = 'data-table-column-order-test:v1'
    window.localStorage.setItem(
      storageKey,
      JSON.stringify([
        'balance',
        'missing',
        'name',
        'balance',
        'actions',
        'select',
      ])
    )

    const { result } = renderHook(() => useTestTable(storageKey))

    expect(result.current.table.getState().columnOrder).toEqual([
      'select',
      'balance',
      'name',
      'ratio',
      'actions',
    ])
    expect(
      result.current.table.getAllLeafColumns().map((column) => column.id)
    ).toEqual(['select', 'balance', 'name', 'ratio', 'actions'])

    await waitFor(() => {
      expect(
        JSON.parse(window.localStorage.getItem(storageKey) ?? '[]')
      ).toEqual(['select', 'balance', 'name', 'ratio', 'actions'])
    })
  })

  test('persists a user-selected order without allowing fixed columns to move', async () => {
    const storageKey = 'data-table-column-order-move-test:v1'
    const { result } = renderHook(() => useTestTable(storageKey))

    act(() => {
      result.current.table.setColumnOrder([
        'actions',
        'ratio',
        'balance',
        'name',
        'select',
      ])
    })

    expect(result.current.table.getState().columnOrder).toEqual([
      'select',
      'ratio',
      'balance',
      'name',
      'actions',
    ])

    await waitFor(() => {
      expect(
        JSON.parse(window.localStorage.getItem(storageKey) ?? '[]')
      ).toEqual(['select', 'ratio', 'balance', 'name', 'actions'])
    })
  })
})
