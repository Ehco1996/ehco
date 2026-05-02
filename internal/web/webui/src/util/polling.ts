import { createSignal, onCleanup } from "solid-js";

export interface PollingHandle {
  /** Current interval in seconds; 0 means paused. */
  interval: () => number;
  setInterval: (s: number) => void;
  /** Tick counter for "live · N" indicator. */
  tick: () => number;
  /** Force one immediate run (also resets the timer). */
  trigger: () => void;
}

const STORAGE_KEY = "ehco.refreshInterval";

/**
 * Drives a periodic callback with a user-adjustable interval. Skipped
 * while the tab is hidden so background tabs don't burn cycles.
 *
 * `defaultSec` is used the first time the user visits; afterwards the
 * picked interval is shared across pages via localStorage.
 */
export function usePolling(
  callback: () => void,
  opts: { defaultSec: number; minSec?: number },
): PollingHandle {
  const min = opts.minSec ?? 2;
  const stored = (() => {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      const n = raw == null ? NaN : Number(raw);
      return Number.isFinite(n) && (n === 0 || n >= min) ? n : null;
    } catch {
      return null;
    }
  })();
  const [interval, setIntervalSig] = createSignal(stored ?? opts.defaultSec);
  const [tick, setTick] = createSignal(0);
  let timer: number | null = null;

  const arm = () => {
    if (timer != null) {
      window.clearInterval(timer);
      timer = null;
    }
    const sec = interval();
    if (sec <= 0) return;
    timer = window.setInterval(() => {
      if (document.visibilityState !== "visible") return;
      setTick((t) => t + 1);
      callback();
    }, sec * 1000);
  };

  arm();
  onCleanup(() => {
    if (timer != null) window.clearInterval(timer);
  });

  return {
    interval,
    setInterval: (s: number) => {
      setIntervalSig(s);
      try {
        localStorage.setItem(STORAGE_KEY, String(s));
      } catch {
        /* ignore */
      }
      arm();
    },
    tick,
    trigger: () => {
      setTick((t) => t + 1);
      callback();
      arm();
    },
  };
}

export const REFRESH_OPTIONS: { value: number; label: string }[] = [
  { value: 0, label: "Off" },
  { value: 5, label: "5s" },
  { value: 15, label: "15s" },
  { value: 30, label: "30s" },
  { value: 60, label: "1m" },
];
