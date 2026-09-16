import { useNavigate } from '@tanstack/react-router'
import {
  Check,
  ChevronDown,
  ChevronUp,
  ExternalLink,
  Layers,
  Loader2,
  RefreshCw,
  Search,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Main } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { createApiKey, fetchTokenKey, getApiKeys } from '@/features/keys/api'
import type { ApiKey } from '@/features/keys/types'
import { cn } from '@/lib/utils'

const CANVAS_EMBED_PATH = '/canvas-app/'
const CANVAS_STANDALONE_URL = '/canvas-app/'
const SELECTED_TOKEN_ID_KEY = 'infinite-canvas:selected-token-id'
const BANNER_DISMISSED_KEY = 'infinite-canvas:banner-dismissed'

interface GroupOption {
  group: string
  label: string
  token: ApiKey
}

interface PersistentCanvasViewProps {
  isVisible: boolean
}

export function PersistentCanvasView({ isVisible }: PersistentCanvasViewProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const iframeRef = useRef<HTMLIFrameElement | null>(null)
  const initialUrlSetRef = useRef(false)

  const [tokens, setTokens] = useState<ApiKey[]>([])
  const [selectedTokenId, setSelectedTokenId] = useState<number | null>(() => {
    try {
      const stored = localStorage.getItem(SELECTED_TOKEN_ID_KEY)
      return stored ? Number(stored) : null
    } catch {
      return null
    }
  })
  const [activeFullKey, setActiveFullKey] = useState<string>('')
  const [initialIframeUrl, setInitialIframeUrl] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [reloading, setReloading] = useState(false)
  const [groupSearch, setGroupSearch] = useState('')
  const [popoverOpen, setPopoverOpen] = useState(false)
  const [bannerDismissed, setBannerDismissed] = useState<boolean>(() => {
    try {
      return localStorage.getItem(BANNER_DISMISSED_KEY) === 'true'
    } catch {
      return false
    }
  })

  // 当窗口失焦时（如点击了 iframe 或外部窗口），自动隐藏分组下拉框
  useEffect(() => {
    if (!popoverOpen) return
    const onBlur = () => setPopoverOpen(false)
    window.addEventListener('blur', onBlur)
    return () => window.removeEventListener('blur', onBlur)
  }, [popoverOpen])

  // 向 iframe 发送实时配置热更新（无感更新通道与密钥，避免刷新画布和弹出配置弹窗）
  const sendConfigToIframe = useCallback(
    (key: string, channelName?: string) => {
      if (!iframeRef.current?.contentWindow) return
      const currentOrigin =
        typeof window !== 'undefined' ? window.location.origin : ''
      const apiBaseUrl = currentOrigin ? `${currentOrigin}/v1` : ''
      try {
        iframeRef.current.contentWindow.postMessage(
          {
            type: 'NEWAPI_CANVAS_CONFIG',
            baseUrl: apiBaseUrl,
            apiKey: key,
            channelName: channelName || '默认分组',
          },
          '*'
        )
      } catch {
        /* ignore postMessage error */
      }
    },
    []
  )

  // 解析并确定当前用户的可用令牌与真实密钥
  const resolveTokenAndKey = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getApiKeys({ p: 1, size: 100 })
      let availableTokens = (res?.data?.items || []).filter(
        (item) =>
          item.status === 1 &&
          (item.expired_time === -1 || item.expired_time * 1000 > Date.now())
      )

      // 如果当前没有任何可用令牌，自动创建无限画布专属令牌
      if (availableTokens.length === 0) {
        try {
          const createRes = await createApiKey({
            name: '无限画布',
            remain_quota: 500000,
            expired_time: -1,
            unlimited_quota: true,
            model_limits_enabled: false,
            model_limits: '',
            allow_ips: '',
            group: '',
            auto_groups: [],
            cross_group_retry: false,
          })
          if (createRes?.success && createRes.data) {
            availableTokens = [createRes.data]
          }
        } catch {
          /* ignore */
        }
      }

      setTokens(availableTokens)

      // 确定选中的令牌
      let chosenToken = availableTokens.find((item) => item.id === selectedTokenId)
      if (!chosenToken) {
        chosenToken = availableTokens.find(
          (item) =>
            item.status === 1 &&
            (item.name.includes('无限画布') ||
              item.name.toLowerCase().includes('canvas'))
        )
      }
      if (!chosenToken) {
        chosenToken =
          availableTokens.find((item) => item.status === 1) ||
          availableTokens[0]
      }

      if (!chosenToken) {
        setActiveFullKey('')
        setLoading(false)
        return
      }

      setSelectedTokenId(chosenToken.id)
      try {
        localStorage.setItem(SELECTED_TOKEN_ID_KEY, String(chosenToken.id))
      } catch {
        /* ignore */
      }

      // 获取完整未脱敏密钥
      let fullKey = chosenToken.key || ''
      if (fullKey.includes('*') || fullKey.length < 20) {
        try {
          const keyRes = await fetchTokenKey(chosenToken.id)
          if (keyRes?.success && keyRes.data?.key) {
            fullKey = keyRes.data.key
          }
        } catch {
          /* ignore */
        }
      }

      const formattedKey = fullKey.startsWith('sk-') ? fullKey : `sk-${fullKey}`
      setActiveFullKey(formattedKey)
      const groupDisplayName =
        (chosenToken.group || '').trim() || t('默认分组')

      if (!initialUrlSetRef.current) {
        initialUrlSetRef.current = true
        const currentOrigin =
          typeof window !== 'undefined' ? window.location.origin : ''
        const apiBaseUrl = currentOrigin ? `${currentOrigin}/v1` : ''
        const params = new URLSearchParams()
        if (apiBaseUrl) params.set('baseUrl', apiBaseUrl)
        if (formattedKey) params.set('apiKey', formattedKey)
        if (groupDisplayName) params.set('channelName', groupDisplayName)
        setInitialIframeUrl(`${CANVAS_EMBED_PATH}?${params.toString()}`)
      }

      sendConfigToIframe(formattedKey, groupDisplayName)
    } catch {
      setActiveFullKey('')
    } finally {
      setLoading(false)
    }
  }, [selectedTokenId, sendConfigToIframe, t])

  useEffect(() => {
    void resolveTokenAndKey()
  }, [resolveTokenAndKey])

  // 跨页面生图通知监听：用户在 New API 其他页面时，生图完毕弹出通知
  useEffect(() => {
    const handleMessage = (event: MessageEvent) => {
      const data = event.data
      if (!data || typeof data !== 'object') return
      if (data.type === 'CANVAS_GENERATION_COMPLETED') {
        const { successCount, prompt } = data
        if (successCount > 0) {
          toast.success(
            t('无限画布：图片生成完成！', { defaultValue: '无限画布：图片生成完成！' }),
            {
              description: prompt ? `"${prompt}"` : undefined,
              action: !isVisible
                ? {
                    label: t('去查看', { defaultValue: '去查看' }),
                    onClick: () => void navigate({ to: '/canvas' }),
                  }
                : undefined,
            }
          )
        }
      }
    }

    window.addEventListener('message', handleMessage)
    return () => window.removeEventListener('message', handleMessage)
  }, [isVisible, navigate, t])

  const activeToken = useMemo(
    () => tokens.find((t) => t.id === selectedTokenId),
    [tokens, selectedTokenId]
  )

  // 将全部令牌按分组归并，仅显示分组名称
  const groupOptions = useMemo(() => {
    const map = new Map<string, ApiKey>()
    for (const token of tokens) {
      const g = (token.group || '').trim() || 'default'
      const existing = map.get(g)
      if (!existing) {
        map.set(g, token)
      } else if (token.unlimited_quota && !existing.unlimited_quota) {
        map.set(g, token)
      } else if (
        token.unlimited_quota === existing.unlimited_quota &&
        token.remain_quota > existing.remain_quota
      ) {
        map.set(g, token)
      } else if (
        token.unlimited_quota === existing.unlimited_quota &&
        token.remain_quota === existing.remain_quota &&
        token.id > existing.id
      ) {
        map.set(g, token)
      }
    }

    return Array.from(map.entries()).map(([g, token]): GroupOption => ({
      group: g,
      label: g === 'default' ? t('默认分组') : g,
      token,
    }))
  }, [tokens, t])

  // 当前激活的分组
  const currentGroup = useMemo(() => {
    if (activeToken) {
      const g = (activeToken.group || '').trim() || 'default'
      return groupOptions.find((opt) => opt.group === g) || groupOptions[0]
    }
    return groupOptions[0]
  }, [activeToken, groupOptions])

  // 过滤后的分组列表
  const filteredGroups = useMemo(() => {
    const tokens = groupSearch.toLowerCase().trim().split(/\s+/).filter(Boolean)
    if (tokens.length === 0) return groupOptions
    return groupOptions.filter((opt) => {
      const target = `${opt.label} ${opt.group}`.toLowerCase()
      return tokens.every((token) => target.includes(token))
    })
  }, [groupOptions, groupSearch])

  // 构造内嵌画布地址
  const embedCanvasUrl = useMemo(() => {
    const currentOrigin =
      typeof window !== 'undefined' ? window.location.origin : ''
    const apiBaseUrl = currentOrigin ? `${currentOrigin}/v1` : ''

    const params = new URLSearchParams()
    if (apiBaseUrl) params.set('baseUrl', apiBaseUrl)
    if (activeFullKey) params.set('apiKey', activeFullKey)
    if (currentGroup?.label) params.set('channelName', currentGroup.label)

    return `${CANVAS_EMBED_PATH}?${params.toString()}`
  }, [activeFullKey, currentGroup])

  // 构造独立新窗口画布地址
  const standaloneCanvasUrl = useMemo(() => {
    const currentOrigin =
      typeof window !== 'undefined' ? window.location.origin : ''
    const apiBaseUrl = currentOrigin ? `${currentOrigin}/v1` : ''

    const params = new URLSearchParams()
    if (apiBaseUrl) params.set('baseUrl', apiBaseUrl)
    if (activeFullKey) params.set('apiKey', activeFullKey)
    if (currentGroup?.label) params.set('channelName', currentGroup.label)

    return `${CANVAS_STANDALONE_URL}?${params.toString()}`
  }, [activeFullKey, currentGroup])

  // 切换分组操作
  const handleSwitchGroup = useCallback(
    async (groupOpt: GroupOption) => {
      const targetToken = groupOpt.token
      setSelectedTokenId(targetToken.id)
      try {
        localStorage.setItem(SELECTED_TOKEN_ID_KEY, String(targetToken.id))
      } catch {
        /* ignore */
      }

      setReloading(true)
      let fullKey = targetToken.key || ''
      if (fullKey.includes('*') || fullKey.length < 20) {
        try {
          const keyRes = await fetchTokenKey(targetToken.id)
          if (keyRes?.success && keyRes.data?.key) {
            fullKey = keyRes.data.key
          }
        } catch {
          /* ignore */
        }
      }
      const formattedKey = fullKey.startsWith('sk-') ? fullKey : `sk-${fullKey}`
      setActiveFullKey(formattedKey)
      sendConfigToIframe(formattedKey, groupOpt.label)
      toast.success(t(`已切换至 ${groupOpt.label}`))
      setTimeout(() => setReloading(false), 400)
    },
    [sendConfigToIframe, t]
  )

  // 重新注入操作
  const handleReinject = useCallback(() => {
    setReloading(true)
    if (activeFullKey) {
      sendConfigToIframe(activeFullKey, currentGroup?.label)
      toast.success(t('已重新同步配置至无限画布'))
    }
    setTimeout(() => setReloading(false), 400)
  }, [activeFullKey, currentGroup, sendConfigToIframe, t])

  return (
    <div
      data-fullbleed
      className={cn(
        'transition-opacity duration-150',
        isVisible
          ? 'relative flex flex-col p-0 h-[calc(100svh-var(--app-header-height,0px))] w-full min-h-0 overflow-hidden bg-background opacity-100'
          : 'fixed -left-[99999px] -top-[99999px] -z-50 h-0 w-0 overflow-hidden pointer-events-none opacity-0'
      )}
    >
      <Main
        data-fullbleed
        className='relative flex flex-col p-0 h-full w-full overflow-hidden bg-background'
      >
        {/* 顶部极简状态栏 */}
        {!bannerDismissed && (
          <div className='z-10 flex h-10 shrink-0 items-center justify-between gap-3 border-b border-border/60 bg-card/95 px-3 backdrop-blur-md sm:px-4'>
            {/* 左侧：就绪状态指示器 + 仅显示分组名称的选择器 */}
            <div className='flex min-w-0 items-center gap-2 sm:gap-2.5'>
              <div className='flex items-center gap-1.5'>
                <span className='relative flex size-2'>
                  <span className='absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75' />
                  <span className='relative inline-flex size-2 rounded-full bg-emerald-500' />
                </span>
                <span className='font-semibold text-xs text-foreground tracking-tight'>
                  {t('无限画布')}
                </span>
              </div>

              <span className='text-muted-foreground/30 text-xs'>·</span>

              {/* 仅展示分组名称的 Popover */}
              {groupOptions.length > 0 && (
                <>
                  {popoverOpen && (
                    <div
                      className='fixed inset-0 z-40 bg-transparent'
                      onClick={(e) => {
                        e.stopPropagation()
                        setPopoverOpen(false)
                      }}
                      onPointerDown={(e) => {
                        e.stopPropagation()
                        setPopoverOpen(false)
                      }}
                    />
                  )}
                  <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
                    <PopoverTrigger
                      render={
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='h-6.5 gap-1.5 rounded-md border-border/70 bg-background px-2.5 text-xs font-medium text-foreground hover:bg-muted/80 shadow-2xs'
                        />
                      }
                    >
                      <Layers className='size-3 text-muted-foreground' />
                      <span className='max-w-32 sm:max-w-40 truncate font-medium'>
                        {currentGroup ? currentGroup.label : t('默认分组')}
                      </span>
                      <ChevronDown className='size-3 opacity-50' />
                    </PopoverTrigger>
                    <PopoverContent
                      align='start'
                      sideOffset={6}
                      className='z-50 w-60 p-2 shadow-xl border-border/80'
                    >
                    <div className='flex flex-col gap-2'>
                      <div className='flex items-center justify-between px-1 text-xs font-semibold text-muted-foreground'>
                        <span>{t('选择分组')}</span>
                        <Badge variant='outline' className='h-4.5 px-1.5 text-[10px] font-normal'>
                          共 {groupOptions.length} 个
                        </Badge>
                      </div>

                      {groupOptions.length > 4 && (
                        <div className='relative'>
                          <Search className='absolute left-2 top-2 size-3.5 text-muted-foreground' />
                          <Input
                            value={groupSearch}
                            onChange={(e) => setGroupSearch(e.target.value)}
                            placeholder={t('搜索分组名称...')}
                            className='h-7.5 pl-7 text-xs'
                          />
                        </div>
                      )}

                      <div className='max-h-52 overflow-y-auto space-y-0.5 pr-0.5'>
                        {filteredGroups.length === 0 ? (
                          <div className='py-3 text-center text-xs text-muted-foreground'>
                            {t('未找到匹配分组')}
                          </div>
                        ) : (
                          filteredGroups.map((opt) => {
                            const isSelected = opt.group === currentGroup?.group
                            return (
                              <button
                                key={opt.group}
                                type='button'
                                onClick={() => {
                                  void handleSwitchGroup(opt)
                                  setPopoverOpen(false)
                                }}
                                className={cn(
                                  'flex w-full items-center justify-between gap-2 rounded-md px-2.5 py-1.5 text-left text-xs transition-colors hover:bg-accent hover:text-accent-foreground',
                                  isSelected && 'bg-accent/80 font-medium text-primary'
                                )}
                              >
                                <span className='truncate font-medium text-foreground text-xs'>
                                  {opt.label}
                                </span>
                                {isSelected && <Check className='size-3.5 text-primary shrink-0' />}
                              </button>
                            )
                          })
                        )}
                      </div>
                    </div>
                  </PopoverContent>
                </Popover>
              </>
            )}
            </div>

            {/* 右侧动作按钮组 */}
            <div className='flex shrink-0 items-center gap-1 sm:gap-1.5'>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                className='h-7 gap-1 px-2 text-xs text-muted-foreground hover:text-foreground'
                onClick={handleReinject}
                disabled={reloading || loading}
                title={t('重新同步配置')}
              >
                <RefreshCw
                  className={cn('size-3', (reloading || loading) && 'animate-spin')}
                />
                <span className='hidden sm:inline'>{t('重新同步')}</span>
              </Button>

              <Button
                type='button'
                variant='default'
                size='sm'
                className='h-7 gap-1 px-2.5 text-xs font-medium'
                onClick={() => window.open(standaloneCanvasUrl, '_blank')}
                title={t('新窗口独立打开')}
              >
                <ExternalLink className='size-3' />
                <span>{t('在新窗口打开')}</span>
              </Button>

              <div className='ml-1 h-3.5 w-px bg-border/60' />

              <Button
                type='button'
                variant='ghost'
                size='icon'
                className='size-7 text-muted-foreground hover:text-foreground'
                onClick={() => {
                  setBannerDismissed(true)
                  try {
                    localStorage.setItem(BANNER_DISMISSED_KEY, 'true')
                  } catch {
                    /* ignore */
                  }
                }}
                title={t('全屏沉浸创作')}
              >
                <ChevronUp className='size-3.5' />
              </Button>
            </div>
          </div>
        )}

        {/* 隐藏顶部条后的浮动恢复按钮 */}
        {bannerDismissed && (
          <div className='absolute top-3 right-3 z-30 flex items-center gap-1.5 rounded-full border border-border/60 bg-background/80 px-2.5 py-1 shadow-md backdrop-blur-md transition-all hover:bg-background'>
            <span className='relative flex size-2'>
              <span className='absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75' />
              <span className='relative inline-flex size-2 rounded-full bg-emerald-500' />
            </span>
            <span className='text-xs font-medium text-foreground max-w-28 truncate'>
              {currentGroup?.label || t('默认分组')}
            </span>
            <button
              type='button'
              className='ml-1 text-muted-foreground hover:text-foreground'
              onClick={() => window.open(standaloneCanvasUrl, '_blank')}
              title={t('在新窗口打开')}
            >
              <ExternalLink className='size-3' />
            </button>
            <button
              type='button'
              className='text-muted-foreground hover:text-foreground'
              onClick={() => {
                setBannerDismissed(false)
                try {
                  localStorage.setItem(BANNER_DISMISSED_KEY, 'false')
                } catch {
                  /* ignore */
                }
              }}
              title={t('展开顶栏')}
            >
              <ChevronDown className='size-3.5' />
            </button>
          </div>
        )}

        {/* 画布核心 iframe 区域 */}
        <div className='relative flex-1 w-full min-h-0 bg-background'>
          {loading ? (
            <div className='flex h-full w-full flex-col items-center justify-center gap-3 text-muted-foreground'>
              <Loader2 className='size-6 animate-spin text-primary' />
              <p className='text-xs'>{t('正在关联分组令牌并载入画布...')}</p>
            </div>
          ) : (
            <iframe
              ref={iframeRef}
              src={initialIframeUrl || embedCanvasUrl}
              onLoad={() => {
                if (activeFullKey) {
                  sendConfigToIframe(activeFullKey, currentGroup?.label)
                }
              }}
              title={t('Infinite Canvas')}
              className='h-full w-full border-0'
              allow='clipboard-read; clipboard-write'
            />
          )}
        </div>
      </Main>
    </div>
  )
}
