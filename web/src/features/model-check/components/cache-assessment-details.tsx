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
import type { CacheAssessment } from '../types'

export function CacheAssessmentDetails(props: {
  assessment: CacheAssessment
  paper?: boolean
}) {
  const { t } = useTranslation()
  const className = props.paper ? 'mc-muted' : 'text-muted-foreground text-xs'
  return (
    <>
      <p className={className}>
        {t(
          'Cache rounds {{rounds}} · Measured reads {{measured}}/{{planned}} · Coverage {{coverage}}%',
          props.assessment
        )}
      </p>
      <p className={className}>
        {t(
          'Each round writes a fresh prefix and reads it three times. Misses and rewrites reduce points; unmeasured reads earn no evidence points. The final score uses all six planned reads.'
        )}
      </p>
    </>
  )
}
