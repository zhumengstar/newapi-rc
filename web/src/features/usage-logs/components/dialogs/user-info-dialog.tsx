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
import { useQuery } from '@tanstack/react-query'
import {
  Coins,
  Gift,
  Layers,
  Loader2,
  MessageSquareText,
  Sparkles,
  User as UserIcon,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { GroupBadge } from '@/components/group-badge'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import { getGroupDetails } from '@/features/users/api'
import {
  inferGroupFamily,
  sortGroupsByFamilyAndRatio,
} from '@/features/users/lib'
import {
  parseUserDisabledGroups,
  parseUserGroupChannels,
  parseUserGroupRatios,
  parseUserModelPriceRules,
} from '@/features/users/lib/user-form'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatQuota, formatCompactNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import { getUserInfo } from '../../api'
import type { UserInfo } from '../../types'

interface UserInfoDialogProps {
  userId: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

function StatCard({
  label,
  value,
  subValue,
  highlight = false,
}: {
  label: string
  value: string | number
  subValue?: string
  highlight?: boolean
}) {
  return (
    <div className='rounded-xl border border-border/60 bg-muted/25 px-3.5 py-2.5 space-y-1'>
      <div className='text-muted-foreground text-[11px] font-medium'>
        {label}
      </div>
      <div
        className={cn(
          'font-mono text-base font-semibold tabular-nums tracking-tight',
          highlight ? 'text-emerald-600 dark:text-emerald-400' : 'text-foreground'
        )}
      >
        {value}
      </div>
      {subValue && (
        <div className='text-muted-foreground text-[10px] font-mono tabular-nums'>
          {subValue}
        </div>
      )}
    </div>
  )
}

export function UserInfoDialog({
  userId,
  open,
  onOpenChange,
}: UserInfoDialogProps) {
  const { t } = useTranslation()
  const [userInfo, setUserInfo] = useState<UserInfo | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [expandedRuleKeys, setExpandedRuleKeys] = useState<
    Record<string, boolean>
  >({})

  const { data: groupDetailsData } = useQuery({
    queryKey: ['group-details'],
    queryFn: async () => requireServerSuccess(await getGroupDetails()),
    enabled: open,
    staleTime: 5 * 60 * 1000,
  })

  const groupMeta = useMemo(
    () => groupDetailsData?.data?.meta || {},
    [groupDetailsData]
  )

  const fetchUserInfo = useCallback(
    async (id: number) => {
      setIsLoading(true)
      try {
        const result = await getUserInfo(id)
        if (result.success) {
          setUserInfo(result.data || null)
        } else {
          handleServerError(result, t('Failed to fetch user information'))
        }
      } catch (error) {
        handleServerError(error, t('Failed to fetch user information'))
      } finally {
        setIsLoading(false)
      }
    },
    [t]
  )

  useEffect(() => {
    if (open && userId) {
      setExpandedRuleKeys({})
      fetchUserInfo(userId)
    }
  }, [open, userId, fetchUserInfo])

  const parsedGroupData = useMemo(() => {
    if (!userInfo) {
      return {
        groups: [] as string[],
        customRatios: {} as Record<string, number>,
        effectiveRatios: {} as Record<string, number>,
        priceRules: [],
        rulesByGroup: {} as Record<
          string,
          Array<{ group: string; models: string[]; price: number }>
        >,
        disabledSet: new Set<string>(),
        groupChannels: {} as Record<string, number[]>,
        customRatioCount: 0,
      }
    }

    const rawGroups = (userInfo.group || 'default')
      .split(',')
      .map((g) => g.trim())
      .filter(Boolean)
    const customRatios = parseUserGroupRatios(userInfo.setting)
    const effectiveRatios = userInfo.effective_group_ratios || {}
    const priceRules = parseUserModelPriceRules(
      userInfo.setting,
      rawGroups[0] || 'default'
    )
    const disabledSet = new Set(parseUserDisabledGroups(userInfo.setting))
    const groupChannels = parseUserGroupChannels(userInfo.setting)

    const groupSet = new Set<string>([
      ...rawGroups,
      ...Object.keys(effectiveRatios),
      ...Object.keys(customRatios),
      ...priceRules.map((r) => r.group),
    ])
    const allGroups = Array.from(groupSet).filter(Boolean)
    const sortedGroups = sortGroupsByFamilyAndRatio(allGroups, {
      userGroupRatios: customRatios,
      groupMeta,
    })

    const rulesByGroup: Record<
      string,
      Array<{ group: string; models: string[]; price: number }>
    > = {}
    for (const rule of priceRules) {
      if (!rulesByGroup[rule.group]) {
        rulesByGroup[rule.group] = []
      }
      rulesByGroup[rule.group].push(rule)
    }

    const customRatioCount = sortedGroups.filter(
      (g) => customRatios[g] !== undefined
    ).length

    return {
      groups: sortedGroups,
      customRatios,
      effectiveRatios,
      priceRules,
      rulesByGroup,
      disabledSet,
      groupChannels,
      customRatioCount,
    }
  }, [userInfo, groupMeta])

  const toggleRuleExpand = (key: string) => {
    setExpandedRuleKeys((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('User Information')}
      description={t(
        'View detailed information about this user including balance, usage statistics, and invitation details.'
      )}
      contentClassName='sm:max-w-xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
    >
      {isLoading && (
        <div className='flex items-center justify-center py-10'>
          <Loader2 className='text-muted-foreground size-6 animate-spin' />
        </div>
      )}
      {!isLoading && userInfo && (
        <div className='space-y-4 py-2'>
          {/* 1. User Header Card */}
          <div className='flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border/60 bg-card/60 px-3.5 py-2.5'>
            <div className='flex items-center gap-2.5 min-w-0'>
              <div className='flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary border border-primary/20'>
                <UserIcon className='size-4' />
              </div>
              <div className='min-w-0'>
                <div className='flex items-center gap-2 flex-wrap'>
                  <span className='text-sm font-semibold text-foreground truncate'>
                    {userInfo.username}
                  </span>
                  <span className='text-muted-foreground text-xs font-mono'>
                    #{userInfo.id}
                  </span>
                </div>
                {userInfo.display_name &&
                  userInfo.display_name !== userInfo.username && (
                    <div className='text-muted-foreground text-xs truncate'>
                      {t('Display Name')}: {userInfo.display_name}
                    </div>
                  )}
              </div>
            </div>

            {/* Real-time RPM / MPM */}
            {((typeof userInfo.rpm === 'number' && userInfo.rpm > 0) ||
              (typeof userInfo.mpm === 'number' && userInfo.mpm > 0)) && (
              <div className='flex items-center gap-1.5 shrink-0'>
                {typeof userInfo.rpm === 'number' && userInfo.rpm > 0 && (
                  <Badge
                    variant='outline'
                    className='h-5 px-2 text-[11px] font-mono border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                  >
                    RPM {userInfo.rpm.toLocaleString()}
                  </Badge>
                )}
                {typeof userInfo.mpm === 'number' && userInfo.mpm > 0 && (
                  <Badge
                    variant='outline'
                    className='h-5 px-2 text-[11px] font-mono border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'
                  >
                    MPM{' '}
                    {formatQuotaWithCurrency(userInfo.mpm, {
                      digitsLarge: 2,
                      digitsSmall: 4,
                      abbreviate: false,
                    })}
                  </Badge>
                )}
              </div>
            )}
          </div>

          {/* 2. Core Stats Grid (Balance, Used Quota, Request Count) */}
          <div className='grid grid-cols-3 gap-2.5'>
            <StatCard
              label={t('Balance')}
              value={formatQuota(userInfo.quota)}
              highlight
            />
            <StatCard
              label={t('Used Quota')}
              value={formatQuota(userInfo.used_quota)}
            />
            <StatCard
              label={t('Request Count')}
              value={formatCompactNumber(userInfo.request_count)}
              subValue={
                userInfo.request_count >= 1000
                  ? `${userInfo.request_count.toLocaleString()} 次`
                  : undefined
              }
            />
          </div>

          {/* 3. Model Groups, Ratios & Per-Call Pricing Section */}
          {parsedGroupData.groups.length > 0 && (
            <div className='space-y-2'>
              <div className='flex items-center justify-between gap-2 px-0.5'>
                <div className='flex items-center gap-1.5'>
                  <Layers className='size-3.5 text-primary' />
                  <Label className='text-xs font-semibold text-foreground'>
                    {t('User Group')} & {t('Ratio', '计费倍率')}
                  </Label>
                  <Badge
                    variant='outline'
                    className='h-4 px-1.5 text-[10px] font-mono text-muted-foreground bg-muted/40'
                  >
                    {parsedGroupData.groups.length}
                  </Badge>
                </div>

                <div className='flex items-center gap-1.5 flex-wrap'>
                  {parsedGroupData.customRatioCount > 0 && (
                    <Badge
                      variant='outline'
                      className='h-4 px-1.5 text-[10px] font-normal border-indigo-200 text-indigo-600 bg-indigo-50/80 dark:border-indigo-900/60 dark:text-indigo-300 dark:bg-indigo-950/40'
                    >
                      专属倍率 {parsedGroupData.customRatioCount} 项
                    </Badge>
                  )}
                  {parsedGroupData.priceRules.length > 0 && (
                    <Badge
                      variant='outline'
                      className='h-4 px-1.5 text-[10px] font-normal border-amber-300 text-amber-700 bg-amber-50 dark:border-amber-800/70 dark:text-amber-300 dark:bg-amber-950/40 flex items-center gap-0.5'
                    >
                      <Sparkles className='size-2.5' />
                      按次配置 {parsedGroupData.priceRules.length} 项
                    </Badge>
                  )}
                </div>
              </div>

              <div className='rounded-xl border border-border/70 bg-card/40 divide-y divide-border/40 overflow-hidden max-h-[300px] overflow-y-auto'>
                {parsedGroupData.groups.map((group) => {
                  const meta = groupMeta[group]
                  const family = inferGroupFamily(
                    group,
                    meta?.desc,
                    meta?.models
                  )
                  const isDisabled = parsedGroupData.disabledSet.has(group)
                  const whitelistedCount =
                    parsedGroupData.groupChannels[group]?.length || 0
                  const customRatio = parsedGroupData.customRatios[group]
                  const defaultRatio = meta?.ratio
                  const effectiveRatio =
                    parsedGroupData.effectiveRatios[group] ??
                    customRatio ??
                    defaultRatio
                  const isCustomRatio = customRatio !== undefined
                  const groupRules = parsedGroupData.rulesByGroup[group] || []
                  const isPerCallGroupByName = group.includes('按次')

                  return (
                    <div
                      key={group}
                      className={cn(
                        'transition-colors',
                        isDisabled
                          ? 'bg-muted/30 opacity-60'
                          : 'hover:bg-muted/20'
                      )}
                    >
                      {/* Group Row Header */}
                      <div className='flex items-center justify-between gap-2.5 px-3 py-2'>
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
                            group={group}
                            copyable={false}
                            className={cn(
                              'text-xs font-medium',
                              isDisabled && 'line-through grayscale'
                            )}
                          />

                          {isDisabled && (
                            <Badge
                              variant='outline'
                              className='h-4 px-1 text-[10px] border-zinc-400/60 text-zinc-500 bg-zinc-100/80 dark:border-zinc-700 dark:text-zinc-400 dark:bg-zinc-900/60'
                            >
                              已置灰
                            </Badge>
                          )}

                          {whitelistedCount > 0 && !isDisabled && (
                            <Badge
                              variant='outline'
                              className='h-4 px-1.5 text-[10px] border-primary/35 text-primary bg-primary/10 font-mono'
                            >
                              限定 {whitelistedCount} 渠道
                            </Badge>
                          )}
                        </div>

                        {/* Right: Per-Call Tag & Effective Ratio Pill */}
                        <div className='flex items-center gap-1.5 shrink-0'>
                          {groupRules.length === 1 ? (
                            <Badge
                              variant='outline'
                              className='h-5 px-2 text-[11px] font-mono font-semibold border-amber-300/90 text-amber-700 bg-amber-50 dark:border-amber-800/80 dark:text-amber-300 dark:bg-amber-950/50 flex items-center gap-1'
                            >
                              <Sparkles className='size-2.5' />
                              按次 ${groupRules[0].price}/次
                            </Badge>
                          ) : groupRules.length > 1 ? (
                            <Badge
                              variant='outline'
                              className='h-5 px-2 text-[11px] font-mono font-semibold border-amber-300/90 text-amber-700 bg-amber-50 dark:border-amber-800/80 dark:text-amber-300 dark:bg-amber-950/50 flex items-center gap-1'
                            >
                              <Sparkles className='size-2.5' />
                              按次 {groupRules.length} 档
                            </Badge>
                          ) : isPerCallGroupByName ? (
                            <Badge
                              variant='outline'
                              className='h-5 px-1.5 text-[10px] font-normal border-amber-200/80 text-amber-600 bg-amber-50/60 dark:border-amber-900/60 dark:text-amber-400 dark:bg-amber-950/30'
                            >
                              按次分组
                            </Badge>
                          ) : null}

                          {effectiveRatio !== undefined && (
                            <div className='inline-flex items-center gap-1.5'>
                              {isCustomRatio &&
                                defaultRatio !== undefined &&
                                defaultRatio !== effectiveRatio && (
                                  <span
                                    className='text-[10px] text-muted-foreground line-through font-mono tabular-nums'
                                    title={`分组默认倍率: ${defaultRatio}x`}
                                  >
                                    {defaultRatio}x
                                  </span>
                                )}
                              <Badge
                                variant='outline'
                                className={cn(
                                  'h-5 px-2 text-[11px] font-mono tabular-nums',
                                  isCustomRatio
                                    ? 'font-semibold border-indigo-300/90 text-indigo-700 bg-indigo-50 dark:border-indigo-800/80 dark:text-indigo-300 dark:bg-indigo-950/50'
                                    : effectiveRatio < 1
                                      ? 'font-medium border-sky-300/70 text-sky-700 bg-sky-50/70 dark:border-sky-800/60 dark:text-sky-300 dark:bg-sky-950/40'
                                      : effectiveRatio > 1
                                        ? 'font-medium border-orange-300/70 text-orange-700 bg-orange-50/70 dark:border-orange-800/60 dark:text-orange-300 dark:bg-orange-950/40'
                                        : 'font-medium border-border text-muted-foreground bg-muted/40'
                                )}
                              >
                                {isCustomRatio ? '专属 ' : ''}
                                {effectiveRatio}x
                              </Badge>
                            </div>
                          )}
                        </div>
                      </div>

                      {/* Nested Per-Call Pricing Rules for this Group */}
                      {groupRules.length > 0 && (
                        <div className='border-t border-amber-200/40 dark:border-amber-900/30 bg-amber-50/30 dark:bg-amber-950/15 px-3 py-2 space-y-1.5'>
                          {groupRules.map((rule, rIdx) => {
                            const ruleKey = `${group}-${rIdx}`
                            const isExpanded = !!expandedRuleKeys[ruleKey]
                            const maxVisible = 4
                            const visibleModels = isExpanded
                              ? rule.models
                              : rule.models.slice(0, maxVisible)
                            const hiddenCount =
                              rule.models.length - visibleModels.length

                            return (
                              <div
                                key={ruleKey}
                                className='flex items-start gap-2 text-xs'
                              >
                                <span className='inline-flex items-center gap-1 text-[11px] font-mono font-semibold text-amber-700 dark:text-amber-300 bg-amber-100/80 dark:bg-amber-900/40 border border-amber-300/70 dark:border-amber-800/60 rounded-md px-1.5 py-0.5 shrink-0 mt-0.5'>
                                  <Coins className='size-3' />${rule.price}/次
                                </span>

                                <div className='flex flex-wrap items-center gap-1 flex-1 min-w-0'>
                                  {visibleModels.map((modelName) => (
                                    <span
                                      key={modelName}
                                      className='font-mono text-[11px] px-1.5 py-0.5 rounded-md bg-background/90 border border-border/70 text-foreground/90 truncate max-w-[230px]'
                                      title={modelName}
                                    >
                                      {modelName}
                                    </span>
                                  ))}
                                  {hiddenCount > 0 && (
                                    <button
                                      type='button'
                                      onClick={() => toggleRuleExpand(ruleKey)}
                                      className='text-[11px] font-medium px-1.5 py-0.5 rounded-md bg-primary/10 text-primary hover:bg-primary/20 transition-colors cursor-pointer'
                                    >
                                      +{hiddenCount} 个模型
                                    </button>
                                  )}
                                  {isExpanded &&
                                    rule.models.length > maxVisible && (
                                      <button
                                        type='button'
                                        onClick={() =>
                                          toggleRuleExpand(ruleKey)
                                        }
                                        className='text-[11px] font-medium px-1.5 py-0.5 rounded-md bg-muted text-muted-foreground hover:text-foreground transition-colors cursor-pointer'
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
                  )
                })}
              </div>
            </div>
          )}

          {/* 4. Invitation Info */}
          {(userInfo.aff_code ||
            (userInfo.aff_count !== undefined && userInfo.aff_count > 0) ||
            (userInfo.aff_quota !== undefined && userInfo.aff_quota > 0)) && (
            <div className='rounded-xl border border-border/60 bg-muted/15 px-3.5 py-2.5 space-y-2'>
              <div className='flex items-center gap-1.5 text-muted-foreground text-xs font-medium'>
                <Gift className='size-3.5 text-primary' />
                <span>邀请信息</span>
              </div>
              <div className='grid grid-cols-3 gap-3 text-xs'>
                {userInfo.aff_code && (
                  <div>
                    <div className='text-muted-foreground text-[11px]'>
                      {t('Invitation Code')}
                    </div>
                    <div className='font-mono font-semibold mt-0.5'>
                      {userInfo.aff_code}
                    </div>
                  </div>
                )}
                {userInfo.aff_count !== undefined && (
                  <div>
                    <div className='text-muted-foreground text-[11px]'>
                      {t('Invited Users')}
                    </div>
                    <div className='font-mono font-semibold mt-0.5'>
                      {formatCompactNumber(userInfo.aff_count)}
                    </div>
                  </div>
                )}
                {userInfo.aff_quota !== undefined && userInfo.aff_quota > 0 && (
                  <div>
                    <div className='text-muted-foreground text-[11px]'>
                      {t('Invitation Quota')}
                    </div>
                    <div className='font-mono font-semibold mt-0.5'>
                      {formatQuota(userInfo.aff_quota)}
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* 5. Remark */}
          {userInfo.remark && (
            <div className='rounded-xl border border-border/60 bg-muted/15 px-3.5 py-2.5 space-y-1'>
              <div className='flex items-center gap-1.5 text-muted-foreground text-xs font-medium'>
                <MessageSquareText className='size-3.5 text-primary' />
                <span>{t('Remark')}</span>
              </div>
              <div className='text-xs leading-relaxed font-medium text-foreground break-words pl-5'>
                {userInfo.remark}
              </div>
            </div>
          )}
        </div>
      )}
      {!isLoading && !userInfo && (
        <div className='text-muted-foreground py-8 text-center text-sm'>
          {t('No user information available')}
        </div>
      )}
    </Dialog>
  )
}
