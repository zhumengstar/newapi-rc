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
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import {
  AlertCircle,
  Check,
  CheckCheck,
  ChevronDown,
  ChevronUp,
  ChevronsUpDown,
  Coins,
  Eraser,
  Eye,
  EyeOff,
  Filter,
  Layers,
  Pencil,
  Plus,
  Search,
  SlidersHorizontal,
  Sparkles,
  Trash2,
  X,
} from 'lucide-react'
import { Fragment, useCallback, useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  SideDrawerSection,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Combobox } from '@/components/ui/combobox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { MultiSelect } from '@/components/multi-select'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  EMPTY_PERMISSION_CATALOG,
  hasPermission,
  normalizeAdminPermissions,
} from '@/lib/admin-permissions'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { formatQuota, parseQuotaFromDollars } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { accountPasswordSchema } from '@/lib/password-policy'
import { ROLE } from '@/lib/roles'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import {
  createUser,
  updateUser,
  getUser,
  getGroups,
  getGroupDetails,
  getPerCallModelPrices,
  getPermissionCatalog,
} from '../api'
import { BINDING_FIELDS, ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import {
  userFormSchema,
  type UserFormValues,
  USER_FORM_DEFAULT_VALUES,
  transformFormDataToPayload,
  transformUserToFormDefaults,
  inferGroupFamily,
  sortGroupsByFamilyAndRatio,
} from '../lib'
import type { User } from '../types'
import { UserQuotaDialog } from './user-quota-dialog'
import { useUsers } from './users-provider'

type UsersMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: User
}

export function UsersMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: UsersMutateDrawerProps) {
  const { t } = useTranslation()
  const isUpdate = !!currentRow
  const { triggerRefresh } = useUsers()
  const currentUser = useAuthStore((s) => s.auth.user)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [quotaDialogOpen, setQuotaDialogOpen] = useState(false)

  // Fetch groups and group details
  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: async () => requireServerSuccess(await getGroups()),
    staleTime: 5 * 60 * 1000,
  })

  const { data: groupDetailsData } = useQuery({
    queryKey: ['group-details'],
    queryFn: async () => requireServerSuccess(await getGroupDetails()),
    staleTime: 5 * 60 * 1000,
  })

  const groupOptions = groupDetailsData?.data?.groups || groupsData?.data || []
  const groupMeta = groupDetailsData?.data?.meta || {}

  // Permission catalog is owned by the backend; fetched once and reused.
  const { data: permissionCatalog = EMPTY_PERMISSION_CATALOG } = useQuery({
    queryKey: ['admin-permission-catalog'],
    queryFn: async () => requireServerSuccess(await getPermissionCatalog()),
    staleTime: 5 * 60 * 1000,
  })

  // Per-call model prices catalog (root user only)
  const isRootUser = currentUser?.role === ROLE.SUPER_ADMIN
  const { data: perCallModelsData } = useQuery({
    queryKey: ['per-call-model-prices'],
    queryFn: async () => requireServerSuccess(await getPerCallModelPrices()),
    enabled: isRootUser,
    staleTime: 15 * 1000,
  })

  const perCallModels = useMemo(
    () =>
      Array.isArray(perCallModelsData?.data) ? perCallModelsData.data : [],
    [perCallModelsData]
  )
  const perCallGroups = useMemo(
    () => new Set(perCallModels.flatMap((item) => item.groups || [])),
    [perCallModels]
  )
  const perCallModelCountByGroup = useMemo(() => {
    const counts: Record<string, number> = {}
    for (const item of perCallModels) {
      for (const g of item.groups || []) {
        counts[g] = (counts[g] || 0) + 1
      }
    }
    return counts
  }, [perCallModels])

  const [groupDropdownOpen, setGroupDropdownOpen] = useState(false)
  const [groupSearchQuery, setGroupSearchQuery] = useState('')
  const [groupFilterTab, setGroupFilterTab] = useState<
    'all' | 'per_call' | 'selected'
  >('all')

  // Channel whitelist modal per group
  const [channelModalGroup, setChannelModalGroup] = useState<string | null>(null)
  const [channelSearchQuery, setChannelSearchQuery] = useState('')

  const [draftPriceGroup, setDraftPriceGroup] = useState('')
  const [draftPriceModels, setDraftPriceModels] = useState<string[]>([])
  const [draftModelPrice, setDraftModelPrice] = useState<string>('')
  const [expandedRuleIndices, setExpandedRuleIndices] = useState<Record<number, boolean>>({})

  // Rule models inline editing state
  const [editingRuleIndex, setEditingRuleIndex] = useState<number | null>(null)
  const [editingRuleModels, setEditingRuleModels] = useState<string[]>([])

  const getAvailableModelsForGroup = useCallback(
    (group: string) => {
      if (!group) return []
      const metaModels: string[] = Array.isArray(groupMeta[group]?.models)
        ? groupMeta[group].models
        : []
      const perCallItems = perCallModels.filter(
        (item) =>
          item.groups?.includes(group) ||
          item.groups?.includes('all')
      )
      const perCallMap = new Map(perCallItems.map((item) => [item.model, item]))

      const modelSet = new Set<string>([...metaModels, ...perCallItems.map((i) => i.model)])
      const allModelNames = Array.from(modelSet).sort()

      return allModelNames.map((modelName) => {
        const perCall = perCallMap.get(modelName)
        return {
          model: modelName,
          price: perCall?.price,
          has_global_price: Boolean(perCall?.has_global_price),
          is_per_call_default: Boolean(perCall),
        }
      })
    },
    [perCallModels, groupMeta]
  )

  const availableModelsForGroup = useMemo(
    () => getAvailableModelsForGroup(draftPriceGroup),
    [getAvailableModelsForGroup, draftPriceGroup]
  )

  const singleSelectedModelMeta = useMemo(() => {
    if (draftPriceModels.length !== 1) return null
    return availableModelsForGroup.find((m) => m.model === draftPriceModels[0])
  }, [draftPriceModels, availableModelsForGroup])

  const handleSelectAllGroupModels = () => {
    const allModels = availableModelsForGroup.map((m) => m.model)
    setDraftPriceModels(allModels)
  }

  const handleClearSelectedModels = () => {
    setDraftPriceModels([])
  }

  const handleUseReferencePrice = () => {
    if (singleSelectedModelMeta && typeof singleSelectedModelMeta.price === 'number') {
      setDraftModelPrice(String(singleSelectedModelMeta.price))
    }
  }

  const toggleRuleExpand = (idx: number) => {
    setExpandedRuleIndices((prev) => ({ ...prev, [idx]: !prev[idx] }))
  }

  const form = useForm<UserFormValues>({
    resolver: zodResolver(userFormSchema),
    defaultValues: USER_FORM_DEFAULT_VALUES,
  })

  // Reset drafts when closing drawer
  useEffect(() => {
    if (!open) {
      setGroupDropdownOpen(false)
      setGroupSearchQuery('')
      setGroupFilterTab('all')
      setDraftPriceGroup('')
      setDraftPriceModels([])
      setDraftModelPrice('')
      setExpandedRuleIndices({})
      setEditingRuleIndex(null)
      setEditingRuleModels([])
    }
  }, [open])

  // Load existing data when updating
  useEffect(() => {
    if (open && isUpdate && currentRow) {
      // For update, fetch fresh data
      getUser(currentRow.id)
        .then((result) => {
          if (result.success && result.data) {
            form.reset(transformUserToFormDefaults(result.data))
          } else {
            handleServerError(result, t('Failed to load'))
          }
        })
        .catch((error) => handleServerError(error, t('Failed to load')))
    } else if (open && !isUpdate) {
      // For create, reset to defaults
      form.reset(USER_FORM_DEFAULT_VALUES)
    }
  }, [open, isUpdate, currentRow, form, t])

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'

  const currentQuotaRaw = form.watch('quota_dollars') || 0
  const selectedRole = form.watch('role')
  const canEditAdminPermissions = currentUser?.role === ROLE.SUPER_ADMIN
  const targetIsAdmin = (selectedRole ?? currentRow?.role ?? 0) >= ROLE.ADMIN

  const handleAddPriceRule = () => {
    const priceNum = Number(draftModelPrice)
    if (!draftPriceGroup) {
      toast.error(t('Please select a group first'))
      return
    }
    if (draftPriceModels.length === 0) {
      toast.error(t('Please select one or more models'))
      return
    }
    if (!Number.isFinite(priceNum) || priceNum < 0) {
      toast.error(t('Price must be a valid non-negative number'))
      return
    }
    const currentRules = form.getValues('user_model_price_rules') || []
    const duplicate = currentRules.some(
      (rule) =>
        rule.group === draftPriceGroup &&
        rule.models.some((m) => draftPriceModels.includes(m))
    )
    if (duplicate) {
      toast.error(t('A model cannot have more than one price rule in the same group'))
      return
    }
    form.setValue('user_model_price_rules', [
      ...currentRules,
      {
        group: draftPriceGroup,
        models: [...draftPriceModels],
        price: priceNum,
      },
    ])

    // If the group is not yet assigned to the user, automatically add it
    const currentGroups = (form.getValues('group') || '')
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    const isPublic = Boolean(groupMeta[draftPriceGroup]?.is_public)
    if (!isPublic && !currentGroups.includes(draftPriceGroup)) {
      form.setValue('group', [...currentGroups, draftPriceGroup].join(','))
      toast.info(t('Auto-assigned group {{group}} to user', { group: draftPriceGroup }))
    }

    setDraftPriceModels([])
    setDraftModelPrice('')
  }

  const handleDeletePriceRule = (index: number) => {
    const currentRules = form.getValues('user_model_price_rules') || []
    form.setValue(
      'user_model_price_rules',
      currentRules.filter((_, i) => i !== index)
    )
    if (editingRuleIndex === index) {
      setEditingRuleIndex(null)
      setEditingRuleModels([])
    } else if (editingRuleIndex !== null && editingRuleIndex > index) {
      setEditingRuleIndex(editingRuleIndex - 1)
    }
  }

  const handleUpdatePriceRule = (index: number, newPrice: number) => {
    const currentRules = form.getValues('user_model_price_rules') || []
    form.setValue(
      'user_model_price_rules',
      currentRules.map((item, i) => (i === index ? { ...item, price: newPrice } : item))
    )
  }

  const handleStartEditRuleModels = (index: number) => {
    const rules = form.getValues('user_model_price_rules') || []
    const rule = rules[index]
    if (!rule) return

    const groupAvail = getAvailableModelsForGroup(rule.group)
    const validSet = new Set(groupAvail.map((m) => m.model))

    // 自动取消掉没有、不存在的模型
    const validSelected = rule.models.filter((m) => validSet.has(m))
    const removedModels = rule.models.filter((m) => !validSet.has(m))

    if (removedModels.length > 0) {
      toast.info(
        t('已自动取消 {{count}} 个不存在的模型: {{models}}', {
          count: removedModels.length,
          models: removedModels.join(', '),
        })
      )
    }

    setEditingRuleIndex(index)
    setEditingRuleModels(validSelected)
  }

  const handleCancelEditRuleModels = () => {
    setEditingRuleIndex(null)
    setEditingRuleModels([])
  }

  const handleSaveEditRuleModels = (index: number) => {
    if (editingRuleModels.length === 0) {
      toast.error(t('请至少选择一个模型，或直接删除该规则'))
      return
    }

    const currentRules = form.getValues('user_model_price_rules') || []
    const targetRule = currentRules[index]
    if (!targetRule) return

    const hasDuplicate = currentRules.some(
      (rule, i) =>
        i !== index &&
        rule.group === targetRule.group &&
        rule.models.some((m) => editingRuleModels.includes(m))
    )
    if (hasDuplicate) {
      toast.error(t('同一分组下的模型不能在多个规则中重复配置'))
      return
    }

    form.setValue(
      'user_model_price_rules',
      currentRules.map((r, i) =>
        i === index ? { ...r, models: [...editingRuleModels] } : r
      )
    )

    setEditingRuleIndex(null)
    setEditingRuleModels([])
    toast.success(t('已更新模型列表'))
  }

  const handleRemoveModelFromRule = (ruleIndex: number, modelToRemove: string) => {
    const currentRules = form.getValues('user_model_price_rules') || []
    const targetRule = currentRules[ruleIndex]
    if (!targetRule) return

    const newModels = targetRule.models.filter((m) => m !== modelToRemove)
    if (newModels.length === 0) {
      handleDeletePriceRule(ruleIndex)
      toast.info(t('规则中无剩余模型，已自动删除该规则'))
    } else {
      form.setValue(
        'user_model_price_rules',
        currentRules.map((r, i) =>
          i === ruleIndex ? { ...r, models: newModels } : r
        )
      )
    }
  }

  const handleAutoCleanInvalidModels = (index: number) => {
    const currentRules = form.getValues('user_model_price_rules') || []
    const targetRule = currentRules[index]
    if (!targetRule) return

    const groupAvail = getAvailableModelsForGroup(targetRule.group)
    const validSet = new Set(groupAvail.map((m) => m.model))
    const validModels = targetRule.models.filter((m) => validSet.has(m))
    const removedModels = targetRule.models.filter((m) => !validSet.has(m))

    if (removedModels.length === 0) {
      toast.info(t('所有模型均有效，无需清理'))
      return
    }

    if (validModels.length === 0) {
      handleDeletePriceRule(index)
      toast.info(t('所有模型均不存在，已自动删除该规则'))
    } else {
      form.setValue(
        'user_model_price_rules',
        currentRules.map((r, i) =>
          i === index ? { ...r, models: validModels } : r
        )
      )
      toast.success(
        t('已自动取消 {{count}} 个不存在的模型: {{models}}', {
          count: removedModels.length,
          models: removedModels.join(', '),
        })
      )
    }
  }

  const onSubmit = async (data: UserFormValues) => {
    if (!isUpdate || data.password) {
      if (!accountPasswordSchema.safeParse(data.password ?? '').success) {
        form.setError('password', {
          type: 'manual',
          message: t('Password must contain between 8 and 128 characters.'),
        })
        return
      }
    }

    setIsSubmitting(true)
    try {
      const finalData = { ...data }
      let finalPriceRules = [...(data.user_model_price_rules || [])]
      if (isRootUser && draftPriceGroup && draftPriceModels.length > 0 && draftModelPrice !== '') {
        const draftPrice = Number(draftModelPrice)
        if (Number.isFinite(draftPrice) && draftPrice >= 0) {
          const duplicate = finalPriceRules.some(
            (rule) =>
              rule.group === draftPriceGroup &&
              rule.models.some((m) => draftPriceModels.includes(m))
          )
          if (!duplicate) {
            finalPriceRules.push({
              group: draftPriceGroup,
              models: draftPriceModels,
              price: draftPrice,
            })
          }
        }
      }

      // 自动取消掉各规则中已不存在的模型
      if (Object.keys(groupMeta).length > 0) {
        finalPriceRules = finalPriceRules
          .map((rule) => {
            const avail = getAvailableModelsForGroup(rule.group)
            if (avail.length === 0) return rule
            const vSet = new Set(avail.map((m) => m.model))
            return {
              ...rule,
              models: rule.models.filter((m) => vSet.has(m)),
            }
          })
          .filter((rule) => rule.models.length > 0)
      }

      finalData.user_model_price_rules = finalPriceRules

      // 确保用户配置的所有非公开分组（包括倍率和按次专属定价中的分组）都包含在 group 列表中
      const currentSelectedGroups = (finalData.group || '')
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
      const allActiveGroups = new Set(currentSelectedGroups)
      if (finalData.user_group_ratios) {
        Object.keys(finalData.user_group_ratios).forEach((g) => {
          if (g && !groupMeta[g]?.is_public) {
            allActiveGroups.add(g)
          }
        })
      }
      finalPriceRules.forEach((rule) => {
        if (rule.group && !groupMeta[rule.group]?.is_public) {
          allActiveGroups.add(rule.group)
        }
      })
      finalData.group = Array.from(allActiveGroups).join(',')

      const payload = transformFormDataToPayload(
        finalData,
        currentRow?.id,
        permissionCatalog
      )
      const result = isUpdate
        ? await updateUser(payload as typeof payload & { id: number })
        : await createUser(payload)

      if (result.success) {
        toast.success(
          isUpdate
            ? t(SUCCESS_MESSAGES.USER_UPDATED)
            : t(SUCCESS_MESSAGES.USER_CREATED)
        )
        onOpenChange(false)
        triggerRefresh()
      } else {
        handleServerError(result, t(ERROR_MESSAGES.CREATE_FAILED))
      }
    } catch (error) {
      handleServerError(error, t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsSubmitting(false)
    }
  }

  const refreshUserData = async () => {
    if (!currentRow) return
    try {
      const result = requireServerSuccess(await getUser(currentRow.id))
      if (result.success && result.data) {
        form.reset(transformUserToFormDefaults(result.data))
      }
      triggerRefresh()
    } catch (error) {
      handleServerError(error, t('Failed to load'))
    }
  }

  return (
    <>
      <Sheet
        open={open}
        onOpenChange={(v) => {
          onOpenChange(v)
          if (!v) {
            form.reset()
          }
        }}
      >
        <SheetContent
          className={sideDrawerContentClassName('sm:max-w-[600px]')}
        >
          <SheetHeader className={sideDrawerHeaderClassName()}>
            <SheetTitle>
              {isUpdate ? t('Update') : t('Create')} {t('User')}
            </SheetTitle>
            <SheetDescription>
              {isUpdate
                ? t('Update the user by providing necessary info.')
                : t('Add a new user by providing necessary info.')}
            </SheetDescription>
          </SheetHeader>
          <Form {...form}>
            <form
              id='user-form'
              onSubmit={form.handleSubmit(onSubmit)}
              className={sideDrawerFormClassName()}
            >
              {/* Basic Information */}
              <SideDrawerSection>
                <h3 className='text-sm font-medium'>
                  {t('Basic Information')}
                </h3>

                <FormField
                  control={form.control}
                  name='username'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Username')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder={t('Enter username')}
                          disabled={isUpdate}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {!isUpdate && (
                  <FormField
                    control={form.control}
                    name='role'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Role')}</FormLabel>
                        <Select
                          items={[
                            { value: '1', label: t('Common User') },
                            { value: '10', label: t('Admin') },
                          ]}
                          onValueChange={(value) =>
                            value !== null &&
                            field.onChange(Number.parseInt(value))
                          }
                          value={String(field.value)}
                        >
                          <FormControl>
                            <SelectTrigger>
                              <SelectValue placeholder={t('Select a role')} />
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent alignItemWithTrigger={false}>
                            <SelectGroup>
                              <SelectItem value='1'>
                                {t('Common User')}
                              </SelectItem>
                              <SelectItem value='10'>{t('Admin')}</SelectItem>
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                        <FormDescription>
                          {t("Set the user's role (cannot be Root)")}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                )}

                <FormField
                  control={form.control}
                  name='display_name'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Display Name')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder={t('Enter display name')}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Leave empty to use username')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='password'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Password')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='password'
                          placeholder={
                            isUpdate
                              ? t('Leave empty to keep unchanged')
                              : t('Enter password (8–128 characters)')
                          }
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </SideDrawerSection>

              {/* Group & Quota Settings (Update only) */}
              {isUpdate && (
                <SideDrawerSection>
                  <h3 className='text-sm font-medium'>{t('Group & Quota')}</h3>

                  <FormField
                    control={form.control}
                    name='group'
                    render={({ field }) => {
                      const selectedGroupList = (field.value || '')
                        .split(',')
                        .map((s) => s.trim())
                        .filter(Boolean)
                      const userGroupRatios = form.watch('user_group_ratios') || {}
                      const disabledGroups = form.watch('disabled_groups') || []
                      const userGroupChannels = form.watch('user_group_channels') || {}

                      const handleUpdateGroupChannels = (group: string, channelIds: number[]) => {
                        const current = { ...(form.getValues('user_group_channels') || {}) }
                        if (!channelIds || channelIds.length === 0) {
                          delete current[group]
                        } else {
                          current[group] = channelIds
                        }
                        form.setValue('user_group_channels', current)
                      }

                      const handleToggleDisableGroup = (group: string) => {
                        const isCurrentlyDisabled = disabledGroups.includes(group)
                        let nextDisabled: string[]
                        if (isCurrentlyDisabled) {
                          nextDisabled = disabledGroups.filter((g) => g !== group)
                        } else {
                          nextDisabled = Array.from(new Set([...disabledGroups, group]))
                        }
                        form.setValue('disabled_groups', nextDisabled)
                      }

                      const handleToggleGroup = (group: string, checked: boolean) => {
                        const meta = groupMeta[group]
                        if (meta?.is_public) {
                          if (checked) {
                            handleRatioChange(
                              group,
                              meta?.ratio !== undefined ? String(meta.ratio) : '1'
                            )
                          } else {
                            handleRatioChange(group, '')
                            if (selectedGroupList.includes(group)) {
                              field.onChange(
                                selectedGroupList.filter((g) => g !== group).join(',')
                              )
                            }
                            // 移除该分组时同时移出 disabledGroups
                            if (disabledGroups.includes(group)) {
                              form.setValue(
                                'disabled_groups',
                                disabledGroups.filter((g) => g !== group)
                              )
                            }
                          }
                          return
                        }
                        let nextGroups: string[]
                        if (checked) {
                          nextGroups = Array.from(
                            new Set([...selectedGroupList, group])
                          )
                        } else {
                          nextGroups = selectedGroupList.filter(
                            (g) => g !== group
                          )
                          const nextRatios = {
                            ...(form.getValues('user_group_ratios') || {}),
                          }
                          delete nextRatios[group]
                          form.setValue('user_group_ratios', nextRatios)
                          // 移除该分组时同时移出 disabledGroups
                          if (disabledGroups.includes(group)) {
                            form.setValue(
                              'disabled_groups',
                              disabledGroups.filter((g) => g !== group)
                            )
                          }
                        }
                        field.onChange(nextGroups.join(','))
                      }

                      const handleRatioChange = (group: string, val: string) => {
                        const nextRatios = {
                          ...(form.getValues('user_group_ratios') || {}),
                        }
                        if (
                          val === '' ||
                          val === null ||
                          val === undefined
                        ) {
                          delete nextRatios[group]
                        } else {
                          const num = Number(val)
                          if (Number.isFinite(num) && num >= 0) {
                            nextRatios[group] = num
                          }
                        }
                        form.setValue('user_group_ratios', nextRatios)
                      }

                      const configuredGroups = Array.from(
                        new Set([
                          ...selectedGroupList,
                          ...Object.keys(userGroupRatios).filter(Boolean),
                        ])
                      )

                      const queryTokens = groupSearchQuery.trim().toLowerCase().split(/\s+/).filter(Boolean)
                      const filteredGroups = groupOptions.filter((group) => {
                        if (queryTokens.length > 0) {
                          const gLower = group.toLowerCase()
                          if (!queryTokens.every((token) => gLower.includes(token))) {
                            return false
                          }
                        }
                        if (groupFilterTab === 'per_call') {
                          return perCallGroups.has(group)
                        }
                        if (groupFilterTab === 'selected') {
                          return (
                            selectedGroupList.includes(group) ||
                            userGroupRatios[group] !== undefined
                          )
                        }
                        return true
                      })

                      const dropdownFilteredGroups = sortGroupsByFamilyAndRatio(filteredGroups, {
                        userGroupRatios,
                        groupMeta,
                      })

                      const sortedSelectedGroupList = sortGroupsByFamilyAndRatio(
                        configuredGroups,
                        {
                          userGroupRatios,
                          groupMeta,
                        }
                      )

                      const familyGroupCounts = dropdownFilteredGroups.reduce<
                        Record<string, number>
                      >((acc, g) => {
                        const fam = inferGroupFamily(
                          g,
                          groupMeta[g]?.desc,
                          groupMeta[g]?.models
                        )
                        acc[fam.id] = (acc[fam.id] || 0) + 1
                        return acc
                      }, {})

                      let lastFamilyId = ''

                      return (
                        <FormItem>
                          <div className='flex items-center justify-between'>
                            <FormLabel>{t('Groups & Multipliers')}</FormLabel>
                            <span className='text-[11px] text-muted-foreground'>
                              {t('下拉勾选分组，并在右侧指定倍率')}
                            </span>
                          </div>

                          <div className='space-y-3'>
                            {/* 下拉多选与行内倍率选择器 */}
                            <Popover open={groupDropdownOpen} onOpenChange={setGroupDropdownOpen}>
                              <PopoverTrigger
                                render={
                                  <Button
                                    variant='outline'
                                    type='button'
                                    className='w-full justify-between h-10 px-3 border-dashed hover:border-primary/60 bg-background'
                                  />
                                }
                              >
                                <div className='flex items-center gap-2 truncate'>
                                    <Layers className='size-4 text-muted-foreground shrink-0' />
                                    <span className='text-xs font-normal'>
                                      {configuredGroups.length > 0
                                        ? t('Selected {{count}} groups', {
                                            count: configuredGroups.length,
                                          })
                                        : t('Click to select groups...')}
                                    </span>
                                    {configuredGroups.length > 0 && (
                                      <Badge
                                        variant='secondary'
                                        className='text-[11px] h-5 px-1.5 font-normal'
                                      >
                                        {configuredGroups.length}
                                      </Badge>
                                    )}
                                  </div>
                                  <ChevronsUpDown className='size-4 text-muted-foreground shrink-0 opacity-60' />
                              </PopoverTrigger>
                              <PopoverContent
                                className='w-[min(500px,calc(100vw-3rem))] p-0'
                                align='start'
                              >
                                <div className='p-2.5 border-b space-y-2'>
                                  <div className='relative'>
                                    <Search className='absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground' />
                                    <Input
                                      placeholder={t('Search groups...')}
                                      value={groupSearchQuery}
                                      onChange={(e) => setGroupSearchQuery(e.target.value)}
                                      className='h-8 pl-8 text-xs'
                                    />
                                  </div>
                                  <div className='flex items-center gap-1 pt-0.5'>
                                    <Button
                                      type='button'
                                      variant={groupFilterTab === 'all' ? 'secondary' : 'ghost'}
                                      size='sm'
                                      className='h-6 text-[11px] px-2'
                                      onClick={() => setGroupFilterTab('all')}
                                    >
                                      {t('All Groups')} ({groupOptions.length})
                                    </Button>
                                    <Button
                                      type='button'
                                      variant={groupFilterTab === 'per_call' ? 'secondary' : 'ghost'}
                                      size='sm'
                                      className={cn(
                                        'h-6 text-[11px] px-2 flex items-center gap-1',
                                        groupFilterTab === 'per_call'
                                          ? 'bg-amber-100 text-amber-900 dark:bg-amber-950/60 dark:text-amber-300'
                                          : 'text-amber-700 hover:bg-amber-50 dark:text-amber-400 dark:hover:bg-amber-950/40'
                                      )}
                                      onClick={() => setGroupFilterTab('per_call')}
                                    >
                                      <Sparkles className='size-3 text-amber-500' />
                                      {t('Per-call Billing Groups')} ({perCallGroups.size})
                                    </Button>
                                    <Button
                                      type='button'
                                      variant={groupFilterTab === 'selected' ? 'secondary' : 'ghost'}
                                      size='sm'
                                      className='h-6 text-[11px] px-2'
                                      onClick={() => setGroupFilterTab('selected')}
                                    >
                                      {t('Selected Groups')} ({configuredGroups.length})
                                    </Button>
                                  </div>
                                </div>

                                <div className='max-h-72 overflow-y-auto p-1.5 space-y-1 divide-y divide-border/20'>
                                  {dropdownFilteredGroups.length === 0 ? (
                                    <div className='py-6 text-center text-xs text-muted-foreground'>
                                      {t('No matching groups')}
                                    </div>
                                  ) : (
                                    dropdownFilteredGroups.map((group) => {
                                      const meta = groupMeta[group]
                                      const isPublic = Boolean(meta?.is_public)
                                      const customRatio = userGroupRatios[group]
                                      const isChecked = isPublic
                                        ? customRatio !== undefined
                                        : selectedGroupList.includes(group)
                                      const hasPerCall = perCallGroups.has(group)
                                      const perCallCount =
                                        perCallModelCountByGroup[group] || 0
                                      const family = inferGroupFamily(
                                        group,
                                        meta?.desc,
                                        meta?.models
                                      )
                                      const isNewFamily = family.id !== lastFamilyId
                                      if (isNewFamily) {
                                        lastFamilyId = family.id
                                      }

                                      return (
                                        <Fragment key={group}>
                                          {isNewFamily && (
                                            <div className='sticky top-0 z-10 flex items-center justify-between px-2.5 py-1 mt-2 first:mt-0 bg-muted/90 backdrop-blur-md rounded text-[11px] font-semibold border border-border/40 shadow-xs'>
                                              <div className='flex items-center gap-1.5'>
                                                <span
                                                  className='size-2 rounded-full inline-block shrink-0'
                                                  style={{ backgroundColor: family.color }}
                                                />
                                                <span className='text-foreground font-medium'>
                                                  {family.name}
                                                </span>
                                              </div>
                                              <span className='text-[10px] font-normal text-muted-foreground'>
                                                {familyGroupCounts[family.id] || 0} {t('个分组')}
                                              </span>
                                            </div>
                                          )}
                                          <div
                                            className={cn(
                                              'flex items-center justify-between gap-2 px-2 py-1.5 rounded-md transition-colors',
                                              isChecked ? 'bg-primary/5' : 'hover:bg-muted/50'
                                            )}
                                          >
                                            {/* 左侧：勾选框 + 分组名 + 标签 */}
                                            <div
                                              className='flex items-center gap-2 min-w-0 flex-1 cursor-pointer select-none'
                                              onClick={() => {
                                                handleToggleGroup(group, !isChecked)
                                              }}
                                            >
                                              <Checkbox
                                                id={`pop-chk-${group}`}
                                                checked={isChecked}
                                                onCheckedChange={(checked) =>
                                                  handleToggleGroup(group, Boolean(checked))
                                                }
                                                onClick={(e) => e.stopPropagation()}
                                              />
                                              <span
                                                className='text-xs font-medium truncate'
                                                title={group}
                                              >
                                                {group}
                                              </span>
                                              {meta?.ratio !== undefined && (
                                                <Badge
                                                  variant='outline'
                                                  className='h-4 px-1 text-[10px] font-mono border-blue-200 text-blue-600 bg-blue-50 dark:border-blue-900/50 dark:text-blue-400 dark:bg-blue-950/40 shrink-0'
                                                >
                                                  {meta.ratio}x
                                                </Badge>
                                              )}
                                              {isPublic && (
                                                <Badge
                                                  variant='outline'
                                                  className='h-4 px-1 text-[10px] font-normal border-emerald-200 text-emerald-600 bg-emerald-50 dark:border-emerald-900/50 dark:text-emerald-400 dark:bg-emerald-950/40 shrink-0'
                                                >
                                                  {t('Public')}
                                                </Badge>
                                              )}
                                              {hasPerCall ? (
                                                <Badge
                                                  variant='outline'
                                                  className='h-4 px-1 text-[10px] font-normal border-amber-300 text-amber-700 bg-amber-50 dark:border-amber-900/50 dark:text-amber-400 dark:bg-amber-950/40 shrink-0 flex items-center gap-0.5'
                                                >
                                                  <Sparkles className='size-2.5' />
                                                  {t('{{count}} per-call models', {
                                                    count: perCallCount,
                                                  })}
                                                </Badge>
                                              ) : (meta?.models?.length || 0) > 0 ? (
                                                <Badge
                                                  variant='outline'
                                                  className='h-4 px-1 text-[10px] font-normal border-border text-muted-foreground bg-muted/40 shrink-0'
                                                >
                                                  {(meta?.models?.length || 0)} {t('个模型')}
                                                </Badge>
                                              ) : (
                                                <Badge
                                                  variant='outline'
                                                  className='h-4 px-1 text-[10px] font-normal border-rose-200 text-rose-500 bg-rose-50 dark:border-rose-950 dark:text-rose-400 dark:bg-rose-950/40 shrink-0'
                                                >
                                                  {t('无关联渠道')}
                                                </Badge>
                                              )}
                                            </div>

                                            {/* 右侧：指定专属倍率 */}
                                            <div
                                              className='flex items-center gap-1.5 shrink-0'
                                              onClick={(e) => e.stopPropagation()}
                                            >
                                              <span className='text-[11px] text-muted-foreground whitespace-nowrap'>
                                                {t('Custom Ratio')}:
                                              </span>
                                              <Input
                                                type='number'
                                                min='0'
                                                step='0.001'
                                                disabled={!isPublic && !isChecked}
                                                className='h-7 w-20 px-1.5 text-xs font-mono text-right'
                                                placeholder={
                                                  meta?.ratio !== undefined
                                                    ? `${meta.ratio}`
                                                    : '1'
                                                }
                                                value={
                                                  customRatio !== undefined
                                                    ? String(customRatio)
                                                    : ''
                                                }
                                                onChange={(e) =>
                                                  handleRatioChange(group, e.target.value)
                                                }
                                              />
                                            </div>
                                          </div>
                                        </Fragment>
                                      )
                                    })
                                  )}
                                </div>
                              </PopoverContent>
                            </Popover>

                            {/* 下方展示已选分组列表，清晰整洁 */}
                            {sortedSelectedGroupList.length > 0 ? (
                              <div className='rounded-lg border border-border/70 overflow-hidden bg-card/40'>
                                <div className='bg-muted/30 px-3 py-1.5 border-b border-border/40 flex items-center justify-between'>
                                  <span className='text-xs font-medium text-muted-foreground'>
                                    {t('Selected Groups')} ({sortedSelectedGroupList.length})
                                  </span>
                                  <span className='text-[11px] text-muted-foreground'>
                                    {t('未填写倍率将继承默认倍率')}
                                  </span>
                                </div>
                                <div className='divide-y divide-border/30 max-h-60 overflow-y-auto'>
                                  {sortedSelectedGroupList.map((group) => {
                                    const meta = groupMeta[group]
                                    const isPublic = Boolean(meta?.is_public)
                                    const customRatio = userGroupRatios[group]
                                    const hasPerCall = perCallGroups.has(group)
                                    const perCallCount =
                                      perCallModelCountByGroup[group] || 0
                                    const family = inferGroupFamily(
                                      group,
                                      meta?.desc,
                                      meta?.models
                                    )
                                    const isDisabled = disabledGroups.includes(group)

                                    return (
                                      <div
                                        key={group}
                                        className={cn(
                                          'flex items-center justify-between gap-3 px-3 py-2 transition-colors',
                                          isDisabled
                                            ? 'bg-muted/40 hover:bg-muted/50 opacity-60'
                                            : 'hover:bg-muted/20'
                                        )}
                                      >
                                        <div className='flex items-center gap-2 min-w-0 flex-1'>
                                          <Badge
                                            variant='outline'
                                            className={cn(
                                              'h-4 px-1 text-[10px] shrink-0 font-normal',
                                              family.badgeClassName
                                            )}
                                          >
                                            {family.name.split(' / ')[0]}
                                          </Badge>
                                          <button
                                            type='button'
                                            onClick={() => {
                                              setChannelModalGroup(group)
                                              setChannelSearchQuery('')
                                            }}
                                            className={cn(
                                              'text-xs font-medium text-foreground truncate hover:text-primary hover:underline underline-offset-2 flex items-center gap-1 cursor-pointer text-left transition-colors',
                                              isDisabled && 'line-through text-muted-foreground'
                                            )}
                                            title={t('点击配置此分组下的允许渠道（默认全部允许）')}
                                          >
                                            {group}
                                            <SlidersHorizontal className='size-2.5 text-muted-foreground/70 shrink-0' />
                                          </button>
                                          {userGroupChannels[group] && userGroupChannels[group].length > 0 && (
                                            <Badge
                                              variant='outline'
                                              className='h-4 px-1.5 text-[10px] border-primary/40 text-primary bg-primary/10 cursor-pointer hover:bg-primary/20 shrink-0 font-medium'
                                              onClick={() => {
                                                setChannelModalGroup(group)
                                                setChannelSearchQuery('')
                                              }}
                                              title={t('点击修改限定渠道')}
                                            >
                                              {t('限定 {{count}} 渠道', {
                                                count: userGroupChannels[group].length,
                                              })}
                                            </Badge>
                                          )}
                                          {isDisabled && (
                                            <Badge
                                              variant='outline'
                                              className='h-4 px-1 text-[10px] border-zinc-400 text-zinc-600 bg-zinc-100 dark:border-zinc-700 dark:text-zinc-400 dark:bg-zinc-900/60'
                                            >
                                              {t('已置灰')}
                                            </Badge>
                                          )}
                                          {meta?.ratio !== undefined && (
                                            <Badge
                                              variant='outline'
                                              className={cn(
                                                'h-4 px-1 text-[10px] font-mono border-blue-200 text-blue-600 bg-blue-50 dark:border-blue-900/50 dark:text-blue-400 dark:bg-blue-950/40',
                                                isDisabled && 'opacity-60 grayscale'
                                              )}
                                            >
                                              {meta.ratio}x
                                            </Badge>
                                          )}
                                          {isPublic && (
                                            <Badge
                                              variant='outline'
                                              className='h-4 px-1 text-[10px] border-emerald-200 text-emerald-600 bg-emerald-50 dark:border-emerald-900/50 dark:text-emerald-400 dark:bg-emerald-950/40'
                                            >
                                              {t('Public')}
                                            </Badge>
                                          )}
                                          {hasPerCall ? (
                                            <Badge
                                              variant='outline'
                                              className={cn(
                                                'h-4 px-1 text-[10px] border-amber-300 text-amber-700 bg-amber-50 dark:border-amber-900/50 dark:text-amber-400 dark:bg-amber-950/40 flex items-center gap-0.5',
                                                isDisabled && 'opacity-60 grayscale'
                                              )}
                                            >
                                              <Sparkles className='size-2.5' />
                                              {t('{{count}} per-call models', {
                                                count: perCallCount,
                                              })}
                                            </Badge>
                                          ) : (meta?.models?.length || 0) > 0 ? (
                                            <Badge
                                              variant='outline'
                                              className='h-4 px-1 text-[10px] border-border text-muted-foreground bg-muted/40'
                                            >
                                              {(meta?.models?.length || 0)} {t('个模型 (按量)')}
                                            </Badge>
                                          ) : (
                                            <Badge
                                              variant='outline'
                                              className='h-4 px-1 text-[10px] border-rose-200 text-rose-500 bg-rose-50 dark:border-rose-950 dark:text-rose-400 dark:bg-rose-950/40'
                                            >
                                              {t('无关联渠道')}
                                            </Badge>
                                          )}
                                        </div>

                                        <div className='flex items-center gap-2 shrink-0'>
                                          <Input
                                            type='number'
                                            min='0'
                                            step='0.001'
                                            className={cn(
                                              'h-7 w-20 px-1.5 text-xs font-mono text-right',
                                              isDisabled && 'opacity-60'
                                            )}
                                            placeholder={
                                              meta?.ratio !== undefined
                                                ? `${meta.ratio}`
                                                : '1'
                                            }
                                            value={
                                              customRatio !== undefined
                                                ? String(customRatio)
                                                : ''
                                            }
                                            onChange={(e) =>
                                              handleRatioChange(group, e.target.value)
                                            }
                                          />
                                          <Button
                                            variant='ghost'
                                            size='icon'
                                            type='button'
                                            className={cn(
                                              'size-7 shrink-0 transition-colors',
                                              isDisabled
                                                ? 'text-amber-600 hover:text-amber-700 bg-amber-50 dark:bg-amber-950/40 hover:bg-amber-100'
                                                : 'text-muted-foreground hover:text-foreground'
                                            )}
                                            title={isDisabled ? t('启用分组') : t('置灰禁用分组 (保留配置)')}
                                            onClick={() => handleToggleDisableGroup(group)}
                                          >
                                            {isDisabled ? (
                                              <EyeOff className='size-3.5' />
                                            ) : (
                                              <Eye className='size-3.5' />
                                            )}
                                          </Button>
                                          {!isPublic ? (
                                            <Button
                                              variant='ghost'
                                              size='icon'
                                              type='button'
                                              className='size-7 text-muted-foreground hover:text-destructive shrink-0'
                                              title={t('Remove')}
                                              onClick={() => handleToggleGroup(group, false)}
                                            >
                                              <X className='size-3.5' />
                                            </Button>
                                          ) : customRatio !== undefined ? (
                                            <Button
                                              variant='ghost'
                                              size='icon'
                                              type='button'
                                              className='size-7 text-muted-foreground hover:text-destructive shrink-0'
                                              title={t('Reset to default ratio')}
                                              onClick={() => handleToggleGroup(group, false)}
                                            >
                                              <X className='size-3.5' />
                                            </Button>
                                          ) : null}
                                        </div>
                                      </div>
                                    )
                                  })}
                                </div>
                              </div>
                            ) : (
                              <p className='text-xs text-destructive'>
                                {t('Please select at least one group')}
                              </p>
                            )}
                          </div>
                          <FormMessage />
                        </FormItem>
                      )
                    }}
                  />

                  {isRootUser && (
                    <div className='space-y-2 pt-1'>
                      <div className='flex items-center justify-between'>
                        <div>
                          <Label className='text-xs font-medium flex items-center gap-1.5'>
                            <SlidersHorizontal className='size-3.5 text-primary' />
                            {t('Per-call Model Pricing')}
                          </Label>
                          <p className='text-[11px] text-muted-foreground mt-0.5'>
                            {t('Set dedicated per-call pricing for specific models in specific groups')}
                          </p>
                        </div>
                        {(form.watch('user_model_price_rules') || []).length > 0 && (
                          <Badge
                            variant='outline'
                            className='text-[10px] font-mono text-muted-foreground bg-muted/40 shrink-0'
                          >
                            {(form.watch('user_model_price_rules') || []).length} {t('Rules configured')}
                          </Badge>
                        )}
                      </div>

                      <div className='rounded-xl border border-border/80 p-3.5 space-y-3.5 bg-card/50'>
                        {/* Layer 1: Target Group */}
                        <div className='space-y-1.5'>
                          <div className='flex items-center justify-between'>
                            <Label className='text-[11px] font-medium text-muted-foreground'>
                              {t('Target Group')}
                            </Label>
                            {draftPriceGroup && (
                              <span className='text-[10px] text-muted-foreground font-mono'>
                                {availableModelsForGroup.length} {t('个可用模型')}
                              </span>
                            )}
                          </div>
                          <Select
                            value={draftPriceGroup || undefined}
                            onValueChange={(val) => {
                              setDraftPriceGroup(val || '')
                              setDraftPriceModels([])
                              setDraftModelPrice('')
                            }}
                          >
                            <SelectTrigger className='h-9 text-xs w-full bg-background'>
                              <SelectValue placeholder={t('Select Group')} />
                            </SelectTrigger>
                            <SelectContent
                              alignItemWithTrigger={false}
                              className='w-auto min-w-[280px] max-w-[min(460px,calc(100vw-2rem))] p-1 shadow-lg'
                            >
                              <SelectGroup>
                                {sortGroupsByFamilyAndRatio(groupOptions, {
                                  groupMeta,
                                }).map((group) => {
                                  const perCallCount = perCallModelCountByGroup[group] || 0
                                  const totalModels = groupMeta[group]?.models?.length || 0
                                  const fam = inferGroupFamily(
                                    group,
                                    groupMeta[group]?.desc,
                                    groupMeta[group]?.models
                                  )
                                  return (
                                    <SelectItem
                                      key={group}
                                      value={group}
                                      className='text-xs py-2 px-2.5 cursor-pointer rounded-md'
                                    >
                                      <div className='flex items-center justify-between gap-3 w-full'>
                                        <div className='flex items-center gap-1.5 min-w-0'>
                                          <Badge
                                            variant='outline'
                                            className={cn(
                                              'h-3.5 px-1 text-[9px] shrink-0 font-normal',
                                              fam.badgeClassName
                                            )}
                                          >
                                            {fam.name.split(' / ')[0]}
                                          </Badge>
                                          <span className='font-medium text-foreground truncate'>
                                            {group}
                                          </span>
                                        </div>
                                        <span className='inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-normal bg-muted text-muted-foreground shrink-0 border border-border/50'>
                                          {perCallCount > 0
                                            ? `${perCallCount} ${t('个按次模型')}`
                                            : `${totalModels} ${t('个模型 (按量)')}`}
                                        </span>
                                      </div>
                                    </SelectItem>
                                  )
                                })}
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </div>

                        {/* Layer 2: Models Selection */}
                        <div className='space-y-1.5'>
                          <div className='flex items-center justify-between'>
                            <Label className='text-[11px] font-medium text-muted-foreground'>
                              {t('Per-call Models')}
                              {draftPriceGroup && availableModelsForGroup.length > 0 && (
                                <span className='ml-1.5 text-[10px] text-muted-foreground font-normal'>
                                  ({draftPriceModels.length}/{availableModelsForGroup.length})
                                </span>
                              )}
                            </Label>
                            {draftPriceGroup && availableModelsForGroup.length > 0 && (
                              <div className='flex items-center gap-2'>
                                {draftPriceModels.length < availableModelsForGroup.length && (
                                  <Button
                                    type='button'
                                    variant='ghost'
                                    size='sm'
                                    className='h-5 px-1.5 text-[10px] text-primary hover:text-primary hover:bg-primary/10'
                                    onClick={handleSelectAllGroupModels}
                                  >
                                    <CheckCheck className='size-3 mr-1' />
                                    {t('Select all in group')} ({availableModelsForGroup.length})
                                  </Button>
                                )}
                                {draftPriceModels.length > 0 && (
                                  <Button
                                    type='button'
                                    variant='ghost'
                                    size='sm'
                                    className='h-5 px-1.5 text-[10px] text-muted-foreground hover:text-destructive'
                                    onClick={handleClearSelectedModels}
                                  >
                                    <X className='size-3 mr-0.5' />
                                    {t('Clear selection')}
                                  </Button>
                                )}
                              </div>
                            )}
                          </div>

                          <MultiSelect
                            disabled={!draftPriceGroup}
                            options={availableModelsForGroup.map((item) => ({
                              value: item.model,
                              label: item.model,
                              description: (
                                <Badge
                                  variant='outline'
                                  className={cn(
                                    'h-4 px-1 text-[10px] font-mono border-border/60',
                                    item.has_global_price
                                      ? 'text-muted-foreground bg-muted/40'
                                      : 'text-amber-600 bg-amber-50 dark:text-amber-400 dark:bg-amber-950/40 border-amber-200 dark:border-amber-900/50'
                                  )}
                                >
                                  {item.has_global_price
                                    ? `$${item.price}/${t('call')}`
                                    : t('按量转按次')}
                                </Badge>
                              ),
                            }))}
                            selected={draftPriceModels}
                            onChange={setDraftPriceModels}
                            placeholder={
                              !draftPriceGroup
                                ? t('Select group first')
                                : availableModelsForGroup.length === 0
                                  ? t('No models available in this group')
                                  : t('Select one or more models')
                            }
                            className='min-h-9 text-xs bg-background w-full'
                            maxVisibleChips={3}
                            copyChipOnClick={true}
                          />
                        </div>

                        {/* Layer 3: Price & Add Action */}
                        <div className='pt-0.5 space-y-1.5'>
                          <div className='flex items-center justify-between'>
                            <Label className='text-[11px] font-medium text-muted-foreground'>
                              {t('Dedicated Price')}
                            </Label>
                            {singleSelectedModelMeta && singleSelectedModelMeta.has_global_price && (
                              <button
                                type='button'
                                onClick={handleUseReferencePrice}
                                className='text-[10px] text-primary hover:underline cursor-pointer flex items-center gap-1 transition-colors'
                                title={t('Click to fill system reference price')}
                              >
                                <span>{t('System reference:')} ${singleSelectedModelMeta.price}/{t('call')}</span>
                                <span className='text-[9px] px-1 py-0.2 rounded bg-primary/10 font-medium'>{t('Fill')}</span>
                              </button>
                            )}
                          </div>
                          <div className='flex items-center gap-2'>
                            <div className='relative flex-1'>
                              <span className='absolute left-2.5 top-1/2 -translate-y-1/2 text-xs font-semibold text-muted-foreground'>
                                $
                              </span>
                              <Input
                                type='number'
                                min='0'
                                step='0.001'
                                className='h-9 pl-6 pr-12 text-xs font-mono bg-background'
                                placeholder={t('Price/call')}
                                value={draftModelPrice}
                                onChange={(e) => setDraftModelPrice(e.target.value)}
                              />
                              <span className='absolute right-2.5 top-1/2 -translate-y-1/2 text-[11px] text-muted-foreground font-mono'>
                                /{t('call')}
                              </span>
                            </div>

                            <Button
                              type='button'
                              size='sm'
                              className='h-9 px-4 shrink-0'
                              disabled={
                                !draftPriceGroup ||
                                draftPriceModels.length === 0 ||
                                draftModelPrice === ''
                              }
                              onClick={handleAddPriceRule}
                            >
                              <Plus className='mr-1.5 h-3.5 w-3.5' />
                              {t('Add Rule')}
                            </Button>
                          </div>
                        </div>

                        {/* Configured Rules list */}
                        {/* Configured Rules list */}
                        {(() => {
                          const configuredRules = form.watch('user_model_price_rules') || []
                          if (configuredRules.length === 0) return null

                          const totalInvalidCount = configuredRules.reduce((acc, rule) => {
                            const groupAvail = getAvailableModelsForGroup(rule.group)
                            const validSet = new Set(groupAvail.map((m) => m.model))
                            return acc + rule.models.filter((m) => !validSet.has(m)).length
                          }, 0)

                          return (
                            <div className='space-y-2 border-t border-border/60 pt-3'>
                              <div className='flex items-center justify-between text-[11px] font-medium text-muted-foreground'>
                                <div className='flex items-center gap-1.5'>
                                  <span>
                                    {t('Configured Rules')} ({configuredRules.length})
                                  </span>
                                  {totalInvalidCount > 0 && (
                                    <Badge
                                      variant='outline'
                                      className='h-4 px-1 text-[9px] text-amber-600 dark:text-amber-400 border-amber-300 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/40'
                                    >
                                      {totalInvalidCount} {t('个失效模型')}
                                    </Badge>
                                  )}
                                </div>

                                {totalInvalidCount > 0 && (
                                  <Button
                                    type='button'
                                    variant='ghost'
                                    size='sm'
                                    className='h-6 px-1.5 text-[10px] text-amber-600 dark:text-amber-400 hover:text-amber-700 dark:hover:text-amber-300 hover:bg-amber-100/50 dark:hover:bg-amber-900/30 gap-1'
                                    onClick={() => {
                                      const currentRules = form.getValues('user_model_price_rules') || []
                                      const cleaned = currentRules
                                        .map((r) => {
                                          const groupAvail = getAvailableModelsForGroup(r.group)
                                          const validSet = new Set(groupAvail.map((m) => m.model))
                                          return {
                                            ...r,
                                            models: r.models.filter((m) => validSet.has(m)),
                                          }
                                        })
                                        .filter((r) => r.models.length > 0)
                                      form.setValue('user_model_price_rules', cleaned)
                                      toast.success(t('已一键清理所有规则中不存在的模型'))
                                    }}
                                  >
                                    <Eraser className='size-3' />
                                    <span>{t('一键清理失效模型')}</span>
                                  </Button>
                                )}
                              </div>

                              <div className='space-y-2 max-h-60 overflow-y-auto pr-1'>
                                {configuredRules.map((rule, idx) => {
                                  const isExpanded = !!expandedRuleIndices[idx]
                                  const isEditingThis = editingRuleIndex === idx
                                  const groupAvail = getAvailableModelsForGroup(rule.group)
                                  const validSet = new Set(groupAvail.map((m) => m.model))
                                  const invalidModels = rule.models.filter((m) => !validSet.has(m))
                                  const hasInvalid = invalidModels.length > 0

                                  const visibleModels = isExpanded ? rule.models : rule.models.slice(0, 3)
                                  const hiddenCount = rule.models.length - visibleModels.length
                                  const fam = inferGroupFamily(
                                    rule.group,
                                    groupMeta[rule.group]?.desc,
                                    groupMeta[rule.group]?.models
                                  )

                                  if (isEditingThis) {
                                    return (
                                      <div
                                        key={`${rule.group}-${idx}`}
                                        className='rounded-lg bg-background p-2.5 text-xs border border-primary/50 ring-1 ring-primary/20 shadow-xs space-y-2.5'
                                      >
                                        <div className='flex items-center justify-between gap-2'>
                                          <div className='flex items-center gap-1.5 min-w-0'>
                                            <Badge
                                              variant='outline'
                                              className={cn(
                                                'h-4 px-1 text-[9px] shrink-0 font-normal',
                                                fam.badgeClassName
                                              )}
                                            >
                                              {fam.name.split(' / ')[0]}
                                            </Badge>
                                            <span className='font-medium text-xs text-foreground truncate'>
                                              {rule.group}
                                            </span>
                                            <Badge
                                              variant='outline'
                                              className='h-4 px-1 text-[10px] font-mono font-normal text-primary border-primary/30 bg-primary/5'
                                            >
                                              {t('编辑模型')}
                                            </Badge>
                                          </div>

                                          <div className='flex items-center gap-1 shrink-0'>
                                            <Button
                                              type='button'
                                              variant='ghost'
                                              size='sm'
                                              className='h-6 px-2 text-[11px] text-muted-foreground hover:text-foreground'
                                              onClick={handleCancelEditRuleModels}
                                            >
                                              {t('Cancel')}
                                            </Button>
                                            <Button
                                              type='button'
                                              size='sm'
                                              className='h-6 px-2 text-[11px] gap-1'
                                              disabled={editingRuleModels.length === 0}
                                              onClick={() => handleSaveEditRuleModels(idx)}
                                            >
                                              <Check className='size-3' />
                                              {t('Save')}
                                            </Button>
                                          </div>
                                        </div>

                                        <div className='space-y-1.5'>
                                          <div className='flex items-center justify-between text-[11px] text-muted-foreground'>
                                            <span>
                                              {t('已选')} {editingRuleModels.length} {t('个可用模型')}
                                            </span>
                                            <span className='text-[10px] text-muted-foreground/80'>
                                              {t('已自动剔除不存在的模型')}
                                            </span>
                                          </div>
                                          <MultiSelect
                                            options={groupAvail.map((item) => ({
                                              value: item.model,
                                              label: item.model,
                                              description: (
                                                <Badge
                                                  variant='outline'
                                                  className={cn(
                                                    'h-4 px-1 text-[10px] font-mono border-border/60',
                                                    item.has_global_price
                                                      ? 'text-muted-foreground bg-muted/40'
                                                      : 'text-amber-600 bg-amber-50 dark:text-amber-400 dark:bg-amber-950/40 border-amber-200 dark:border-amber-900/50'
                                                  )}
                                                >
                                                  {item.has_global_price
                                                    ? `$${item.price}/${t('call')}`
                                                    : t('按量转按次')}
                                                </Badge>
                                              ),
                                            }))}
                                            selected={editingRuleModels}
                                            onChange={setEditingRuleModels}
                                            placeholder={
                                              groupAvail.length === 0
                                                ? t('No models available in this group')
                                                : t('Select one or more models')
                                            }
                                            className='min-h-8 text-xs bg-background w-full'
                                            maxVisibleChips={4}
                                            copyChipOnClick={true}
                                          />
                                        </div>
                                      </div>
                                    )
                                  }

                                  return (
                                    <div
                                      key={`${rule.group}-${idx}`}
                                      className={cn(
                                        'rounded-lg bg-background p-2.5 text-xs border shadow-2xs space-y-2 transition-colors',
                                        hasInvalid
                                          ? 'border-amber-400/60 dark:border-amber-700/60 bg-amber-50/20 dark:bg-amber-950/10'
                                          : 'border-border/70 hover:border-border'
                                      )}
                                    >
                                      <div className='flex items-center justify-between gap-2'>
                                        <div className='flex items-center gap-1.5 min-w-0 flex-wrap'>
                                          <Badge
                                            variant='outline'
                                            className={cn(
                                              'h-4 px-1 text-[9px] shrink-0 font-normal',
                                              fam.badgeClassName
                                            )}
                                          >
                                            {fam.name.split(' / ')[0]}
                                          </Badge>
                                          <span className='font-medium text-xs text-foreground truncate'>
                                            {rule.group}
                                          </span>
                                          <Badge
                                            variant='secondary'
                                            className='h-4 px-1 text-[10px] font-mono font-normal'
                                          >
                                            {rule.models.length} {t('个模型')}
                                          </Badge>
                                          {hasInvalid && (
                                            <Badge
                                              variant='outline'
                                              className='h-4 px-1 text-[9px] font-normal text-amber-600 dark:text-amber-400 border-amber-300 dark:border-amber-800 bg-amber-50 dark:bg-amber-950/40 flex items-center gap-0.5'
                                            >
                                              <AlertCircle className='size-2.5 shrink-0' />
                                              <span>{invalidModels.length} {t('个不存在')}</span>
                                            </Badge>
                                          )}
                                        </div>

                                        <div className='flex items-center gap-1.5 shrink-0'>
                                          <div className='relative w-24'>
                                            <span className='absolute left-2 top-1/2 -translate-y-1/2 text-xs text-muted-foreground'>
                                              $
                                            </span>
                                            <Input
                                              type='number'
                                              min='0'
                                              step='0.001'
                                              className='h-7 pl-5 pr-6 text-xs font-mono'
                                              value={rule.price}
                                              onChange={(e) => {
                                                const val = Number(e.target.value)
                                                if (Number.isFinite(val) && val >= 0) {
                                                  handleUpdatePriceRule(idx, val)
                                                }
                                              }}
                                            />
                                            <span className='absolute right-1.5 top-1/2 -translate-y-1/2 text-[10px] text-muted-foreground font-mono'>
                                              /{t('call')}
                                            </span>
                                          </div>
                                          <Button
                                            type='button'
                                            variant='ghost'
                                            size='icon'
                                            className='size-7 text-muted-foreground hover:text-primary'
                                            title={t('编辑模型')}
                                            onClick={() => handleStartEditRuleModels(idx)}
                                          >
                                            <Pencil className='h-3.5 w-3.5' />
                                          </Button>
                                          <Button
                                            type='button'
                                            variant='ghost'
                                            size='icon'
                                            className='size-7 text-muted-foreground hover:text-destructive'
                                            title={t('删除规则')}
                                            onClick={() => handleDeletePriceRule(idx)}
                                          >
                                            <Trash2 className='h-3.5 w-3.5' />
                                          </Button>
                                        </div>
                                      </div>

                                      {/* Models Chips */}
                                      <div className='flex flex-wrap items-center gap-1 pt-0.5'>
                                        {visibleModels.map((m) => {
                                          const isInvalid = !validSet.has(m)
                                          return (
                                            <Badge
                                              key={m}
                                              variant='outline'
                                              className={cn(
                                                'text-[10px] font-mono font-normal flex items-center gap-1 group/chip py-0.5',
                                                isInvalid
                                                  ? 'bg-amber-100/50 dark:bg-amber-950/60 text-amber-700 dark:text-amber-400 border-amber-300 dark:border-amber-800'
                                                  : 'bg-muted/30 text-foreground/80'
                                              )}
                                            >
                                              <span className={cn(isInvalid && 'line-through opacity-80')}>
                                                {m}
                                              </span>
                                              {isInvalid && (
                                                <span className='text-[8px] font-sans px-0.5 rounded bg-amber-200/60 dark:bg-amber-900/60 text-amber-800 dark:text-amber-300'>
                                                  {t('不存在')}
                                                </span>
                                              )}
                                              <button
                                                type='button'
                                                onClick={(e) => {
                                                  e.stopPropagation()
                                                  handleRemoveModelFromRule(idx, m)
                                                }}
                                                className='hover:bg-muted-foreground/20 rounded p-0.5 transition-colors cursor-pointer text-muted-foreground hover:text-foreground'
                                                title={t('移除此模型')}
                                              >
                                                <X className='size-2.5' />
                                              </button>
                                            </Badge>
                                          )
                                        })}
                                        {hiddenCount > 0 && (
                                          <button
                                            type='button'
                                            onClick={() => toggleRuleExpand(idx)}
                                            className='text-[10px] font-mono text-muted-foreground hover:text-foreground px-1.5 py-0.5 rounded bg-muted hover:bg-muted/80 transition-colors'
                                          >
                                            +{hiddenCount}
                                          </button>
                                        )}
                                        {isExpanded && rule.models.length > 3 && (
                                          <button
                                            type='button'
                                            onClick={() => toggleRuleExpand(idx)}
                                            className='text-[10px] font-mono text-muted-foreground hover:text-foreground px-1.5 py-0.5 rounded bg-muted hover:bg-muted/80 transition-colors'
                                          >
                                            {t('Collapse')}
                                          </button>
                                        )}
                                        {hasInvalid && (
                                          <Button
                                            type='button'
                                            variant='ghost'
                                            size='sm'
                                            onClick={() => handleAutoCleanInvalidModels(idx)}
                                            className='h-5 px-1.5 text-[10px] text-amber-600 dark:text-amber-400 hover:text-amber-700 dark:hover:text-amber-300 hover:bg-amber-100/50 dark:hover:bg-amber-900/40 gap-1'
                                          >
                                            <Eraser className='size-2.5' />
                                            <span>{t('清除不存在模型')}</span>
                                          </Button>
                                        )}
                                      </div>
                                    </div>
                                  )
                                })}
                              </div>
                            </div>
                          )
                        })()}
                      </div>
                    </div>
                  )}

                  <FormField
                    control={form.control}
                    name='quota_dollars'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t('Remaining Quota ({{currency}})', {
                            currency: currencyLabel,
                          })}
                        </FormLabel>
                        <div className='flex gap-2'>
                          <FormControl>
                            <Input
                              value={
                                tokensOnly
                                  ? String(field.value || 0)
                                  : (field.value || 0).toFixed(6)
                              }
                              readOnly
                              className='flex-1'
                            />
                          </FormControl>
                          <Button
                            type='button'
                            variant='outline'
                            onClick={() => setQuotaDialogOpen(true)}
                          >
                            <Pencil className='mr-1 h-4 w-4' />
                            {t('Adjust Quota')}
                          </Button>
                        </div>
                        <FormDescription>
                          {formatQuota(parseQuotaFromDollars(field.value || 0))}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='remark'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Remark')}</FormLabel>
                        <FormControl>
                          <Textarea
                            {...field}
                            placeholder={t(
                              'Admin notes (only visible to admins)'
                            )}
                            rows={3}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </SideDrawerSection>
              )}

              {canEditAdminPermissions &&
                targetIsAdmin &&
                permissionCatalog.resources.length > 0 && (
                  <SideDrawerSection>
                    <h3 className='text-sm font-medium'>
                      {t('Admin Permissions')}
                    </h3>
                    <p className='text-muted-foreground text-xs'>
                      {t(
                        'Default administrator permissions can be overridden for this user.'
                      )}
                    </p>
                    <FormField
                      control={form.control}
                      name='admin_permissions'
                      render={({ field }) => {
                        const selected = normalizeAdminPermissions(
                          field.value,
                          permissionCatalog
                        )
                        return (
                          <FormItem>
                            <div className='space-y-3'>
                              {permissionCatalog.resources.map((resource) => (
                                <div
                                  key={resource.resource}
                                  className='space-y-2 rounded-md border p-3'
                                >
                                  <div className='text-sm font-medium'>
                                    {t(resource.label_key)}
                                  </div>
                                  <div className='space-y-2'>
                                    {resource.actions.map((option) => (
                                      <label
                                        key={option.action}
                                        className='flex items-start gap-3'
                                      >
                                        <Checkbox
                                          checked={
                                            selected[resource.resource]?.[
                                              option.action
                                            ] === true
                                          }
                                          onCheckedChange={(checked) => {
                                            field.onChange({
                                              ...selected,
                                              [resource.resource]: {
                                                ...selected[resource.resource],
                                                [option.action]:
                                                  checked === true,
                                              },
                                            })
                                          }}
                                        />
                                        <span className='flex flex-col gap-1'>
                                          <span className='text-sm font-medium'>
                                            {t(option.label_key)}
                                          </span>
                                          <span className='text-muted-foreground text-xs'>
                                            {t(option.description_key)}
                                          </span>
                                        </span>
                                      </label>
                                    ))}
                                  </div>
                                </div>
                              ))}
                            </div>
                            <FormMessage />
                          </FormItem>
                        )
                      }}
                    />
                    {currentUser && (
                      <p className='text-muted-foreground text-xs'>
                        {hasPermission(
                          currentUser,
                          ADMIN_PERMISSION_RESOURCES.CHANNEL,
                          ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
                        )
                          ? t(
                              'Your account can edit sensitive channel settings.'
                            )
                          : t(
                              'Your account cannot edit sensitive channel settings.'
                            )}
                      </p>
                    )}
                  </SideDrawerSection>
                )}

              {/* Binding Information (Read-only) */}
              {isUpdate && (
                <SideDrawerSection>
                  <h3 className='text-sm font-medium'>
                    {t('Binding Information')}
                  </h3>
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'Third-party account bindings (read-only, managed by user in Security & Access)'
                    )}
                  </p>

                  <div className='flex flex-col gap-3'>
                    {BINDING_FIELDS.map(({ key, label }) => (
                      <div key={key}>
                        <Label className='text-muted-foreground text-xs'>
                          {t(label)}
                        </Label>
                        <Input
                          value={
                            (currentRow?.[key as keyof User] as string) || '-'
                          }
                          disabled
                          className='mt-1'
                        />
                      </div>
                    ))}
                  </div>
                </SideDrawerSection>
              )}
            </form>
          </Form>
          <SheetFooter className={sideDrawerFooterClassName()}>
            <SheetClose render={<Button variant='outline' />}>
              {t('Close')}
            </SheetClose>
            <Button form='user-form' type='submit' disabled={isSubmitting}>
              {isSubmitting ? t('Saving...') : t('Save changes')}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      {/* Adjust Quota Dialog */}
      {currentRow && (
        <UserQuotaDialog
          open={quotaDialogOpen}
          onOpenChange={setQuotaDialogOpen}
          userId={currentRow.id}
          currentQuota={parseQuotaFromDollars(currentQuotaRaw || 0)}
          onSuccess={refreshUserData}
        />
      )}

      {/* Group Channel Whitelist Dialog */}
      <Dialog
        open={Boolean(channelModalGroup)}
        onOpenChange={(isOpen) => {
          if (!isOpen) {
            setChannelModalGroup(null)
            setChannelSearchQuery('')
          }
        }}
      >
        <DialogContent className='sm:max-w-md max-h-[85vh] flex flex-col p-4'>
          <DialogHeader className='space-y-1.5'>
            <DialogTitle className='flex items-center gap-2 text-base'>
              <Layers className='size-4 text-primary' />
              <span>
                {t('限定渠道设置')}:{' '}
                <span className='font-mono font-semibold text-primary'>
                  {channelModalGroup}
                </span>
              </span>
            </DialogTitle>
            <DialogDescription className='text-xs text-muted-foreground leading-relaxed'>
              {t(
                '留空表示默认允许走该分组下的所有渠道；若勾选，则用户请求仅能在所选渠道中分流。支持选择启用和已禁用渠道。'
              )}
            </DialogDescription>
          </DialogHeader>

          {channelModalGroup && (() => {
            const groupChannels: Array<{
              id: number
              name: string
              status: number
              type: number
              priority: number
              weight: number
              response_time: number
            }> = groupMeta[channelModalGroup]?.channels || []

            const selectedChannels =
              (form.watch('user_group_channels') || {})[channelModalGroup] || []

            const filteredChannels = groupChannels.filter((c) => {
              if (!channelSearchQuery.trim()) return true
              const q = channelSearchQuery.toLowerCase()
              return (
                String(c.id).includes(q) ||
                c.name.toLowerCase().includes(q)
              )
            })

            const isAllSelected =
              groupChannels.length > 0 &&
              groupChannels.every((c) => selectedChannels.includes(c.id))

            const handleToggleChannel = (channelId: number) => {
              const current = [...selectedChannels]
              const idx = current.indexOf(channelId)
              if (idx >= 0) {
                current.splice(idx, 1)
              } else {
                current.push(channelId)
              }
              const allChannelsMap = { ...(form.getValues('user_group_channels') || {}) }
              if (current.length === 0) {
                delete allChannelsMap[channelModalGroup]
              } else {
                allChannelsMap[channelModalGroup] = current.sort((a, b) => a - b)
              }
              form.setValue('user_group_channels', allChannelsMap)
            }

            const handleSelectAll = () => {
              const allChannelsMap = { ...(form.getValues('user_group_channels') || {}) }
              allChannelsMap[channelModalGroup] = groupChannels.map((c) => c.id).sort((a, b) => a - b)
              form.setValue('user_group_channels', allChannelsMap)
            }

            const handleClearAll = () => {
              const allChannelsMap = { ...(form.getValues('user_group_channels') || {}) }
              delete allChannelsMap[channelModalGroup]
              form.setValue('user_group_channels', allChannelsMap)
            }

            return (
              <div className='flex flex-col gap-3 flex-1 min-h-0 pt-1'>
                <div className='flex items-center gap-2'>
                  <div className='relative flex-1'>
                    <Search className='absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground' />
                    <Input
                      placeholder={t('搜索渠道 ID 或名称...')}
                      value={channelSearchQuery}
                      onChange={(e) => setChannelSearchQuery(e.target.value)}
                      className='h-8 pl-8 text-xs'
                    />
                  </div>
                  <div className='flex items-center gap-1.5 shrink-0'>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      className='h-8 px-2 text-xs'
                      onClick={handleSelectAll}
                      disabled={groupChannels.length === 0 || isAllSelected}
                    >
                      {t('全选')}
                    </Button>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      className='h-8 px-2 text-xs text-muted-foreground hover:text-foreground'
                      onClick={handleClearAll}
                      disabled={selectedChannels.length === 0}
                    >
                      {t('清空')}
                    </Button>
                  </div>
                </div>

                <div className='flex items-center justify-between text-xs text-muted-foreground px-1'>
                  <span>
                    {t('共 {{total}} 个渠道', { total: groupChannels.length })}
                    {selectedChannels.length > 0 && (
                      <span className='ml-2 font-medium text-primary'>
                        (已勾选 {selectedChannels.length} 个)
                      </span>
                    )}
                  </span>
                  {selectedChannels.length === 0 && (
                    <span className='text-[11px] text-emerald-600 dark:text-emerald-400 bg-emerald-50 dark:bg-emerald-950/40 px-1.5 py-0.5 rounded'>
                      {t('默认允许全部')}
                    </span>
                  )}
                </div>

                <div className='border rounded-lg divide-y overflow-y-auto max-h-[300px] bg-card/40'>
                  {groupChannels.length === 0 ? (
                    <div className='p-6 text-center text-xs text-muted-foreground'>
                      {t('此分组暂无关联渠道')}
                    </div>
                  ) : filteredChannels.length === 0 ? (
                    <div className='p-6 text-center text-xs text-muted-foreground'>
                      {t('未找到匹配的渠道')}
                    </div>
                  ) : (
                    filteredChannels.map((channel) => {
                      const isChecked = selectedChannels.includes(channel.id)
                      const isEnabled = channel.status === 1
                      return (
                        <div
                          key={channel.id}
                          onClick={() => handleToggleChannel(channel.id)}
                          className={cn(
                            'flex items-center gap-2.5 px-3 py-2 text-xs cursor-pointer select-none transition-colors hover:bg-muted/30',
                            isChecked && 'bg-primary/5 font-medium'
                          )}
                        >
                          <Checkbox
                            checked={isChecked}
                            onCheckedChange={() => handleToggleChannel(channel.id)}
                            className='shrink-0'
                          />
                          <div className='flex items-center gap-2 min-w-0 flex-1'>
                            <span className='font-mono font-semibold text-muted-foreground shrink-0'>
                              #{channel.id}
                            </span>
                            <span className='truncate text-foreground'>
                              {channel.name}
                            </span>
                          </div>
                          <div className='flex items-center gap-1.5 shrink-0'>
                            {isEnabled ? (
                              <Badge
                                variant='outline'
                                className='h-4 px-1 text-[10px] border-emerald-300 text-emerald-600 bg-emerald-50 dark:border-emerald-900/50 dark:text-emerald-400 dark:bg-emerald-950/40'
                              >
                                {t('已启用')}
                              </Badge>
                            ) : (
                              <Badge
                                variant='outline'
                                className='h-4 px-1 text-[10px] border-zinc-400 text-zinc-500 bg-zinc-100 dark:border-zinc-700 dark:text-zinc-400 dark:bg-zinc-900/50'
                              >
                                {t('已禁用')}
                              </Badge>
                            )}
                            {channel.response_time > 0 && (
                              <span className='text-[10px] text-muted-foreground font-mono'>
                                {channel.response_time}ms
                              </span>
                            )}
                          </div>
                        </div>
                      )
                    })
                  )}
                </div>

                <div className='flex justify-end pt-2'>
                  <Button
                    type='button'
                    size='sm'
                    className='h-8 px-4 text-xs'
                    onClick={() => {
                      setChannelModalGroup(null)
                      setChannelSearchQuery('')
                    }}
                  >
                    {t('完成')}
                  </Button>
                </div>
              </div>
            )
          })()}
        </DialogContent>
      </Dialog>
    </>
  )
}
