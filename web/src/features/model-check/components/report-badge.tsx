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
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { BadgeTier } from '../lib/badge'

// Accent (text) on a tint of the same hue. Same pairs as the badge images the
// server draws, in a light and a dark variant, so the two always match.
const TIER_STYLE: Record<BadgeTier, string> = {
  conformant: 'bg-[#e4f4ea] text-[#116532] dark:bg-[#10301e] dark:text-[#4ade80]',
  mostly: 'bg-[#fbefd6] text-[#8a4f00] dark:bg-[#33270a] dark:text-[#fbbf24]',
  review: 'bg-[#fcebe9] text-[#a9211a] dark:bg-[#3a1416] dark:text-[#ff8a84]',
  incomplete: 'bg-[#eceff4] text-[#505b6b] dark:bg-[#1c2330] dark:text-[#a9b4c6]',
  running: 'bg-[#e6eefc] text-[#1d4fb8] dark:bg-[#0f2347] dark:text-[#8db4ff]',
  unscored: 'bg-[#eceff4] text-[#505b6b] dark:bg-[#1c2330] dark:text-[#a9b4c6]',
}

export function useBadgeLabels(): Record<BadgeTier, string> {
  const { t } = useTranslation()
  return {
    conformant: t('Conformant'),
    mostly: t('Mostly conformant'),
    review: t('Review suggested'),
    incomplete: t('Incomplete'),
    running: t('In progress'),
    unscored: t('Not scored'),
  }
}

function ShieldMark() {
  return (
    <svg
      viewBox='0 0 16 16'
      className='size-3.5 shrink-0'
      aria-hidden='true'
      focusable='false'
    >
      <path
        d='M8 1.2 2.6 3.1v4.2c0 3.2 2.2 5.7 5.4 7.5 3.2-1.8 5.4-4.3 5.4-7.5V3.1L8 1.2Z'
        fill='none'
        stroke='currentColor'
        strokeWidth='1.3'
        strokeLinejoin='round'
      />
      <path
        d='m5.5 8.1 1.8 1.8 3.3-3.6'
        fill='none'
        stroke='currentColor'
        strokeWidth='1.3'
        strokeLinecap='round'
        strokeLinejoin='round'
      />
    </svg>
  )
}

// Plate badge: the issuer's mark on the left, the verdict on a tinted
// right half. Colour is never the only signal: the verdict is always written out.
export function ReportBadge(props: {
  tier: BadgeTier
  score?: number | null
  size?: 'sm' | 'md'
  className?: string
}) {
  const { t } = useTranslation()
  const labels = useBadgeLabels()
  const showScore =
    props.score != null && !['running', 'incomplete'].includes(props.tier)
  const md = props.size === 'md'
  return (
    <span
      role='img'
      aria-label={`${t('ModelsExam')}: ${labels[props.tier]}${
        showScore ? ` ${props.score}/100` : ''
      }`}
      className={cn(
        'bg-card text-foreground inline-flex items-stretch overflow-hidden rounded-lg border font-semibold whitespace-nowrap shadow-xs',
        md ? 'text-sm' : 'text-xs',
        props.className
      )}
    >
      <span
        className={cn(
          'flex items-center gap-1.5',
          md ? 'px-2.5 py-1.5' : 'px-2 py-1'
        )}
      >
        <ShieldMark />
      </span>
      <span
        className={cn(
          'flex items-center gap-2 border-l',
          md ? 'px-3 py-1.5' : 'px-2 py-1',
          TIER_STYLE[props.tier]
        )}
      >
        {labels[props.tier]}
        {showScore && (
          <span className='font-mono font-bold tabular-nums'>{props.score}</span>
        )}
      </span>
    </span>
  )
}
