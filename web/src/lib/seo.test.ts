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
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { seoFor } from './seo'

const t = (k: string) => k

test('public pages are indexable and have a description', () => {
  for (const name of ['home', 'records', 'baselines', 'getbadge', 'method', 'integrate'] as const) {
    const seo = seoFor({ name }, t)
    assert.equal(seo.noindex, false, name)
    assert.ok(seo.title.includes('ModelsExam'), name)
    assert.ok(seo.description.length > 40 && seo.description.length <= 200, name)
  }
})

test('reports and unknown paths are noindex', () => {
  assert.equal(seoFor({ name: 'report', id: 'x' }, t).noindex, true)
  assert.equal(seoFor({ name: 'notfound' }, t).noindex, true)
})

test('titles are unique per page', () => {
  const names = ['home', 'records', 'baselines', 'getbadge', 'method', 'integrate', 'report'] as const
  const titles = names.map((name) => seoFor({ name }, t).title)
  assert.equal(new Set(titles).size, titles.length)
})
