import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Check,
  Coins,
  DollarSign,
  Globe,
  Layers,
  Loader2,
  MessageSquare,
  Plus,
  Settings2,
  Sparkles,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'

import { GroupBadge } from '@/components/group-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { updateChannelSource } from '@/features/exclusive-channels/api'
import type { UpstreamSourceDTO } from '@/features/exclusive-channels/types'
import { inferGroupFamily } from '@/features/users/lib'
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
  const [draftRatios, setDraftRatios] = useState<Record<string, string>>({})
  const [isDirty, setIsDirty] = useState(false)

  useEffect(() => {
    if (!source) {
      setDraftRatios({})
      setIsDirty(false)
      return
    }
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
  }, [source])

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

  if (!source) {
    return (
      <div className='p-3.5 text-xs text-muted-foreground space-y-1.5'>
        <div className='flex items-center gap-1.5 font-mono font-medium text-foreground'>
          <Globe className='size-3.5 text-primary' />
          <span className='truncate'>{baseUrl}</span>
        </div>
        <div>正在加载该地址下的分组与成本配置...</div>
      </div>
    )
  }

  const rules = source.model_cost_rules || []

  return (
    <div className='p-3 space-y-3 text-xs'>
      {/* 1. 顶部地址信息与完整配置按钮 */}
      <div className='flex items-start justify-between gap-2 border-b border-border/60 pb-2.5'>
        <div className='min-w-0 space-y-1'>
          <div className='flex items-center gap-1.5'>
            <Globe className='size-3.5 text-primary shrink-0' />
            <span
              className='font-mono font-semibold text-foreground truncate max-w-[210px]'
              title={source.display_base_url}
            >
              {source.display_base_url}
            </span>
          </div>
          <div className='flex items-center gap-1.5 flex-wrap text-[11px] text-muted-foreground'>
            <span>
              {source.channel_count} 个渠道 ({source.enabled_channel_count} 启用)
            </span>
            <span
              className={cn(
                'inline-flex items-center rounded px-1.5 py-0.5 font-mono text-[10px]',
                (source.today_quota || 0) > 0
                  ? 'bg-emerald-500/10 font-semibold text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                  : 'bg-muted/60 text-muted-foreground'
              )}
            >
              当天: {(source.today_quota || 0) > 0 ? formatLogQuota(source.today_quota) : '$0'}
            </span>
            {source.wechat_name && (
              <Badge
                variant='outline'
                className='h-4 px-1.5 text-[10px] border-emerald-300 text-emerald-700 bg-emerald-50 dark:border-emerald-800 dark:text-emerald-300 dark:bg-emerald-950/40 flex items-center gap-0.5'
              >
                <MessageSquare className='size-2.5' />
                {source.wechat_name}
              </Badge>
            )}
          </div>
        </div>

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

        <div className='rounded-lg border border-border/70 bg-muted/20 divide-y divide-border/40 max-h-[190px] overflow-y-auto'>
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

              return (
                <div
                  key={grp}
                  className='flex items-center justify-between gap-2 px-2.5 py-1.5 hover:bg-muted/40 transition-colors'
                >
                  <div className='flex items-center gap-1.5 min-w-0 flex-1 flex-wrap'>
                    <Badge
                      variant='outline'
                      className={cn(
                        'h-3.5 px-1 text-[9px] shrink-0 font-normal',
                        family.badgeClassName
                      )}
                    >
                      {family.name.split(' / ')[0]}
                    </Badge>
                    <GroupBadge
                      group={grp}
                      copyable={false}
                      className='text-[11px] font-medium'
                    />
                    <span
                      className={cn(
                        'inline-flex items-center rounded px-1.5 py-0.5 font-mono text-[10px] shrink-0',
                        effectiveQuota > 0
                          ? 'bg-emerald-500/10 font-semibold text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                          : 'bg-muted/60 text-muted-foreground font-normal'
                      )}
                      title='该分组在此地址下的当天消耗收入'
                    >
                      当天 {effectiveQuota > 0 ? formatLogQuota(effectiveQuota) : '$0'}
                    </span>
                  </div>

                  <div className='flex items-center gap-1.5 shrink-0'>
                    {custRatio !== undefined && (
                      <span
                        className='text-[10px] text-muted-foreground font-mono'
                        title='平台对该分组的默认对客倍率'
                      >
                        售{custRatio}x
                      </span>
                    )}
                    <div className='relative w-[68px]'>
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
                          valStr.trim() !== '' &&
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
      <div className='space-y-1.5 pt-0.5'>
        <div className='flex items-center justify-between'>
          <span className='text-[11px] font-semibold text-foreground flex items-center gap-1'>
            <DollarSign className='size-3 text-emerald-600' />
            模型成本单价 ({rules.length})
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

        {rules.length === 0 ? (
          <div
            onClick={() => onOpenFullConfig(source)}
            className='rounded-lg border border-dashed border-border/80 bg-muted/15 px-3 py-2 text-center text-[11px] text-muted-foreground hover:border-primary/50 hover:text-foreground cursor-pointer transition-colors'
          >
            暂未配置专属模型成本单价，点击配置按次 ($/次) 或按量 ($/M) 成本
          </div>
        ) : (
          <div className='space-y-1 max-h-28 overflow-y-auto'>
            {rules.map((r, idx) => (
              <div
                key={idx}
                onClick={() => onOpenFullConfig(source)}
                className='flex items-center justify-between gap-2 rounded-md border border-amber-300/60 dark:border-amber-800/60 bg-amber-50/40 dark:bg-amber-950/20 px-2 py-1 text-[11px] cursor-pointer hover:bg-amber-50/80 transition-colors'
              >
                <div className='flex items-center gap-1 min-w-0 flex-1'>
                  <Badge
                    variant='outline'
                    className='h-3.5 px-1 text-[9px] font-mono shrink-0 bg-background/80'
                  >
                    {r.group === '*' ? '全部组' : r.group}
                  </Badge>
                  <span
                    className='font-mono truncate text-foreground/90'
                    title={r.models.join(', ')}
                  >
                    {r.models.length === 1
                      ? r.models[0]
                      : `${r.models[0]} 等${r.models.length}个`}
                  </span>
                </div>
                <span className='font-mono font-semibold text-amber-700 dark:text-amber-300 shrink-0 flex items-center gap-0.5'>
                  {r.mode === 'per_token' ? (
                    <>
                      <Coins className='size-2.5' />入${r.input_price ?? 0}/出$
                      {r.output_price ?? 0}/M
                    </>
                  ) : (
                    <>
                      <Sparkles className='size-2.5' />${r.price ?? 0}/次
                    </>
                  )}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
