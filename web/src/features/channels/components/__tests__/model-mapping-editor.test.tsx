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
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { useState } from 'react'
import { I18nextProvider } from 'react-i18next'
import { expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

import { ModelMappingEditor } from '../model-mapping-editor'

test('external mapping changes update the editor without emitting an edit', () => {
  const onChange = vi.fn()
  const view = render(
    <ModelMappingEditor value='{"client-a":"upstream-a"}' onChange={onChange} />
  )
  expect(screen.getByDisplayValue('client-a')).toBeVisible()
  expect(screen.getByDisplayValue('upstream-a')).toBeVisible()

  view.rerender(
    <ModelMappingEditor value='{"client-b":"upstream-b"}' onChange={onChange} />
  )
  expect(screen.getByDisplayValue('client-b')).toBeVisible()
  expect(screen.getByDisplayValue('upstream-b')).toBeVisible()
  expect(screen.queryByDisplayValue('client-a')).not.toBeInTheDocument()
  expect(onChange).not.toHaveBeenCalled()
})

test('language changes preserve draft mappings and explain the same direction in JSON mode', async () => {
  const i18n = createInstance()
  await i18n.init({
    lng: 'en',
    fallbackLng: 'en',
    resources: { en, zh },
    keySeparator: false,
    interpolation: { escapeValue: false },
  })
  const user = userEvent.setup()
  const onChange = vi.fn()
  render(
    <I18nextProvider i18n={i18n}>
      <ModelMappingEditor value='' onChange={onChange} />
    </I18nextProvider>
  )
  await user.click(screen.getByRole('button', { name: 'Add Mapping' }))
  await user.type(screen.getByPlaceholderText('gpt-3.5-turbo'), 'client-alias')
  await user.type(
    screen.getByPlaceholderText('gpt-3.5-turbo-0125'),
    'provider-model'
  )

  await act(() => i18n.changeLanguage('zh'))
  expect(screen.getByText('请求模型名称')).toBeVisible()
  expect(screen.getByText('上游模型名称')).toBeVisible()
  expect(screen.getByDisplayValue('client-alias')).toBeVisible()
  expect(screen.getByDisplayValue('provider-model')).toBeVisible()

  await user.click(screen.getByRole('tab', { name: 'JSON' }))
  expect(
    screen.getByText('JSON 的键是请求模型名称，值是上游模型名称。')
  ).toBeVisible()
  expect(screen.getByRole('textbox', { name: '模型映射' })).toHaveValue(
    '{\n  "client-alias": "provider-model"\n}'
  )
  expect(onChange).toHaveBeenLastCalledWith(
    '{\n  "client-alias": "provider-model"\n}'
  )
})

test('controlled mode preserves new empty row when parent reflects empty onChange', async () => {
  const user = userEvent.setup()
  function ControlledWrapper() {
    const [value, setValue] = useState('')
    return <ModelMappingEditor value={value} onChange={setValue} />
  }

  render(<ControlledWrapper />)
  expect(
    screen.getByText('No model mappings configured. Click "Add Mapping" to get started.')
  ).toBeVisible()

  // Clicking "Add Mapping" must not be wiped out by an immediate sync loop
  await user.click(screen.getByRole('button', { name: 'Add Mapping' }))
  const fromInput = screen.getByPlaceholderText('gpt-3.5-turbo')
  const toInput = screen.getByPlaceholderText('gpt-3.5-turbo-0125')
  expect(fromInput).toBeVisible()
  expect(toInput).toBeVisible()

  // User types into the inputs
  await user.type(fromInput, 'gemini-3.8-flash')
  await user.type(toInput, 'gemini-3.8-flash-tiered')
  expect(fromInput).toHaveValue('gemini-3.8-flash')
  expect(toInput).toHaveValue('gemini-3.8-flash-tiered')

  // Adding a second row must preserve the first row and not destroy either
  await user.click(screen.getByRole('button', { name: 'Add Mapping' }))
  const allFromInputs = screen.getAllByPlaceholderText('gpt-3.5-turbo')
  expect(allFromInputs).toHaveLength(2)
  expect(allFromInputs[0]).toHaveValue('gemini-3.8-flash')
  expect(allFromInputs[1]).toHaveValue('')

  // Clearing the second input text must not wipe out the second row
  await user.type(allFromInputs[1], 'abc')
  expect(allFromInputs[1]).toHaveValue('abc')
  await user.clear(allFromInputs[1])
  expect(screen.getAllByPlaceholderText('gpt-3.5-turbo')).toHaveLength(2)
})

test('initial value is immediately rendered in visual mode without empty placeholder', () => {
  const onChange = vi.fn()
  render(
    <ModelMappingEditor
      value='{"source-model":"target-model"}'
      onChange={onChange}
    />
  )
  expect(screen.getByDisplayValue('source-model')).toBeVisible()
  expect(screen.getByDisplayValue('target-model')).toBeVisible()
  expect(
    screen.queryByText(
      'No model mappings configured. Click "Add Mapping" to get started.'
    )
  ).not.toBeInTheDocument()
})
