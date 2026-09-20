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
import { Loader2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getChannelGroupPriorities, getGroups } from '../../api'
import { handleCopyChannel } from '../../lib'
import { useChannels } from '../channels-provider'

type CopyChannelDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CopyChannelDialog({
  open,
  onOpenChange,
}: CopyChannelDialogProps) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [targetGroup, setTargetGroup] = useState('')
  const [resetBalance, setResetBalance] = useState(true)
  const [isCopying, setIsCopying] = useState(false)

  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: async () => requireServerSuccess(await getGroups()),
    enabled: open,
  })

  const { data: groupPrioritiesData } = useQuery({
    queryKey: ['channel-group-priorities'],
    queryFn: async () => requireServerSuccess(await getChannelGroupPriorities()),
    enabled: open,
  })

  useEffect(() => {
    if (open && currentRow) {
      setName(currentRow.name || '')
      setTargetGroup(currentRow.group || '')
    }
  }, [open, currentRow])

  const groupOptions = useMemo(() => {
    const list = groupsData?.data || []
    const set = new Set([
      ...list,
      ...(currentRow?.group ? [currentRow.group] : []),
    ])
    return Array.from(set).filter(Boolean)
  }, [groupsData, currentRow?.group])

  const remappedPriorityPreview = useMemo(() => {
    if (!targetGroup || !currentRow || targetGroup === currentRow.group)
      return null
    const basePrio = groupPrioritiesData?.data?.[targetGroup]
    if (!basePrio || basePrio <= 0) return null
    const base = Math.floor((basePrio - 1) / 10) * 10
    const offset = (currentRow.priority ?? 1) % 10
    const validOffset = offset >= 1 && offset <= 9 ? offset : 1
    return base + validOffset
  }, [targetGroup, currentRow, groupPrioritiesData])

  if (!currentRow) return null

  const handleCopy = async () => {
    const trimmedName = name.trim()
    if (!trimmedName) return

    setIsCopying(true)

    await handleCopyChannel(
      currentRow.id,
      {
        name: trimmedName,
        reset_balance: resetBalance,
        group: targetGroup || undefined,
      },
      queryClient,
      () => {
        onOpenChange(false)
        setResetBalance(true)
      }
    )

    setIsCopying(false)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Copy Channel')}
      description={
        <>
          {t('Create a copy of:')} <strong>{currentRow.name}</strong>
        </>
      }
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={isCopying}
          >
            {t('Cancel')}
          </Button>
          <Button onClick={handleCopy} disabled={isCopying || !name.trim()}>
            {isCopying && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {isCopying ? t('Copying...') : t('Copy Channel')}
          </Button>
        </>
      }
    >
      <div className='space-y-4 py-4'>
        <div className='space-y-2'>
          <Label htmlFor='channel-name'>{t('Channel Name')}</Label>
          <Input
            id='channel-name'
            placeholder={t('Enter channel name')}
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={isCopying}
            autoFocus
          />
        </div>

        <div className='space-y-2'>
          <Label htmlFor='target-group'>{t('Target Group')}</Label>
          <Select
            value={targetGroup}
            onValueChange={(val) => setTargetGroup(val || '')}
            disabled={isCopying}
          >
            <SelectTrigger id='target-group' className='w-full'>
              <SelectValue placeholder={t('Select target group')} />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {groupOptions.map((g) => (
                  <SelectItem key={g} value={g}>
                    {g}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          {remappedPriorityPreview !== null && (
            <div className='rounded border border-amber-500/20 bg-amber-500/10 p-2 text-xs text-amber-700 dark:text-amber-300'>
              💡{' '}
              {t(
                'Target group differs from source; priority will be automatically remapped to:'
              )}{' '}
              <strong>{remappedPriorityPreview}</strong>
            </div>
          )}
        </div>

        <div className='flex items-center space-x-2'>
          <Checkbox
            id='reset-balance'
            checked={resetBalance}
            onCheckedChange={(checked) => setResetBalance(!!checked)}
            disabled={isCopying}
          />
          <Label htmlFor='reset-balance' className='text-sm font-normal'>
            {t('Reset balance and used quota')}
          </Label>
        </div>
      </div>
    </Dialog>
  )
}
