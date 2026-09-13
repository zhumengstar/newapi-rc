import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/console/canvas')({
  beforeLoad: () => {
    throw redirect({ to: '/canvas' })
  },
})
