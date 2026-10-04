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
import {
  IMAGE_CHECK_DETAILS,
  IMAGE_CHECK_TITLES,
  IMAGE_CODE_NOTES,
  IMAGE_STAGES,
  PROVENANCE_LEVELS,
  PROVENANCE_LEVEL_NOTES,
} from '../lib/catalog'
import type { ImageCheckStatus } from '../types'

export function useImageLabels() {
  const { t } = useTranslation()
  const states: Record<ImageCheckStatus, string> = {
    pass: t('Passed'),
    fail: t('Failed'),
    inconclusive: t('Inconclusive'),
    skipped: t('Skipped'),
  }
  const lookup = (table: Record<string, string>, key: string) =>
    table[key] ? t(table[key]) : key
  return {
    states,
    stage: (id: string) =>
      lookup(Object.fromEntries(IMAGE_STAGES.map((s) => [s.id, s.title])), id),
    title: (id: string) => lookup(IMAGE_CHECK_TITLES, id),
    detail: (id: string) =>
      IMAGE_CHECK_DETAILS[id] ? t(IMAGE_CHECK_DETAILS[id]) : '',
    code: (code: string) => lookup(IMAGE_CODE_NOTES, code),
    level: (level: string) => lookup(PROVENANCE_LEVELS, level),
    levelNote: (level: string) =>
      PROVENANCE_LEVEL_NOTES[level] ? t(PROVENANCE_LEVEL_NOTES[level]) : '',
  }
}
