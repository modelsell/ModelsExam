import assert from 'node:assert/strict'
import { test } from 'node:test'
import { passwordProblems, USERNAME_PATTERN } from './password'

test('password shape rules match the server', () => {
  assert.deepEqual(passwordProblems('bob', 'Tr4in-Cactus-Lamp'), [])
  assert.ok(passwordProblems('bob', 'Short1!').includes('Password must be at least 10 characters'))
  assert.ok(passwordProblems('bob', 'onlylowercaseletters').some((p) => p.startsWith('Password must use at least 3')))
  assert.ok(passwordProblems('alice', 'Alice-Rocks-2026').includes('Password must not contain the username'))
})

test('usernames', () => {
  assert.ok(USERNAME_PATTERN.test('Alice_01'))
  assert.ok(!USERNAME_PATTERN.test('ab'))
  assert.ok(!USERNAME_PATTERN.test('has space'))
})
