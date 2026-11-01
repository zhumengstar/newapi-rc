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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeft,
  CheckCheck,
  ChevronDown,
  ChevronUp,
  Coins,
  Copy,
  DollarSign,
  Eye,
  EyeOff,
  Globe,
  KeyRound,
  Layers,
  Loader2,
  LogIn,
  MessageSquare,
  Plus,
  RefreshCw,
  Server,
  Sparkles,
  Trash2,
  TrendingUp,
  UserCheck,
  X,
  Zap,
} from 'lucide-react'
import { useState, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { GroupBadge } from '@/components/group-badge'
import { MultiSelect } from '@/components/multi-select'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { inferGroupFamily } from '@/features/users/lib'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { formatLogQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import { probeUpstreamSource, updateChannelSource } from '../api'
import {
  openUpstreamOneClickLogin,
  resolveUpstreamConsoleUrl,
} from '../lib/one-click-login'
import type { SourceModelCostRule, UpstreamSourceDTO } from '../types'

interface QuickSourceDialogProps {
  source: UpstreamSourceDTO | null
  allGroups?: string[]
  open: boolean
  onOpenChange: (open: boolean) => void
  onBack?: () => void
}

export function QuickSourceDialog({
  source,
  open,
  onOpenChange,
  onBack,
}: QuickSourceDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { copyToClipboard } = useCopyToClipboard()

  const [wechatName, setWechatName] = useState('')
  const [dashboardUrl, setDashboardUrl] = useState('')
  const [upstreamUsername, setUpstreamUsername] = useState('')
  const [upstreamPassword, setUpstreamPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [upstreamPlatform, setUpstreamPlatform] = useState<string>('auto')
  const [costDiscountScale, setCostDiscountScale] = useState<string>('1')
  const [upstreamBalance, setUpstreamBalance] = useState<string>('')
  const [defaultCostRatio, setDefaultCostRatio] = useState<string>('')
  const [costTier, setCostTier] = useState<number>(0)
  const [groupCostRatios, setGroupCostRatios] = useState<
    Record<string, string>
  >({})
  const [channelCostRatios, setChannelCostRatios] = useState<
    Record<number, string>
  >({})
  const [channelStatuses, setChannelStatuses] = useState<
    Record<number, number>
  >({})
  const [showChannelsPanel, setShowChannelsPanel] = useState(false)
  const [modelCostRules, setModelCostRules] = useState<SourceModelCostRule[]>(
    []
  )

  // 新增模型成本规则草稿状态
  const [draftGroup, setDraftGroup] = useState<string>('*')
  const [draftMode, setDraftMode] = useState<
    'per_call' | 'per_token' | 'ratio'
  >('per_call')
  const [draftModels, setDraftModels] = useState<string[]>([])
  const [customModelInput, setCustomModelInput] = useState<string>('')
  const [draftCallPrice, setDraftCallPrice] = useState<string>('')
  const [draftInputPrice, setDraftInputPrice] = useState<string>('')
  const [draftOutputPrice, setDraftOutputPrice] = useState<string>('')
  const [draftModelRatio, setDraftModelRatio] = useState<string>('')
  const [expandedRuleIdx, setExpandedRuleIdx] = useState<
    Record<number, boolean>
  >({})

  useEffect(() => {
    if (source && open) {
      setWechatName(source.wechat_name || '')
      setDashboardUrl(
        source.dashboard_url || resolveUpstreamConsoleUrl(source) || ''
      )
      setUpstreamUsername(source.upstream_username || '')
      setUpstreamPassword(source.upstream_password || '')
      setShowPassword(false)
      setUpstreamPlatform(source.upstream_platform || 'auto')
      setCostDiscountScale(
        source.cost_discount_scale && source.cost_discount_scale > 0
          ? String(source.cost_discount_scale)
          : '1'
      )
      setUpstreamBalance(
        source.upstream_balance !== undefined &&
          source.upstream_balance !== null
          ? String(source.upstream_balance)
          : ''
      )
      const defRatio =
        source.default_cost_ratio ??
        (source.channel_ratio > 0 ? source.channel_ratio : undefined)
      setDefaultCostRatio(
        defRatio !== undefined && defRatio > 0 ? String(defRatio) : ''
      )
      setCostTier(source.cost_tier || 0)

      // 初始化各分组成本倍率
      const initGroupRatios: Record<string, string> = {}
      if (source.group_cost_ratios) {
        for (const [g, r] of Object.entries(source.group_cost_ratios)) {
          if (typeof r === 'number' && r >= 0) {
            initGroupRatios[g] = String(r)
          }
        }
      }
      setGroupCostRatios(initGroupRatios)

      // 初始化下属渠道独立倍率与启停状态
      const initChRatios: Record<number, string> = {}
      const initChStatuses: Record<number, number> = {}
      if (source.channels) {
        for (const ch of source.channels) {
          initChStatuses[ch.id] = ch.status
          const savedOverride = source.channel_cost_ratios?.[ch.id]
          if (typeof savedOverride === 'number' && savedOverride > 0) {
            initChRatios[ch.id] = String(savedOverride)
          } else if (ch.channel_ratio > 0) {
            initChRatios[ch.id] = String(ch.channel_ratio)
          }
        }
      }
      setChannelCostRatios(initChRatios)
      setChannelStatuses(initChStatuses)
      setShowChannelsPanel(false)

      // 初始化模型成本规则
      if (source.model_cost_rules && source.model_cost_rules.length > 0) {
        setModelCostRules(
          source.model_cost_rules.map((r) => ({
            group: r.group || '*',
            models: [...(r.models || [])],
            mode:
              r.mode === 'per_token'
                ? 'per_token'
                : r.mode === 'ratio'
                  ? 'ratio'
                  : 'per_call',
            price: r.price,
            input_price: r.input_price,
            output_price: r.output_price,
            cost_ratio: r.cost_ratio,
            cache_read_ratio: r.cache_read_ratio,
            cache_create_ratio: r.cache_create_ratio,
          }))
        )
      } else if (
        source.model_prices &&
        Object.keys(source.model_prices).length > 0
      ) {
        const byPrice: Record<string, string[]> = {}
        for (const [m, p] of Object.entries(source.model_prices)) {
          if (typeof p === 'number' && p >= 0) {
            const k = String(p)
            if (!byPrice[k]) byPrice[k] = []
            byPrice[k].push(m)
          }
        }
        setModelCostRules(
          Object.entries(byPrice).map(([pStr, ms]) => ({
            group: '*',
            models: ms.sort(),
            mode: 'per_call',
            price: Number(pStr),
          }))
        )
      } else {
        setModelCostRules([])
      }

      const perCallGrp = (source.groups || []).find((g) => g.includes('按次'))
      setDraftGroup(perCallGrp || '*')
      setDraftMode('per_call')
      setDraftModels([])
      setCustomModelInput('')
      setDraftCallPrice('')
      setDraftInputPrice('')
      setDraftOutputPrice('')
      setDraftModelRatio('')
      setExpandedRuleIdx({})
    }
  }, [source, open])

  // 该来源下的所有分组（按今日收入从高到低排序）
  const sourceGroups = useMemo(() => {
    if (!source) return []
    const set = new Set<string>(source.groups || [])
    if (source.group_incomes) {
      for (const g of Object.keys(source.group_incomes)) {
        if (g) set.add(g)
      }
    }
    for (const g of Object.keys(groupCostRatios)) {
      if (g) set.add(g)
    }
    return Array.from(set).sort((a, b) => {
      const incA = source.group_incomes?.[a]?.today_income || 0
      const incB = source.group_incomes?.[b]?.today_income || 0
      if (incB !== incA) return incB - incA
      return a.localeCompare(b)
    })
  }, [source, groupCostRatios])

  // 当前选中的草稿分组下可选的模型列表
  const availableModelsForDraftGroup = useMemo(() => {
    if (!source) return []
    if (
      draftGroup &&
      draftGroup !== '*' &&
      source.group_models?.[draftGroup]?.length
    ) {
      return source.group_models[draftGroup]
    }
    return source.models || []
  }, [source, draftGroup])

  const handleGroupRatioChange = (group: string, val: string) => {
    setGroupCostRatios((prev) => {
      const next = { ...prev }
      if (val.trim() === '') {
        delete next[group]
      } else {
        next[group] = val
      }
      return next
    })
  }

  const handleApplyDefaultToAllGroups = () => {
    const defVal = defaultCostRatio.trim()
    if (!defVal || isNaN(Number(defVal)) || Number(defVal) < 0) {
      toast.warning('请先输入有效的默认兜底成本倍率')
      return
    }
    setGroupCostRatios((prev) => {
      const next = { ...prev }
      for (const g of sourceGroups) {
        if (next[g] === undefined || next[g].trim() === '') {
          next[g] = defVal
        }
      }
      return next
    })
    toast.success('已将默认成本倍率填入所有未单独设置的分组')
  }

  const handleApplyDiscountScaleToCurrentRatios = (scaleFactor: number) => {
    if (!scaleFactor || scaleFactor <= 0 || scaleFactor === 1) return
    setCostDiscountScale(String(scaleFactor))
    if (defaultCostRatio.trim() !== '') {
      const n = parseFloat(defaultCostRatio)
      if (!isNaN(n) && n > 0) {
        setDefaultCostRatio(String(Math.round(n * scaleFactor * 10000) / 10000))
      }
    }
    setGroupCostRatios((prev) => {
      const next: Record<string, string> = {}
      for (const [g, v] of Object.entries(prev)) {
        const n = parseFloat(v)
        if (!isNaN(n) && n >= 0) {
          next[g] = String(Math.round(n * scaleFactor * 10000) / 10000)
        } else {
          next[g] = v
        }
      }
      return next
    })
    setModelCostRules((prev) =>
      prev.map((r) => ({
        ...r,
        price:
          r.price !== undefined
            ? Math.round(r.price * scaleFactor * 10000) / 10000
            : undefined,
        input_price:
          r.input_price !== undefined
            ? Math.round(r.input_price * scaleFactor * 10000) / 10000
            : undefined,
        output_price:
          r.output_price !== undefined
            ? Math.round(r.output_price * scaleFactor * 10000) / 10000
            : undefined,
        cost_ratio:
          r.cost_ratio !== undefined
            ? Math.round(r.cost_ratio * scaleFactor * 10000) / 10000
            : undefined,
      }))
    )
    toast.success(`已按折算系数 ×${scaleFactor} 自动折算当前分组倍率与模型单价`)
  }

  const handleAddModelCostRule = () => {
    const manualModels = customModelInput
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    const combinedModels = Array.from(
      new Set([...draftModels, ...manualModels])
    ).sort()

    if (combinedModels.length === 0) {
      toast.warning('请至少选择或输入一个模型名称')
      return
    }

    if (draftMode === 'per_call') {
      const p = parseFloat(draftCallPrice)
      if (isNaN(p) || p < 0) {
        toast.warning('请输入有效的按次成本单价 ($/次)')
        return
      }
      setModelCostRules((prev) => {
        const cleaned = prev
          .map((r) => {
            if (r.group === draftGroup) {
              return {
                ...r,
                models: r.models.filter((m) => !combinedModels.includes(m)),
              }
            }
            return r
          })
          .filter((r) => r.models.length > 0)

        return [
          ...cleaned,
          {
            group: draftGroup || '*',
            models: combinedModels,
            mode: 'per_call',
            price: p,
          },
        ]
      })
      setDraftModels([])
      setCustomModelInput('')
      setDraftCallPrice('')
    } else if (draftMode === 'ratio') {
      const ratioVal = parseFloat(draftModelRatio)
      if (isNaN(ratioVal) || ratioVal < 0) {
        toast.warning('请输入有效的模型独立成本倍率 (x)')
        return
      }
      setModelCostRules((prev) => {
        const cleaned = prev
          .map((r) => {
            if (r.group === draftGroup) {
              return {
                ...r,
                models: r.models.filter((m) => !combinedModels.includes(m)),
              }
            }
            return r
          })
          .filter((r) => r.models.length > 0)

        return [
          ...cleaned,
          {
            group: draftGroup || '*',
            models: combinedModels,
            mode: 'ratio',
            cost_ratio: ratioVal,
          },
        ]
      })
      setDraftModels([])
      setCustomModelInput('')
      setDraftModelRatio('')
    } else {
      const inP = draftInputPrice.trim() ? parseFloat(draftInputPrice) : 0
      const outP = draftOutputPrice.trim() ? parseFloat(draftOutputPrice) : 0
      if (
        isNaN(inP) ||
        inP < 0 ||
        isNaN(outP) ||
        outP < 0 ||
        (draftInputPrice.trim() === '' && draftOutputPrice.trim() === '')
      ) {
        toast.warning('请输入有效的输入或输出 Token 成本单价 ($/M)')
        return
      }
      setModelCostRules((prev) => {
        const cleaned = prev
          .map((r) => {
            if (r.group === draftGroup) {
              return {
                ...r,
                models: r.models.filter((m) => !combinedModels.includes(m)),
              }
            }
            return r
          })
          .filter((r) => r.models.length > 0)

        return [
          ...cleaned,
          {
            group: draftGroup || '*',
            models: combinedModels,
            mode: 'per_token',
            input_price: inP,
            output_price: outP,
          },
        ]
      })
      setDraftModels([])
      setCustomModelInput('')
      setDraftInputPrice('')
      setDraftOutputPrice('')
    }
  }

  const handleUpdateRulePrice = (
    idx: number,
    field: 'price' | 'input_price' | 'output_price' | 'cost_ratio',
    valStr: string
  ) => {
    const num = parseFloat(valStr)
    setModelCostRules((prev) =>
      prev.map((r, i) => {
        if (i !== idx) return r
        return {
          ...r,
          [field]: isNaN(num) || num < 0 ? 0 : num,
        }
      })
    )
  }

  const handleRemoveRule = (idx: number) => {
    setModelCostRules((prev) => prev.filter((_, i) => i !== idx))
  }

  const probeMutation = useMutation({
    mutationFn: async () => {
      if (!source) throw new Error('未选择来源')
      const scaleNum = parseFloat(costDiscountScale)
      return await probeUpstreamSource({
        base_url: source.base_url,
        dashboard_url: dashboardUrl.trim(),
        upstream_username: upstreamUsername.trim(),
        upstream_password: upstreamPassword.trim(),
        upstream_platform: upstreamPlatform,
        cost_discount_scale: !isNaN(scaleNum) && scaleNum > 0 ? scaleNum : 1,
      })
    },
    onSuccess: (res) => {
      const d = res.data
      if (d.upstream_balance !== undefined && d.upstream_balance !== null) {
        setUpstreamBalance(String(d.upstream_balance))
      }
      if (d.platform && d.platform !== 'auto') {
        setUpstreamPlatform(d.platform)
      }
      const detectedRatios = d.detected_group_ratios || {}
      const ratioEntries = Object.entries(detectedRatios)
      if (ratioEntries.length > 0) {
        setGroupCostRatios((prev) => {
          const next = { ...prev }
          for (const [g, r] of ratioEntries) {
            next[g] = String(r)
          }
          return next
        })
      }
      toast.success(res.message || '探测上游余额与真实倍率成功')
      queryClient.invalidateQueries({ queryKey: ['channel-sources-list'] })
    },
    onError: (err: any) => {
      toast.error(err?.message || '探测上游失败')
    },
  })

  const updateMutation = useMutation({
    mutationFn: async () => {
      if (!source) return
      const defRatioNum =
        defaultCostRatio.trim() !== '' ? parseFloat(defaultCostRatio) : 0
      const scaleNum =
        costDiscountScale.trim() !== '' ? parseFloat(costDiscountScale) : 1
      const balNum =
        upstreamBalance.trim() !== '' ? parseFloat(upstreamBalance) : undefined

      const parsedGroupRatios: Record<string, number> = {}
      for (const [g, valStr] of Object.entries(groupCostRatios)) {
        const num = parseFloat(valStr)
        if (!isNaN(num) && num >= 0) {
          parsedGroupRatios[g] = num
        }
      }

      const parsedChannelRatios: Record<number, number> = {}
      for (const [chIdStr, valStr] of Object.entries(channelCostRatios)) {
        const num = parseFloat(valStr)
        if (!isNaN(num) && num > 0) {
          parsedChannelRatios[Number(chIdStr)] = num
        }
      }

      await updateChannelSource({
        base_url: source.base_url,
        wechat_name: wechatName.trim(),
        dashboard_url: dashboardUrl.trim(),
        upstream_username: upstreamUsername.trim(),
        upstream_password: upstreamPassword.trim(),
        upstream_platform: upstreamPlatform,
        cost_discount_scale: !isNaN(scaleNum) && scaleNum > 0 ? scaleNum : 1,
        upstream_balance:
          balNum !== undefined && !isNaN(balNum) ? balNum : undefined,
        default_cost_ratio:
          !isNaN(defRatioNum) && defRatioNum >= 0 ? defRatioNum : 0,
        channel_ratio:
          !isNaN(defRatioNum) && defRatioNum >= 0 ? defRatioNum : 0,
        group_cost_ratios: parsedGroupRatios,
        channel_cost_ratios: parsedChannelRatios,
        channel_statuses: channelStatuses,
        model_cost_rules: modelCostRules,
        cost_tier: costTier,
      })
    },
    onSuccess: () => {
      toast.success(
        t(
          'Source cost configuration saved',
          '来源分组成本倍率、登录信息与模型成本单价已保存生效'
        )
      )
      queryClient.invalidateQueries({ queryKey: ['channel-sources-list'] })
      queryClient.invalidateQueries({ queryKey: ['channels'] })
      queryClient.invalidateQueries({ queryKey: ['exclusive-channels-list'] })
      onOpenChange(false)
    },
    onError: (err: any) => {
      toast.error(
        err?.message ||
          t('Failed to update source configuration', '更新来源成本配置失败')
      )
    },
  })

  if (!source) return null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[92vh] overflow-y-auto sm:max-w-3xl'>
        <DialogHeader>
          <div className='flex flex-wrap items-center justify-between gap-2 pr-6'>
            <div className='flex items-center gap-2'>
              {onBack && (
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={onBack}
                  className='h-7 px-2 text-xs gap-1 shrink-0'
                  title='返回上一弹窗'
                >
                  <ArrowLeft className='size-3.5' />
                  <span>返回</span>
                </Button>
              )}
              <DialogTitle className='flex items-center gap-2 text-base font-semibold'>
                <Globe className='size-4 text-primary' />
                <span>
                  {t(
                    'Configure Source Cost (Groups & Models)',
                    '配置来源分组成本倍率、控制台登录与模型单价'
                  )}
                </span>
              </DialogTitle>
            </div>

            <div className='flex items-center gap-1.5'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={probeMutation.isPending}
                onClick={() => probeMutation.mutate()}
                className='h-7 px-2.5 text-xs gap-1 border-emerald-500/40 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-500/10'
                title='自动登录上游拉取实时余额，并穿透 /api/log/token 解码真实扣费倍率'
              >
                <RefreshCw
                  className={cn(
                    'size-3',
                    probeMutation.isPending && 'animate-spin'
                  )}
                />
                <span>自动探测余额与真实扣费</span>
              </Button>

              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={() =>
                  window.open(
                    '/api/channel/sources/autologin.user.js',
                    '_blank'
                  )
                }
                className='h-7 px-2 text-[11px] text-muted-foreground hover:text-primary gap-1'
                title='安装油猴 (Tampermonkey) 免密自动登录脚本，点击任意上游域名即可全自动免密直达后台'
              >
                <Zap className='size-3 text-amber-500' />
                <span>安装免密脚本</span>
              </Button>
            </div>
          </div>

          <DialogDescription className='flex flex-wrap items-center gap-2 text-xs text-muted-foreground pt-1'>
            <span>{t('Upstream Address', '来源地址')}:</span>
            <button
              type='button'
              onClick={() =>
                openUpstreamOneClickLogin(source, {
                  dashboardUrl,
                  username: upstreamUsername,
                  password: upstreamPassword,
                  platform: upstreamPlatform,
                })
              }
              className='group inline-flex items-center gap-1.5 font-mono font-semibold text-foreground bg-muted hover:bg-primary/10 hover:text-primary hover:border-primary/40 px-2 py-0.5 rounded border border-border/60 cursor-pointer transition-colors'
              title='点击一键登录该上游控制台'
            >
              <span>{source.display_base_url}</span>
              <span className='inline-flex items-center gap-0.5 rounded bg-primary text-primary-foreground px-1.5 py-0 text-[10px] font-sans font-medium'>
                <LogIn className='size-2.5' />
                一键登录
              </span>
            </button>

            <Badge variant='outline' className='text-[10px] font-mono'>
              {source.channel_count} 个关联渠道 ({source.enabled_channel_count}{' '}
              启用)
            </Badge>

            {source.today_income !== undefined && source.today_income > 0 && (
              <Badge
                variant='outline'
                className='text-[10px] font-mono border-emerald-300 text-emerald-700 bg-emerald-50 dark:border-emerald-800 dark:text-emerald-300 dark:bg-emerald-950/40'
              >
                今日消耗 ${source.today_income.toFixed(2)}
              </Badge>
            )}

            {source.today_estimated_cost !== undefined &&
              source.today_estimated_cost > 0 && (
                <Badge
                  variant='outline'
                  className='text-[10px] font-mono border-indigo-300 text-indigo-700 bg-indigo-50 dark:border-indigo-800 dark:text-indigo-300 dark:bg-indigo-950/40'
                >
                  预计成本 ~${source.today_estimated_cost.toFixed(2)}
                </Badge>
              )}

            {source.today_gross_profit !== undefined &&
              source.today_income !== undefined &&
              source.today_income > 0 &&
              source.today_estimated_cost !== undefined &&
              source.today_estimated_cost > 0 && (
                <Badge
                  variant='outline'
                  className={cn(
                    'text-[10px] font-mono gap-0.5',
                    source.today_gross_profit >= 0
                      ? 'border-emerald-300 text-emerald-700 bg-emerald-50 dark:border-emerald-800 dark:text-emerald-300 dark:bg-emerald-950/40'
                      : 'border-rose-300 text-rose-700 bg-rose-50 dark:border-rose-800 dark:text-rose-300 dark:bg-rose-950/40'
                  )}
                >
                  <TrendingUp className='size-2.5' />
                  {source.today_gross_profit >= 0
                    ? `净毛利 +$${source.today_gross_profit.toFixed(2)} (${source.today_gross_margin ?? 0}%)`
                    : `倒挂亏损 -$${Math.abs(source.today_gross_profit).toFixed(2)}`}
                </Badge>
              )}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-4 py-1'>
          {/* 1. 供应商微信备注 + 默认兜底倍率 + 充值汇率折算系数 + 控制台一键登录信息 */}
          <div className='rounded-xl border border-border/70 bg-muted/20 p-3 space-y-3'>
            <div className='grid grid-cols-1 gap-3 sm:grid-cols-4'>
              <div className='space-y-1.5'>
                <Label className='text-xs font-medium flex items-center gap-1.5'>
                  <MessageSquare className='size-3.5 text-emerald-600' />
                  <span>供应商微信/备注</span>
                </Label>
                <Input
                  placeholder='如: 张三(总代)、老王专供'
                  value={wechatName}
                  onChange={(e) => setWechatName(e.target.value)}
                  className='h-8 text-xs bg-background'
                />
              </div>

              <div className='space-y-1.5'>
                <div className='flex items-center justify-between'>
                  <Label className='text-xs font-medium flex items-center gap-1'>
                    <Coins className='size-3.5 text-amber-500' />
                    <span>默认兜底成本倍率</span>
                  </Label>
                  {defaultCostRatio.trim() !== '' &&
                    sourceGroups.length > 0 && (
                      <button
                        type='button'
                        onClick={handleApplyDefaultToAllGroups}
                        className='text-[10px] text-primary hover:underline cursor-pointer'
                      >
                        填入未设分组
                      </button>
                    )}
                </div>
                <div className='relative'>
                  <Input
                    type='number'
                    step='0.001'
                    min='0'
                    placeholder='如 0.12 (1.2折)'
                    value={defaultCostRatio}
                    onChange={(e) => setDefaultCostRatio(e.target.value)}
                    className='h-8 pr-6 text-xs font-mono bg-background'
                  />
                  <span className='absolute right-2 top-1/2 -translate-y-1/2 text-xs text-muted-foreground font-mono'>
                    x
                  </span>
                </div>
              </div>

              <div className='space-y-1.5'>
                <div className='flex items-center justify-between'>
                  <Label
                    className='text-xs font-medium flex items-center gap-1'
                    title='例如创想AI/Yottsuno充值1:10虚拟刀，系数设为0.1'
                  >
                    <Zap className='size-3.5 text-violet-500' />
                    <span>充值汇率/虚拟刀系数</span>
                  </Label>
                  <div className='flex items-center gap-1'>
                    <button
                      type='button'
                      onClick={() => setCostDiscountScale('1')}
                      className={cn(
                        'text-[10px] px-1 rounded border cursor-pointer',
                        costDiscountScale === '1'
                          ? 'bg-primary text-primary-foreground border-primary'
                          : 'text-muted-foreground border-border hover:text-foreground'
                      )}
                    >
                      1:1
                    </button>
                    <button
                      type='button'
                      onClick={() => handleApplyDiscountScaleToCurrentRatios(0.1)}
                      className={cn(
                        'text-[10px] px-1 rounded border cursor-pointer',
                        costDiscountScale === '0.1'
                          ? 'bg-violet-600 text-white border-violet-600'
                          : 'text-violet-600 border-violet-300 hover:bg-violet-50'
                      )}
                      title='设置为 1:10 虚拟刀 (×0.1) 并自动将当前填写的账面倍率除以 10'
                    >
                      1:10(×0.1)
                    </button>
                  </div>
                </div>
                <Input
                  type='number'
                  step='0.01'
                  min='0.001'
                  placeholder='1 (标准1:1刀) 或 0.1 (1:10虚拟刀)'
                  value={costDiscountScale}
                  onChange={(e) => setCostDiscountScale(e.target.value)}
                  className='h-8 text-xs font-mono bg-background'
                />
              </div>

              <div className='space-y-1.5'>
                <Label className='text-xs font-medium flex items-center gap-1.5'>
                  <Layers className='size-3.5 text-blue-500' />
                  <span>成本优先级 & 实时余额($)</span>
                </Label>
                <div className='grid grid-cols-2 gap-1.5'>
                  <Input
                    type='number'
                    min='0'
                    max='10'
                    placeholder='Tier 0'
                    title='成本优先级 Cost Tier (0=默认)'
                    value={costTier}
                    onChange={(e) => setCostTier(parseInt(e.target.value) || 0)}
                    className='h-8 text-xs font-mono bg-background'
                  />
                  <div className='relative'>
                    <span className='absolute left-1.5 top-1/2 -translate-y-1/2 text-[10px] text-muted-foreground font-mono'>
                      $
                    </span>
                    <Input
                      type='number'
                      step='0.01'
                      placeholder='余额'
                      title='上游账户当前剩余美金余额 ($)'
                      value={upstreamBalance}
                      onChange={(e) => setUpstreamBalance(e.target.value)}
                      className='h-8 pl-4 pr-1 text-xs font-mono bg-background'
                    />
                  </div>
                </div>
              </div>
            </div>

            {/* 第二排：上游控制台登录地址 + 账号 + 密码 + 平台类型 */}
            <div className='grid grid-cols-1 gap-2.5 sm:grid-cols-12 pt-1 border-t border-border/50'>
              <div className='sm:col-span-5 space-y-1'>
                <div className='flex items-center justify-between'>
                  <Label className='text-[11px] font-medium flex items-center gap-1 text-muted-foreground'>
                    <Globe className='size-3 text-primary' />
                    <span>上游控制台登录地址 (支持与转发地址不同)</span>
                  </Label>
                  <button
                    type='button'
                    onClick={() =>
                      openUpstreamOneClickLogin(source, {
                        dashboardUrl,
                        username: upstreamUsername,
                        password: upstreamPassword,
                        platform: upstreamPlatform,
                      })
                    }
                    className='text-[10px] font-medium text-primary hover:underline flex items-center gap-0.5 cursor-pointer'
                  >
                    <LogIn className='size-2.5' />
                    测试一键登录
                  </button>
                </div>
                <Input
                  placeholder={source.display_base_url}
                  value={dashboardUrl}
                  onChange={(e) => setDashboardUrl(e.target.value)}
                  className='h-7.5 text-xs font-mono bg-background'
                />
              </div>

              <div className='sm:col-span-3 space-y-1'>
                <div className='flex items-center justify-between'>
                  <Label className='text-[11px] font-medium flex items-center gap-1 text-muted-foreground'>
                    <UserCheck className='size-3 text-emerald-600' />
                    <span>上游登录账号/邮箱</span>
                  </Label>
                  {upstreamUsername && (
                    <button
                      type='button'
                      onClick={() => copyToClipboard(upstreamUsername)}
                      className='text-[10px] text-muted-foreground hover:text-foreground cursor-pointer'
                      title='复制账号'
                    >
                      <Copy className='size-2.5' />
                    </button>
                  )}
                </div>
                <Input
                  placeholder='如 1378484330@qq.com'
                  value={upstreamUsername}
                  onChange={(e) => setUpstreamUsername(e.target.value)}
                  className='h-7.5 text-xs font-mono bg-background'
                />
              </div>

              <div className='sm:col-span-4 space-y-1'>
                <div className='flex items-center justify-between'>
                  <Label className='text-[11px] font-medium flex items-center gap-1 text-muted-foreground'>
                    <KeyRound className='size-3 text-amber-600' />
                    <span>上游登录密码</span>
                  </Label>
                  <div className='flex items-center gap-1.5'>
                    {upstreamPassword && (
                      <button
                        type='button'
                        onClick={() => copyToClipboard(upstreamPassword)}
                        className='text-[10px] text-muted-foreground hover:text-foreground cursor-pointer flex items-center gap-0.5'
                        title='复制密码'
                      >
                        <Copy className='size-2.5' />
                        <span>复制</span>
                      </button>
                    )}
                    <button
                      type='button'
                      onClick={() => setShowPassword(!showPassword)}
                      className='text-[10px] text-muted-foreground hover:text-foreground cursor-pointer'
                    >
                      {showPassword ? (
                        <EyeOff className='size-3' />
                      ) : (
                        <Eye className='size-3' />
                      )}
                    </button>
                  </div>
                </div>
                <Input
                  type={showPassword ? 'text' : 'password'}
                  placeholder='输入密码后点击域名即可一键免密登录'
                  value={upstreamPassword}
                  onChange={(e) => setUpstreamPassword(e.target.value)}
                  className='h-7.5 text-xs font-mono bg-background'
                />
              </div>
            </div>
          </div>

          {/* 2. 该地址下的分组成本倍率配置 */}
          <div className='space-y-2'>
            <div className='flex items-center justify-between'>
              <div>
                <Label className='text-xs font-semibold flex items-center gap-1.5'>
                  <Layers className='size-3.5 text-primary' />
                  <span>该地址下的分组成本倍率 ({sourceGroups.length})</span>
                </Label>
                <p className='text-[11px] text-muted-foreground mt-0.5'>
                  针对该地址下不同分组分别设定上游拿货真实折扣倍率（上游成本 =
                  请求刀 1x 官价 × 分组成本倍率）
                </p>
              </div>
              {Object.keys(groupCostRatios).length > 0 && (
                <Badge
                  variant='outline'
                  className='text-[10px] font-mono border-indigo-200 text-indigo-600 bg-indigo-50 dark:border-indigo-900 dark:text-indigo-300 dark:bg-indigo-950/40'
                >
                  已设 {Object.keys(groupCostRatios).length}/
                  {sourceGroups.length} 个分组
                </Badge>
              )}
            </div>

            <div className='rounded-xl border border-border/80 bg-card/50 divide-y divide-border/50 overflow-hidden max-h-56 overflow-y-auto'>
              {sourceGroups.length === 0 ? (
                <div className='py-6 text-center text-xs text-muted-foreground'>
                  该来源地址下暂无关联分组
                </div>
              ) : (
                sourceGroups.map((grp) => {
                  const family = inferGroupFamily(
                    grp,
                    undefined,
                    source.group_models?.[grp]
                  )
                  const chCount = source.group_channel_counts?.[grp] || 0
                  const grpQuota = source.group_incomes?.[grp]?.today_quota || 0
                  const effectiveQuota =
                    grpQuota > 0
                      ? grpQuota
                      : sourceGroups.length === 1
                        ? source.today_quota || 0
                        : 0
                  const effectiveIncome = effectiveQuota / 500000
                  const custRatio = source.group_customer_ratios?.[grp]
                  const costStr = groupCostRatios[grp] ?? ''
                  const costNum =
                    costStr.trim() !== ''
                      ? parseFloat(costStr)
                      : defaultCostRatio.trim() !== ''
                        ? parseFloat(defaultCostRatio)
                        : NaN
                  const hasExplicitCost = costStr.trim() !== ''
                  const marginPct =
                    !isNaN(costNum) &&
                    custRatio !== undefined &&
                    custRatio > 0
                      ? Math.round(((custRatio - costNum) / custRatio) * 100)
                      : null
                  const estGroupCost =
                    !isNaN(costNum) &&
                    effectiveIncome > 0 &&
                    custRatio !== undefined &&
                    custRatio > 0
                      ? grp.includes('按次') && custRatio >= 0.8 && costNum < 0.8
                        ? effectiveIncome * costNum
                        : effectiveIncome * (costNum / custRatio)
                      : null

                  return (
                    <div
                      key={grp}
                      className='flex items-center justify-between gap-3 px-3 py-2 hover:bg-muted/20 transition-colors'
                    >
                      <div className='flex items-center gap-2 min-w-0 flex-1 flex-wrap'>
                        <Badge
                          variant='outline'
                          className={cn(
                            'h-4 px-1.5 text-[10px] shrink-0 font-normal',
                            family.badgeClassName
                          )}
                        >
                          {family.name.split(' / ')[0]}
                        </Badge>

                        <GroupBadge
                          group={grp}
                          copyable={false}
                          className='text-xs font-medium'
                        />

                        {chCount > 0 && (
                          <span className='text-[10px] text-muted-foreground font-mono'>
                            {chCount}渠
                          </span>
                        )}

                        <Badge
                          variant='outline'
                          className={cn(
                            'h-4 px-1.5 text-[10px] font-mono',
                            effectiveQuota > 0
                              ? 'border-emerald-200 text-emerald-700 bg-emerald-50/70 dark:border-emerald-900 dark:text-emerald-300 dark:bg-emerald-950/30 font-semibold'
                              : 'border-border/60 text-muted-foreground bg-muted/40 font-normal'
                          )}
                        >
                          当天{' '}
                          {effectiveQuota > 0
                            ? formatLogQuota(effectiveQuota)
                            : '$0'}
                        </Badge>

                        {estGroupCost !== null && estGroupCost > 0 && (
                          <span className='text-[10px] font-mono text-indigo-600 dark:text-indigo-400'>
                            成本 ~${estGroupCost.toFixed(2)}
                          </span>
                        )}

                        {custRatio !== undefined && (
                          <span className='text-[11px] text-muted-foreground font-mono'>
                            对客: {custRatio}x
                          </span>
                        )}
                      </div>

                      <div className='flex items-center gap-2 shrink-0'>
                        {marginPct !== null && (
                          <Badge
                            variant='outline'
                            className={cn(
                              'h-5 px-1.5 text-[10px] font-mono',
                              marginPct >= 30
                                ? 'border-emerald-300 text-emerald-700 bg-emerald-50 dark:border-emerald-800 dark:text-emerald-300 dark:bg-emerald-950/40'
                                : marginPct >= 0
                                  ? 'border-amber-300 text-amber-700 bg-amber-50 dark:border-amber-800 dark:text-amber-300 dark:bg-amber-950/40'
                                  : 'border-rose-300 text-rose-700 bg-rose-50 dark:border-rose-800 dark:text-rose-300 dark:bg-rose-950/40'
                            )}
                            title='按对客默认倍率对比当前来源成本倍率的毛利空间'
                          >
                            {marginPct >= 0
                              ? `毛利 ~${marginPct}%`
                              : `⚠️ 倒挂 ${marginPct}%`}
                          </Badge>
                        )}

                        <div className='relative w-24'>
                          <Input
                            type='number'
                            step='0.001'
                            min='0'
                            placeholder={
                              defaultCostRatio.trim() !== ''
                                ? `默认 ${defaultCostRatio}`
                                : '成本倍率'
                            }
                            value={costStr}
                            onChange={(e) =>
                              handleGroupRatioChange(grp, e.target.value)
                            }
                            className={cn(
                              'h-6.5 pr-5 pl-1.5 text-[11px] md:text-[11px] placeholder:text-[10px] font-mono text-right bg-background',
                              hasExplicitCost &&
                                'border-primary/60 ring-1 ring-primary/15 font-semibold'
                            )}
                          />
                          <span className='absolute right-1.5 top-1/2 -translate-y-1/2 text-[10px] text-muted-foreground font-mono'>
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

          {/* 3. 下属关联渠道明细与单渠道独立倍率/启停控制 */}
          {source.channels && source.channels.length > 0 && (
            <div className='rounded-xl border border-border/80 bg-card/40 overflow-hidden'>
              <button
                type='button'
                onClick={() => setShowChannelsPanel(!showChannelsPanel)}
                className='flex w-full items-center justify-between px-3 py-2 text-xs font-semibold hover:bg-muted/30 cursor-pointer transition-colors'
              >
                <div className='flex items-center gap-1.5'>
                  <Server className='size-3.5 text-indigo-500' />
                  <span>
                    下属关联渠道明细 ({source.channels.length} 个渠道，可单独覆盖倍率或一键启停)
                  </span>
                </div>
                <div className='flex items-center gap-1 text-[11px] text-muted-foreground font-normal'>
                  <span>{showChannelsPanel ? '收起明细' : '展开设置'}</span>
                  {showChannelsPanel ? (
                    <ChevronUp className='size-3.5' />
                  ) : (
                    <ChevronDown className='size-3.5' />
                  )}
                </div>
              </button>

              {showChannelsPanel && (
                <div className='divide-y divide-border/50 border-t border-border/60 max-h-48 overflow-y-auto'>
                  {source.channels.map((ch) => {
                    const currentStatus = channelStatuses[ch.id] ?? ch.status
                    const isEnabled = currentStatus === 1
                    const chRatioStr = channelCostRatios[ch.id] ?? ''

                    return (
                      <div
                        key={ch.id}
                        className='flex items-center justify-between gap-2 px-3 py-1.5 text-xs hover:bg-muted/20'
                      >
                        <div className='flex items-center gap-2 min-w-0 flex-1'>
                          <button
                            type='button'
                            onClick={() =>
                              setChannelStatuses((prev) => ({
                                ...prev,
                                [ch.id]: isEnabled ? 2 : 1,
                              }))
                            }
                            className={cn(
                              'rounded px-1.5 py-0.5 text-[10px] font-mono font-medium border cursor-pointer shrink-0 transition-colors',
                              isEnabled
                                ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300 border-emerald-500/30 hover:bg-emerald-500/25'
                                : 'bg-muted text-muted-foreground border-border hover:text-foreground'
                            )}
                            title='点击切换该渠道启用/停用状态（保存后生效）'
                          >
                            {isEnabled ? '🟢 启用' : '⏸️ 停用'}
                          </button>

                          <span className='font-mono font-semibold text-muted-foreground shrink-0'>
                            #{ch.id}
                          </span>
                          <span
                            className='truncate font-medium text-foreground'
                            title={ch.name}
                          >
                            {ch.name}
                          </span>
                          <Badge
                            variant='outline'
                            className='h-4 px-1 text-[10px] font-mono shrink-0'
                          >
                            {ch.group}
                          </Badge>
                          {ch.today_quota > 0 && (
                            <span className='text-[10px] font-mono text-emerald-600 shrink-0'>
                              {formatLogQuota(ch.today_quota)}
                            </span>
                          )}
                        </div>

                        <div className='flex items-center gap-1.5 shrink-0'>
                          <span className='text-[10px] text-muted-foreground'>
                            单渠倍率:
                          </span>
                          <div className='relative w-20'>
                            <Input
                              type='number'
                              step='0.001'
                              min='0'
                              placeholder='跟随分组'
                              value={chRatioStr}
                              onChange={(e) => {
                                const val = e.target.value
                                setChannelCostRatios((prev) => ({
                                  ...prev,
                                  [ch.id]: val,
                                }))
                              }}
                              className='h-6 pr-4 pl-1.5 text-[11px] font-mono text-right bg-background'
                            />
                            <span className='absolute right-1 top-1/2 -translate-y-1/2 text-[9px] text-muted-foreground font-mono'>
                              x
                            </span>
                          </div>
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          )}

          {/* 4. 模型成本单价规则器 (支持按次 $/次 · 按量 $/M · 按模型倍率 x) */}
          <div className='space-y-2.5'>
            <div className='flex items-center justify-between'>
              <div>
                <Label className='text-xs font-semibold flex items-center gap-1.5'>
                  <DollarSign className='size-3.5 text-emerald-600' />
                  <span>
                    模型成本单价设置 (支持按次 $/次 · 按量 $/M · 按模型独立倍率
                    x)
                  </span>
                </Label>
                <p className='text-[11px] text-muted-foreground mt-0.5'>
                  优先级高于分组成本倍率；仅用于核算该来源的上游采购成本，不会修改全局对客售价
                </p>
              </div>
              {modelCostRules.length > 0 && (
                <Badge
                  variant='outline'
                  className='text-[10px] font-mono border-amber-300 text-amber-700 bg-amber-50 dark:border-amber-800 dark:text-amber-300 dark:bg-amber-950/40'
                >
                  {modelCostRules.length} 条模型成本规则
                </Badge>
              )}
            </div>

            <div className='rounded-xl border border-border/80 bg-card/50 p-3 space-y-3'>
              {/* Row 1: 适用分组 + 计费模式 (3种) */}
              <div className='grid grid-cols-1 gap-2.5 sm:grid-cols-12'>
                <div className='sm:col-span-5 space-y-1'>
                  <Label className='text-[11px] text-muted-foreground'>
                    适用分组范围
                  </Label>
                  <Select
                    value={draftGroup}
                    onValueChange={(val) => {
                      setDraftGroup(val || '*')
                      setDraftModels([])
                    }}
                  >
                    <SelectTrigger className='h-8 text-xs bg-background'>
                      <SelectValue>
                        {draftGroup === '*'
                          ? '🌐 该地址下全部分组通用 (*)'
                          : draftGroup}
                      </SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='*' className='text-xs font-medium'>
                        🌐 该地址下全部分组通用 (*)
                      </SelectItem>
                      {sourceGroups.map((g) => (
                        <SelectItem
                          key={g}
                          value={g}
                          className='text-xs font-mono'
                        >
                          {g} ({source.group_models?.[g]?.length || 0} 个模型)
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                <div className='sm:col-span-7 space-y-1'>
                  <Label className='text-[11px] text-muted-foreground'>
                    模型成本计费方式
                  </Label>
                  <div className='grid grid-cols-3 gap-1 rounded-lg border border-border/70 bg-background p-0.5 h-8'>
                    <button
                      type='button'
                      onClick={() => setDraftMode('per_call')}
                      className={cn(
                        'rounded-md text-xs font-medium transition-colors flex items-center justify-center gap-1 cursor-pointer',
                        draftMode === 'per_call'
                          ? 'bg-primary text-primary-foreground shadow-2xs'
                          : 'text-muted-foreground hover:text-foreground'
                      )}
                    >
                      <Sparkles className='size-3' />
                      按次 ($/次)
                    </button>
                    <button
                      type='button'
                      onClick={() => setDraftMode('per_token')}
                      className={cn(
                        'rounded-md text-xs font-medium transition-colors flex items-center justify-center gap-1 cursor-pointer',
                        draftMode === 'per_token'
                          ? 'bg-primary text-primary-foreground shadow-2xs'
                          : 'text-muted-foreground hover:text-foreground'
                      )}
                    >
                      <Coins className='size-3' />
                      按量 ($/M)
                    </button>
                    <button
                      type='button'
                      onClick={() => setDraftMode('ratio')}
                      className={cn(
                        'rounded-md text-xs font-medium transition-colors flex items-center justify-center gap-1 cursor-pointer',
                        draftMode === 'ratio'
                          ? 'bg-primary text-primary-foreground shadow-2xs'
                          : 'text-muted-foreground hover:text-foreground'
                      )}
                    >
                      <Zap className='size-3' />
                      按模型倍率 (x)
                    </button>
                  </div>
                </div>
              </div>

              {/* Row 2: 选择模型 */}
              <div className='space-y-1.5'>
                <div className='flex items-center justify-between'>
                  <Label className='text-[11px] text-muted-foreground'>
                    选择模型 ({draftModels.length}/
                    {availableModelsForDraftGroup.length})
                  </Label>
                  {availableModelsForDraftGroup.length > 0 && (
                    <div className='flex items-center gap-2'>
                      {draftModels.length <
                        availableModelsForDraftGroup.length && (
                        <button
                          type='button'
                          onClick={() =>
                            setDraftModels([...availableModelsForDraftGroup])
                          }
                          className='text-[11px] text-primary hover:underline flex items-center gap-1 cursor-pointer'
                        >
                          <CheckCheck className='size-3' />
                          全选该组模型 ({availableModelsForDraftGroup.length})
                        </button>
                      )}
                      {draftModels.length > 0 && (
                        <button
                          type='button'
                          onClick={() => setDraftModels([])}
                          className='text-[11px] text-muted-foreground hover:text-destructive flex items-center gap-0.5 cursor-pointer'
                        >
                          <X className='size-3' />
                          清空
                        </button>
                      )}
                    </div>
                  )}
                </div>

                <MultiSelect
                  options={availableModelsForDraftGroup.map((m) => ({
                    value: m,
                    label: m,
                  }))}
                  selected={draftModels}
                  onChange={setDraftModels}
                  placeholder={
                    availableModelsForDraftGroup.length > 0
                      ? '从该地址支持的模型中勾选一个或多个模型...'
                      : '该地址暂无自动探测模型，请在下方直接输入模型名'
                  }
                  className='min-h-8 text-xs bg-background w-full'
                  maxVisibleChips={4}
                />

                <Input
                  placeholder='补充手动输入模型名（多个用英文逗号分隔，可选）...'
                  value={customModelInput}
                  onChange={(e) => setCustomModelInput(e.target.value)}
                  className='h-7 text-xs font-mono bg-background/80'
                />
              </div>

              {/* Row 3: 单价输入与添加按钮 */}
              <div className='flex flex-wrap items-center gap-2 pt-0.5'>
                {draftMode === 'per_call' ? (
                  <div className='relative flex-1 min-w-[180px]'>
                    <span className='absolute left-2.5 top-1/2 -translate-y-1/2 text-xs font-semibold text-muted-foreground'>
                      $
                    </span>
                    <Input
                      type='number'
                      step='0.001'
                      min='0'
                      placeholder='输入上游单次成本，如 0.024'
                      value={draftCallPrice}
                      onChange={(e) => setDraftCallPrice(e.target.value)}
                      className='h-8 pl-6 pr-12 text-xs font-mono bg-background'
                    />
                    <span className='absolute right-2.5 top-1/2 -translate-y-1/2 text-[11px] text-muted-foreground font-mono'>
                      / 次
                    </span>
                  </div>
                ) : draftMode === 'ratio' ? (
                  <div className='relative flex-1 min-w-[180px]'>
                    <Input
                      type='number'
                      step='0.001'
                      min='0'
                      placeholder='输入该模型专属成本倍率，如 0.15 (即官价×0.15)'
                      value={draftModelRatio}
                      onChange={(e) => setDraftModelRatio(e.target.value)}
                      className='h-8 pr-10 text-xs font-mono bg-background'
                    />
                    <span className='absolute right-2.5 top-1/2 -translate-y-1/2 text-[11px] text-muted-foreground font-mono'>
                      x 倍
                    </span>
                  </div>
                ) : (
                  <div className='flex items-center gap-2 flex-1 min-w-[260px]'>
                    <div className='relative flex-1'>
                      <span className='absolute left-2 top-1/2 -translate-y-1/2 text-[11px] text-muted-foreground'>
                        输入$
                      </span>
                      <Input
                        type='number'
                        step='0.01'
                        min='0'
                        placeholder='如 0.30'
                        value={draftInputPrice}
                        onChange={(e) => setDraftInputPrice(e.target.value)}
                        className='h-8 pl-11 pr-9 text-xs font-mono bg-background'
                      />
                      <span className='absolute right-2 top-1/2 -translate-y-1/2 text-[10px] text-muted-foreground font-mono'>
                        /M
                      </span>
                    </div>
                    <div className='relative flex-1'>
                      <span className='absolute left-2 top-1/2 -translate-y-1/2 text-[11px] text-muted-foreground'>
                        输出$
                      </span>
                      <Input
                        type='number'
                        step='0.01'
                        min='0'
                        placeholder='如 1.50'
                        value={draftOutputPrice}
                        onChange={(e) => setDraftOutputPrice(e.target.value)}
                        className='h-8 pl-11 pr-9 text-xs font-mono bg-background'
                      />
                      <span className='absolute right-2 top-1/2 -translate-y-1/2 text-[10px] text-muted-foreground font-mono'>
                        /M
                      </span>
                    </div>
                  </div>
                )}

                <Button
                  type='button'
                  size='sm'
                  onClick={handleAddModelCostRule}
                  className='h-8 px-3 text-xs shrink-0'
                >
                  <Plus className='size-3.5 mr-1' />
                  添加成本规则
                </Button>
              </div>

              {/* 已配置模型成本规则列表 */}
              {modelCostRules.length > 0 && (
                <div className='space-y-2 border-t border-border/60 pt-2.5 max-h-56 overflow-y-auto'>
                  {modelCostRules.map((rule, idx) => {
                    const isExpanded = !!expandedRuleIdx[idx]
                    const maxShow = 4
                    const shownModels = isExpanded
                      ? rule.models
                      : rule.models.slice(0, maxShow)
                    const moreCount = rule.models.length - shownModels.length

                    return (
                      <div
                        key={`${rule.group}-${idx}`}
                        className='rounded-lg border border-border/70 bg-background p-2.5 space-y-2 text-xs'
                      >
                        <div className='flex items-center justify-between gap-2 flex-wrap'>
                          <div className='flex items-center gap-1.5'>
                            <Badge
                              variant='outline'
                              className='h-4 px-1.5 text-[10px] font-mono bg-muted/50'
                            >
                              {rule.group === '*'
                                ? '🌐 全部分组'
                                : rule.group}
                            </Badge>
                            <Badge
                              variant='outline'
                              className={cn(
                                'h-4 px-1.5 text-[10px]',
                                rule.mode === 'per_call'
                                  ? 'border-amber-300 text-amber-700 bg-amber-50 dark:border-amber-800 dark:text-amber-300 dark:bg-amber-950/40'
                                  : rule.mode === 'ratio'
                                    ? 'border-violet-300 text-violet-700 bg-violet-50 dark:border-violet-800 dark:text-violet-300 dark:bg-violet-950/40'
                                    : 'border-sky-300 text-sky-700 bg-sky-50 dark:border-sky-800 dark:text-sky-300 dark:bg-sky-950/40'
                              )}
                            >
                              {rule.mode === 'per_call'
                                ? '按次成本'
                                : rule.mode === 'ratio'
                                  ? '模型倍率'
                                  : '按量成本'}
                            </Badge>
                          </div>

                          <div className='flex items-center gap-1.5'>
                            {rule.mode === 'per_call' ? (
                              <div className='flex items-center gap-1'>
                                <span className='text-[11px] text-muted-foreground font-mono'>
                                  $
                                </span>
                                <Input
                                  type='number'
                                  step='0.001'
                                  min='0'
                                  value={rule.price ?? 0}
                                  onChange={(e) =>
                                    handleUpdateRulePrice(
                                      idx,
                                      'price',
                                      e.target.value
                                    )
                                  }
                                  className='h-6 w-20 px-1.5 text-xs font-mono text-right'
                                />
                                <span className='text-[11px] text-muted-foreground font-mono'>
                                  /次
                                </span>
                              </div>
                            ) : rule.mode === 'ratio' ? (
                              <div className='flex items-center gap-1'>
                                <Input
                                  type='number'
                                  step='0.001'
                                  min='0'
                                  value={rule.cost_ratio ?? 0}
                                  onChange={(e) =>
                                    handleUpdateRulePrice(
                                      idx,
                                      'cost_ratio',
                                      e.target.value
                                    )
                                  }
                                  className='h-6 w-20 px-1.5 text-xs font-mono text-right'
                                />
                                <span className='text-[11px] text-muted-foreground font-mono'>
                                  x
                                </span>
                              </div>
                            ) : (
                              <div className='flex items-center gap-1.5 font-mono text-[11px]'>
                                <span className='text-muted-foreground'>
                                  入$
                                </span>
                                <Input
                                  type='number'
                                  step='0.01'
                                  min='0'
                                  value={rule.input_price ?? 0}
                                  onChange={(e) =>
                                    handleUpdateRulePrice(
                                      idx,
                                      'input_price',
                                      e.target.value
                                    )
                                  }
                                  className='h-6 w-16 px-1 text-xs font-mono text-right'
                                />
                                <span className='text-muted-foreground'>
                                  出$
                                </span>
                                <Input
                                  type='number'
                                  step='0.01'
                                  min='0'
                                  value={rule.output_price ?? 0}
                                  onChange={(e) =>
                                    handleUpdateRulePrice(
                                      idx,
                                      'output_price',
                                      e.target.value
                                    )
                                  }
                                  className='h-6 w-16 px-1 text-xs font-mono text-right'
                                />
                                <span className='text-muted-foreground'>
                                  /M
                                </span>
                              </div>
                            )}

                            <button
                              type='button'
                              onClick={() => handleRemoveRule(idx)}
                              className='rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive cursor-pointer'
                              title='删除此规则'
                            >
                              <Trash2 className='size-3.5' />
                            </button>
                          </div>
                        </div>

                        <div className='flex flex-wrap items-center gap-1'>
                          {shownModels.map((m) => (
                            <span
                              key={m}
                              className='font-mono text-[11px] px-1.5 py-0.5 rounded bg-muted/60 border border-border/60 text-foreground/90'
                            >
                              {m}
                            </span>
                          ))}
                          {moreCount > 0 && (
                            <button
                              type='button'
                              onClick={() =>
                                setExpandedRuleIdx((prev) => ({
                                  ...prev,
                                  [idx]: !prev[idx],
                                }))
                              }
                              className='text-[11px] px-1.5 py-0.5 rounded bg-primary/10 text-primary hover:bg-primary/20 cursor-pointer'
                            >
                              +{moreCount} 个模型
                            </button>
                          )}
                          {isExpanded && rule.models.length > maxShow && (
                            <button
                              type='button'
                              onClick={() =>
                                setExpandedRuleIdx((prev) => ({
                                  ...prev,
                                  [idx]: !prev[idx],
                                }))
                              }
                              className='text-[11px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground cursor-pointer'
                            >
                              收起
                            </button>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          </div>
        </div>

        <DialogFooter className='flex items-center justify-between sm:justify-between gap-2 pt-3 border-t'>
          <div>
            {onBack && (
              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={onBack}
                className='gap-1 text-muted-foreground hover:text-foreground'
              >
                <ArrowLeft className='size-3.5' />
                <span>返回上一弹窗</span>
              </Button>
            )}
          </div>
          <div className='flex items-center gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => (onBack ? onBack() : onOpenChange(false))}
            >
              {t('Cancel', '取消')}
            </Button>
            <Button
              type='button'
              size='sm'
              disabled={updateMutation.isPending}
              onClick={() => updateMutation.mutate()}
            >
              {updateMutation.isPending && (
                <Loader2 className='size-3.5 mr-1.5 animate-spin' />
              )}
              {t('Save Source Cost Config', '保存来源成本配置')}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
