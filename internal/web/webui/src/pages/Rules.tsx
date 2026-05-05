import { createMemo, createResource, createSignal, Show } from "solid-js";
import { ServerCog, Heart } from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import Button from "../ui/Button";
import { Pill } from "../ui/Pill";
import EmptyState from "../ui/EmptyState";
import DataTable, { Column } from "../ui/DataTable";
import { api, ApiError } from "../api/client";
import type { RelayConfig } from "../api/types";

interface HCResult {
  state: "running" | "ok" | "err";
  text: string;
}

interface Row {
  cfg: RelayConfig;
  remote: string;
}

export default function Rules() {
  const [config] = createResource(() => api.config());
  const [hc, setHc] = createSignal<Record<string, HCResult>>({});

  const ruleList = (): RelayConfig[] => {
    const c = config()?.relay_configs;
    return Array.isArray(c) ? c : [];
  };

  const rows = createMemo<Row[]>(() =>
    ruleList().map((cfg) => {
      const remotes = [...(cfg.tcp_remotes ?? []), ...(cfg.udp_remotes ?? [])];
      return { cfg, remote: remotes[0] ?? "" };
    }),
  );

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
        title="rules"
        subtitle="static relay rules — per-rule metrics removed pending rewrite"
      />

      <DataTable<Row>
        rows={rows()}
        columns={columns}
        rowKey={(r) => `${r.cfg.label}|${r.remote}`}
        pageSize={50}
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
