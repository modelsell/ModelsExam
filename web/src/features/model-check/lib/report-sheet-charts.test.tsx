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
import { renderToStaticMarkup } from 'react-dom/server'
import { ReportRadar } from '../components/report-sheet-charts'

const render = (scores: Array<number | null>) =>
  renderToStaticMarkup(
    <ReportRadar
      dimensions={scores.map((score, index) => ({
        id: String(index),
        label: `Dimension ${index}`,
        score,
      }))}
      title='Measured dimensions'
      partialLabel='Only scored vertices are connected. Dashed edges span unscored dimensions.'
    />
  )
const area = (html: string) =>
  html.match(/<polygon data-radar-area="measured"[^>]+>/)?.[0]
const count = (html: string, name: string) =>
  (html.match(new RegExp(name, 'g')) ?? []).length

test('the reported 4/5 radar fills its measured polygon without fabricating the missing prompt vertex', () => {
  const html = render([100, 100, null, 100, 67])
  assert.ok(area(html))
  assert.equal(
    area(html)!
      .match(/points="([^"]+)"/)![1]
      .split(' ').length,
    4
  )
  assert.equal(count(html, 'data-radar-edge='), 4)
  assert.equal(count(html, 'stroke-dasharray="4 4"'), 1)
  assert.equal(count(html, 'data-radar-vertex='), 4)
  assert.doesNotMatch(html, /data-radar-vertex="2"/)
  assert.match(html, /Dimension 2<tspan[^>]*>—<\/tspan>/)
  assert.match(html, /Dashed edges span unscored dimensions/)
})
test('complete radars keep all five vertices and zero remains a measured value', () => {
  const html = render([100, 100, 0, 100, 67])
  assert.equal(
    area(html)!
      .match(/points="([^"]+)"/)![1]
      .split(' ').length,
    5
  )
  assert.equal(count(html, 'data-radar-vertex='), 5)
  assert.match(html, /data-radar-vertex="2" cx="160" cy="116"/)
  assert.doesNotMatch(html, /stroke-dasharray/)
})
test('three vertices form an area, two form a line, and one or no vertices have no invented area', () => {
  assert.ok(area(render([100, null, 70, null, 80])))
  const two = render([100, null, 70, null, null])
  assert.equal(area(two), undefined)
  assert.equal(count(two, 'data-radar-edge='), 1)
  const one = render([100, null, null, null, null])
  assert.equal(area(one), undefined)
  assert.equal(count(one, 'data-radar-edge='), 0)
  const empty = render([null, null, null, null, null])
  assert.equal(area(empty), undefined)
  assert.equal(count(empty, 'data-radar-vertex='), 0)
})
test('invalid scores are unscored rather than invalid SVG coordinates', () => {
  const html = render([100, NaN, Infinity, -1, 101])
  assert.equal(count(html, 'data-radar-vertex='), 1)
  assert.doesNotMatch(html, /NaN|Infinity/)
})
