/**
 * Inline-SVG sparkline. No deps, no library — small enough that the
 * cost of any chart lib would dwarf the feature.
 *
 * Designed for embedding inside a table cell or card: caller controls
 * width/height. Empty / single-value series renders a flat line so the
 * UI doesn't shift when data lags.
 */
export default function Sparkline(props: {
  values: number[];
  width?: number;
  height?: number;
  stroke?: string;
  fill?: string;
}) {
  const w = props.width ?? 90;
  const h = props.height ?? 22;
  const stroke = props.stroke ?? "currentColor";
  const fill = props.fill ?? "none";

  const vs = props.values ?? [];
  if (vs.length < 2) {
    return (
      <svg viewBox={`0 0 ${w} ${h}`} width={w} height={h} class="block">
        <line
          x1="0"
          y1={h - 1}
          x2={w}
          y2={h - 1}
          stroke={stroke}
          stroke-width="1"
          opacity="0.3"
        />
      </svg>
    );
  }

  const max = Math.max(...vs);
  const min = Math.min(...vs);
  const range = max - min || 1;
  const step = w / (vs.length - 1);

  const pts = vs.map((v, i) => {
    const x = i * step;
    const y = h - 1 - ((v - min) / range) * (h - 2);
    return [x, y] as const;
  });

  const linePath = pts
    .map(([x, y], i) => `${i === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)}`)
    .join(" ");

  // optional fill underneath the line
  const areaPath =
    fill !== "none"
      ? `${linePath} L${(w).toFixed(1)},${h} L0,${h} Z`
      : null;

  return (
    <svg
      viewBox={`0 0 ${w} ${h}`}
      width={w}
      height={h}
      class="block overflow-visible"
    >
      {areaPath && <path d={areaPath} fill={fill} opacity="0.18" />}
      <path
        d={linePath}
        fill="none"
        stroke={stroke}
        stroke-width="1.5"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
    </svg>
  );
}
