type SparklineProps = { values: Array<number | null>; max?: number };

export function sparklinePoints(
  values: Array<number | null>,
  max: number,
  width: number,
  height: number,
): string {
  if (values.length < 2 || max <= 0) return '';
  const step = width / (values.length - 1);
  const points: string[] = [];
  values.forEach((value, index) => {
    if (value == null || !Number.isFinite(value)) return;
    const clamped = Math.max(0, Math.min(max, value));
    points.push(`${(index * step).toFixed(2)},${(height - (clamped / max) * height).toFixed(2)}`);
  });
  return points.length < 2 ? '' : points.join(' ');
}

export function Sparkline({ values, max }: SparklineProps) {
  const finite = values.filter((value): value is number => value != null && Number.isFinite(value));
  const scale = max ?? (finite.length ? Math.max(...finite) : 0);
  const points = sparklinePoints(values, scale || 1, 60, 16);
  if (!points) return null;
  return (
    <svg className="metric-sparkline" viewBox="0 0 60 16" preserveAspectRatio="none" aria-hidden="true" focusable="false">
      <polyline points={points} fill="none" stroke="currentColor" strokeWidth={1.2} vectorEffect="non-scaling-stroke" />
    </svg>
  );
}
