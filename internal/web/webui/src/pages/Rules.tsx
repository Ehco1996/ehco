import { createMemo, createResource, createSignal, Show } from "solid-js";
import { RefreshCcw, ServerCog, Heart } from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import Button from "../ui/Button";
import { Pill } from "../ui/Pill";
import EmptyState from "../ui/EmptyState";
import Sparkline from "../ui/Sparkline";
import DataTable, { Column } from "../ui/DataTable";
import { api, ApiError } from "../api/client";
import { bytes } from "../util/format";
import type { RelayConfig, RuleMetric } from "../api/types";

const HISTORY_SECONDS = 60 * 60;

interface HCResult {
  state: "running" | "ok" | "err";
  text: string;
}

interface Row {
  cfg: RelayConfig;
  remote: string;
  metric: RuleMetric | undefined;
  series: number[];
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

  const rows = createMemo<Row[]>(() =>
    ruleList().map((cfg) => {
      const remotes = [...(cfg.tcp_remotes ?? []), ...(cfg.udp_remotes ?? [])];
      const remote = remotes[0] ?? "";
      return {
        cfg,
        remote,
        metric: latestFor(cfg.label ?? "", remote),
        series: historyByKey().get(`${cfg.label}|${remote}`) ?? [],
      };
    }),
  );

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

  const columns: Column<Row>[] = [
    {
      key: "label",
      header: "label",
      cell: (r) => <span class="font-mono">{r.cfg.label ?? "—"}</span>,
      sortable: true,
      sortBy: (r) => r.cfg.label ?? "",
    },
    {
      key: "listen",
      header: "listen",
      cell: (r) => (
        <span class="font-mono text-xs">{r.cfg.listen ?? "—"}</span>
      ),
      mdOnly: true,
    },
    {
      key: "type",
      header: "type",
      cell: (r) => (
        <div class="inline-flex gap-1">
          <Pill>{r.cfg.listen_type ?? "—"}</Pill>
          <Pill>{r.cfg.transport_type ?? "—"}</Pill>
        </div>
      ),
      mdOnly: true,
    },
    {
      key: "remote",
      header: "remote",
      cell: (r) => (
        <span class="block max-w-[220px] truncate font-mono text-xs text-zinc-500">
          {r.remote || "—"}
        </span>
      ),
      mdOnly: true,
    },
    {
      key: "trend",
      header: "1h trend",
      cell: (r) => (
        <span class="text-emerald-600 dark:text-emerald-400">
          <Sparkline values={r.series} width={90} height={22} />
        </span>
      ),
      mdOnly: true,
    },
    {
      key: "xfer",
      header: "tcp xfer",
      align: "right",
      cell: (r) => (
        <span class="font-mono text-xs">
          {r.metric ? bytes(r.metric.tcp_network_transmit_bytes) : "—"}
        </span>
      ),
      sortable: true,
      sortBy: (r) => r.metric?.tcp_network_transmit_bytes ?? 0,
    },
    {
      key: "conns",
      header: "conns",
      align: "right",
      cell: (r) => (
        <span class="font-mono">{r.metric?.tcp_connection_count ?? "—"}</span>
      ),
      sortable: true,
      sortBy: (r) => r.metric?.tcp_connection_count ?? 0,
      width: "80px",
    },
    {
      key: "ping",
      header: "ping",
      align: "right",
      cell: (r) => (
        <span class="font-mono">
          {r.metric?.ping_latency != null ? `${r.metric.ping_latency}ms` : "—"}
        </span>
      ),
      sortable: true,
      sortBy: (r) => r.metric?.ping_latency ?? Infinity,
      width: "80px",
    },
    {
      key: "probe",
      header: "probe",
      align: "right",
      width: "120px",
      cell: (r) => {
        const probe = () => hc()[r.cfg.label ?? ""];
        return (
          <Show
            when={probe()}
            fallback={
              <Button
                size="sm"
                leadingIcon={<Heart size={12} />}
                onClick={() => checkOne(r.cfg.label ?? "")}
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
        );
      },
    },
  ];

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

      <DataTable<Row>
        rows={rows()}
        columns={columns}
        rowKey={(r) => `${r.cfg.label}|${r.remote}`}
        pageSize={50}
        defaultSort={{ key: "xfer", dir: "desc" }}
        empty={
          <EmptyState
            icon={<ServerCog size={28} />}
            title="No relay rules configured"
          />
        }
      />
    </>
  );
}
