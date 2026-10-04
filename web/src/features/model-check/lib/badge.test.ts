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
import { badgeTier } from './badge'

test('badge tiers follow the score of completed runs', () => {
  assert.equal(badgeTier('completed', 100), 'conformant')
  assert.equal(badgeTier('completed', 99), 'mostly')
  assert.equal(badgeTier('completed', 80), 'mostly')
  assert.equal(badgeTier('completed', 79), 'review')
  assert.equal(badgeTier('failed', 0), 'review')
  assert.equal(badgeTier('completed', null), 'unscored')
})

test('unfinished runs never get a verdict, whatever their score', () => {
  for (const status of ['stopped', 'cancelled', 'interrupted'] as const)
    assert.equal(badgeTier(status, 100), 'incomplete')
  assert.equal(badgeTier('running', 100), 'running')
})
