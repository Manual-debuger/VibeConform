import assert from 'node:assert/strict'
import { test } from 'node:test'

import { greetAll } from './index.ts'

await test('greetAll greets the name of a greeting', () => {
  assert.equal(greetAll({ name: 'Ada' }), 'Hello, Ada!')
})
