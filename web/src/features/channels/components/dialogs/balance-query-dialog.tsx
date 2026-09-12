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
import { useQueryClient } from '@tanstack/react-query'
import { Loader2, RefreshCw, DollarSign } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { Input } from '@/components/ui/input'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'

import {
  getChannelBalanceSettings,
  getCodexUsage,
  setChannelBalance,
  updateChannelBalance,
  updateChannelBalanceSettings,
} from '../../api'
import { channelsQueryKeys } from '../../lib'
import { useChannels } from '../channels-provider'
import {
  CodexUsageDialog,
  type CodexUsageDialogData,
} from './codex-usage-dialog'

type BalanceQueryDialogProps = {
  initialRawResponse?: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function BalanceQueryDialog(props: BalanceQueryDialogProps) {
  const { t } = useTranslation()
  const { currentRow, setCurrentRow } = useChannels()
  const queryClient = useQueryClient()
  const [isQuerying, setIsQuerying] = useState(false)
  const [balance, setBalance] = useState<number | null>(null)
  const [balanceUpdatedTime, setBalanceUpdatedTime] = useState<number | null>(
    null
  )
  const [rawResponse, setRawResponse] = useState<string | null>(
    props.initialRawResponse ?? null
  )
  const [codexUsageResponse, setCodexUsageResponse] =
    useState<CodexUsageDialogData | null>(null)
  const [manualBalance, setManualBalance] = useState('')
  const [balanceMode, setBalanceMode] = useState<'auto' | 'manual'>('auto')
  const [balanceToken, setBalanceToken] = useState('')
  const [balanceUsername, setBalanceUsername] = useState('')
  const [balancePassword, setBalancePassword] = useState('')
  const [storedCredentialsConfigured, setStoredCredentialsConfigured] =
    useState(false)

  const isCodex = currentRow?.type === 57

  const handleQueryCodexUsage = async () => {
    const row = currentRow
    if (!row) return
    setIsQuerying(true)
    try {
      const res = await getCodexUsage(row.id)
      if (!res.success) {
        throw createServerError(res, t('Failed to fetch usage'))
      }
      setCodexUsageResponse(res)
    } catch (error: unknown) {
      handleServerError(error, t('Failed to fetch usage'))
    } finally {
      setIsQuerying(false)
    }
  }

  useEffect(() => {
    if (!isCodex) return
    if (!props.open) return
    handleQueryCodexUsage()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.open, isCodex])

  useEffect(() => {
    if (props.open && currentRow && !isCodex) {
      // Reset values whenever the dialog switches rows to avoid displaying a
      // previous channel's credentials while the new settings are loading.
      setBalanceToken('')
      setBalancePassword('')
      setStoredCredentialsConfigured(false)
      setBalanceMode(currentRow.balance_mode === 'manual' ? 'manual' : 'auto')

      void getChannelBalanceSettings(currentRow.id)
        .then((response) => {
          if (!response.success || !response.data) return
          setBalanceUsername(response.data.username)
          setBalancePassword(response.data.password)
          setBalanceToken(response.data.access_token)
          setBalanceMode(response.data.mode)
          setStoredCredentialsConfigured(
            response.data.access_token_configured ||
              response.data.login_configured
          )
        })
        .catch((error: unknown) => {
          toast.error(
            error instanceof Error
              ? error.message
              : t('Failed to load balance settings')
          )
        })
    }
  }, [props.open, currentRow, isCodex, t])

  if (!currentRow) return null

  const handleQueryBalance = async () => {
    setIsQuerying(true)
    try {
      const hasCredentialInput = Boolean(
        balanceToken.trim() || balanceUsername.trim() || balancePassword
      )
      const hasConfiguredCredentials = Boolean(
        currentRow.balance_access_token_configured ||
        currentRow.balance_login_configured ||
        storedCredentialsConfigured
      )
      if (!hasCredentialInput && !hasConfiguredCredentials) {
        toast.error(t('Please log in with the appropriate credentials'))
        return
      }

      // The update button is the primary action in this dialog. Persist any
      // newly entered site credentials before querying, so users do not have
      // to perform two separate actions and the backend sees the same values.
      if (hasCredentialInput) {
        await updateChannelBalanceSettings(currentRow.id, {
          access_token: balanceToken.trim() || undefined,
          username: balanceUsername.trim() || undefined,
          password: balancePassword || undefined,
          mode: balanceMode,
        })
        setCurrentRow({
          ...currentRow,
          balance_access_token_configured:
            currentRow.balance_access_token_configured ||
            Boolean(balanceToken.trim()),
          balance_login_configured:
            currentRow.balance_login_configured ||
            Boolean(balanceUsername.trim() && balancePassword),
          balance_mode: balanceMode,
        })
      }

      const response = await updateChannelBalance(currentRow.id)
      if (response.success && response.balance !== undefined) {
        const newBalance = response.balance
        const now = Math.floor(Date.now() / 1000)

        setBalance(newBalance)
        setBalanceUpdatedTime(now)
        toast.success(t('Balance updated successfully'))

        // Update currentRow immediately with new balance and timestamp
        setCurrentRow({
          ...currentRow,
          balance: newBalance,
          balance_updated_time: now,
        })

        // Invalidate queries to refresh the table
        await queryClient.invalidateQueries({
          queryKey: channelsQueryKeys.lists(),
        })
        setRawResponse(null)
      } else if (response.success && response.raw_response !== undefined) {
        setRawResponse(response.raw_response)
      } else {
        handleServerError(response, t('Failed to query balance'))
      }
    } catch (error: unknown) {
      handleServerError(error, t('Failed to query balance'))
    } finally {
      setIsQuerying(false)
    }
  }

  const handleSaveBalanceSettings = async () => {
    try {
      await updateChannelBalanceSettings(currentRow.id, {
        access_token: balanceToken || undefined,
        username: balanceUsername || undefined,
        password: balancePassword || undefined,
        mode: balanceMode,
      })
      toast.success(t('Balance settings saved'))
      setCurrentRow({
        ...currentRow,
        balance_access_token_configured:
          currentRow.balance_access_token_configured ||
          Boolean(balanceToken.trim()),
        balance_login_configured:
          currentRow.balance_login_configured ||
          Boolean(balanceUsername.trim() && balancePassword),
        balance_mode: balanceMode,
      })
      await queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.lists(),
      })
    } catch (error: unknown) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to save balance settings')
      )
    }
  }

  const handleSaveManualBalance = async () => {
    const value = Number(manualBalance)
    if (!Number.isFinite(value) || value < 0) {
      toast.error(t('Enter a valid non-negative balance'))
      return
    }
    try {
      await setChannelBalance(currentRow.id, value, balanceMode)
      setBalance(value)
      setBalanceUpdatedTime(Math.floor(Date.now() / 1000))
      setCurrentRow({
        ...currentRow,
        balance: value,
        balance_mode: balanceMode,
      })
      toast.success(t('Balance saved'))
      await queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.lists(),
      })
    } catch (error: unknown) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to save balance')
      )
    }
  }

  const handleClose = () => {
    setBalance(null)
    setBalanceUpdatedTime(null)
    setRawResponse(null)
    setCodexUsageResponse(null)
    setManualBalance('')
    setBalanceToken('')
    setBalanceUsername('')
    setBalancePassword('')
    props.onOpenChange(false)
  }

  const formatBalance = (bal: number) =>
    formatCurrencyFromUSD(bal, {
      digitsLarge: 2,
      digitsSmall: 4,
      abbreviate: false,
    })

  const formatDate = (timestamp: number) => {
    if (!timestamp) return 'Never'
    return formatTimestampToDate(timestamp)
  }

  if (isCodex) {
    return (
      <CodexUsageDialog
        open={props.open}
        onOpenChange={(v) => {
          if (!v) handleClose()
        }}
        channelName={currentRow.name}
        channelId={currentRow.id}
        response={codexUsageResponse}
        onRefresh={handleQueryCodexUsage}
        isRefreshing={isQuerying}
      />
    )
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={handleClose}
      title={t('Query Balance')}
      description={
        <>
          {t('Update balance for:')}
          <strong>{currentRow.name}</strong>
        </>
      }
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <Button variant='outline' onClick={handleClose} disabled={isQuerying}>
          {t('Close')}
        </Button>
      }
    >
      <div className='space-y-4 py-4'>
        {rawResponse !== null ? (
          <>
            <Alert>
              <AlertTitle>{t('Balance response not recognized')}</AlertTitle>
              <AlertDescription>
                {t(
                  'The upstream response is valid JSON, but it does not match the OpenAI credit_summary format. The channel balance was not updated.'
                )}
              </AlertDescription>
            </Alert>
            <CodeBlock
              code={rawResponse}
              language='json'
              maxExpandedLines={24}
              showLineNumbers
              title={t('Upstream JSON response')}
            >
              <CodeBlockCopyButton />
            </CodeBlock>
          </>
        ) : (
          <>
            {/* Current Balance Display */}
            <div className='bg-muted/50 rounded-lg border p-4'>
              <div className='text-muted-foreground mb-2 flex items-center gap-2 text-sm'>
                <IconBadge tone='success' size='xs'>
                  <DollarSign />
                </IconBadge>
                <span>{t('Current Balance')}</span>
              </div>
              <div className='text-2xl font-bold'>
                {balance !== null
                  ? formatBalance(balance)
                  : formatBalance(currentRow.balance)}
              </div>
              <div className='text-muted-foreground mt-2 text-xs'>
                {t('Last updated:')}{' '}
                {formatDate(
                  balanceUpdatedTime ?? currentRow.balance_updated_time
                )}
              </div>
            </div>
          </>
        )}

        {/* Balance Update Button */}
        <Button
          className='w-full'
          onClick={handleQueryBalance}
          disabled={isQuerying}
        >
          {isQuerying && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
          {!isQuerying && <RefreshCw className='mr-2 h-4 w-4' />}
          {isQuerying ? t('Querying...') : t('Update Balance')}
        </Button>
        <div className='space-y-3 rounded-lg border p-4'>
          <div className='text-sm font-medium'>{t('Balance access')}</div>
          <Input
            value={balanceUsername}
            onChange={(e) => setBalanceUsername(e.target.value)}
            placeholder={t('Login username or email')}
          />
          <Input
            type='text'
            value={balancePassword}
            onChange={(e) => setBalancePassword(e.target.value)}
            placeholder={t('Login password (leave empty to keep)')}
          />
          <Input
            type='text'
            value={balanceToken}
            onChange={(e) => setBalanceToken(e.target.value)}
            placeholder={t('Access token (leave empty to keep)')}
          />
          <div className='flex items-center gap-2'>
            <span className='text-muted-foreground text-sm'>{t('Mode')}</span>
            <select
              className='bg-background h-9 rounded-md border px-2 text-sm'
              value={balanceMode}
              onChange={(e) =>
                setBalanceMode(e.target.value as 'auto' | 'manual')
              }
            >
              <option value='auto'>{t('Automatic refresh')}</option>
              <option value='manual'>{t('Manual balance')}</option>
            </select>
          </div>
          <Button variant='outline' onClick={handleSaveBalanceSettings}>
            {t('Save balance settings')}
          </Button>
          <div className='flex gap-2'>
            <Input
              type='number'
              min='0'
              step='any'
              value={manualBalance}
              onChange={(e) => setManualBalance(e.target.value)}
              placeholder={t('Manual balance value')}
            />
            <Button onClick={handleSaveManualBalance}>
              {t('Save balance')}
            </Button>
          </div>
        </div>
      </div>
    </Dialog>
  )
}
