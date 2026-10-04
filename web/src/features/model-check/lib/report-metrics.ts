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
import type { ClaudeCheckSample } from '../types'
import { isCountProbe } from './check-plan'

export function median(values: number[]): number | null {
  if (!values.length) return null
  const sorted = [...values].sort((a, b) => a - b)
  const middle = Math.floor(sorted.length / 2)
  return sorted.length % 2
    ? sorted[middle]
    : (sorted[middle - 1] + sorted[middle]) / 2
}

export function reportMetrics(samples: ClaudeCheckSample[]) {
  const valid = (sample: ClaudeCheckSample) => sample.valid_response === true
  const repeats = samples.filter((sample) => sample.probe.startsWith('repeat_'))
  const successful = repeats.filter(valid)
  const times = successful
    .map((sample) => sample.stream?.first_text_ms)
    .filter((value): value is number => value != null)
  const durations = successful.map((sample) => sample.duration_ms)
  const cache = samples.filter((sample) => sample.probe.startsWith('cache_'))
  const warm = cache.filter(
    (sample) => sample.probe.startsWith('cache_read_') && valid(sample)
  )
  const reads = warm.filter(
    (sample) => sample.usage.cache_read_input_tokens != null
  )
  const sumUsage = (field: keyof ClaudeCheckSample['usage']) => {
    const inference = samples.filter(
      (sample) => !isCountProbe(sample.probe) && sample.http_status === 200
    )
    const values = inference.map((sample) => sample.usage[field])
    if (!values.length || values.some((value) => value == null || value < 0))
      return null
    return values.reduce<number>((sum, value) => sum + (value ?? 0), 0)
  }
  return {
    requests: samples.length,
    repeatAttempts: repeats.length,
    repeatSuccess: successful.length,
    ttftMedian: median(times),
    ttftSamples: times.length,
    durationMedian: median(durations),
    durationMin: durations.length ? Math.min(...durations) : null,
    durationMax: durations.length ? Math.max(...durations) : null,
    warmAttempts: cache.filter((sample) =>
      sample.probe.startsWith('cache_read_')
    ).length,
    warmMeasured: reads.length,
    warmHits: reads.filter(
      (sample) => (sample.usage.cache_read_input_tokens ?? 0) > 0
    ).length,
    input: sumUsage('input_tokens'),
    output: sumUsage('output_tokens'),
    cacheWrite: sumUsage('cache_creation_input_tokens'),
    cacheRead: sumUsage('cache_read_input_tokens'),
    stream: samples.find((sample) => sample.probe === 'stream'),
  }
}
