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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Layers, Loader2, Sparkles } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { GroupBadge } from '@/components/group-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { usePricingData } from '@/features/pricing/hooks'
import { getGroupDetails } from '@/features/users/api'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { updateGroup } from '../../api'

type EditGroupDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  group: string | null
}

export function EditGroupDialog({
  open,
  onOpenChange,
  group,
}: EditGroupDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { groupRatio } = usePricingData()

  const [name, setName] = useState('')
  const [ratio, setRatio] = useState<string | number>('1')
  const [desc, setDesc] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  // Fetch full details of groups (channel counts, models, descriptions)
  const { data: groupDetailsData } = useQuery({
    queryKey: ['group-details'],
    queryFn: async () => requireServerSuccess(await getGroupDetails()),
    enabled: open,
    staleTime: 60 * 1000,
  })

  const currentMeta = group ? groupDetailsData?.meta?.[group] : undefined

  useEffect(() => {
    if (open && group) {
      setName(group)
      const currentRatio =
        currentMeta?.ratio ?? groupRatio[group] ?? 1
      setRatio(currentRatio)
      setDesc((currentMeta?.desc as string) || '')
    } else if (!open) {
      setName('')
      setRatio('1')
      setDesc('')
    }
  }, [open, group, currentMeta, groupRatio])

  if (!group) return null

  const isRenaming = name.trim() !== '' && name.trim() !== group
  const channelsCount = currentMeta?.channels?.length ?? 0
  const modelsCount = currentMeta?.models?.length ?? 0

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const trimmedName = name.trim()

    if (!trimmedName) {
      toast.error(t('分组名称不能为空'))
      return
    }
    if (trimmedName === 'auto') {
      toast.error(t('不能使用系统保留的 auto 作为分组名称'))
      return
    }

    const numRatio = Number(ratio)
    if (!Number.isFinite(numRatio) || numRatio < 0) {
      toast.error(t('分组倍率必须是大于或等于 0 的有效数字'))
      return
    }

    setIsSubmitting(true)
    try {
      const result = await updateGroup({
        old_group: group,
        new_group: trimmedName,
        ratio: numRatio,
        desc: desc.trim(),
      })

      if (result.success) {
        toast.success(
          isRenaming
            ? t('分组已成功重命名并全局同步生效')
            : t('分组倍率已成功更新并全局生效')
        )
        // Invalidate all related caches
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: ['pricing'] }),
          queryClient.invalidateQueries({ queryKey: ['channels'] }),
          queryClient.invalidateQueries({ queryKey: ['groups'] }),
          queryClient.invalidateQueries({ queryKey: ['group-details'] }),
          queryClient.invalidateQueries({ queryKey: ['users'] }),
        ])
        onOpenChange(false)
      } else {
        handleServerError(result, t('更新分组失败'))
      }
    } catch (error) {
      handleServerError(error, t('更新分组时发生异常'))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        <div className='flex items-center gap-2'>
          <Layers className='size-5 text-primary' />
          <span>{t('编辑分组')}</span>
        </div>
      }
      description={t(
        '修改分组名称及系统默认模型倍率。修改后将全局自动生效，同步更新相关渠道、用户授权与配置。'
      )}
      contentClassName='sm:max-w-[480px]'
    >
      <form onSubmit={handleSubmit} className='space-y-4 pt-1'>
        {/* Current status bar */}
        <div className='rounded-lg bg-muted/40 p-3 border border-border/70 space-y-2'>
          <div className='flex items-center justify-between text-xs'>
            <span className='text-muted-foreground'>{t('当前编辑分组')}:</span>
            <GroupBadge group={group} ratio={groupRatio[group]} size='sm' />
          </div>
          {(channelsCount > 0 || modelsCount > 0) && (
            <div className='flex items-center gap-2 pt-1 border-t border-border/50 text-[11px] text-muted-foreground'>
              <span>
                {t('关联渠道')}:{' '}
                <strong className='text-foreground font-medium font-mono'>
                  {channelsCount}
                </strong>
              </span>
              <span>•</span>
              <span>
                {t('包含模型')}:{' '}
                <strong className='text-foreground font-medium font-mono'>
                  {modelsCount}
                </strong>
              </span>
            </div>
          )}
        </div>

        {/* Name input */}
        <div className='space-y-1.5'>
          <div className='flex items-center justify-between'>
            <Label htmlFor='group-name' className='text-xs font-medium'>
              {t('分组名称')} <span className='text-destructive'>*</span>
            </Label>
            {isRenaming && (
              <Badge
                variant='outline'
                className='h-4 px-1 text-[9px] text-amber-600 dark:text-amber-400 border-amber-300 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/40'
              >
                {t('重命名')}
              </Badge>
            )}
          </div>
          <Input
            id='group-name'
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('请输入分组名称')}
            className='h-9 text-xs font-mono'
            disabled={isSubmitting}
            autoFocus
          />
        </div>

        {/* Ratio input */}
        <div className='space-y-1.5'>
          <div className='flex items-center justify-between'>
            <Label htmlFor='group-ratio' className='text-xs font-medium'>
              {t('默认模型倍率')} <span className='text-destructive'>*</span>
            </Label>
            <span className='text-[10px] text-muted-foreground font-mono'>
              {Number(ratio) > 0 ? `${ratio}x` : t('免费')}
            </span>
          </div>
          <div className='relative'>
            <Input
              id='group-ratio'
              type='number'
              step='0.001'
              min='0'
              value={ratio}
              onChange={(e) => setRatio(e.target.value)}
              placeholder='1.0'
              className='h-9 text-xs font-mono pr-8'
              disabled={isSubmitting}
            />
            <span className='absolute right-2.5 top-1/2 -translate-y-1/2 text-xs font-semibold text-muted-foreground'>
              x
            </span>
          </div>
          <p className='text-[11px] text-muted-foreground leading-normal'>
            {t('该分组下模型计费的默认倍率（全局生效，用户未单独配置时以此倍率为准）')}
          </p>
        </div>

        {/* Description input */}
        <div className='space-y-1.5'>
          <Label htmlFor='group-desc' className='text-xs font-medium'>
            {t('分组描述')} <span className='text-muted-foreground text-[10px] font-normal'>({t('可选')})</span>
          </Label>
          <Input
            id='group-desc'
            value={desc}
            onChange={(e) => setDesc(e.target.value)}
            placeholder={t('如：Gemini 4 按次专用池')}
            className='h-9 text-xs'
            disabled={isSubmitting}
          />
        </div>

        {/* Renaming Alert */}
        {isRenaming && (
          <div className='rounded-lg bg-amber-500/10 border border-amber-500/30 p-2.5 text-xs text-amber-800 dark:text-amber-300 space-y-1.5 animate-in fade-in-50 duration-150'>
            <div className='font-medium flex items-center gap-1.5'>
              <AlertTriangle className='size-3.5 shrink-0 text-amber-600 dark:text-amber-400' />
              <span>{t('全局级联同步警告')}</span>
            </div>
            <p className='text-[11px] leading-relaxed text-muted-foreground'>
              {t(
                '确认将分组从「{{oldGroup}}」重命名为「{{newGroup}}」？系统将自动在数据库事务中无缝同步更新所有关联渠道、Abilities 映射、用户已绑分组与自定义价格规则、API 令牌和全局配置。',
                { oldGroup: group, newGroup: name.trim() }
              )}
            </p>
          </div>
        )}

        {/* Dialog footer buttons */}
        <div className='flex items-center justify-end gap-2 pt-2 border-t border-border/60'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => onOpenChange(false)}
            disabled={isSubmitting}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            size='sm'
            disabled={isSubmitting || !name.trim()}
            className='min-w-[70px]'
          >
            {isSubmitting ? (
              <Loader2 className='size-3.5 animate-spin' />
            ) : (
              t('Save')
            )}
          </Button>
        </div>
      </form>
    </Dialog>
  )
}
