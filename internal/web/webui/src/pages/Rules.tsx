import { createMemo, createResource, createSignal, For, Show } from "solid-js";
import { RefreshCcw, ServerCog, Heart } from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import Button from "../ui/Button";
import { Pill } from "../ui/Pill";
import { TableScroll, tableClasses as t } from "../ui/Table";
import EmptyState from "../ui/EmptyState";
import { Card } from "../ui/Card";
import Sparkline from "../ui/Sparkline";
import { api, ApiError } from "../api/client";
import { bytes } from "../util/format";
import type { RelayConfig, RuleMetric } from "../api/types";

const HISTORY_SECONDS = 60 * 60; // 1h sparkline window

interface HCResult {
  state: "running" | "ok" | "err";
  text: string;
}

export default function Rules() {
  const [config] = createResource(() => api.config());
  const [latest, { refetch: rcLatest }] = createResource(() =>
    api.ruleMetrics({ latest: true }),
  );
  const [history, { refetch: rcHistory }] = createResource(async () => {
    const end = Math.floor(Date.now() / 1000);
    return api.ruleMetrics({ start_ts: end - HISTORY_SECONDS, end_ts: end });
  });
  const [hc, setHc] = createSignal<Record<string, HCResult>>({});

  const ruleList = (): RelayConfig[] => {
    const c = config()?.relay_configs;
    return Array.isArray(c) ? c : [];
  };

  const latestFor = (label: string, remote: string) =>
    (latest()?.data ?? []).find(
      (m) => m.label === label && m.remote === remote,
    );

  // Group history points by (label, remote) and compute deltas of the
  // cumulative TCP byte counter — what we want to plot is throughput per
  // sample interval, not the monotonically-increasing total.
  const historyByKey = createMemo(() => {
    const out = new Map<string, number[]>();
    const points = history()?.data ?? [];
    if (!points.length) return out;
    const grouped = new Map<string, RuleMetric[]>();
    for (const p of points) {
      const k = `${p.label}|${p.remote}`;
      (grouped.get(k) ?? grouped.set(k, []).get(k)!).push(p);
    }
    for (const [k, arr] of grouped) {
      arr.sort((a, b) => a.timestamp - b.timestamp);
      const deltas: number[] = [];
      for (let i = 1; i < arr.length; i++) {
        const d =
          arr[i].tcp_network_transmit_bytes -
          arr[i - 1].tcp_network_transmit_bytes;
        deltas.push(Math.max(0, d));
      }
      out.set(k, deltas);
    }
    return out;
  });

  const seriesFor = (label: string, remote: string) =>
    historyByKey().get(`${label}|${remote}`) ?? [];

  const refreshAll = () => {
    rcLatest();
    rcHistory();
  };

  const checkOne = async (label: string) => {
    if (!label) return;
    setHc({ ...hc(), [label]: { state: "running", text: "checking…" } });
    try {
      const r = await api.healthCheck(label);
      setHc({
        ...hc(),
        [label]:
          r.error_code === 0
            ? { state: "ok", text: `${r.latency} ms` }
            : { state: "err", text: r.msg },
      });
    } catch (e) {
      setHc({
        ...hc(),
        [label]: {
          state: "err",
          text: e instanceof ApiError ? `HTTP ${e.status}` : "fail",
        },
      });
    }
  };

  return (
    <>
      <PageHeader
        title="Relay Rules"
        subtitle="Static rules with the last hour of throughput per remote."
        actions={
          <Button
            size="sm"
            leadingIcon={<RefreshCcw size={13} />}
            onClick={refreshAll}
          >
            Refresh
          </Button>
        }
      />

      {/* Desktop table */}
      <TableScroll class="hidden md:block">
        <table class={t.table}>
          <thead class={t.thead}>
            <tr>
              <th class={t.th}>label</th>
              <th class={t.th}>listen</th>
              <th class={t.th}>type</th>
              <th class={t.th}>remote</th>
              <th class={t.th}>1h trend</th>
              <th class={t.th + " text-right"}>tcp xfer</th>
              <th class={t.th + " text-right"}>conns</th>
              <th class={t.th + " text-right"}>ping</th>
              <th class={t.th + " text-right"}>probe</th>
            </tr>
          </thead>
          <tbody class={t.tbody}>
            <Show
              when={ruleList().length}
              fallback={
                <tr>
                  <td colspan={9}>
                    <EmptyState
                      icon={<ServerCog size={28} />}
                      title="No relay rules configured"
                    />
                  </td>
                </tr>
              }
            >
              <For each={ruleList()}>
                {(r) => {
                  const remotes = [
                    ...(r.tcp_remotes ?? []),
                    ...(r.udp_remotes ?? []),
                  ];
                  const remote = remotes[0] ?? "";
                  const m = () => latestFor(r.label ?? "", remote);
                  const probe = () => hc()[r.label ?? ""];
                  const series = () => seriesFor(r.label ?? "", remote);
                  return (
                    <tr class={t.tr}>
                      <td class={t.td + " font-mono"}>{r.label ?? "—"}</td>
                      <td class={t.td + " font-mono text-xs"}>
                        {r.listen ?? "—"}
                      </td>
                      <td class={t.td + " text-xs"}>
                        <div class="inline-flex gap-1">
                          <Pill>{r.listen_type ?? "—"}</Pill>
                          <Pill>{r.transport_type ?? "—"}</Pill>
                        </div>
                      </td>
                      <td
                        class={
                          t.td +
                          " max-w-[220px] truncate font-mono text-xs text-zinc-500"
                        }
                      >
                        {remote || "—"}
                      </td>
                      <td class={t.td + " text-emerald-600 dark:text-emerald-400"}>
                        <Sparkline values={series()} width={90} height={22} />
                      </td>
                      <td class={t.td + " text-right font-mono text-xs"}>
                        {m() ? bytes(m()!.tcp_network_transmit_bytes) : "—"}
                      </td>
                      <td class={t.td + " text-right font-mono"}>
                        {m()?.tcp_connection_count ?? "—"}
                      </td>
                      <td class={t.td + " text-right font-mono"}>
                        {m()?.ping_latency != null
                          ? `${m()!.ping_latency}ms`
                          : "—"}
                      </td>
                      <td class={t.td + " text-right"}>
                        <Show
                          when={probe()}
                          fallback={
                            <Button
                              size="sm"
                              leadingIcon={<Heart size={12} />}
                              onClick={() => checkOne(r.label ?? "")}
                            >
                              probe
                            </Button>
                          }
                        >
                          <Pill
                            tone={
                              probe()!.state === "ok"
                                ? "ok"
                                : probe()!.state === "err"
                                  ? "error"
                                  : "neutral"
                            }
                            dot
                          >
                            {probe()!.text}
                          </Pill>
                        </Show>
                      </td>
                    </tr>
                  );
                }}
              </For>
            </Show>
          </tbody>
        </table>
      </TableScroll>

      {/* Mobile card list */}
      <div class="flex flex-col gap-2 md:hidden">
        <Show
          when={ruleList().length}
          fallback={
            <Card>
              <EmptyState
                icon={<ServerCog size={28} />}
                title="No relay rules configured"
              />
            </Card>
          }
        >
          <For each={ruleList()}>
            {(r) => {
              const remotes = [
                ...(r.tcp_remotes ?? []),
                ...(r.udp_remotes ?? []),
              ];
              const remote = remotes[0] ?? "";
              const m = () => latestFor(r.label ?? "", remote);
              const probe = () => hc()[r.label ?? ""];
              const series = () => seriesFor(r.label ?? "", remote);
              return (
                <div class="rounded-xl border border-zinc-200 bg-white p-3 dark:border-zinc-800 dark:bg-zinc-900">
                  <div class="flex items-center gap-2">
                    <span class="font-mono text-sm font-semibold">
                      {r.label ?? "—"}
                    </span>
                    <Pill>{r.listen_type ?? "—"}</Pill>
                    <Pill>{r.transport_type ?? "—"}</Pill>
                  </div>
                  <div class="mt-2 text-emerald-600 dark:text-emerald-400">
                    <Sparkline values={series()} width={300} height={32} />
                  </div>
                  <div class="mt-2 grid grid-cols-2 gap-x-3 gap-y-1 text-xs">
                    <span class="text-zinc-500">listen</span>
                    <span class="truncate text-right font-mono">
                      {r.listen ?? "—"}
                    </span>
                    <span class="text-zinc-500">remote</span>
                    <span class="truncate text-right font-mono">
                      {remote || "—"}
                    </span>
                    <span class="text-zinc-500">xfer</span>
                    <span class="text-right font-mono">
                      {m() ? bytes(m()!.tcp_network_transmit_bytes) : "—"}
                    </span>
                    <span class="text-zinc-500">conns</span>
                    <span class="text-right font-mono">
                      {m()?.tcp_connection_count ?? "—"}
                    </span>
                    <span class="text-zinc-500">ping</span>
                    <span class="text-right font-mono">
                      {m()?.ping_latency != null
                        ? `${m()!.ping_latency}ms`
                        : "—"}
                    </span>
                  </div>
                  <div class="mt-3 flex items-center justify-between">
                    <Show
                      when={probe()}
                      fallback={<span class="text-xs text-zinc-500">no probe</span>}
                    >
                      <Pill
                        tone={
                          probe()!.state === "ok"
                            ? "ok"
                            : probe()!.state === "err"
                              ? "error"
                              : "neutral"
                        }
                        dot
                      >
                        {probe()!.text}
                      </Pill>
                    </Show>
                    <Button
                      size="sm"
                      leadingIcon={<Heart size={12} />}
                      onClick={() => checkOne(r.label ?? "")}
                    >
                      probe
                    </Button>
                  </div>
                </div>
              );
            }}
          </For>
        </Show>
      </div>
    </>
  );
}
