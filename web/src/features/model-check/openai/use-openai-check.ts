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
import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { streamOpenAICheck } from './api'
import {
  applyOpenAIEvent,
  INITIAL_OPENAI_RUN,
  interruptOpenAIRun,
} from './lib/run-state'
import type { OpenAICheckTarget, OpenAIRunState } from './types'

export function useOpenAICheck() {
  const queryClient = useQueryClient()
  const [state, setState] = useState<OpenAIRunState>(INITIAL_OPENAI_RUN)
  const [elapsed, setElapsed] = useState(0)
  const active = useRef<AbortController | null>(null)
  const alive = useRef(true)
  useEffect(() => {
    alive.current = true
    return () => {
      alive.current = false
      active.current?.abort()
    }
  }, [])

  async function start(target: OpenAICheckTarget): Promise<void> {
    if (active.current) return
    const controller = new AbortController()
    active.current = controller
    const started = Date.now()
    const deadline = setTimeout(() => controller.abort(), 615_000)
    setElapsed(0)
    setState({ ...INITIAL_OPENAI_RUN, phase: 'connecting' })
    const timer = setInterval(() => {
      if (alive.current) setElapsed(Date.now() - started)
    }, 250)
    // Credentials stay in this request and the form; nothing is cached or stored.
    try {
      await streamOpenAICheck(target, controller.signal, (event) => {
        // The run is stored from its first event, so the list can show it.
        if (event.type === 'start')
          void queryClient.invalidateQueries({
            queryKey: ['model-check-history'],
          })
        if (alive.current)
          setState((previous) => applyOpenAIEvent(previous, event))
      })
    } catch (error) {
      if (alive.current)
        setState((previous) =>
          interruptOpenAIRun(
            previous,
            controller.signal.aborted,
            Date.now() - started,
            controller.signal.aborted
              ? null
              : String(error instanceof Error ? error.message : error)
          )
        )
    } finally {
      clearTimeout(deadline)
      clearInterval(timer)
      if (active.current === controller) active.current = null
      void queryClient.invalidateQueries({ queryKey: ['model-check-history'] })
    }
  }
  return {
    state,
    elapsed,
    start,
    cancel: () => active.current?.abort(),
    busy: state.phase === 'connecting' || state.phase === 'running',
  }
}
