import type { RetestPlan } from './retest'

// "Schedule this check" on a record hands its rebuilt configuration to the
// schedules page.
let pending: RetestPlan | undefined

export function setSchedulePrefill(plan: RetestPlan) {
  pending = plan
}

export function takeSchedulePrefill(): RetestPlan | undefined {
  const value = pending
  pending = undefined
  return value
}
