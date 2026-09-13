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
import { useRouterState } from '@tanstack/react-router'
import { useEffect, useState } from 'react'

import { AnimatedOutlet } from '@/components/page-transition'
import { SkipToMain } from '@/components/skip-to-main'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { LayoutProvider } from '@/context/layout-provider'
import { SearchProvider } from '@/context/search-provider'
import { PersistentCanvasView } from '@/features/canvas/components/persistent-canvas-view'
import { getCookie } from '@/lib/cookies'
import { cn } from '@/lib/utils'

import { AppHeader } from './app-header'
import { AppSidebar } from './app-sidebar'

type AuthenticatedLayoutProps = {
  children?: React.ReactNode
}

export function AuthenticatedLayout(props: AuthenticatedLayoutProps) {
  const defaultOpen = getCookie('sidebar_state') !== 'false'
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const isCanvasRoute = pathname.startsWith('/canvas')

  // 懒加载常驻：一旦访问过 /canvas，便保持其 iframe 实例常驻后台，不因路由切换销毁
  const [hasCanvasMounted, setHasCanvasMounted] = useState(isCanvasRoute)

  useEffect(() => {
    if (isCanvasRoute && !hasCanvasMounted) {
      setHasCanvasMounted(true)
    }
  }, [isCanvasRoute, hasCanvasMounted])

  return (
    <LayoutProvider>
      <SearchProvider>
        <SidebarProvider defaultOpen={defaultOpen} className='flex-col'>
          <SkipToMain />
          <AppHeader />
          <div className='flex min-h-0 w-full flex-1'>
            <AppSidebar />
            <SidebarInset
              className={cn(
                '@container/content',
                'h-[calc(100svh-var(--app-header-height,0px))]',
                'min-h-0 overflow-hidden',
                'peer-data-[variant=inset]:h-[calc(100svh-var(--app-header-height,0px)-(var(--spacing)*4))]',
                'has-[[data-fullbleed]]:peer-data-[variant=inset]:h-[calc(100svh-var(--app-header-height,0px))]'
              )}
            >
              {/* 普通页面路由（非 /canvas 时展示，切到 /canvas 时隐藏） */}
              <div
                className={cn(
                  'flex h-full w-full min-h-0 flex-col overflow-hidden',
                  isCanvasRoute && 'hidden'
                )}
              >
                {props.children ?? <AnimatedOutlet />}
              </div>

              {/* 全局常驻 Canvas Host：一旦激活便常驻 DOM 树，在后台继续执行异步生图 */}
              {hasCanvasMounted && (
                <PersistentCanvasView isVisible={isCanvasRoute} />
              )}
            </SidebarInset>
          </div>
        </SidebarProvider>
      </SearchProvider>
    </LayoutProvider>
  )
}
