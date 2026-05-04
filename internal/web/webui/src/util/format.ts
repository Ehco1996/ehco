export function bytes(n: number | undefined | null): string {
  if (!n || n < 0) return "0 B";
  const u = ["B", "KB", "MB", "GB", "TB", "PB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${u[i]}`;
}

export function relTime(iso: string): string {
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return "—";
  const diff = Math.max(0, (Date.now() - t) / 1000);
  if (diff < 60) return `${Math.floor(diff)}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

export function pct(n: number | undefined): string {
  if (n == null || !Number.isFinite(n)) return "—";
  return `${n.toFixed(1)}%`;
}

// rate formats a bytes-per-second number as a compact "12.4 MB/s".
export function rate(bps: number | undefined | null): string {
  if (!bps || bps < 0) return "0 B/s";
  return `${bytes(bps)}/s`;
}

// pickStep returns a server-side bucket size (in seconds) for a given
// time window. Aim is ≤ ~360 points per chart, regardless of window —
// keeps payload small and uPlot snappy at 30d zoom levels.
export function pickStep(windowSec: number): number {
  if (windowSec <= 5 * 60) return 0;        // ≤5m: raw 5s samples
  if (windowSec <= 60 * 60) return 30;      // 1h: 30s
  if (windowSec <= 6 * 60 * 60) return 120; // 6h: 2m
  if (windowSec <= 24 * 60 * 60) return 600; // 24h: 10m
  if (windowSec <= 7 * 24 * 60 * 60) return 3600;     // 7d: 1h
  return 4 * 3600;                                     // 30d: 4h
}
