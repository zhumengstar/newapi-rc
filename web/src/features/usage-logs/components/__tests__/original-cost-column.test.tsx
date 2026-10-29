import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { OriginalCostDisplay } from '../log-cost-display'

describe('OriginalCostDisplay', () => {
  it('renders $ 0 when both original and actual quotas are 0', () => {
    render(
      <OriginalCostDisplay originalQuota={0} actualQuota={0} ratio={null} />
    )
    expect(screen.getByText('$ 0')).toBeInTheDocument()
  })

  it('renders correctly formatted amount for discounted request', () => {
    // 500,000 quota = $1.00
    // originalQuota = 500,000 ($1.00), ratio = 0.5, actualQuota = 250,000 ($0.50)
    render(
      <OriginalCostDisplay
        originalQuota={500000}
        actualQuota={250000}
        ratio={0.5}
      />
    )
    expect(screen.getByText('$')).toBeInTheDocument()
    expect(screen.getByText('1')).toBeInTheDocument()
  })

  it('calculates higher precision for small amounts', () => {
    // 45,151 prompt ($0.3/M) + 257 completion ($2.5/M) = $0.014188 = ~7094 quota
    render(
      <OriginalCostDisplay
        originalQuota={7094}
        actualQuota={1135}
        ratio={0.16}
      />
    )
    expect(screen.getByText('$')).toBeInTheDocument()
    expect(screen.getByText('0.014188')).toBeInTheDocument()
  })
})
