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
import { z } from 'zod'

import {
  type PermissionCatalog,
  type AdminPermissionMatrix,
  normalizeAdminPermissions,
} from '@/lib/admin-permissions'
import { quotaUnitsToDollars } from '@/lib/format'
import { ROLE } from '@/lib/roles'

import { DEFAULT_GROUP } from '../constants'
import { type UserFormData, type User, type UserModelPriceRule } from '../types'

// ============================================================================
// Form Schema
// ============================================================================

export const userModelPriceRuleSchema = z.object({
  group: z.string().min(1),
  models: z.array(z.string()).min(1),
  price: z.number().min(0),
})

export const userFormSchema = z.object({
  username: z.string().min(1, 'Username is required'),
  display_name: z.string().optional(),
  password: z.string().optional(),
  role: z.number().optional(),
  quota_dollars: z.number().min(0).optional(),
  group: z.string().optional(),
  user_group_ratios: z.record(z.string(), z.number().min(0)).optional(),
  user_model_price_rules: z.array(userModelPriceRuleSchema).optional(),
  remark: z.string().optional(),
  admin_permissions: z
    .record(z.string(), z.record(z.string(), z.boolean()))
    .optional(),
})

export type UserFormValues = z.infer<typeof userFormSchema>

// ============================================================================
// Form Defaults
// ============================================================================

export const USER_FORM_DEFAULT_VALUES: UserFormValues = {
  username: '',
  display_name: '',
  password: '',
  role: 1, // Default to common user
  quota_dollars: 0,
  group: DEFAULT_GROUP,
  user_group_ratios: {},
  user_model_price_rules: [],
  remark: '',
  // Filled against the backend catalog at render time; see UsersMutateDrawer.
  admin_permissions: {},
}

// ============================================================================
// Helper Parsers
// ============================================================================

export function parseUserGroupRatios(setting?: string): Record<string, number> {
  if (!setting) return {}
  try {
    const parsed = typeof setting === 'string' ? JSON.parse(setting) : setting
    const source = parsed?.user_group_ratios || {}
    const result: Record<string, number> = {}
    for (const [k, v] of Object.entries(source)) {
      const num = Number(v)
      if (k && Number.isFinite(num) && num >= 0) {
        result[k] = num
      }
    }
    return result
  } catch {
    return {}
  }
}

export function parseUserModelPriceRules(
  setting?: string,
  fallbackGroup = 'default'
): UserModelPriceRule[] {
  if (!setting) return []
  try {
    const parsed = typeof setting === 'string' ? JSON.parse(setting) : setting
    if (Array.isArray(parsed?.user_model_price_rules)) {
      return parsed.user_model_price_rules
        .map((r: any) => ({
          group: String(r?.group || '').trim(),
          models: Array.from(
            new Set(
              (Array.isArray(r?.models) ? r.models : [])
                .map((m: any) => String(m).trim())
                .filter(Boolean)
            )
          ),
          price: Number(r?.price ?? 0),
        }))
        .filter(
          (r: UserModelPriceRule) =>
            r.group &&
            r.models.length > 0 &&
            Number.isFinite(r.price) &&
            r.price >= 0
        )
    }
    if (parsed?.user_model_prices && typeof parsed.user_model_prices === 'object') {
      return Object.entries(parsed.user_model_prices)
        .map(([m, p]) => ({
          group: fallbackGroup,
          models: [m],
          price: Number(p),
        }))
        .filter((r) => r.models[0] && Number.isFinite(r.price) && r.price >= 0)
    }
    return []
  } catch {
    return []
  }
}

// ============================================================================
// Form Data Transformation
// ============================================================================

/**
 * Transform form data to API payload
 */
export function transformFormDataToPayload(
  data: UserFormValues,
  userId?: number,
  catalog?: PermissionCatalog
): UserFormData & { id?: number } {
  const payload: UserFormData & { id?: number } = {
    username: data.username,
    display_name: data.display_name || data.username,
    password: data.password || undefined,
  }

  const role = userId === undefined ? data.role || 1 : (data.role ?? 0)

  // Only send the permission matrix when the target is an admin and the catalog
  // is available; without the catalog we cannot build a full matrix, so we omit
  // the field (the backend then leaves existing permissions untouched).
  if (role >= ROLE.ADMIN && catalog) {
    payload.admin_permissions = normalizeAdminPermissions(
      data.admin_permissions as AdminPermissionMatrix | undefined,
      catalog
    )
  }

  // For create: only send required fields
  if (userId === undefined) {
    payload.role = role
  } else {
    // For update: quota is adjusted atomically via /api/user/manage, not sent here
    payload.group = data.group
    payload.user_group_ratios = data.user_group_ratios
    payload.user_model_price_rules = data.user_model_price_rules
    payload.remark = data.remark || undefined
    payload.id = userId
  }

  return payload
}

/**
 * Transform user data to form defaults. The admin permission matrix is passed
 * through as-is (the backend already returns a full matrix); it is filled against
 * the catalog at render time in UsersMutateDrawer.
 */
export function transformUserToFormDefaults(user: User): UserFormValues {
  const groups = (user.group || DEFAULT_GROUP)
    .split(',')
    .map((g) => g.trim())
    .filter(Boolean)
  const groupRatios = parseUserGroupRatios(user.setting)
  const priceRules = parseUserModelPriceRules(user.setting, groups[0] || 'default')

  return {
    username: user.username,
    display_name: user.display_name,
    password: '',
    role: user.role,
    quota_dollars: quotaUnitsToDollars(user.quota),
    group: user.group || DEFAULT_GROUP,
    user_group_ratios: groupRatios,
    user_model_price_rules: priceRules,
    remark: user.remark || '',
    admin_permissions: user.admin_permissions ?? {},
  }
}
