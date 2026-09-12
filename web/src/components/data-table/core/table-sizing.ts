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
import type { Table as TanstackTable } from '@tanstack/react-table'
import type * as React from 'react'

import { isContentSizedColumn } from './content-sized-columns'

export function getTableSizeStyle<TData>(
  table: TanstackTable<TData>
): React.CSSProperties {
  const visibleColumns = table.getVisibleLeafColumns()
  const width = visibleColumns
    .filter(
      (column) =>
        table.options.enableColumnResizing === true ||
        !isContentSizedColumn(column.id)
    )
    .reduce((total, column) => total + column.getSize(), 0)

  if (table.options.enableColumnResizing === true) {
    // Keep the table exactly as wide as its columns. Filling the container
    // makes the browser distribute unused space across the other columns when
    // one column is narrowed, even with a fixed table layout.
    return {
      minWidth: `${width}px`,
      tableLayout: 'fixed',
      width: `${width}px`,
    }
  }

  return {
    minWidth: `max(100%, ${width}px)`,
    tableLayout: 'auto',
    width: '100%',
  }
}
