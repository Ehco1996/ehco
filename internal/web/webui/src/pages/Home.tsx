import { createEffect, createMemo, createResource, createSignal, For, Show } from "solid-js";
import { useNavigate } from "@solidjs/router";
import { Cable, Users as UsersIcon, ArrowRight, Activity } from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import { Card, CardHeader } from "../ui/Card";
import { Pill } from "../ui/Pill";
import Sparkline from "../ui/Sparkline";
import EmptyState from "../ui/EmptyState";
import Chart from "../ui/Chart";
import Segmented from "../ui/Segmented";
import RefreshPicker from "../ui/RefreshPicker";
import { api } from "../api/client";
import { bytes, pct, rate, relTime, pickStep } from "../util/format";
import { ipKind, ipKindLabel, ipKindTone } from "../util/ip";
import { usePolling } from "../util/polling";
import { recordUserSnapshot, userSamples } from "../store/userTrafficHistory";
import type { XrayUser } from "../api/types";

const TOP_N = 8;

// Single time-window selector drives every chart on the page. The
// upper bound is what the metrics-store retention permits (30d).
const WINDOWS = [
  { value: 5 * 60, label: "5m" },
  { value: 60 * 60, label: "1h" },
  { value: 6 * 60 * 60, label: "6h" },
  { value: 24 * 60 * 60, label: "24h" },
  { value: 7 * 24 * 60 * 60, label: "7d" },
  { value: 30 * 24 * 60 * 60, label: "30d" },
] as const;

export default function Home() {
  const nav = useNavigate();
  const [windowSec, setWindowSec] = createSignal<number>(WINDOWS[1].value);

  const [overview, { refetch: rcOverview }] = createResource(() => api.overview());
  const [users, { refetch: rcUsers }] = createResource(() => api.xrayUsers());
  const [conns, { refetch: rcConns }] = createResource(() => api.xrayConns());
  const [history, { refetch: rcHistory }] = createResource(
    windowSec,
    async (sec) => {
      const end = Math.floor(Date.now() / 1000);
      return api.nodeMetrics({
        start_ts: end - sec,
        end_ts: end,
        step: pickStep(sec),
      });
    },
  );

  const poll = usePolling(
    () => {
      rcOverview();
      rcUsers();
      rcConns();
      rcHistory();
    },
    { defaultSec: 15 },
  );
  void poll;

  createEffect(() => {
    const u = users();
    if (u) recordUserSnapshot(u);
  });

  const allUsers = () => users() ?? [];
  const allConns = () => conns() ?? [];
  // Charts want ascending time; the API returns DESC for LIMIT semantics.
  const series = () => [...(history()?.data ?? [])].sort((a, b) => a.timestamp - b.timestamp);

  const xray = () => overview()?.xray;
  const host = () => overview()?.host;

  const topUsers = createMemo<XrayUser[]>(() => {
    const recentBytes = (u: XrayUser) =>
      userSamples(u.user_id).reduce((a, b) => a + b, 0);
    return allUsers()
      .slice()
      .sort((a, b) => {
        const ra = recentBytes(a);
        const rb = recentBytes(b);
        if (rb !== ra) return rb - ra;
        return b.upload_total + b.download_total - (a.upload_total + a.download_total);
      })
      .slice(0, TOP_N);
  });

  const recentConns = createMemo(() =>
    allConns()
      .slice()
      .sort((a, b) => (a.since < b.since ? 1 : -1))
      .slice(0, TOP_N),
  );

  return (
    <>
      <PageHeader
        title="status"
        subtitle="live state of this relay box"
        actions={
          <>
            <Segmented
              options={WINDOWS.map((w) => ({ value: w.value, label: w.label }))}
              value={windowSec()}
              onChange={setWindowSec}
              size="sm"
            />
            <RefreshPicker handle={poll} />
          </>
        }
      />

      {/* ===== Anchor: live throughput strip ===== */}
      <ThroughputAnchor
        rateIn={overview()?.net_rate_in ?? 0}
        rateOut={overview()?.net_rate_out ?? 0}
        conns={xray()?.conns ?? allConns().length}
        users={xray()?.running_users ?? 0}
        rules={overview()?.rules ?? 0}
        cpu={host()?.cpu_usage}
        mem={host()?.memory_usage}
      />

      {/* ===== Throughput chart — the visual anchor ===== */}
      <Card class="mt-3" padded={false}>
        <div class="flex items-center justify-between gap-3 border-b border-zinc-200 px-4 py-2.5 dark:border-zinc-800">
          <CardHeader title="throughput" subtitle="host network rate (in / out)" />
          <span class="font-mono text-[11px] text-zinc-500">
            {series().length} pts · step{" "}
            {pickStep(windowSec()) === 0 ? "raw" : `${pickStep(windowSec())}s`}
          </span>
        </div>
        <Show
          when={series().length > 1}
          fallback={
            <div class="px-4 py-8 text-center text-xs text-zinc-500">
              <Activity size={20} class="mx-auto mb-2" />
              collecting samples…
            </div>
          }
        >
          <div class="px-2 pb-2 pt-3">
            <Chart
              height={180}
              timestamps={series().map((d) => d.timestamp)}
              series={[
                {
                  label: "in",
                  stroke: "#10b981",
                  values: series().map((d) => d.network_in),
                },
                {
                  label: "out",
                  stroke: "#f97316",
                  values: series().map((d) => d.network_out),
                },
              ]}
              yFormat={(v) => `${bytes(v)}/s`}
            />
          </div>
        </Show>
      </Card>

      {/* ===== Top users / Recent conns ===== */}
      <div class="mt-3 grid grid-cols-1 gap-3 lg:grid-cols-2">
        <Card padded={false}>
          <div class="flex items-center justify-between gap-3 border-b border-zinc-200 px-4 py-2.5 dark:border-zinc-800">
            <CardHeader title="top users" subtitle="by recent throughput · last 5m" />
            <button
              class="inline-flex items-center gap-1 text-xs text-emerald-700 hover:underline dark:text-emerald-400"
              onClick={() => nav("/users")}
            >
              all <ArrowRight size={12} />
            </button>
          </div>
          <Show
            when={topUsers().length}
            fallback={
              <EmptyState
                icon={<UsersIcon size={24} />}
                title="No users registered"
              />
            }
          >
            <ul class="divide-y divide-zinc-100 dark:divide-zinc-800/70">
              <For each={topUsers()}>
                {(u) => {
                  const samples = () => userSamples(u.user_id);
                  const recentSum = () => samples().reduce((a, b) => a + b, 0);
                  return (
                    <li
                      class="flex cursor-pointer items-center gap-3 px-4 py-2 hover:bg-emerald-500/5"
                      onClick={() => nav(`/conns?user=${u.user_id}`)}
                    >
                      <span class="w-12 shrink-0 font-mono text-[13px]">
                        {u.user_id}
                      </span>
                      <Pill tone={u.running ? "ok" : "neutral"} dot pulse={u.running}>
                        {u.protocol || "—"}
                      </Pill>
                      <span class="ml-auto hidden text-emerald-600 sm:block dark:text-emerald-400">
                        <Sparkline values={samples()} width={80} height={20} />
                      </span>
                      <span class="w-20 shrink-0 text-right font-mono text-[11px] tabular-nums text-zinc-500">
                        {recentSum() > 0 ? bytes(recentSum()) : "—"}
                      </span>
                      <span class="w-10 shrink-0 text-right font-mono text-[11px] tabular-nums text-zinc-500">
                        {u.tcp_conn_count}c
                      </span>
                    </li>
                  );
                }}
              </For>
            </ul>
          </Show>
        </Card>

        <Card padded={false}>
          <div class="flex items-center justify-between gap-3 border-b border-zinc-200 px-4 py-2.5 dark:border-zinc-800">
            <CardHeader title="recent conns" subtitle="newest first" />
            <button
              class="inline-flex items-center gap-1 text-xs text-emerald-700 hover:underline dark:text-emerald-400"
              onClick={() => nav("/conns")}
            >
              all <ArrowRight size={12} />
            </button>
          </div>
          <Show
            when={recentConns().length}
            fallback={
              <EmptyState
                icon={<Cable size={24} />}
                title="No live connections"
              />
            }
          >
            <ul class="divide-y divide-zinc-100 dark:divide-zinc-800/70">
              <For each={recentConns()}>
                {(c) => (
                  <li
                    class="flex cursor-pointer items-center gap-2 px-4 py-2 hover:bg-emerald-500/5"
                    onClick={() => nav(`/conns?user=${c.user_id}`)}
                  >
                    <Pill tone={c.network === "tcp" ? "info" : "accent"}>
                      {c.network}
                    </Pill>
                    <span class="w-10 shrink-0 font-mono text-[13px]">
                      {c.user_id}
                    </span>
                    <Pill tone={ipKindTone[ipKind(c.source_ip)]}>
                      {ipKindLabel[ipKind(c.source_ip)]}
                    </Pill>
                    <span class="min-w-0 flex-1 truncate font-mono text-[11px]">
                      {c.target}
                    </span>
                    <span class="shrink-0 text-[11px] text-zinc-500">
                      {relTime(c.since)}
                    </span>
                  </li>
                )}
              </For>
            </ul>
          </Show>
        </Card>
      </div>

      {/* ===== Host charts (CPU/Mem + Disk) — embedded from old Node page ===== */}
      <Show when={series().length > 1}>
        <div class="mt-3 grid grid-cols-1 gap-3 lg:grid-cols-2">
          <Card padded={false}>
            <div class="border-b border-zinc-200 px-4 py-2.5 dark:border-zinc-800">
              <CardHeader title="cpu / memory" />
            </div>
            <div class="px-2 py-3">
              <Chart
                height={150}
                timestamps={series().map((d) => d.timestamp)}
                series={[
                  { label: "cpu", stroke: "#10b981", values: series().map((d) => d.cpu_usage) },
                  { label: "mem", stroke: "#6366f1", values: series().map((d) => d.memory_usage) },
                ]}
                yFormat={(v) => pct(v)}
              />
            </div>
          </Card>
          <Card padded={false}>
            <div class="border-b border-zinc-200 px-4 py-2.5 dark:border-zinc-800">
              <CardHeader title="disk" />
            </div>
            <div class="px-2 py-3">
              <Chart
                height={150}
                timestamps={series().map((d) => d.timestamp)}
                series={[
                  { label: "disk", stroke: "#f59e0b", values: series().map((d) => d.disk_usage) },
                ]}
                yFormat={(v) => pct(v)}
              />
            </div>
          </Card>
        </div>
      </Show>
    </>
  );
}

function ThroughputAnchor(props: {
  rateIn: number;
  rateOut: number;
  conns: number;
  users: number;
  rules: number;
  cpu?: number;
  mem?: number;
}) {
  return (
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
      {/* Rate panel — the page's main visual anchor */}
      <Card padded={false}>
        <div class="px-5 py-4">
          <div class="text-[10px] font-semibold uppercase tracking-[0.18em] text-zinc-500">
            now · rate
          </div>
          <div class="mt-1 flex items-baseline gap-5">
            <div>
              <span class="text-emerald-500 dark:text-emerald-400">↓</span>
              <span class="ml-1 font-mono text-[28px] font-semibold tabular-nums tracking-tight sm:text-[32px]">
                {rate(props.rateIn)}
              </span>
            </div>
            <div>
              <span class="text-orange-500">↑</span>
              <span class="ml-1 font-mono text-[28px] font-semibold tabular-nums tracking-tight sm:text-[32px]">
                {rate(props.rateOut)}
              </span>
            </div>
          </div>
          <div class="mt-1 text-[11px] text-zinc-500">host nic · 5s sample</div>
        </div>
      </Card>

      {/* Side stats panel */}
      <Card padded={false}>
        <div class="grid grid-cols-2 divide-x divide-zinc-200 sm:grid-cols-4 dark:divide-zinc-800">
          <Stat label="conns" value={props.conns} />
          <Stat label="users" value={props.users} hint="running" />
          <Stat label="cpu" value={props.cpu != null ? pct(props.cpu) : "—"} />
          <Stat label="mem" value={props.mem != null ? pct(props.mem) : "—"} />
        </div>
      </Card>
    </div>
  );
}

function Stat(props: { label: string; value: string | number; hint?: string }) {
  return (
    <div class="px-4 py-4 sm:py-3.5">
      <div class="text-[10px] font-semibold uppercase tracking-[0.16em] text-zinc-500">
        {props.label}
        {props.hint && (
          <span class="ml-1 normal-case tracking-normal text-zinc-400">
            · {props.hint}
          </span>
        )}
      </div>
      <div class="mt-1 font-mono text-[18px] font-semibold tabular-nums tracking-tight">
        {props.value}
      </div>
    </div>
  );
}
