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
  Check,
  CheckCheck,
  ChevronDown,
  ChevronUp,
  ChevronsUpDown,
  Coins,
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
import { Fragment, useEffect, useMemo, useState } from 'react'
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
    staleTime: 5 * 60 * 1000,
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

  const [draftPriceGroup, setDraftPriceGroup] = useState('')
  const [draftPriceModels, setDraftPriceModels] = useState<string[]>([])
  const [draftModelPrice, setDraftModelPrice] = useState<string>('')
  const [expandedRuleIndices, setExpandedRuleIndices] = useState<Record<number, boolean>>({})

  const availableModelsForGroup = useMemo(() => {
    if (!draftPriceGroup) return []
    return perCallModels.filter(
      (item) =>
        item.groups?.includes(draftPriceGroup) ||
        item.groups?.includes('all')
    )
  }, [perCallModels, draftPriceGroup])

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
  }

  const handleUpdatePriceRule = (index: number, newPrice: number) => {
    const currentRules = form.getValues('user_model_price_rules') || []
    form.setValue(
      'user_model_price_rules',
      currentRules.map((item, i) => (i === index ? { ...item, price: newPrice } : item))
    )
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
      finalData.user_model_price_rules = finalPriceRules

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

                      const filteredGroups = groupOptions.filter((group) => {
                        if (groupSearchQuery.trim()) {
                          if (!group.toLowerCase().includes(groupSearchQuery.toLowerCase())) {
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
                                              {hasPerCall && (
                                                <Badge
                                                  variant='outline'
                                                  className='h-4 px-1 text-[10px] font-normal border-amber-300 text-amber-700 bg-amber-50 dark:border-amber-900/50 dark:text-amber-400 dark:bg-amber-950/40 shrink-0 flex items-center gap-0.5'
                                                >
                                                  <Sparkles className='size-2.5' />
                                                  {t('{{count}} per-call models', {
                                                    count: perCallCount,
                                                  })}
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

                                    return (
                                      <div
                                        key={group}
                                        className='flex items-center justify-between gap-3 px-3 py-2 hover:bg-muted/20 transition-colors'
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
                                          <span className='text-xs font-medium text-foreground truncate'>
                                            {group}
                                          </span>
                                          {meta?.ratio !== undefined && (
                                            <Badge
                                              variant='outline'
                                              className='h-4 px-1 text-[10px] font-mono border-blue-200 text-blue-600 bg-blue-50 dark:border-blue-900/50 dark:text-blue-400 dark:bg-blue-950/40'
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
                                          {hasPerCall && (
                                            <Badge
                                              variant='outline'
                                              className='h-4 px-1 text-[10px] border-amber-300 text-amber-700 bg-amber-50 dark:border-amber-900/50 dark:text-amber-400 dark:bg-amber-950/40 flex items-center gap-0.5'
                                            >
                                              <Sparkles className='size-2.5' />
                                              {t('{{count}} per-call models', {
                                                count: perCallCount,
                                              })}
                                            </Badge>
                                          )}
                                        </div>

                                        <div className='flex items-center gap-2 shrink-0'>
                                          <Input
                                            type='number'
                                            min='0'
                                            step='0.001'
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
                                {availableModelsForGroup.length} {t('个按次模型')}
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
                                {sortGroupsByFamilyAndRatio(Array.from(perCallGroups), {
                                  groupMeta,
                                }).map((group) => {
                                  const count = perCallModelCountByGroup[group] || 0
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
                                          {count} {t('个按次模型')}
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
                                  className='h-4 px-1 text-[10px] font-mono text-muted-foreground bg-muted/40 border-border/60'
                                >
                                  {item.has_global_price
                                    ? `$${item.price}/${t('call')}`
                                    : t('Default/call')}
                                </Badge>
                              ),
                            }))}
                            selected={draftPriceModels}
                            onChange={setDraftPriceModels}
                            placeholder={
                              !draftPriceGroup
                                ? t('Select group first')
                                : availableModelsForGroup.length === 0
                                  ? t('No per-call models available in this group')
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
                        {(form.watch('user_model_price_rules') || []).length > 0 && (
                          <div className='space-y-2 border-t border-border/60 pt-3'>
                            <div className='flex items-center justify-between text-[11px] font-medium text-muted-foreground'>
                              <span>
                                {t('Configured Rules')} ({(form.watch('user_model_price_rules') || []).length})
                              </span>
                            </div>
                            <div className='space-y-2 max-h-60 overflow-y-auto pr-1'>
                              {(form.watch('user_model_price_rules') || []).map((rule, idx) => {
                                const isExpanded = !!expandedRuleIndices[idx]
                                const visibleModels = isExpanded ? rule.models : rule.models.slice(0, 3)
                                const hiddenCount = rule.models.length - visibleModels.length
                                const fam = inferGroupFamily(
                                  rule.group,
                                  groupMeta[rule.group]?.desc,
                                  groupMeta[rule.group]?.models
                                )

                                return (
                                  <div
                                    key={`${rule.group}-${idx}`}
                                    className='rounded-lg bg-background p-2.5 text-xs border border-border/70 shadow-2xs space-y-2 transition-colors hover:border-border'
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
                                        <Badge variant='secondary' className='h-4 px-1 text-[10px] font-mono font-normal'>
                                          {rule.models.length} {t('个模型')}
                                        </Badge>
                                      </div>

                                      <div className='flex items-center gap-2 shrink-0'>
                                        <div className='relative w-28'>
                                          <span className='absolute left-2 top-1/2 -translate-y-1/2 text-xs text-muted-foreground'>
                                            $
                                          </span>
                                          <Input
                                            type='number'
                                            min='0'
                                            step='0.001'
                                            className='h-7 pl-5 pr-7 text-xs font-mono'
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
                                          className='size-7 text-muted-foreground hover:text-destructive'
                                          onClick={() => handleDeletePriceRule(idx)}
                                        >
                                          <Trash2 className='h-3.5 w-3.5' />
                                        </Button>
                                      </div>
                                    </div>

                                    {/* Models Chips */}
                                    <div className='flex flex-wrap items-center gap-1 pt-0.5'>
                                      {visibleModels.map((m) => (
                                        <Badge
                                          key={m}
                                          variant='outline'
                                          className='text-[10px] font-mono font-normal bg-muted/30 text-foreground/80'
                                        >
                                          {m}
                                        </Badge>
                                      ))}
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
                                    </div>
                                  </div>
                                )
                              })}
                            </div>
                          </div>
                        )}
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
    </>
  )
}
