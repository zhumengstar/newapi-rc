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
import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/canvas/')({
  component: CanvasPage,
})

function CanvasPage() {
  // 无限画布与生图工作台已提升至 AuthenticatedLayout 全局常驻渲染（Keep-Alive Host），
  // 当用户在 New API 内部切换到其他页面（如渠道、令牌、使用日志等）时，
  // iframe 绝不销毁，后台异步生图与网络请求不中断，切回秒级恢复。
  return null
}
