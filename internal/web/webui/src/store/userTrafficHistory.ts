import { createSignal } from "solid-js";
import type { XrayUser } from "../api/types";

/**
 * Module-scope rolling buffer of per-user throughput samples. Survives
 * route navigation within the same SPA session; resets on full reload.
 *
 * Each tick records (upload_total_now - upload_total_prev) plus
 * (download_total_now - download_total_prev) — i.e. bytes transferred
 * during the polling interval. Capped at MAX_SAMPLES so memory is
 * bounded.
 *
 * Note: server-side cumulative counters reset only on process restart.
 * If we observe a negative delta we treat it as a restart and reset our
 * baseline rather than emit a misleading downward spike.
 */
const MAX_SAMPLES = 60; // ~5 minutes at a 5s polling interval

interface Entry {
  lastUp: number;
  lastDown: number;
  samples: number[];
}

const state = new Map<number, Entry>();
const [version, setVersion] = createSignal(0);

export function recordUserSnapshot(users: XrayUser[]) {
  const seen = new Set<number>();
  for (const u of users) {
    seen.add(u.user_id);
    const prev = state.get(u.user_id);
    if (!prev) {
      state.set(u.user_id, {
        lastUp: u.upload_total,
        lastDown: u.download_total,
        samples: [],
      });
      continue;
    }
    const dUp = u.upload_total - prev.lastUp;
    const dDown = u.download_total - prev.lastDown;
    if (dUp < 0 || dDown < 0) {
      // counter regression → likely server restart; rebaseline silently
      prev.lastUp = u.upload_total;
      prev.lastDown = u.download_total;
      continue;
    }
    prev.lastUp = u.upload_total;
    prev.lastDown = u.download_total;
    prev.samples.push(dUp + dDown);
    if (prev.samples.length > MAX_SAMPLES) {
      prev.samples.splice(0, prev.samples.length - MAX_SAMPLES);
    }
  }
  // Drop entries for users we no longer see so memory doesn't grow
  // unbounded across long sessions with churning user pools.
  for (const id of state.keys()) {
    if (!seen.has(id)) state.delete(id);
  }
  setVersion((v) => v + 1);
}

export function userSamples(userId: number): number[] {
  // Reading version() inside the getter keeps Solid reactive subscribers
  // up to date when we tick.
  void version();
  return state.get(userId)?.samples ?? [];
}
