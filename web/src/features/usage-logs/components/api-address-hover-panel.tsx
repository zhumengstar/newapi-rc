import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Check,
  Coins,
  Copy,
  DollarSign,
  ExternalLink,
  Globe,
  KeyRound,
  Layers,
  Loader2,
  LogIn,
  MessageSquare,
  Plus,
  RefreshCw,
  Settings2,
  Sparkles,
  TrendingUp,
  UserCheck,
  Zap,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'

import { GroupBadge } from '@/components/group-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  probeUpstreamSource,
  updateChannelSource,
} from '@/features/exclusive-channels/api'
import {
  openUpstreamOneClickLogin,
  resolveUpstreamConsoleUrl,
} from '@/features/exclusive-channels/lib/one-click-login'
import type { UpstreamSourceDTO } from '@/features/exclusive-channels/types'
import { inferGroupFamily } from '@/features/users/lib'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { formatLogQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

interface ApiAddressHoverPanelProps {
  baseUrl: string
  source?: UpstreamSourceDTO
  onOpenFullConfig: (source: UpstreamSourceDTO) => void
}

export function ApiAddressHoverPanel({
  baseUrl,
  source,
  onOpenFullConfig,
}: ApiAddressHoverPanelProps) {
  const queryClient = useQueryClient()
  const { copyToClipboard } = useCopyToClipboard()
  const [draftRatios, setDraftRatios] = useState<Record<string, string>>({})
  const [isDirty, setIsDirty] = useState(false)

  const lastBaseUrlRef = useState<{ url: string }>({ url: '' })[0]

  useEffect(() => {
    if (!source) {
      setDraftRatios({})
      setIsDirty(false)
      lastBaseUrlRef.url = ''
      return
    }
    const isSameUrl = lastBaseUrlRef.url === source.base_url
    if (isSameUrl && isDirty) {
      return
    }
    lastBaseUrlRef.url = source.base_url
    const init: Record<string, string> = {}
    if (source.group_cost_ratios) {
      for (const [g, r] of Object.entries(source.group_cost_ratios)) {
        if (typeof r === 'number' && r >= 0) {
          init[g] = String(r)
        }
      }
    }
    setDraftRatios(init)
    setIsDirty(false)
  }, [source, isDirty, lastBaseUrlRef])

  const sortedGroups = useMemo(() => {
    if (!source) return []
    const set = new Set<string>(source.groups || [])
    if (source.group_incomes) {
      for (const g of Object.keys(source.group_incomes)) {
        if (g) set.add(g)
      }
    }
    return Array.from(set).sort((a, b) => {
      const incA = source.group_incomes?.[a]?.today_income || 0
      const incB = source.group_incomes?.[b]?.today_income || 0
      if (incB !== incA) return incB - incA
      return a.localeCompare(b)
    })
  }, [source])

  const saveRatiosMutation = useMutation({
    mutationFn: async () => {
      if (!source) return
      const nextGroupRatios: Record<string, number> = {}
      for (const [g, valStr] of Object.entries(draftRatios)) {
        const num = parseFloat(valStr)
        if (!isNaN(num) && num >= 0) {
          nextGroupRatios[g] = num
        }
      }
      await updateChannelSource({
        base_url: source.base_url,
        group_cost_ratios: nextGroupRatios,
        model_cost_rules: source.model_cost_rules || [],
      })
    },
    onSuccess: () => {
      toast.success('该地址下的分组成本倍率已保存')
      setIsDirty(false)
      queryClient.invalidateQueries({ queryKey: ['channel-sources-list'] })
      queryClient.invalidateQueries({ queryKey: ['channels'] })
    },
    onError: (err: any) => {
      toast.error(err?.message || '保存分组成本倍率失败')
    },
  })

  const probeMutation = useMutation({
    mutationFn: async () => {
      if (!source) throw new Error('未找到来源信息')
      return await probeUpstreamSource({
        base_url: source.base_url,
      })
    },
    onSuccess: (res) => {
      const detected = res.data?.detected_group_ratios || {}
      const detectedCount = Object.keys(detected).length
      if (detectedCount > 0) {
        setDraftRatios((prev) => {
          const next = { ...prev }
          for (const [g, r] of Object.entries(detected)) {
            if (next[g] === undefined || next[g].trim() === '') {
              next[g] = String(r)
              setIsDirty(true)
            }
          }
          return next
        })
      }
      toast.success(res.message || '上游实时余额与扣费探测完成')
      queryClient.invalidateQueries({ queryKey: ['channel-sources-list'] })
    },
    onError: (err: any) => {
      toast.error(err?.message || '探测上游失败')
    },
  })

  if (!source) {
    return (
      <div className='p-3.5 text-xs text-muted-foreground space-y-1.5 select-text'>
        <div className='flex items-center justify-between gap-2'>
          <button
            type='button'
            className='flex items-center gap-1.5 font-mono font-medium text-foreground hover:text-primary transition-colors cursor-pointer min-w-0'
            title={`点击一键登录打开: ${baseUrl}`}
            onClick={() => openUpstreamOneClickLogin(null, { rawBaseUrl: baseUrl })}
          >
            <Globe className='size-3.5 text-primary shrink-0' />
            <span className='truncate underline-offset-2 hover:underline'>
              {baseUrl}
            </span>
            <ExternalLink className='size-3 text-primary shrink-0' />
          </button>
          <button
            type='button'
            className='p-1 rounded hover:bg-muted text-muted-foreground hover:text-foreground shrink-0 cursor-pointer'
            title='复制 API 地址'
            onClick={() => copyToClipboard(baseUrl)}
          >
            <Copy className='size-3' />
          </button>
        </div>
        <div>正在加载该地址下的分组与成本配置...</div>
      </div>
    )
  }

  const rules = source.model_cost_rules || []
  const consoleUrl = resolveUpstreamConsoleUrl(source)
  const hasDiffConsole =
    consoleUrl &&
    consoleUrl.toLowerCase() !==
      source.display_base_url.trim().toLowerCase().replace(/\/+$/, '')

  return (
    <div className='p-3 space-y-2.5 text-xs select-text'>
      {/* 1. 顶部地址信息（点击域名一键登录）与完整配置按钮 */}
      <div className='space-y-1.5 border-b border-border/60 pb-2.5'>
        <div className='flex items-center justify-between gap-2'>
          <div className='flex items-center gap-1 min-w-0 flex-1'>
            <button
              type='button'
              className='group/url inline-flex max-w-full items-center gap-1.5 min-w-0 cursor-pointer text-left hover:text-primary transition-colors'
              title={
                source.upstream_username
                  ? `🚀 点击一键免密登录控制台: ${consoleUrl} (账号: ${source.upstream_username})`
                  : `🌐 点击打开上游控制台: ${consoleUrl}`
              }
              onClick={() => openUpstreamOneClickLogin(source)}
            >
              <Globe className='size-3.5 text-primary shrink-0' />
              <span className='font-mono font-semibold text-foreground group-hover/url:text-primary group-hover/url:underline underline-offset-2 truncate select-text'>
                {source.display_base_url}
              </span>
              <span className='inline-flex items-center gap-0.5 rounded bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium text-primary shrink-0 group-hover/url:bg-primary group-hover/url:text-primary-foreground transition-colors'>
                <LogIn className='size-2.5' />
                <span>一键登录</span>
              </span>
            </button>

            <button
              type='button'
              className='rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground shrink-0 cursor-pointer transition-colors'
              title={`复制地址: ${source.display_base_url}`}
              onClick={(e) => {
                e.stopPropagation()
                copyToClipboard(source.display_base_url)
              }}
            >
              <Copy className='size-3' />
            </button>
          </div>

          <div className='flex items-center gap-1 shrink-0'>
            <Button
              type='button'
              size='sm'
              variant='outline'
              disabled={probeMutation.isPending}
              className='h-7 px-2 text-[11px] gap-1'
              title='实时探测上游账户余额与令牌真实扣费倍率'
              onClick={() => probeMutation.mutate()}
            >
              <RefreshCw
                className={cn(
                  'size-3 text-emerald-600',
                  probeMutation.isPending && 'animate-spin'
                )}
              />
              <span>探测</span>
            </Button>
            <Button
              type='button'
              size='sm'
              variant='default'
              className='h-7 px-2.5 text-[11px] shrink-0 gap-1'
              onClick={() => onOpenFullConfig(source)}
            >
              <Settings2 className='size-3' />
              <span>成本与模型单价</span>
            </Button>
          </div>
        </div>

        {/* 第二行：渠道数 + 今日收入 + 预计成本 + 净毛利 + 上游实时余额 */}
        <div className='flex flex-wrap items-center gap-1.5 text-[11px] text-muted-foreground'>
          <span className='shrink-0'>
            {source.channel_count}渠({source.enabled_channel_count}启)
          </span>

          <span
            className={cn(
              'inline-flex shrink-0 items-center rounded px-1.5 py-0.5 font-mono text-[10px] tabular-nums',
              (source.today_quota || 0) > 0
                ? 'bg-emerald-500/10 font-semibold text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                : 'bg-muted/60 text-muted-foreground'
            )}
            title='今日对客消耗总收入'
          >
            当天:{' '}
            {(source.today_quota || 0) > 0
              ? formatLogQuota(source.today_quota || 0)
              : '$0'}
          </span>

          {source.today_estimated_cost !== undefined &&
            source.today_estimated_cost > 0 && (
              <>
                <span
                  className='inline-flex shrink-0 items-center rounded px-1.5 py-0.5 font-mono text-[10px] tabular-nums bg-indigo-500/10 text-indigo-700 dark:text-indigo-300 border border-indigo-500/20'
                  title='按已配置分组成本倍率折算的今日预计上游采购成本'
                >
                  成本: ~${source.today_estimated_cost.toFixed(2)}
                </span>
                {source.today_gross_profit !== undefined && (
                  <span
                    className={cn(
                      'inline-flex shrink-0 items-center gap-0.5 rounded px-1.5 py-0.5 font-mono text-[10px] font-semibold tabular-nums border',
                      source.today_gross_profit >= 0
                        ? 'bg-emerald-500/12 text-emerald-700 dark:text-emerald-300 border-emerald-500/25'
                        : 'bg-rose-500/15 text-rose-700 dark:text-rose-300 border-rose-500/35'
                    )}
                    title='今日预计净毛利 = 当天收入 - 预计采购成本'
                  >
                    <TrendingUp className='size-2.5' />
                    {source.today_gross_profit >= 0
                      ? `毛利 +$${source.today_gross_profit.toFixed(2)} (${source.today_gross_margin ?? 0}%)`
                      : `倒挂 -$${Math.abs(source.today_gross_profit).toFixed(2)}`}
                  </span>
                )}
              </>
            )}

          {source.upstream_balance !== undefined &&
            source.upstream_balance !== null && (
              <span
                className={cn(
                  'inline-flex shrink-0 items-center gap-0.5 rounded px-1.5 py-0.5 font-mono text-[10px] font-semibold tabular-nums border',
                  source.upstream_balance <= 0
                    ? 'bg-rose-500/15 text-rose-700 dark:text-rose-300 border-rose-500/30'
                    : source.upstream_balance < 20
                      ? 'bg-amber-500/15 text-amber-700 dark:text-amber-300 border-amber-500/30'
                      : 'bg-sky-500/12 text-sky-700 dark:text-sky-300 border-sky-500/25'
                )}
                title={
                  source.upstream_used_quota
                    ? `上游账户当前余额: $${source.upstream_balance.toFixed(2)} | 历史已消耗: $${source.upstream_used_quota.toFixed(2)}`
                    : `上游账户当前余额: $${source.upstream_balance.toFixed(2)}`
                }
              >
                💰 余额 ${source.upstream_balance.toFixed(2)}
              </span>
            )}
        </div>

        {/* 第三行：控制台域名 / 登录账号密码快捷复制 / 虚拟刀折算 / 微信备注 */}
        {(hasDiffConsole ||
          source.upstream_username ||
          (source.cost_discount_scale && source.cost_discount_scale !== 1) ||
          source.wechat_name) && (
          <div className='flex flex-wrap items-center gap-1.5 pt-0.5'>
            {hasDiffConsole && (
              <Badge
                variant='outline'
                onClick={() => openUpstreamOneClickLogin(source)}
                className='h-4 px-1.5 text-[10px] font-mono cursor-pointer border-primary/30 text-primary bg-primary/5 hover:bg-primary/10'
                title='转发域名与控制台域名不同，点击打开控制台域名'
              >
                后台: {consoleUrl.replace(/^https?:\/\//, '')}
              </Badge>
            )}

            {source.upstream_username && (
              <Badge
                variant='outline'
                onClick={() => {
                  copyToClipboard(source.upstream_username || '')
                }}
                className='h-4 px-1.5 text-[10px] font-mono cursor-copy border-slate-300 dark:border-slate-700 flex items-center gap-1 hover:border-primary'
                title='点击复制上游登录账号'
              >
                <UserCheck className='size-2.5 text-primary' />
                <span className='truncate max-w-[130px]'>
                  {source.upstream_username}
                </span>
              </Badge>
            )}

            {source.upstream_password && (
              <Badge
                variant='outline'
                onClick={() => {
                  copyToClipboard(source.upstream_password || '')
                }}
                className='h-4 px-1.5 text-[10px] cursor-copy border-amber-300 text-amber-700 bg-amber-50/70 dark:border-amber-800 dark:text-amber-300 dark:bg-amber-950/40 flex items-center gap-0.5 hover:bg-amber-100'
                title='点击复制上游登录密码'
              >
                <KeyRound className='size-2.5' />
                <span>复制密码</span>
              </Badge>
            )}

            {source.cost_discount_scale !== undefined &&
              source.cost_discount_scale > 0 &&
              source.cost_discount_scale !== 1 && (
                <Badge
                  variant='outline'
                  className='h-4 px-1.5 text-[10px] font-mono border-violet-300 text-violet-700 bg-violet-50 dark:border-violet-800 dark:text-violet-300 dark:bg-violet-950/40 flex items-center gap-0.5'
                  title={`该站点充值存在虚拟刀/汇率折算系数 ×${source.cost_discount_scale}`}
                >
                  <Zap className='size-2.5' />
                  <span>折算×{source.cost_discount_scale}</span>
                </Badge>
              )}

            {source.wechat_name && (
              <Badge
                variant='outline'
                className='h-4 px-1.5 text-[10px] shrink-0 border-emerald-300 text-emerald-700 bg-emerald-50 dark:border-emerald-800 dark:text-emerald-300 dark:bg-emerald-950/40 flex items-center gap-0.5'
              >
                <MessageSquare className='size-2.5' />
                <span className='truncate max-w-[100px]'>
                  {source.wechat_name}
                </span>
              </Badge>
            )}
          </div>
        )}
      </div>

      {/* 2. 该地址下的分组列表与快捷分组成本倍率设置 */}
      <div className='space-y-1.5'>
        <div className='flex items-center justify-between'>
          <span className='text-[11px] font-semibold text-foreground flex items-center gap-1'>
            <Layers className='size-3 text-primary' />
            该地址下的分组与成本倍率 ({sortedGroups.length})
          </span>
          {isDirty ? (
            <Button
              type='button'
              size='sm'
              variant='outline'
              disabled={saveRatiosMutation.isPending}
              onClick={() => saveRatiosMutation.mutate()}
              className='h-5 px-2 text-[10px] border-primary text-primary hover:bg-primary/10 gap-1'
            >
              {saveRatiosMutation.isPending ? (
                <Loader2 className='size-2.5 animate-spin' />
              ) : (
                <Check className='size-2.5' />
              )}
              保存倍率
            </Button>
          ) : (
            <span className='text-[10px] text-muted-foreground'>
              输入后回车或点保存
            </span>
          )}
        </div>

        <div className='rounded-lg border border-border/70 bg-muted/20 p-1.5 space-y-1 max-h-[210px] overflow-y-auto'>
          {sortedGroups.length === 0 ? (
            <div className='py-4 text-center text-[11px] text-muted-foreground'>
              暂无关联分组
            </div>
          ) : (
            sortedGroups.map((grp) => {
              const family = inferGroupFamily(
                grp,
                undefined,
                source.group_models?.[grp]
              )
              const grpQuota = source.group_incomes?.[grp]?.today_quota || 0
              const effectiveQuota =
                grpQuota > 0
                  ? grpQuota
                  : sortedGroups.length === 1
                    ? source.today_quota || 0
                    : 0
              const custRatio = source.group_customer_ratios?.[grp]
              const valStr = draftRatios[grp] ?? ''
              const costNum =
                valStr.trim() !== ''
                  ? parseFloat(valStr)
                  : source.default_cost_ratio || NaN
              const isInverted =
                !isNaN(costNum) &&
                custRatio !== undefined &&
                custRatio > 0 &&
                !grp.includes('按次') &&
                costNum > custRatio

              return (
                <div
                  key={grp}
                  className={cn(
                    'flex items-center justify-between gap-1.5 rounded-md border px-2 py-1 whitespace-nowrap transition-colors',
                    isInverted
                      ? 'border-rose-400/60 bg-rose-500/5 hover:bg-rose-500/10'
                      : 'border-border/50 bg-background/80 hover:bg-muted/40'
                  )}
                >
                  <div className='flex items-center gap-1.5 min-w-0 flex-1 overflow-hidden'>
                    <Badge
                      variant='outline'
                      className={cn(
                        'h-4 w-[46px] justify-center px-1 text-[9px] shrink-0 font-medium',
                        family.badgeClassName
                      )}
                    >
                      {family.name.split(' / ')[0]}
                    </Badge>
                    <div className='min-w-0 flex-1 overflow-hidden'>
                      <GroupBadge
                        group={grp}
                        copyable={true}
                        className='max-w-full truncate text-[11px] font-medium select-text'
                      />
                    </div>
                  </div>

                  <div className='flex items-center gap-1.5 shrink-0'>
                    <span
                      className={cn(
                        'inline-flex w-[108px] shrink-0 items-center justify-between gap-1 rounded border px-1.5 py-0.5 font-mono text-[10px] tabular-nums whitespace-nowrap',
                        effectiveQuota > 0
                          ? 'bg-emerald-500/10 font-semibold text-emerald-600 dark:text-emerald-400 border-emerald-500/20'
                          : 'bg-muted/60 text-muted-foreground font-normal border-border/50'
                      )}
                      title='该分组在此地址下的当天消耗收入'
                    >
                      <span className='font-sans text-[9px] opacity-80'>
                        当天
                      </span>
                      <span>
                        {effectiveQuota > 0
                          ? formatLogQuota(effectiveQuota)
                          : '$0'}
                      </span>
                    </span>
                    <span
                      className={cn(
                        'inline-flex w-[48px] shrink-0 items-center justify-end text-[10px] font-mono tabular-nums whitespace-nowrap',
                        isInverted
                          ? 'text-rose-600 font-bold'
                          : 'text-muted-foreground'
                      )}
                      title={
                        isInverted
                          ? `⚠️ 成本倍率 (${costNum}x) 高于对客倍率 (${custRatio}x)，存在价格倒挂！`
                          : '平台对该分组的默认对客倍率'
                      }
                    >
                      {custRatio !== undefined ? `售${custRatio}x` : '—'}
                    </span>
                    <div className='relative w-[66px] shrink-0'>
                      <Input
                        type='number'
                        step='0.001'
                        min='0'
                        placeholder={
                          source.default_cost_ratio
                            ? `${source.default_cost_ratio}`
                            : '成本倍率'
                        }
                        value={valStr}
                        onChange={(e) => {
                          const v = e.target.value
                          setDraftRatios((prev) => ({ ...prev, [grp]: v }))
                          setIsDirty(true)
                        }}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') {
                            e.preventDefault()
                            saveRatiosMutation.mutate()
                          }
                        }}
                        className={cn(
                          'h-5.5 pr-3.5 pl-1 text-[10px] md:text-[10px] placeholder:text-[10px] font-mono text-right bg-background rounded-md',
                          isInverted
                            ? 'border-rose-500 text-rose-700 dark:text-rose-300 font-bold'
                            : valStr.trim() !== '' &&
                                'border-indigo-400/70 font-semibold text-indigo-700 dark:text-indigo-300'
                        )}
                      />
                      <span className='absolute right-1 top-1/2 -translate-y-1/2 text-[9px] text-muted-foreground font-mono'>
                        x
                      </span>
                    </div>
                  </div>
                </div>
              )
            })
          )}
        </div>
      </div>

      {/* 3. 模型成本单价配置概览与入口 */}
      {(() => {
        // 将规则展开为按单个模型维度的展示项，并自动推导 group='*' 在当前地址下实际归属的分组
        const resolveGroupsForModel = (ruleGroup: string, modelName: string): string[] => {
          if (ruleGroup && ruleGroup !== '*') {
            return [ruleGroup]
          }
          if (!source.group_models) return []
          const matched: string[] = []
          for (const g of sortedGroups) {
            const gModels = source.group_models[g] || []
            if (gModels.includes(modelName)) {
              matched.push(g)
            }
          }
          return matched
        }

        const expandedItems = rules.flatMap((r, ruleIdx) => {
          const models = r.models && r.models.length > 0 ? r.models : ['*']
          return models.map((m) => ({
            key: `${ruleIdx}-${m}`,
            model: m,
            rule: r,
            matchedGroups: resolveGroupsForModel(r.group, m),
          }))
        })

        return (
          <div className='space-y-1.5 pt-0.5'>
            <div className='flex items-center justify-between'>
              <span className='text-[11px] font-semibold text-foreground flex items-center gap-1'>
                <DollarSign className='size-3 text-emerald-600' />
                模型成本单价 ({expandedItems.length})
              </span>
              <button
                type='button'
                onClick={() => onOpenFullConfig(source)}
                className='text-[11px] font-medium text-primary hover:underline flex items-center gap-0.5 cursor-pointer'
              >
                <Plus className='size-3' />
                设置模型成本单价
              </button>
            </div>

            {expandedItems.length === 0 ? (
              <div
                onClick={() => onOpenFullConfig(source)}
                className='rounded-lg border border-dashed border-border/80 bg-muted/15 px-3 py-2 text-center text-[11px] whitespace-nowrap text-muted-foreground hover:border-primary/50 hover:text-foreground cursor-pointer transition-colors'
              >
                暂未配置专属模型成本单价，点击配置按次($/次)、按量($/M)或模型倍率(x)
              </div>
            ) : (
              <div className='rounded-lg border border-border/70 bg-muted/20 p-1.5 space-y-1 max-h-36 overflow-y-auto'>
                {expandedItems.map(({ key, model, rule: r, matchedGroups }) => {
                  const primaryGroup = matchedGroups[0]
                  const extraGroupCount =
                    matchedGroups.length > 1 ? matchedGroups.length - 1 : 0
                  const groupTooltip =
                    r.group === '*'
                      ? matchedGroups.length > 0
                        ? `全局规则 (当前地址下匹配分组: ${matchedGroups.join(', ')})`
                        : '全部关联分组通用'
                      : `指定分组: ${r.group}`

                  return (
                    <div
                      key={key}
                      onClick={() => onOpenFullConfig(source)}
                      title={`点击编辑 · ${groupTooltip} · 模型: ${model}`}
                      className='flex items-center justify-between gap-1.5 rounded-md border border-border/50 bg-background/80 px-2 py-1 text-[11px] whitespace-nowrap cursor-pointer hover:bg-muted/40 transition-colors'
                    >
                      <div className='flex items-center gap-1.5 min-w-0 flex-1 overflow-hidden'>
                        <div
                          className='w-[98px] shrink-0 flex items-center gap-1 overflow-hidden'
                          title={groupTooltip}
                        >
                          {primaryGroup ? (
                            <>
                              <GroupBadge
                                group={primaryGroup}
                                className='max-w-full truncate text-[10px] font-medium'
                              />
                              {extraGroupCount > 0 && (
                                <span className='text-[9px] font-mono text-muted-foreground shrink-0'>
                                  +{extraGroupCount}
                                </span>
                              )}
                            </>
                          ) : (
                            <Badge
                              variant='outline'
                              className='h-4 px-1.5 text-[10px] font-medium shrink-0 bg-muted/50 text-muted-foreground border-border/60'
                            >
                              全部组
                            </Badge>
                          )}
                        </div>
                        <span className='font-mono text-[11px] font-medium truncate text-foreground/90'>
                          {model}
                        </span>
                      </div>

                      <span
                        className={cn(
                          'inline-flex shrink-0 items-center gap-0.5 rounded border px-1.5 py-0.5 font-mono text-[10px] font-semibold tabular-nums',
                          r.mode === 'per_token'
                            ? 'bg-sky-500/10 text-sky-700 dark:text-sky-300 border-sky-500/25'
                            : r.mode === 'ratio'
                              ? 'bg-violet-500/10 text-violet-700 dark:text-violet-300 border-violet-500/25'
                              : 'bg-amber-500/10 text-amber-700 dark:text-amber-300 border-amber-500/25'
                        )}
                      >
                        {r.mode === 'per_token' ? (
                          <>
                            <Coins className='size-2.5' />入${r.input_price ?? 0}
                            /出${r.output_price ?? 0}/M
                          </>
                        ) : r.mode === 'ratio' ? (
                          <>
                            <Zap className='size-2.5' />
                            {r.cost_ratio ?? 0}x
                          </>
                        ) : (
                          <>
                            <Sparkles className='size-2.5' />${r.price ?? 0}/次
                          </>
                        )}
                      </span>
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        )
      })()}
    </div>
  )
}
