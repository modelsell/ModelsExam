import { pathToFileURL } from 'node:url'
import { existsSync } from 'node:fs'
export async function resolve(specifier, context, next) {
  if ((specifier.startsWith('./') || specifier.startsWith('../')) && !/\.[a-z]+$/.test(specifier)) {
    const base = new URL(specifier, context.parentURL).pathname
    for (const ext of ['.ts', '/index.ts']) if (existsSync(base + ext)) return next(pathToFileURL(base + ext).href, context)
  }
  return next(specifier, context)
}
