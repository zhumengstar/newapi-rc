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
  Coins,
  DollarSign,
  Globe,
  Layers,
  Loader2,
  MessageSquare,
  Plus,
  Trash2,
} from 'lucide-react'
import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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

import { updateChannelSource } from '../api'
import type { UpstreamSourceDTO } from '../types'

interface QuickSourceDialogProps {
  source: UpstreamSourceDTO | null
  allGroups: string[]
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function QuickSourceDialog({
  source,
  allGroups,
  open,
  onOpenChange,
}: QuickSourceDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [wechatName, setWechatName] = useState('')
  const [channelRatio, setChannelRatio] = useState<string>('')
  const [selectedGroups, setSelectedGroups] = useState<string[]>([])
  const [costTier, setCostTier] = useState<number>(0)
  const [modelPrices, setModelPrices] = useState<Record<string, number>>({})

  // 新增模型单价临时输入
  const [newModelName, setNewModelName] = useState('')
  const [newModelPrice, setNewModelPrice] = useState('')

  useEffect(() => {
    if (source && open) {
      setWechatName(source.wechat_name || '')
      setChannelRatio(source.channel_ratio > 0 ? String(source.channel_ratio) : '')
      setSelectedGroups(source.groups.length > 0 ? source.groups : ['default'])
      setCostTier(source.cost_tier || 0)
      setModelPrices(source.model_prices || {})
      setNewModelName('')
      setNewModelPrice('')
    }
  }, [source, open])

  const toggleGroup = (grp: string) => {
    setSelectedGroups((prev) => {
      if (prev.includes(grp)) {
        if (prev.length <= 1) {
          toast.warning(t('Source must have at least one group', '来源渠道必须至少保留一个分组'))
          return prev
        }
        return prev.filter((g) => g !== grp)
      } else {
        return [...prev, grp]
      }
    })
  }

  const handleUpdateModelPrice = (mName: string, priceStr: string) => {
    const val = parseFloat(priceStr)
    if (isNaN(val) || val < 0) {
      const next = { ...modelPrices }
      delete next[mName]
      setModelPrices(next)
    } else {
      setModelPrices((prev) => ({ ...prev, [mName]: val }))
    }
  }

  const handleRemoveModelPrice = (mName: string) => {
    const next = { ...modelPrices }
    delete next[mName]
    setModelPrices(next)
  }

  const handleAddNewModelPrice = () => {
    const trimmed = newModelName.trim()
    const val = parseFloat(newModelPrice)
    if (!trimmed) {
      toast.warning(t('Please enter model name', '请输入模型名称'))
      return
    }
    if (isNaN(val) || val < 0) {
      toast.warning(t('Please enter a valid non-negative price', '请输入有效的单请求价格'))
      return
    }
    setModelPrices((prev) => ({ ...prev, [trimmed]: val }))
    setNewModelName('')
    setNewModelPrice('')
  }

  const updateMutation = useMutation({
    mutationFn: async () => {
      if (!source) return
      const ratioNum = channelRatio ? parseFloat(channelRatio) : undefined

      await updateChannelSource({
        base_url: source.base_url,
        wechat_name: wechatName.trim(),
        channel_ratio: ratioNum,
        group: selectedGroups.join(','),
        cost_tier: costTier,
        model_prices: modelPrices,
      })
    },
    onSuccess: () => {
      toast.success(t('Source cost and WeChat name updated', '来源成本与微信名称已成功更新并同步'))
      queryClient.invalidateQueries({ queryKey: ['channel-sources-list'] })
      queryClient.invalidateQueries({ queryKey: ['channels'] })
      queryClient.invalidateQueries({ queryKey: ['exclusive-channels-list'] })
      onOpenChange(false)
    },
    onError: (err: any) => {
      toast.error(err?.message || t('Failed to update source configuration', '更新来源配置失败'))
    },
  })

  if (!source) return null

  const displayGroups = Array.from(new Set([...allGroups, ...selectedGroups])).sort()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-2xl'>
        <DialogHeader>
          <div className='flex items-center justify-between pr-6'>
            <DialogTitle className='flex items-center gap-2 text-base font-semibold'>
              <Globe className='size-4 text-primary' />
              <span>{t('Manage Upstream Source Cost & WeChat', '按地址管理来源成本与微信名称')}</span>
            </DialogTitle>
          </div>
          <DialogDescription className='text-xs text-muted-foreground'>
            {t('Unique Base URL', '唯一请求地址')}:{' '}
            <span className='font-mono font-medium text-foreground bg-muted/60 px-1.5 py-0.5 rounded'>
              {source.display_base_url}
            </span>{' '}
            ({source.channel_count} {t('channels aggregated', '个关联渠道批量同步')})
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-5 py-2'>
          {/* 1. 微信联系人名称 */}
          <div className='space-y-1.5'>
            <Label className='text-xs font-semibold flex items-center gap-1.5'>
              <MessageSquare className='size-3.5 text-emerald-600' />
              <span>{t('Source WeChat Name', '渠道来源微信名称')}</span>
            </Label>
            <Input
              placeholder={t('e.g. WeChat: Zhang San (OpenAI Wholesale), Lao Wang...', '如：微信: 张三(中转总代)、老王 Claude 专供...')}
              value={wechatName}
              onChange={(e) => setWechatName(e.target.value)}
              className='h-8 text-xs'
            />
            <p className='text-[11px] text-muted-foreground'>
              * {t('Will automatically sync to the contact info of all channels under this address.', '设置后将自动同步至此地址下所有渠道的联系方式中，随时看清进货来源。')}
            </p>
          </div>

          {/* 2. 渠道分组与分组结算倍率 */}
          <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
            <div className='space-y-1.5'>
              <Label className='text-xs font-semibold flex items-center gap-1.5'>
                <Coins className='size-3.5 text-amber-500' />
                <span>{t('Channel / Group Settlement Ratio', '渠道/分组结算倍率 (成本折率)')}</span>
              </Label>
              <Input
                type='number'
                step='0.01'
                min='0'
                placeholder={t('e.g. 0.25 (meaning 25% cost ratio)', '如：0.25 (表示该来源按2.5折结算)')}
                value={channelRatio}
                onChange={(e) => setChannelRatio(e.target.value)}
                className='h-8 text-xs'
              />
              <p className='text-[11px] text-muted-foreground'>
                {t('Applied to channels under this address as base cost ratio.', '作为该来源渠道的基础结算倍率批量同步。')}
              </p>
            </div>

            <div className='space-y-1.5'>
              <Label className='text-xs font-semibold flex items-center gap-1.5'>
                <Layers className='size-3.5 text-blue-500' />
                <span>{t('Cost Tier', '成本优先级层级 (Cost Tier)')}</span>
              </Label>
              <Input
                type='number'
                min='0'
                max='10'
                placeholder='0 (0=默认, 1=低成本优选, 2=备选)'
                value={costTier}
                onChange={(e) => setCostTier(parseInt(e.target.value) || 0)}
                className='h-8 text-xs'
              />
              <p className='text-[11px] text-muted-foreground'>
                {t('Lower tiers are prioritized for lower procurement costs.', '层级越小优先调度，优先跑低成本来源。')}
              </p>
            </div>
          </div>

          {/* 3. 业务分组分配 */}
          <div className='space-y-1.5'>
            <div className='flex items-center justify-between'>
              <Label className='text-xs font-semibold'>
                {t('Applied Groups for this Source', '来源渠道应用分组')} ({selectedGroups.length})
              </Label>
              <span className='text-[11px] text-muted-foreground'>
                {t('Click to toggle groups for all channels under this URL', '点击切换该地址下所有渠道归属的业务分组')}
              </span>
            </div>

            <div className='flex flex-wrap gap-1.5 p-2.5 rounded-lg border bg-background min-h-14'>
              {displayGroups.map((grp) => {
                const isSelected = selectedGroups.includes(grp)
                return (
                  <Badge
                    key={grp}
                    variant={isSelected ? 'default' : 'outline'}
                    className={`cursor-pointer transition-all ${
                      isSelected
                        ? 'bg-primary text-primary-foreground hover:bg-primary/90'
                        : 'text-muted-foreground hover:bg-muted'
                    }`}
                    onClick={() => toggleGroup(grp)}
                  >
                    {grp}
                    {isSelected && <span className='ml-1 text-[10px]'>✓</span>}
                  </Badge>
                )
              })}
            </div>
          </div>

          {/* 4. 模型单请求价格设置 */}
          <div className='space-y-2'>
            <div className='flex items-center justify-between'>
              <Label className='text-xs font-semibold flex items-center gap-1.5'>
                <DollarSign className='size-3.5 text-emerald-600' />
                <span>{t('Model Per-Request Price', '模型单请求固定价格 (按次计费成本)')}</span>
              </Label>
              <span className='text-[11px] text-muted-foreground'>
                {t('Price in USD per request. Configured models charge per call.', '单位为美元/次。设置后该模型按次固定计费')}
              </span>
            </div>

            {/* 已配置或已包含的模型价格列表 */}
            <div className='space-y-1.5 rounded-lg border p-2.5 bg-background max-h-56 overflow-y-auto'>
              {source.models.length === 0 && Object.keys(modelPrices).length === 0 ? (
                <div className='py-4 text-center text-xs text-muted-foreground'>
                  {t('No models associated with this source yet', '该来源暂未关联模型，可在下方直接添加')}
                </div>
              ) : (
                <div className='space-y-2'>
                  {/* 合并来源模型与已有定价 */}
                  {Array.from(new Set([...source.models, ...Object.keys(modelPrices)])).map((mName) => {
                    const price = modelPrices[mName]
                    const hasPrice = typeof price === 'number' && price >= 0

                    return (
                      <div
                        key={mName}
                        className='flex items-center justify-between gap-3 rounded-md border p-1.5 px-2 text-xs bg-muted/20'
                      >
                        <div className='flex items-center gap-2 min-w-0 flex-1'>
                          <span className='font-mono font-medium text-foreground truncate'>
                            {mName}
                          </span>
                          {hasPrice ? (
                            <Badge variant='default' className='text-[10px] bg-emerald-600 text-white px-1 py-0 h-4'>
                              ${price.toFixed(4)} / 次
                            </Badge>
                          ) : (
                            <span className='text-[10px] text-muted-foreground'>
                              ({t('Standard Token Billing', '标准Token倍率计费')})
                            </span>
                          )}
                        </div>

                        <div className='flex items-center gap-1.5 shrink-0'>
                          <div className='flex items-center gap-1'>
                            <span className='text-[11px] text-muted-foreground'>$</span>
                            <Input
                              type='number'
                              step='0.001'
                              min='0'
                              placeholder='单次价'
                              value={hasPrice ? price : ''}
                              onChange={(e) => handleUpdateModelPrice(mName, e.target.value)}
                              className='h-7 w-24 text-xs'
                            />
                          </div>
                          {hasPrice && (
                            <button
                              type='button'
                              onClick={() => handleRemoveModelPrice(mName)}
                              className='rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive'
                              title={t('Clear fixed price', '清除固定单价')}
                            >
                              <Trash2 className='size-3.5' />
                            </button>
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>

            {/* 新增其他模型单价 */}
            <div className='flex items-center gap-2 pt-1'>
              <Input
                placeholder={t('Add other model name (e.g. midjourney, flux-pro)...', '添加其他模型名称 (如 midjourney, flux-pro)...')}
                value={newModelName}
                onChange={(e) => setNewModelName(e.target.value)}
                className='h-8 text-xs flex-1'
              />
              <Input
                type='number'
                step='0.001'
                min='0'
                placeholder={t('Price $/req', '单次价格 $')}
                value={newModelPrice}
                onChange={(e) => setNewModelPrice(e.target.value)}
                className='h-8 w-24 text-xs'
              />
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={handleAddNewModelPrice}
                className='h-8 shrink-0 text-xs'
              >
                <Plus className='size-3.5 mr-1' />
                {t('Add Price', '添加定价')}
              </Button>
            </div>
          </div>
        </div>

        <DialogFooter className='flex items-center justify-end gap-2 pt-3 border-t'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => onOpenChange(false)}
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
            {t('Save & Sync to All Channels', '保存并批量同步至所有渠道')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
