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

export function useBaselineLabels() {
  const { t } = useTranslation()
  const names: Record<string, string> = {
    number: t('Number preference'),
    letter: t('Letter preference'),
    color: t('Color preference'),
    animal: t('Chinese animal preference'),
    mod_product: t('Modular multiplication'),
    mod_power: t('Modular exponentiation'),
    digit_count: t('Digit counting'),
    blocked_grid: t('Blocked-grid paths'),
    boolean_count: t('Boolean constraints'),
    dependency_schedule: t('Dependency scheduling'),
    python_alias: t('Python aliasing'),
    javascript_queue: t('JavaScript microtasks'),
    unicode_length: t('Unicode length'),
    sql_null: t('SQL null semantics'),
    zh_constraint: t('Chinese filtering constraints'),
    context_retrieval: t('Long-context retrieval'),
    instruction_format: t('Multi-constraint formatting'),
    json_extraction: t('Structured data extraction'),
    json_types: t('JSON types and missing values'),
    context_multihop: t('Cross-record retrieval'),
    tool_selection: t('Tool selection and arguments'),
    tool_roundtrip: t('Tool result roundtrip'),
  }
  return names
}
