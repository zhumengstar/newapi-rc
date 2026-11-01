import { toast } from 'sonner'

import type { UpstreamSourceDTO } from '../types'

const KNOWN_RELAY_DASHBOARD_MAP: Record<string, string> = {
  'https://direct.apiloop.top': 'https://www.apiloop.top',
  'https://direct-vapi.chuangxiangai.asia': 'https://vapi.chuangxiangai.asia',
  'https://global-api.mysandbox.vip': 'https://mysandbox.vip',
  'https://cnd-coo-new.pages.dev': 'https://api.code-relay.com',
  'https://fuck.cccapi.top': 'https://cccapi.top',
  'https://api.vakv.cn': 'https://vakv.cn',
}

export function resolveUpstreamConsoleUrl(
  source?: Partial<UpstreamSourceDTO> | null,
  rawBaseUrl?: string
): string {
  const configured = source?.dashboard_url?.trim()
  if (configured) {
    return configured.replace(/\/+$/, '')
  }
  const baseKey = (source?.base_url || rawBaseUrl || '')
    .trim()
    .toLowerCase()
    .replace(/\/+$/, '')
  if (baseKey && KNOWN_RELAY_DASHBOARD_MAP[baseKey]) {
    return KNOWN_RELAY_DASHBOARD_MAP[baseKey]
  }
  const display = (source?.display_base_url || rawBaseUrl || source?.base_url || '')
    .trim()
    .replace(/\/+$/, '')
  if (!display || display === 'default' || display.includes('官方直连')) {
    return ''
  }
  if (!/^https?:\/\//i.test(display)) {
    return `https://${display}`
  }
  return display
}

function encodeBase64UrlUTF8(obj: Record<string, unknown>): string {
  const jsonStr = JSON.stringify(obj)
  const b64 = window.btoa(unescape(encodeURIComponent(jsonStr)))
  return b64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export function openUpstreamOneClickLogin(
  source?: Partial<UpstreamSourceDTO> | null,
  options?: {
    rawBaseUrl?: string
    dashboardUrl?: string
    username?: string
    password?: string
    platform?: string
  }
) {
  const consoleUrl =
    options?.dashboardUrl?.trim().replace(/\/+$/, '') ||
    resolveUpstreamConsoleUrl(source, options?.rawBaseUrl)

  if (!consoleUrl) {
    toast.warning('该来源为官方直连或未配置有效域名')
    return
  }

  const username = (
    options?.username ??
    source?.upstream_username ??
    ''
  ).trim()
  const password = (
    options?.password ??
    source?.upstream_password ??
    ''
  ).trim()
  const platform = (
    options?.platform ??
    source?.upstream_platform ??
    'auto'
  ).trim()

  if (password) {
    try {
      void navigator.clipboard.writeText(password)
    } catch {
      // ignore clipboard errors
    }
  } else if (username) {
    try {
      void navigator.clipboard.writeText(username)
    } catch {
      // ignore clipboard errors
    }
  }

  if (username && password) {
    const ssoToken = encodeBase64UrlUTF8({
      u: username,
      p: password,
      platform: platform || 'auto',
      ts: Date.now(),
    })
    const entryPath = platform === 'sub2api' ? '/dashboard' : '/console'
    const targetUrl = `${consoleUrl}${entryPath}#muling_sso=${ssoToken}`
    window.open(targetUrl, '_blank', 'noopener')
    toast.success(`🚀 正在一键登录 ${consoleUrl}`, {
      description: `账号: ${username}（已自动复制密码并注入免密令牌）`,
    })
    return
  }

  const fallbackPath = platform === 'sub2api' ? '/dashboard' : '/console'
  window.open(`${consoleUrl}${fallbackPath}`, '_blank', 'noopener')
  if (username) {
    toast.info(`🌐 已打开控制台 ${consoleUrl}`, {
      description: `已复制账号: ${username}（可在来源设置中补充密码以启用全自动免密登录）`,
    })
  } else {
    toast.info(`🌐 已打开控制台 ${consoleUrl}`, {
      description: '可在「成本与模型单价」中配置上游登录账号与密码，实现点击域名一键免密登录',
    })
  }
}
