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
export function ReportRing(props: { score: number | null; label: string }) {
  const radius = 64,
    length = 2 * Math.PI * radius
  return (
    <svg
      viewBox='0 0 180 180'
      role='img'
      aria-label={props.label}
      className='mc-ring'
    >
      <circle
        cx='90'
        cy='85'
        r={radius}
        fill='none'
        stroke='var(--sheet-line)'
        strokeWidth='8'
      />
      {props.score !== null && (
        <circle
          cx='90'
          cy='85'
          r={radius}
          fill='none'
          stroke='var(--sheet-accent)'
          strokeWidth='8'
          strokeLinecap='round'
          strokeDasharray={`${(length * props.score) / 100} ${length}`}
          transform='rotate(-90 90 85)'
        />
      )}
      <text x='90' y='90' textAnchor='middle' className='mc-ring-value'>
        {props.score ?? '—'}
      </text>
      <text x='90' y='112' textAnchor='middle' className='mc-svg-note'>
        / 100
      </text>
    </svg>
  )
}
export function ReportRadar(props: {
  dimensions: Array<{ id: string; label: string; score: number | null }>
  title: string
  partialLabel: string
}) {
  const dimensions = props.dimensions.map((dimension) => ({
    ...dimension,
    score:
      dimension.score !== null &&
      Number.isFinite(dimension.score) &&
      dimension.score >= 0 &&
      dimension.score <= 100
        ? dimension.score
        : null,
  }))
  const axisCount = dimensions.length || 5
  const point = (i: number, r: number) => ({
    x: 160 + r * Math.cos(-Math.PI / 2 + (i * 2 * Math.PI) / axisCount),
    y: 116 + r * Math.sin(-Math.PI / 2 + (i * 2 * Math.PI) / axisCount),
  })
  const polygon = (r: number) =>
    dimensions
      .map((_, i) => {
        const p = point(i, r)
        return `${p.x},${p.y}`
      })
      .join(' ')
  // Keep every measured vertex, including zero, in its original axis position.
  // Missing dimensions are omitted rather than projected to zero or full marks.
  const vertices = dimensions.flatMap((d, i) =>
    d.score === null ? [] : [{ index: i, ...point(i, d.score * 0.8) }]
  )
  const partial = vertices.length > 0 && vertices.length < dimensions.length
  const edgeCount =
    vertices.length > 2 ? vertices.length : Math.max(0, vertices.length - 1)
  const edges = vertices.slice(0, edgeCount).map((from, i) => {
    const to = vertices[(i + 1) % vertices.length]
    const gap = (to.index - from.index + axisCount) % axisCount
    return {
      from,
      to,
      missing:
        vertices.length === 2 ? Math.min(gap, axisCount - gap) > 1 : gap > 1,
    }
  })
  return (
    <div>
      <svg
        viewBox='0 0 320 235'
        role='img'
        aria-label={props.title}
        className='mc-radar'
      >
        {partial && <desc>{props.partialLabel}</desc>}
        {[20, 40, 60, 80].map((r) => (
          <polygon
            key={r}
            points={polygon(r)}
            fill='none'
            stroke='var(--sheet-line)'
          />
        ))}
        {dimensions.map((d, i) => {
          const end = point(i, 80)
          return (
            <line
              key={d.id}
              x1='160'
              y1='116'
              x2={end.x}
              y2={end.y}
              stroke='var(--sheet-line)'
            />
          )
        })}
        {vertices.length >= 3 && (
          <polygon
            data-radar-area='measured'
            points={vertices.map((v) => `${v.x},${v.y}`).join(' ')}
            fill='var(--sheet-tint)'
            fillOpacity='0.75'
          />
        )}
        {edges.map(({ from, to, missing }) => (
          <line
            key={from.index}
            data-radar-edge='measured'
            x1={from.x}
            y1={from.y}
            x2={to.x}
            y2={to.y}
            stroke='var(--sheet-accent)'
            strokeWidth='2'
            strokeDasharray={missing ? '4 4' : undefined}
          />
        ))}
        {dimensions.map((d, i) => {
          const label = point(i, 108),
            mark = point(i, (d.score ?? 0) * 0.8)
          return (
            <g key={d.id}>
              {d.score !== null && (
                <circle
                  data-radar-vertex={d.id}
                  cx={mark.x}
                  cy={mark.y}
                  r='4'
                  fill='var(--sheet-accent)'
                />
              )}
              <text
                x={label.x}
                y={label.y}
                textAnchor='middle'
                className='mc-svg-note'
              >
                {d.label}
                <tspan x={label.x} dy='15'>
                  {d.score ?? '—'}
                </tspan>
              </text>
            </g>
          )
        })}
      </svg>
      {partial && vertices.length >= 2 && (
        <p className='mc-muted'>{props.partialLabel}</p>
      )}
    </div>
  )
}
