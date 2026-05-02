import { createSignal, For, onCleanup, onMount, Show } from "solid-js";
import { Pause, Play, Trash2, ArrowDown, ScrollText } from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import Toolbar from "../ui/Toolbar";
import Button from "../ui/Button";
import { Input, Select } from "../ui/Input";
import { Pill } from "../ui/Pill";
import EmptyState from "../ui/EmptyState";
import { connectLogs } from "../api/ws";
import type { LogFrame } from "../api/types";

const BUFFER_CAP = 5000;

const levelTone: Record<
  string,
  "info" | "warn" | "error" | "neutral" | "ok"
> = {
  debug: "neutral",
  info: "info",
  warn: "warn",
  warning: "warn",
  error: "error",
  dpanic: "error",
  panic: "error",
  fatal: "error",
};

export default function Logs() {
  const [frames, setFrames] = createSignal<LogFrame[]>([]);
  const [status, setStatus] = createSignal<"open" | "closed" | "error">(
    "closed",
  );
  const [filter, setFilter] = createSignal("");
  const [levelMin, setLevelMin] = createSignal("");
  const [paused, setPaused] = createSignal(false);
  const [tail, setTail] = createSignal(true);
  let pane!: HTMLDivElement;

  onMount(() => {
    const handle = connectLogs(
      (raw) => {
        if (paused()) return;
        const f = raw as LogFrame;
        setFrames((cur) =>
          cur.length >= BUFFER_CAP
            ? [...cur.slice(cur.length - BUFFER_CAP + 1), f]
            : [...cur, f],
        );
        if (tail()) {
          queueMicrotask(() => {
            pane.scrollTop = pane.scrollHeight;
          });
        }
      },
      (s) => setStatus(s),
    );
    onCleanup(() => handle.close());
  });

  const filtered = () => {
    const f = filter().trim().toLowerCase();
    const lvl = levelMin().toLowerCase();
    return frames().filter((x) => {
      if (lvl && (x.level ?? "").toLowerCase() !== lvl) return false;
      if (!f) return true;
      const blob =
        `${x.msg ?? ""} ${x.logger ?? ""} ${x.caller ?? ""}`.toLowerCase();
      return blob.includes(f);
    });
  };

  return (
    <>
      <PageHeader
        title="Logs"
        subtitle="Live tail of the in-process zap logger via /ws/logs."
        actions={
          <Pill
            tone={
              status() === "open"
                ? "ok"
                : status() === "error"
                  ? "error"
                  : "neutral"
            }
            dot
            pulse={status() === "open"}
          >
            {status() === "open"
              ? "connected"
              : status() === "error"
                ? "error"
                : "disconnected"}
          </Pill>
        }
      />

      <Toolbar>
        <Input
          class="min-w-[160px] flex-1"
          placeholder="search msg / logger / caller"
          value={filter()}
          onInput={(e) => setFilter(e.currentTarget.value)}
        />
        <Select
          value={levelMin()}
          onChange={(e) => setLevelMin(e.currentTarget.value)}
        >
          <option value="">all levels</option>
          <option value="debug">debug</option>
          <option value="info">info</option>
          <option value="warn">warn</option>
          <option value="error">error</option>
        </Select>
        <Button
          size="sm"
          leadingIcon={paused() ? <Play size={13} /> : <Pause size={13} />}
          onClick={() => setPaused(!paused())}
        >
          {paused() ? "Resume" : "Pause"}
        </Button>
        <Button
          size="sm"
          variant={tail() ? "primary" : "secondary"}
          leadingIcon={<ArrowDown size={13} />}
          onClick={() => setTail(!tail())}
        >
          Tail
        </Button>
        <Button
          size="sm"
          leadingIcon={<Trash2 size={13} />}
          onClick={() => setFrames([])}
        >
          Clear
        </Button>
        <span class="ml-auto text-xs text-zinc-500">
          {filtered().length} / {frames().length}
        </span>
      </Toolbar>

      <div
        ref={pane}
        class="scroll-pretty h-[calc(100vh-280px)] min-h-[300px] overflow-y-auto rounded-xl border border-zinc-200 bg-zinc-50 p-2 font-mono text-[11px] leading-relaxed sm:text-xs md:h-[calc(100vh-260px)] dark:border-zinc-800 dark:bg-zinc-950"
      >
        <Show
          when={filtered().length}
          fallback={
            <div class="flex h-full items-center justify-center">
              <EmptyState
                icon={<ScrollText size={28} />}
                title={
                  frames().length === 0 ? "Waiting for logs…" : "No matches"
                }
                hint={
                  frames().length === 0
                    ? "Logs will appear here as they're emitted."
                    : "Try clearing the search or level filter."
                }
              />
            </div>
          }
        >
          <For each={filtered()}>
            {(l) => (
              <div class="flex gap-2 px-1.5 py-0.5 hover:bg-white dark:hover:bg-zinc-900">
                <span class="hidden shrink-0 text-zinc-500 sm:inline">
                  {(l.ts ?? "").replace(/T/, " ").slice(0, 19)}
                </span>
                <span class="w-12 shrink-0">
                  <Pill tone={levelTone[(l.level ?? "").toLowerCase()] ?? "neutral"}>
                    {l.level ?? "?"}
                  </Pill>
                </span>
                <span class="shrink-0 text-emerald-700 dark:text-emerald-400">
                  {l.logger ?? ""}
                </span>
                <span class="break-all">{l.msg}</span>
              </div>
            )}
          </For>
        </Show>
      </div>
    </>
  );
}
