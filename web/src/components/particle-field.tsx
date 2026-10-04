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
import { useEffect, useRef } from 'react'

// Decorative background: slow drifting points joined by faint lines when close,
// nudged away from the pointer. It is purely visual (aria-hidden), stops when
// off screen or in a background tab, and draws one still frame when the visitor
// prefers reduced motion.
export function ParticleField(props: { className?: string }) {
  const ref = useRef<HTMLCanvasElement>(null)
  useEffect(() => {
    const canvas = ref.current
    const ctx = canvas?.getContext('2d')
    if (!canvas || !ctx) return
    const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    type P = { x: number; y: number; vx: number; vy: number; r: number }
    let w = 0
    let h = 0
    let dpr = 1
    let pts: P[] = []
    let raf = 0
    let visible = true
    const mouse = { x: -9999, y: -9999 }
    const LINK = 130

    const resize = () => {
      const box = canvas.getBoundingClientRect()
      dpr = Math.min(window.devicePixelRatio || 1, 2)
      w = box.width
      h = box.height
      canvas.width = Math.round(w * dpr)
      canvas.height = Math.round(h * dpr)
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      const n = Math.max(24, Math.min(90, Math.round((w * h) / 14000)))
      pts = Array.from({ length: n }, () => ({
        x: Math.random() * w,
        y: Math.random() * h,
        vx: (Math.random() - 0.5) * 0.25,
        vy: (Math.random() - 0.5) * 0.25,
        r: Math.random() * 1.2 + 0.6,
      }))
    }

    const frame = () => {
      ctx.clearRect(0, 0, w, h)
      const dark = document.documentElement.classList.contains('dark')
      const line = dark ? '110,160,255' : '36,87,197'
      const dot = dark ? 'rgba(180,205,255,0.75)' : 'rgba(36,87,197,0.6)'
      for (const p of pts) {
        if (!still) {
          const dx = p.x - mouse.x
          const dy = p.y - mouse.y
          const d2 = dx * dx + dy * dy
          if (d2 < 140 * 140 && d2 > 1) {
            const f = (1 - Math.sqrt(d2) / 140) * 0.35
            p.vx += (dx / Math.sqrt(d2)) * f * 0.1
            p.vy += (dy / Math.sqrt(d2)) * f * 0.1
          }
          p.vx *= 0.995
          p.vy *= 0.995
          p.x += p.vx
          p.y += p.vy
          if (p.x < 0 || p.x > w) p.vx *= -1
          if (p.y < 0 || p.y > h) p.vy *= -1
        }
      }
      for (let i = 0; i < pts.length; i++) {
        const a = pts[i]
        for (let j = i + 1; j < pts.length; j++) {
          const b = pts[j]
          const d = Math.hypot(a.x - b.x, a.y - b.y)
          if (d < LINK) {
            ctx.strokeStyle = `rgba(${line},${(1 - d / LINK) * (dark ? 0.28 : 0.22)})`
            ctx.lineWidth = 0.6
            ctx.beginPath()
            ctx.moveTo(a.x, a.y)
            ctx.lineTo(b.x, b.y)
            ctx.stroke()
          }
        }
        ctx.fillStyle = dot
        ctx.beginPath()
        ctx.arc(a.x, a.y, a.r, 0, Math.PI * 2)
        ctx.fill()
      }
      if (!still && visible && !document.hidden) raf = requestAnimationFrame(frame)
      else raf = 0
    }
    const start = () => {
      if (!raf && !still) raf = requestAnimationFrame(frame)
    }
    const onMove = (e: PointerEvent) => {
      const box = canvas.getBoundingClientRect()
      mouse.x = e.clientX - box.left
      mouse.y = e.clientY - box.top
    }
    const onLeave = () => {
      mouse.x = mouse.y = -9999
    }
    resize()
    frame()
    const ro = new ResizeObserver(() => {
      resize()
      if (still) frame()
    })
    ro.observe(canvas)
    const io = new IntersectionObserver(([e]) => {
      visible = e.isIntersecting
      if (visible) start()
    })
    io.observe(canvas)
    const onVis = () => !document.hidden && start()
    document.addEventListener('visibilitychange', onVis)
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerleave', onLeave)
    return () => {
      cancelAnimationFrame(raf)
      ro.disconnect()
      io.disconnect()
      document.removeEventListener('visibilitychange', onVis)
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerleave', onLeave)
    }
  }, [])
  return (
    <canvas
      ref={ref}
      aria-hidden='true'
      className={props.className ?? 'pointer-events-none absolute inset-0 size-full'}
    />
  )
}
