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
import { Loader2, Plus, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
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
import { Switch } from '@/components/ui/switch'

import {
  deleteChannelControlPolicy,
  getChannelControlPolicies,
  getChannelGroups,
  upsertChannelControlPolicy,
} from '../../api'
import type {
  ChannelControlPolicy,
  ChannelControlPolicyInput,
  ChannelProbeMode,
} from '../../types'

const controlPoliciesQueryKey = ['channel-control-policies'] as const

const emptyPolicy = (): ChannelControlPolicyInput => ({
  group: '',
  enabled: true,
  adaptive_enabled: true,
  adaptive_window_seconds: 300,
  adaptive_min_samples: 5,
  adaptive_slow_threshold_ms: 4000,
  adaptive_min_weight: 1,
  adaptive_max_weight: 1000,
  adaptive_recovery_weight: 5,
  adaptive_cooldown_seconds: 180,
  probe_enabled: true,
  probe_mode: 'chat',
  probe_model: '',
  recovery_enabled: true,
  recovery_successes_required: 2,
})

const policyInputFrom = (
  policy: ChannelControlPolicy
): ChannelControlPolicyInput => ({
  group: policy.group,
  enabled: policy.enabled,
  adaptive_enabled: policy.adaptive_enabled,
  adaptive_window_seconds: policy.adaptive_window_seconds,
  adaptive_min_samples: policy.adaptive_min_samples,
  adaptive_slow_threshold_ms: policy.adaptive_slow_threshold_ms,
  adaptive_min_weight: policy.adaptive_min_weight,
  adaptive_max_weight: policy.adaptive_max_weight,
  adaptive_recovery_weight: policy.adaptive_recovery_weight,
  adaptive_cooldown_seconds: policy.adaptive_cooldown_seconds,
  probe_enabled: policy.probe_enabled,
  probe_mode: policy.probe_mode,
  probe_model: policy.probe_model,
  recovery_enabled: policy.recovery_enabled,
  recovery_successes_required: policy.recovery_successes_required,
})

type ChannelControlPoliciesDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ChannelControlPoliciesDialog({
  open,
  onOpenChange,
}: ChannelControlPoliciesDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<ChannelControlPolicyInput>(emptyPolicy)
  const [editingGroup, setEditingGroup] = useState<string | null>(null)
  const [creatingNew, setCreatingNew] = useState(false)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [showDeleteDialog, setShowDeleteDialog] = useState(false)
  const policiesQuery = useQuery({
    queryKey: controlPoliciesQueryKey,
    queryFn: getChannelControlPolicies,
    enabled: open,
  })
  const groupsQuery = useQuery({
    queryKey: ['channel-groups'],
    queryFn: getChannelGroups,
    enabled: open,
  })

  const policies = policiesQuery.data ?? []
  const channelGroups = groupsQuery.data?.data ?? []

  useEffect(() => {
    if (!open) {
      setCreatingNew(false)
      setEditingGroup(null)
      return
    }
    if (creatingNew) return
    if (editingGroup) {
      const current = policies.find((policy) => policy.group === editingGroup)
      if (current) setDraft(policyInputFrom(current))
      return
    }
    if (policies.length > 0) {
      const firstPolicy = policies[0]
      setEditingGroup(firstPolicy.group)
      setDraft(policyInputFrom(firstPolicy))
    }
  }, [creatingNew, editingGroup, open, policies])

  const selectPolicy = (policy: ChannelControlPolicy) => {
    setCreatingNew(false)
    setEditingGroup(policy.group)
    setDraft(policyInputFrom(policy))
  }

  const startNewPolicy = () => {
    setCreatingNew(true)
    setEditingGroup(null)
    setDraft(emptyPolicy())
  }

  const setNumber = (
    key:
      | 'adaptive_window_seconds'
      | 'adaptive_min_samples'
      | 'adaptive_slow_threshold_ms'
      | 'adaptive_min_weight'
      | 'adaptive_max_weight'
      | 'adaptive_recovery_weight'
      | 'adaptive_cooldown_seconds'
      | 'recovery_successes_required',
    value: string
  ) => {
    const number = Number(value)
    if (!Number.isFinite(number)) return
    setDraft((current) => ({ ...current, [key]: number }))
  }

  const handleSave = async () => {
    const group = draft.group.trim()
    if (!group) {
      toast.error(t('Group is required'))
      return
    }

    setSaving(true)
    try {
      const saved = await upsertChannelControlPolicy({ ...draft, group })
      setCreatingNew(false)
      setEditingGroup(saved.group)
      setDraft(policyInputFrom(saved))
      await queryClient.invalidateQueries({ queryKey: controlPoliciesQueryKey })
      toast.success(t('Policy saved'))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Failed to save'))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!editingGroup) return
    setDeleting(true)
    try {
      await deleteChannelControlPolicy(editingGroup)
      await queryClient.invalidateQueries({ queryKey: controlPoliciesQueryKey })
      setShowDeleteDialog(false)
      startNewPolicy()
      toast.success(t('Policy deleted'))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Failed to delete'))
    } finally {
      setDeleting(false)
    }
  }

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={onOpenChange}
        title={t('Group Control Policies')}
        contentClassName='sm:max-w-4xl'
        contentHeight='min(680px, calc(100vh - 12rem))'
        footer={
          <>
            {editingGroup ? (
              <Button
                variant='outline'
                className='text-destructive hover:text-destructive'
                onClick={() => setShowDeleteDialog(true)}
                disabled={saving || deleting}
              >
                <Trash2 className='h-4 w-4' />
                {t('Delete')}
              </Button>
            ) : null}
            <Button onClick={handleSave} disabled={saving || deleting}>
              {saving ? <Loader2 className='h-4 w-4 animate-spin' /> : null}
              {t('Save Policy')}
            </Button>
          </>
        }
      >
        <div className='grid gap-5 md:grid-cols-[12rem_minmax(0,1fr)]'>
          <div className='flex min-w-0 flex-col gap-2 md:border-r md:pr-5'>
            <Button variant='outline' size='sm' onClick={startNewPolicy}>
              <Plus className='h-4 w-4' />
              {t('New Policy')}
            </Button>
            {policiesQuery.isLoading ? (
              <div className='flex justify-center py-5'>
                <Loader2 className='text-muted-foreground h-4 w-4 animate-spin' />
              </div>
            ) : policies.length > 0 ? (
              policies.map((policy) => (
                <Button
                  key={policy.group}
                  variant={
                    editingGroup === policy.group ? 'secondary' : 'ghost'
                  }
                  className='justify-start truncate'
                  title={policy.group}
                  onClick={() => selectPolicy(policy)}
                >
                  {policy.group}
                </Button>
              ))
            ) : (
              <p className='text-muted-foreground px-1 py-3 text-sm'>
                {t('No group policies configured')}
              </p>
            )}
          </div>

          <div className='min-w-0 space-y-5'>
            <div className='space-y-2'>
              <Label htmlFor='control-policy-group'>{t('Group')}</Label>
              <Input
                id='control-policy-group'
                list='channel-control-policy-groups'
                value={draft.group}
                disabled={editingGroup !== null}
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    group: event.target.value,
                  }))
                }
              />
              <datalist id='channel-control-policy-groups'>
                {channelGroups.map((group) => (
                  <option key={group} value={group} />
                ))}
              </datalist>
            </div>

            <div className='grid gap-3 sm:grid-cols-2'>
              <label className='flex items-center justify-between gap-3 rounded-md border px-3 py-2'>
                <span className='text-sm'>{t('Policy enabled')}</span>
                <Switch
                  checked={draft.enabled}
                  onCheckedChange={(checked) =>
                    setDraft((current) => ({ ...current, enabled: checked }))
                  }
                />
              </label>
              <label className='flex items-center justify-between gap-3 rounded-md border px-3 py-2'>
                <span className='text-sm'>{t('Adaptive routing')}</span>
                <Switch
                  checked={draft.adaptive_enabled}
                  disabled={!draft.enabled}
                  onCheckedChange={(checked) =>
                    setDraft((current) => ({
                      ...current,
                      adaptive_enabled: checked,
                    }))
                  }
                />
              </label>
              <label className='flex items-center justify-between gap-3 rounded-md border px-3 py-2'>
                <span className='text-sm'>{t('Dedicated probe')}</span>
                <Switch
                  checked={draft.probe_enabled}
                  disabled={!draft.enabled}
                  onCheckedChange={(checked) =>
                    setDraft((current) => ({
                      ...current,
                      probe_enabled: checked,
                    }))
                  }
                />
              </label>
              <label className='flex items-center justify-between gap-3 rounded-md border px-3 py-2'>
                <span className='text-sm'>{t('Automatic recovery')}</span>
                <Switch
                  checked={draft.recovery_enabled}
                  disabled={!draft.enabled || !draft.probe_enabled}
                  onCheckedChange={(checked) =>
                    setDraft((current) => ({
                      ...current,
                      recovery_enabled: checked,
                    }))
                  }
                />
              </label>
            </div>

            <div className='grid gap-4 sm:grid-cols-2'>
              <div className='space-y-2'>
                <Label htmlFor='control-policy-probe-mode'>
                  {t('Probe mode')}
                </Label>
                <Select<ChannelProbeMode>
                  value={draft.probe_mode}
                  onValueChange={(value) => {
                    if (value === null) return
                    setDraft((current) => ({ ...current, probe_mode: value }))
                  }}
                  disabled={!draft.enabled || !draft.probe_enabled}
                >
                  <SelectTrigger
                    id='control-policy-probe-mode'
                    className='w-full'
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value='auto'>{t('Automatic')}</SelectItem>
                      <SelectItem value='chat'>
                        {t('Chat completions')}
                      </SelectItem>
                      <SelectItem value='responses'>{t('Responses')}</SelectItem>
                      <SelectItem value='image'>
                        {t('Image generation')}
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
              <div className='space-y-2'>
                <Label htmlFor='control-policy-probe-model'>
                  {t('Probe model')}
                </Label>
                <Input
                  id='control-policy-probe-model'
                  value={draft.probe_model}
                  disabled={!draft.enabled || !draft.probe_enabled}
                  onChange={(event) =>
                    setDraft((current) => ({
                      ...current,
                      probe_model: event.target.value,
                    }))
                  }
                />
              </div>
            </div>

            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
              <NumberField
                id='control-policy-window'
                label={t('Adaptive window (seconds)')}
                value={draft.adaptive_window_seconds}
                disabled={!draft.enabled || !draft.adaptive_enabled}
                onChange={(value) => setNumber('adaptive_window_seconds', value)}
              />
              <NumberField
                id='control-policy-samples'
                label={t('Minimum samples')}
                value={draft.adaptive_min_samples}
                disabled={!draft.enabled || !draft.adaptive_enabled}
                onChange={(value) => setNumber('adaptive_min_samples', value)}
              />
              <NumberField
                id='control-policy-slow-threshold'
                label={t('Slow threshold (ms)')}
                value={draft.adaptive_slow_threshold_ms}
                disabled={!draft.enabled || !draft.adaptive_enabled}
                onChange={(value) =>
                  setNumber('adaptive_slow_threshold_ms', value)
                }
              />
              <NumberField
                id='control-policy-min-weight'
                label={t('Minimum weight')}
                value={draft.adaptive_min_weight}
                disabled={!draft.enabled || !draft.adaptive_enabled}
                onChange={(value) => setNumber('adaptive_min_weight', value)}
              />
              <NumberField
                id='control-policy-max-weight'
                label={t('Maximum weight')}
                value={draft.adaptive_max_weight}
                disabled={!draft.enabled || !draft.adaptive_enabled}
                onChange={(value) => setNumber('adaptive_max_weight', value)}
              />
              <NumberField
                id='control-policy-recovery-weight'
                label={t('Recovery weight')}
                value={draft.adaptive_recovery_weight}
                disabled={!draft.enabled || !draft.adaptive_enabled}
                onChange={(value) =>
                  setNumber('adaptive_recovery_weight', value)
                }
              />
              <NumberField
                id='control-policy-cooldown'
                label={t('Cooldown (seconds)')}
                value={draft.adaptive_cooldown_seconds}
                disabled={!draft.enabled || !draft.adaptive_enabled}
                onChange={(value) =>
                  setNumber('adaptive_cooldown_seconds', value)
                }
              />
              <NumberField
                id='control-policy-recovery-successes'
                label={t('Required successful probes')}
                value={draft.recovery_successes_required}
                disabled={
                  !draft.enabled ||
                  !draft.probe_enabled ||
                  !draft.recovery_enabled
                }
                onChange={(value) =>
                  setNumber('recovery_successes_required', value)
                }
              />
            </div>
          </div>
        </div>
      </Dialog>

      <ConfirmDialog
        open={showDeleteDialog}
        onOpenChange={setShowDeleteDialog}
        title={t('Delete Policy?')}
        desc={t('This action cannot be undone.')}
        destructive
        isLoading={deleting}
        handleConfirm={handleDelete}
      />
    </>
  )
}

type NumberFieldProps = {
  id: string
  label: string
  value: number
  disabled: boolean
  onChange: (value: string) => void
}

function NumberField({
  id,
  label,
  value,
  disabled,
  onChange,
}: NumberFieldProps) {
  return (
    <div className='space-y-2'>
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type='number'
        min={0}
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
      />
    </div>
  )
}
