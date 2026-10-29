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
import { MultiSelect, type Option } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getChannelGroupPriorities, getGroups } from '../../api'
import { handleCopyChannel } from '../../lib'
import { useChannels } from '../channels-provider'

type CopyChannelDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

function splitGroupStr(str?: string | null): string[] {
  if (!str) return []
  return str
    .split(/[,，]+/)
    .map((g) => g.trim())
    .filter(Boolean)
}

export function CopyChannelDialog({
  open,
  onOpenChange,
}: CopyChannelDialogProps) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [contact, setContact] = useState('')
  const [targetGroups, setTargetGroups] = useState<string[]>([])
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
      setContact(currentRow.contact || '')
      setTargetGroups(splitGroupStr(currentRow.group))
    }
  }, [open, currentRow])

  const groupOptions: Option[] = useMemo(() => {
    const list = groupsData?.data || []
    const allSet = new Set<string>()

    for (const g of list) {
      for (const item of splitGroupStr(g)) {
        allSet.add(item)
      }
    }
    for (const item of splitGroupStr(currentRow?.group)) {
      allSet.add(item)
    }
    for (const item of targetGroups) {
      allSet.add(item)
    }

    const allGroups = Array.from(allSet).filter(Boolean)
    const selectedSet = new Set(targetGroups)

    allGroups.sort((a, b) => {
      const aSel = selectedSet.has(a)
      const bSel = selectedSet.has(b)
      if (aSel && !bSel) return -1
      if (!aSel && bSel) return 1
      return a.localeCompare(b, 'zh-CN')
    })

    return allGroups.map((g) => ({
      value: g,
      label: g,
    }))
  }, [groupsData, currentRow?.group, targetGroups])

  const remappedPriorityPreview = useMemo(() => {
    const primaryTarget = targetGroups[0]
    if (!primaryTarget || !currentRow) return null
    const origPrimary = splitGroupStr(currentRow.group)[0]
    if (!origPrimary || primaryTarget === origPrimary) return null
    const basePrio = groupPrioritiesData?.data?.[primaryTarget]
    if (!basePrio || basePrio <= 0) return null
    const base = Math.floor((basePrio - 1) / 10) * 10
    const offset = (currentRow.priority ?? 1) % 10
    const validOffset = offset >= 1 && offset <= 9 ? offset : 1
    return base + validOffset
  }, [targetGroups, currentRow, groupPrioritiesData])

  if (!currentRow) return null

  const handleCopy = async () => {
    const trimmedName = name.trim()
    if (!trimmedName) return

    setIsCopying(true)

    await handleCopyChannel(
      currentRow.id,
      {
        name: trimmedName,
        contact: contact.trim(),
        reset_balance: resetBalance,
        group: targetGroups.length > 0 ? targetGroups.join(',') : undefined,
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
          <Label htmlFor='channel-contact'>
            {t('Channel Source / Contact', '来源联系方式')}
          </Label>
          <Input
            id='channel-contact'
            placeholder={t(
              'e.g. TG / WeChat / User ID / Email',
              '例如：TG/微信/用户ID/邮箱等来源联系方式'
            )}
            value={contact}
            onChange={(e) => setContact(e.target.value)}
            disabled={isCopying}
          />
        </div>

        <div className='space-y-2'>
          <Label htmlFor='target-group'>{t('Target Group')}</Label>
          <MultiSelect
            id='target-group'
            options={groupOptions}
            selected={targetGroups}
            onChange={setTargetGroups}
            allowCreate={true}
            placeholder={t('Select target groups', '选择目标分组')}
            disabled={isCopying}
          />
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
