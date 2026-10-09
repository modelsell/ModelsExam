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
import { parseRoute, reportPath, routePath } from './route'

test('model boards are gone with the public records', () => {
  assert.deepEqual(parseRoute('/models'), { name: 'notfound' })
  assert.deepEqual(parseRoute('/models/claude-sonnet-4-5'), { name: 'notfound' })
  assert.deepEqual(parseRoute('/guides'), { name: 'notfound' })
  assert.deepEqual(parseRoute('/sites'), { name: 'notfound' })
})

test('parses the known pages', () => {
  assert.deepEqual(parseRoute('/'), { name: 'home' })
  assert.deepEqual(parseRoute('/records/'), { name: 'records' })
  assert.deepEqual(parseRoute('/baselines'), { name: 'baselines' })
  assert.deepEqual(parseRoute('/sponsors'), { name: 'notfound' })
  assert.deepEqual(parseRoute('/method'), { name: 'method' })
  assert.deepEqual(parseRoute('/integrate'), { name: 'integrate' })
  assert.equal(routePath({ name: 'integrate' }), '/integrate')
})

test('parses the account pages', () => {
  for (const name of ['login', 'register', 'account', 'keys', 'schedules'] as const) {
    assert.deepEqual(parseRoute(`/${name}`), { name })
    assert.equal(routePath({ name }), `/${name}`)
  }
})

test('parses a report path and round-trips it', () => {
  const id = '3f2b9c1e-0000-4000-8000-000000000001'
  assert.deepEqual(parseRoute(reportPath(id)), { name: 'report', id })
  assert.equal(routePath({ name: 'report', id }), `/reports/${id}`)
})

test('parses the badge pages', () => {
  assert.deepEqual(parseRoute('/get-badge'), { name: 'getbadge' })
  assert.deepEqual(parseRoute('/sites/Relay.Example.com'), { name: 'site', id: 'relay.example.com' })
  assert.deepEqual(parseRoute('/sites/a/b'), { name: 'notfound' })
})

test('legacy ?history_id links still open the report', () => {
  assert.deepEqual(parseRoute('/', '?history_id=abc'), { name: 'report', id: 'abc' })
})

test('unknown paths and bad encodings are not found', () => {
  assert.deepEqual(parseRoute('/nope'), { name: 'notfound' })
  assert.deepEqual(parseRoute('/reports/%E0%A4%A'), { name: 'notfound' })
  assert.deepEqual(parseRoute('/reports/a/b'), { name: 'notfound' })
})
