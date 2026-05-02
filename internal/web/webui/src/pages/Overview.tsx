import { createEffect, createMemo, createResource, createSignal, For, onCleanup, Show } from "solid-js";
import { useNavigate } from "@solidjs/router";
import {
  Cable,
  Users as UsersIcon,
  ServerCog,
  ArrowDownUp,
  ArrowRight,
  Activity,
} from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import KpiCard from "../ui/KpiCard";
import { Card, CardHeader } from "../ui/Card";
import { Pill } from "../ui/Pill";
import Sparkline from "../ui/Sparkline";
import EmptyState from "../ui/EmptyState";
import { api } from "../api/client";
import { bytes, pct, relTime } from "../util/format";
import { ipKind, ipKindLabel, ipKindTone } from "../util/ip";
import { recordUserSnapshot, userSamples } from "../store/userTrafficHistory";
import type { XrayUser } from "../api/types";

const POLL_MS = 5000;
const TOP_N = 8;

export default function Overview() {
  const nav = useNavigate();
  const [config] = createResource(() => api.config());
  const [conns, { refetch: rcConns }] = createResource(() => api.xrayConns());
  const [users, { refetch: rcUsers }] = createResource(() => api.xrayUsers());
  const [node, { refetch: rcNode }] = createResource(() =>
    api.nodeMetrics({ latest: true }),
  );

  const [tick, setTick] = createSignal(0);
  const iv = window.setInterval(() => {
    if (document.visibilityState !== "visible") return;
    setTick((t) => t + 1);
    rcConns();
    rcUsers();
    rcNode();
  }, POLL_MS);
  onCleanup(() => window.clearInterval(iv));

  // Feed throughput samples so the Top Users sparkline has signal.
  createEffect(() => {
    const u = users();
    if (u) recordUserSnapshot(u);
  });

  const allUsers = () => users() ?? [];
  const allConns = () => conns() ?? [];
  const latestNode = () => node()?.data?.[node()!.data.length - 1];

  const totalUp = () => allUsers().reduce((a, u) => a + u.upload_total, 0);
  const totalDown = () => allUsers().reduce((a, u) => a + u.download_total, 0);
  const ruleCount = () =>
    Array.isArray(config()?.relay_configs) ? config()!.relay_configs!.length : 0;

  // Active in last ~5 min: any sparkline sample > 0.
  const activeUsers = () =>
    allUsers().filter((u) => userSamples(u.user_id).some((v) => v > 0)).length;

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
        title="Overview"
        subtitle="What's flowing right now."
        actions={
          <Pill tone="ok" dot pulse>
            live · {tick()}
          </Pill>
        }
      />

      <div class="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
        <KpiCard
          label="Active conns"
          icon={<Cable size={14} />}
          value={allConns().length}
          hint={`${activeUsers()} users w/ traffic`}
        />
        <KpiCard
          label="Known users"
          icon={<UsersIcon size={14} />}
          value={allUsers().length}
          hint={`${allUsers().filter((u) => u.enable).length} enabled`}
        />
        <KpiCard
          label="Relay rules"
          icon={<ServerCog size={14} />}
          value={ruleCount()}
          hint={
            config()
              ? `:${config()!.web_port} · ${config()!.log_level ?? "info"}`
              : ""
          }
        />
        <KpiCard
          label="Total xfer"
          icon={<ArrowDownUp size={14} />}
          value={bytes(totalUp() + totalDown())}
          hint={
            <span class="font-mono">
              ↑ {bytes(totalUp())} · ↓ {bytes(totalDown())}
            </span>
          }
        />
      </div>

      <div class="mt-4 grid grid-cols-1 gap-3 lg:grid-cols-2">
        <Card padded={false}>
          <div class="flex items-center justify-between gap-3 px-4 py-3 sm:px-5">
            <CardHeader title="Top users · last 5m" subtitle="By recent throughput" />
            <button
              class="inline-flex items-center gap-1 text-xs text-emerald-700 hover:underline dark:text-emerald-400"
              onClick={() => nav("/xray/users")}
            >
              all users <ArrowRight size={12} />
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
                      class="flex cursor-pointer items-center gap-3 px-4 py-2.5 hover:bg-emerald-50/40 sm:px-5 dark:hover:bg-emerald-500/5"
                      onClick={() => nav(`/xray/conns?user=${u.user_id}`)}
                    >
                      <span class="w-12 shrink-0 font-mono text-sm">
                        {u.user_id}
                      </span>
                      <Pill tone={u.running ? "ok" : "neutral"} dot pulse={u.running}>
                        {u.protocol || "—"}
                      </Pill>
                      <span class="ml-auto hidden text-emerald-600 sm:block dark:text-emerald-400">
                        <Sparkline values={samples()} width={80} height={20} />
                      </span>
                      <span class="w-20 shrink-0 text-right font-mono text-xs tabular-nums text-zinc-500">
                        {recentSum() > 0 ? bytes(recentSum()) : "—"}
                      </span>
                      <span class="w-10 shrink-0 text-right font-mono text-xs tabular-nums text-zinc-500">
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
          <div class="flex items-center justify-between gap-3 px-4 py-3 sm:px-5">
            <CardHeader title="Recent connections" subtitle="Newest first" />
            <button
              class="inline-flex items-center gap-1 text-xs text-emerald-700 hover:underline dark:text-emerald-400"
              onClick={() => nav("/xray/conns")}
            >
              all conns <ArrowRight size={12} />
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
                    class="flex cursor-pointer items-center gap-2 px-4 py-2 hover:bg-emerald-50/40 sm:px-5 dark:hover:bg-emerald-500/5"
                    onClick={() => nav(`/xray/conns?user=${c.user_id}`)}
                  >
                    <Pill tone={c.network === "tcp" ? "info" : "accent"}>
                      {c.network}
                    </Pill>
                    <span class="w-12 shrink-0 font-mono text-sm">
                      {c.user_id}
                    </span>
                    <Pill tone={ipKindTone[ipKind(c.source_ip)]}>
                      {ipKindLabel[ipKind(c.source_ip)]}
                    </Pill>
                    <span class="min-w-0 flex-1 truncate font-mono text-xs">
                      {c.target}
                    </span>
                    <span class="shrink-0 text-xs text-zinc-500">
                      {relTime(c.since)}
                    </span>
                  </li>
                )}
              </For>
            </ul>
          </Show>
        </Card>
      </div>

      <Show when={latestNode()}>
        <Card class="mt-4" padded={false}>
          <div class="flex items-center justify-between gap-3 px-4 py-3 sm:px-5">
            <CardHeader
              title="Host"
              subtitle="System resource snapshot"
            />
            <button
              class="inline-flex items-center gap-1 text-xs text-emerald-700 hover:underline dark:text-emerald-400"
              onClick={() => nav("/host")}
            >
              detail <ArrowRight size={12} />
            </button>
          </div>
          <div class="grid grid-cols-2 gap-3 px-4 pb-4 sm:grid-cols-4 sm:px-5 sm:pb-5">
            <HostStat label="CPU" value={pct(latestNode()!.cpu_usage)} />
            <HostStat label="Memory" value={pct(latestNode()!.memory_usage)} />
            <HostStat label="Net in" value={bytes(latestNode()!.network_in)} />
            <HostStat label="Net out" value={bytes(latestNode()!.network_out)} />
          </div>
        </Card>
      </Show>

      <Show when={!latestNode()}>
        <div class="mt-4 flex items-center gap-2 rounded-xl border border-zinc-200 bg-zinc-50 px-4 py-2 text-xs text-zinc-500 dark:border-zinc-800 dark:bg-zinc-950">
          <Activity size={13} /> Host metrics not reporting yet.
        </div>
      </Show>
    </>
  );
}

function HostStat(props: { label: string; value: string }) {
  return (
    <div>
      <div class="text-[11px] font-medium uppercase tracking-wider text-zinc-500">
        {props.label}
      </div>
      <div class="mt-1 font-mono text-lg font-semibold tabular-nums">
        {props.value}
      </div>
    </div>
  );
}
