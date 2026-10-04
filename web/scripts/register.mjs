// Lets `node --experimental-strip-types` resolve extensionless relative imports.
import { register } from 'node:module'
register('./ts-resolve-hook.mjs', import.meta.url)
