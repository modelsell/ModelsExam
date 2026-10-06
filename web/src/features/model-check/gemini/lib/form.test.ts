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
import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { geminiCheckSchema, geminiRequestBudget } from './form'

const valid = {
  base_url: 'https://generativelanguage.googleapis.com',
  key: 'AIza-test',
  model: 'gemini-2.5-flash',
  suite: 'standard' as const,
  vision: false,
}

describe('gemini check form', () => {
  it('matches the server request bounds', () => {
    assert.equal(geminiRequestBudget({ suite: 'basic', vision: false }), 4)
    assert.equal(geminiRequestBudget({ suite: 'standard', vision: false }), 16)
    assert.equal(geminiRequestBudget({ suite: 'full', vision: true }), 22)
  })
  it('accepts model ids and refuses path-like names', () => {
    assert.ok(geminiCheckSchema.safeParse(valid).success)
    assert.ok(
      geminiCheckSchema.safeParse({ ...valid, model: 'models/gemini-2.5-pro' })
        .success
    )
    for (const model of ['../x', 'a/b', 'gemini:generateContent', '..'])
      assert.ok(!geminiCheckSchema.safeParse({ ...valid, model }).success, model)
  })
  it('refuses a key in the URL query', () => {
    assert.ok(
      !geminiCheckSchema.safeParse({
        ...valid,
        base_url: 'https://generativelanguage.googleapis.com/v1beta?key=x',
      }).success
    )
  })
})
