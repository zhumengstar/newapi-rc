import { useNavigate } from '@tanstack/react-router'
import {
  Check,
  ChevronDown,
  ChevronUp,
  ExternalLink,
  Layers,
  Loader2,
  Network,
  RefreshCw,
  RotateCw,
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

const CANVAS_EMBED_PATH = '/canvas-app/canvas'
const CANVAS_STANDALONE_URL = '/canvas-app/canvas'
const SELECTED_TOKEN_ID_KEY = 'infinite-canvas:selected-token-id'
const BANNER_DISMISSED_KEY = 'infinite-canvas:banner-dismissed'
const CANVAS_BASE_URL_KEY = 'infinite-canvas:api-base-url'
const DEFAULT_INTERNAL_IP_URL = 'http://147.124.216.251:3000/v1'
const GPM_INTERNAL_IP_URL = 'http://147.124.216.251:18331/v1'

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
  const [iframeKey, setIframeKey] = useState<number>(0)
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
  const [apiBaseUrl, setApiBaseUrl] = useState<string>(() => {
    try {
      const stored = localStorage.getItem(CANVAS_BASE_URL_KEY)
      if (stored && stored.trim()) {
        if (stored.includes('vip.muling.store')) {
          localStorage.setItem(CANVAS_BASE_URL_KEY, DEFAULT_INTERNAL_IP_URL)
          return DEFAULT_INTERNAL_IP_URL
        }
        return stored.trim()
      }
    } catch {
      /* ignore */
    }
    return DEFAULT_INTERNAL_IP_URL
  })
  const [customUrlInput, setCustomUrlInput] = useState<string>('')
  const [urlPopoverOpen, setUrlPopoverOpen] = useState<boolean>(false)

  // 检测当前是否处于 HTTPS 环境
  const isHttpsOrigin = useMemo(() => {
    return typeof window !== 'undefined' && window.location.protocol === 'https:'
  }, [])

  // 当窗口失焦时（如点击了 iframe 或外部窗口），自动隐藏下拉框
  useEffect(() => {
    if (!popoverOpen && !urlPopoverOpen) return
    const onBlur = () => {
      setPopoverOpen(false)
      setUrlPopoverOpen(false)
    }
    window.addEventListener('blur', onBlur)
    return () => window.removeEventListener('blur', onBlur)
  }, [popoverOpen, urlPopoverOpen])

  // 向 iframe 发送实时配置热更新（无感更新通道、密钥与内部IP地址，避免刷新画布和弹出配置弹窗）
  const sendConfigToIframe = useCallback(
    (key: string, channelName?: string, overrideBaseUrl?: string) => {
      if (!iframeRef.current?.contentWindow) return
      const targetBaseUrl = (
        overrideBaseUrl !== undefined ? overrideBaseUrl : apiBaseUrl
      ).trim()
      try {
        iframeRef.current.contentWindow.postMessage(
          {
            type: 'NEWAPI_CANVAS_CONFIG',
            baseUrl: targetBaseUrl,
            apiKey: key,
            channelName: channelName || '默认分组',
          },
          '*'
        )
      } catch {
        /* ignore postMessage error */
      }
    },
    [apiBaseUrl]
  )

  // 切换接口调用地址
  const handleUpdateApiBaseUrl = useCallback(
    (newUrl: string) => {
      const trimmed = newUrl.trim()
      setApiBaseUrl(trimmed)
      try {
        localStorage.setItem(CANVAS_BASE_URL_KEY, trimmed)
      } catch {
        /* ignore */
      }
      if (activeFullKey) {
        sendConfigToIframe(activeFullKey, currentGroup?.label, trimmed)
      }
      toast.success(t('已切换接口调用地址'))
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [activeFullKey, sendConfigToIframe, t]
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
        const params = new URLSearchParams()
        if (apiBaseUrl) params.set('baseUrl', apiBaseUrl)
        if (formattedKey) params.set('apiKey', formattedKey)
        if (groupDisplayName) params.set('channelName', groupDisplayName)
        setInitialIframeUrl(`${CANVAS_EMBED_PATH}?${params.toString()}`)
      }

      sendConfigToIframe(formattedKey, groupDisplayName, apiBaseUrl)
    } catch {
      setActiveFullKey('')
    } finally {
      setLoading(false)
    }
  }, [apiBaseUrl, selectedTokenId, sendConfigToIframe, t])

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
    const params = new URLSearchParams()
    if (apiBaseUrl) params.set('baseUrl', apiBaseUrl)
    if (activeFullKey) params.set('apiKey', activeFullKey)
    if (currentGroup?.label) params.set('channelName', currentGroup.label)

    return `${CANVAS_EMBED_PATH}?${params.toString()}`
  }, [apiBaseUrl, activeFullKey, currentGroup])

  // 构造独立新窗口画布地址（如果配置了内部 IP，优先以内部 IP 端口打开纯 HTTP 页面，免受 Mixed Content 限制并实现零 CDN 延迟）
  const standaloneCanvasUrl = useMemo(() => {
    const params = new URLSearchParams()
    if (apiBaseUrl) params.set('baseUrl', apiBaseUrl)
    if (activeFullKey) params.set('apiKey', activeFullKey)
    if (currentGroup?.label) params.set('channelName', currentGroup.label)

    let originPrefix = ''
    if (apiBaseUrl.startsWith('http://147.124.216.251:3000')) {
      originPrefix = 'http://147.124.216.251:3000'
    } else if (apiBaseUrl.startsWith('http://147.124.216.251:18331')) {
      originPrefix = 'http://147.124.216.251:3000'
    }

    return `${originPrefix}${CANVAS_STANDALONE_URL}?${params.toString()}`
  }, [apiBaseUrl, activeFullKey, currentGroup])

  // 当前接口调用地址显示标签
  const currentUrlLabel = useMemo(() => {
    if (apiBaseUrl === DEFAULT_INTERNAL_IP_URL) {
      return t('内部IP直连 (3000)')
    }
    if (apiBaseUrl === GPM_INTERNAL_IP_URL) {
      return t('GPM调度直连 (18331)')
    }
    const currentOrigin =
      typeof window !== 'undefined' ? window.location.origin : ''
    if (currentOrigin && apiBaseUrl === `${currentOrigin}/v1`) {
      return t('同源域名')
    }
    return apiBaseUrl.replace(/^https?:\/\//, '').replace(/\/v1$/, '') || t('内部IP')
  }, [apiBaseUrl, t])

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
      setIframeKey((k) => k + 1)
      toast.success(t(`已切换至 ${groupOpt.label}`))
      setTimeout(() => setReloading(false), 400)
    },
    [sendConfigToIframe, t]
  )

  // 强制刷新 iframe 画布实例
  const handleReloadIframe = useCallback(() => {
    setReloading(true)
    setIframeKey((k) => k + 1)
    toast.success(t('正在重新加载无限画布...'))
    setTimeout(() => setReloading(false), 500)
  }, [t])

  // 重新注入操作
  const handleReinject = useCallback(() => {
    setReloading(true)
    if (activeFullKey) {
      sendConfigToIframe(activeFullKey, currentGroup?.label)
      toast.success(t('已重新同步配置至无限画布'))
    } else {
      setIframeKey((k) => k + 1)
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

            {/* 接口调用地址 Popover（支持配置内部IP，消除域名CDN延迟） */}
            {urlPopoverOpen && (
              <div
                className='fixed inset-0 z-40 bg-transparent'
                onClick={(e) => {
                  e.stopPropagation()
                  setUrlPopoverOpen(false)
                }}
                onPointerDown={(e) => {
                  e.stopPropagation()
                  setUrlPopoverOpen(false)
                }}
              />
            )}
            <Popover open={urlPopoverOpen} onOpenChange={setUrlPopoverOpen}>
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
                <Network className='size-3 text-emerald-500' />
                <span className='max-w-32 sm:max-w-44 truncate font-medium'>
                  {currentUrlLabel}
                </span>
                <ChevronDown className='size-3 opacity-50' />
              </PopoverTrigger>
              <PopoverContent
                align='start'
                sideOffset={6}
                className='z-50 w-80 p-3 shadow-xl border-border/80'
              >
                <div className='flex flex-col gap-2.5'>
                  <div className='flex items-center justify-between px-0.5 text-xs font-semibold text-muted-foreground'>
                    <span className='flex items-center gap-1.5'>
                      <Network className='size-3.5 text-primary' />
                      {t('接口调用地址配置')}
                    </span>
                    <Badge
                      variant='outline'
                      className='h-4.5 px-1.5 text-[10px] font-normal text-emerald-600 border-emerald-500/30 bg-emerald-50/50 dark:bg-emerald-950/30'
                    >
                      {apiBaseUrl.startsWith('http://147.')
                        ? t('内部IP直连')
                        : t('自定义地址')}
                    </Badge>
                  </div>

                  <div className='space-y-1'>
                    {/* 预设1：内部 IP 直连（3000）推荐 */}
                    <button
                      type='button'
                      onClick={() => {
                        handleUpdateApiBaseUrl(DEFAULT_INTERNAL_IP_URL)
                        setUrlPopoverOpen(false)
                      }}
                      className={cn(
                        'flex w-full flex-col items-start gap-0.5 rounded-md p-2 text-left text-xs transition-colors hover:bg-accent hover:text-accent-foreground border border-transparent',
                        apiBaseUrl === DEFAULT_INTERNAL_IP_URL &&
                          'bg-accent/80 border-primary/30'
                      )}
                    >
                      <div className='flex w-full items-center justify-between'>
                        <span className='font-semibold text-foreground flex items-center gap-1.5'>
                          <span>{t('内部IP直连 (推荐)')}</span>
                          <Badge className='h-4 px-1 text-[9px] font-normal bg-primary text-primary-foreground'>
                            3000端口
                          </Badge>
                        </span>
                        {apiBaseUrl === DEFAULT_INTERNAL_IP_URL && (
                          <Check className='size-3.5 text-primary shrink-0' />
                        )}
                      </div>
                      <span className='text-[11px] font-mono text-muted-foreground truncate w-full'>
                        {DEFAULT_INTERNAL_IP_URL}
                      </span>
                    </button>

                    {/* 预设2：GPM 调度直连（18331） */}
                    <button
                      type='button'
                      onClick={() => {
                        handleUpdateApiBaseUrl(GPM_INTERNAL_IP_URL)
                        setUrlPopoverOpen(false)
                      }}
                      className={cn(
                        'flex w-full flex-col items-start gap-0.5 rounded-md p-2 text-left text-xs transition-colors hover:bg-accent hover:text-accent-foreground border border-transparent',
                        apiBaseUrl === GPM_INTERNAL_IP_URL &&
                          'bg-accent/80 border-primary/30'
                      )}
                    >
                      <div className='flex w-full items-center justify-between'>
                        <span className='font-semibold text-foreground flex items-center gap-1.5'>
                          <span>{t('GPM调度网关直连')}</span>
                          <Badge
                            variant='outline'
                            className='h-4 px-1 text-[9px] font-normal'
                          >
                            18331端口
                          </Badge>
                        </span>
                        {apiBaseUrl === GPM_INTERNAL_IP_URL && (
                          <Check className='size-3.5 text-primary shrink-0' />
                        )}
                      </div>
                      <span className='text-[11px] font-mono text-muted-foreground truncate w-full'>
                        {GPM_INTERNAL_IP_URL}
                      </span>
                    </button>
                  </div>

                  {/* 自定义输入框 */}
                  <div className='flex items-center gap-1.5 pt-1 border-t border-border/60'>
                    <Input
                      value={customUrlInput}
                      onChange={(e) => setCustomUrlInput(e.target.value)}
                      placeholder={t('自定义地址，如 http://...')}
                      className='h-7.5 text-xs font-mono'
                    />
                    <Button
                      type='button'
                      size='sm'
                      className='h-7.5 px-2.5 text-xs shrink-0'
                      onClick={() => {
                        if (customUrlInput.trim()) {
                          handleUpdateApiBaseUrl(customUrlInput.trim())
                          setCustomUrlInput('')
                          setUrlPopoverOpen(false)
                        }
                      }}
                    >
                      {t('保存')}
                    </Button>
                  </div>

                  {/* Mixed Content 提示 */}
                  {isHttpsOrigin && apiBaseUrl.startsWith('http://') && (
                    <div className='rounded bg-amber-500/10 p-2 text-[11px] text-amber-600 dark:text-amber-400 border border-amber-500/20'>
                      <p className='font-medium mb-0.5'>
                        {t('浏览器 Mixed Content 提示：')}
                      </p>
                      <p className='text-[10px] leading-relaxed text-muted-foreground'>
                        {t(
                          '当前处于 HTTPS 域名，内嵌 iframe 直接请求 HTTP 内部 IP 可能会被浏览器拦截。推荐点击右上角「在新窗口打开」，直接在纯内部 IP 下极速使用！'
                        )}
                      </p>
                    </div>
                  )}
                </div>
              </PopoverContent>
            </Popover>
            </div>

            {/* 右侧动作按钮组 */}
            <div className='flex shrink-0 items-center gap-1 sm:gap-1.5'>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                className='h-7 gap-1 px-2 text-xs text-muted-foreground hover:text-foreground'
                onClick={handleReloadIframe}
                disabled={reloading || loading}
                title={t('重新加载画布')}
              >
                <RotateCw
                  className={cn('size-3', reloading && 'animate-spin')}
                />
                <span className='hidden sm:inline'>{t('刷新画布')}</span>
              </Button>

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
                <span className='hidden sm:inline'>{t('同步配置')}</span>
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
        <div className='relative flex-1 w-full h-full min-h-[400px] bg-background'>
          {loading ? (
            <div className='flex h-full w-full flex-col items-center justify-center gap-3 text-muted-foreground'>
              <Loader2 className='size-6 animate-spin text-primary' />
              <p className='text-xs'>{t('正在关联分组令牌并载入画布...')}</p>
            </div>
          ) : (
            <iframe
              key={iframeKey}
              ref={iframeRef}
              src={embedCanvasUrl}
              onLoad={() => {
                if (activeFullKey) {
                  sendConfigToIframe(activeFullKey, currentGroup?.label)
                }
              }}
              title={t('Infinite Canvas')}
              className='absolute inset-0 h-full w-full border-0'
              allow='clipboard-read; clipboard-write'
            />
          )}
        </div>
      </Main>
    </div>
  )
}
