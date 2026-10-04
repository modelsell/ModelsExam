import path from 'node:path'
import { defineConfig } from '@rsbuild/core'
import { pluginReact } from '@rsbuild/plugin-react'

const apiTarget = process.env.API_URL ?? 'http://localhost:8080'
// Dev server port. Deliberately not 3000 (commonly taken); override with WEB_PORT.
const webPort = Number(process.env.WEB_PORT ?? 5178)

export default defineConfig({
  plugins: [pluginReact()],
  source: { entry: { index: './src/main.tsx' } },
  resolve: { alias: { '@': path.resolve(__dirname, './src') } },
  html: { template: './index.html' },
  server: { host: '0.0.0.0', port: webPort, strictPort: true, proxy: { '/api': { target: apiTarget, changeOrigin: true } } },
  output: { target: 'web', distPath: { root: 'dist' } },
})
