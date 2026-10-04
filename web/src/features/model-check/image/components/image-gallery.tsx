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
import { useImageLabels } from '../hooks/use-image-labels'
import type { ImageRef } from '../types'

// Thumbnails exist only in the live stream. Stored reports keep metadata, so
// the gallery degrades to a list of sizes and hashes.
export function ImageGallery(props: { images: ImageRef[] }) {
  const { t } = useTranslation()
  const labels = useImageLabels()
  if (props.images.length === 0) return null
  return (
    <details className='rounded-xl border p-4' open>
      <summary className='cursor-pointer text-sm font-medium'>
        {t('Returned images')} · {props.images.length}
      </summary>
      <ul className='mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4'>
        {props.images.map((image) => (
          <li
            key={`${image.probe}-${image.index}-${image.sha256}`}
            className='flex flex-col gap-1 text-xs'
          >
            {image.thumb ? (
              <img
                src={image.thumb}
                alt={labels.title(image.probe)}
                className='bg-muted aspect-square w-full rounded-md border object-contain'
              />
            ) : (
              <div className='bg-muted/50 text-muted-foreground flex aspect-square w-full items-center justify-center rounded-md border text-center'>
                {t('No preview')}
              </div>
            )}
            <span className='truncate font-medium'>
              {labels.title(image.probe)}
            </span>
            <span className='text-muted-foreground tabular-nums'>
              {image.width}×{image.height} · {image.format} ·{' '}
              {(image.bytes / 1024).toFixed(0)} KB
            </span>
          </li>
        ))}
      </ul>
    </details>
  )
}
