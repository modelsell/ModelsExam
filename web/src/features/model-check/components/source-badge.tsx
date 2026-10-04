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
import { SOURCE_STYLE, type ClaudeSource } from '../lib/claude-source'

export function useSourceLabels(): Record<ClaudeSource, string> {
  const { t } = useTranslation()
  return {
    official: t('Official Anthropic API'),
    aws: t('AWS Bedrock'),
    relay: t('Other type'),
  }
}

export function SourceBadge(props: { source: ClaudeSource; className?: string }) {
  const labels = useSourceLabels()
  return (
    <span
      className={cn(
        'inline-block rounded border px-1.5 py-0.5 text-[10px] leading-none font-medium whitespace-nowrap',
        SOURCE_STYLE[props.source],
        props.className
      )}
    >
      {labels[props.source]}
    </span>
  )
}
