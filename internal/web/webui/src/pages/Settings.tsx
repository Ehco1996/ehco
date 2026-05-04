import { createResource, createSignal, For, Show } from "solid-js";
import {
  RotateCw,
  Copy,
  Check,
  Trash2,
  HardDrive,
  AlertTriangle,
} from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import Button from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { Pill } from "../ui/Pill";
import DescList from "../ui/DescList";
import { api } from "../api/client";
import type { DBHealth, DBMaintenanceResult } from "../api/types";
import { authInfo } from "../store/auth";
import { bytes } from "../util/format";
import { copyText } from "../util/clipboard";
import UpdatesPanel from "./UpdatesPanel";

// Wire-shape literal — must match the constant in
// internal/cmgr/ms/health.go. The button label can change freely; what
// we POST cannot.
const TRUNCATE_CONFIRM = "yes I am sure";

type ToneStatus = {
  tone: "ok" | "error" | "neutral";
  text: string;
} | null;

export default function Settings() {
  const [config] = createResource(() => api.config());
  const [health, { refetch: refetchHealth }] = createResource(() =>
    api.dbHealth(),
  );
  const [reloadStatus, setReloadStatus] = createSignal<ToneStatus>(null);
  const [maintStatus, setMaintStatus] = createSignal<ToneStatus>(null);
  const [cleanupDays, setCleanupDays] = createSignal(30);
  const [busyOp, setBusyOp] = createSignal<string | null>(null);
  const [copied, setCopied] = createSignal(false);

  const triggerReload = async () => {
    if (
      !confirm(
        "Trigger config reload? Active xray conns may be killed if listeners changed.",
      )
    )
      return;
    setReloadStatus({ tone: "neutral", text: "reloading…" });
    try {
      const r = await api.reload();
      setReloadStatus({ tone: "ok", text: typeof r === "string" ? r : "ok" });
    } catch (e) {
      setReloadStatus({ tone: "error", text: String(e) });
    }
  };

  const copySync = async () => {
    const v = String(config()?.sync_traffic_endpoint ?? "");
    if (!v) return;
    if (await copyText(v)) {
      setCopied(true);
      setTimeout(() => setCopied(false), 1200);
    }
  };

  // runMaint funnels every maintenance op through the same loading +
  // status pipeline so the four buttons stay free of try/catch
  // boilerplate and the health card always refreshes on completion.
  const runMaint = async (
    op: string,
    fn: () => Promise<DBMaintenanceResult | string>,
    fmtOk: (r: DBMaintenanceResult | string) => string,
  ) => {
    setBusyOp(op);
    setMaintStatus({ tone: "neutral", text: `${op}…` });
    try {
      const r = await fn();
      setMaintStatus({ tone: "ok", text: fmtOk(r) });
      refetchHealth();
    } catch (e) {
      setMaintStatus({ tone: "error", text: String(e) });
    } finally {
      setBusyOp(null);
    }
  };

  const onCleanup = () =>
    runMaint(
      "cleanup",
      () => api.dbCleanup(cleanupDays()),
      (r) => {
        const m = r as DBMaintenanceResult;
        return `pruned node=${m.node_deleted ?? 0} rule=${m.rule_deleted ?? 0} in ${m.duration_ms}ms`;
      },
    );

  const onVacuum = () => {
    if (
      !confirm(
        "VACUUM rewrites the db file and locks all queries until done. Cheap on small dbs (~ms), seconds at GB scale. Proceed?",
      )
    )
      return;
    runMaint(
      "vacuum",
      () => api.dbVacuum(),
      (r) => {
        const m = r as DBMaintenanceResult;
        return `${bytes(m.bytes_before)} → ${bytes(m.bytes_after)} in ${m.duration_ms}ms`;
      },
    );
  };

  const onTruncate = () => {
    const got = prompt(
      `Wipe ALL local metrics history. This cannot be undone.\n\nType the confirmation phrase exactly:\n  ${TRUNCATE_CONFIRM}`,
    );
    if (got == null) return;
    if (got !== TRUNCATE_CONFIRM) {
      setMaintStatus({ tone: "error", text: "confirmation phrase mismatch" });
      return;
    }
    runMaint(
      "truncate",
      () => api.dbTruncate(got),
      (r) => {
        const m = r as DBMaintenanceResult;
        return `wiped node=${m.node_deleted ?? 0} rule=${m.rule_deleted ?? 0}`;
      },
    );
  };

  const onResetStats = () =>
    runMaint(
      "reset_stats",
      async () => {
        await api.dbResetStats();
        return "ok";
      },
      () => "stats reset",
    );

  return (
    <>
      <PageHeader
        title="settings"
        subtitle="runtime config · storage health · self-update"
      />

      <div class="grid gap-3 lg:grid-cols-2">
        <Show when={config()}>
          <Card>
            <CardHeader
              title="runtime configuration"
              subtitle="read-only snapshot · re-fetch from upstream"
              right={
                <div class="flex items-center gap-2">
                  {reloadStatus() && (
                    <Pill tone={reloadStatus()!.tone} dot>
                      {reloadStatus()!.text}
                    </Pill>
                  )}
                  <Button
                    size="sm"
                    variant="primary"
                    leadingIcon={<RotateCw size={12} />}
                    onClick={triggerReload}
                  >
                    Reload
                  </Button>
                </div>
              }
            />
            <DescList
              items={[
                ["log level", String(config()!.log_level ?? "—")],
                ["reload interval", `${config()!.reload_interval ?? 0}s`],
                ["ping", config()!.enable_ping ? "enabled" : "disabled"],
                [
                  "web bind",
                  `${config()!.web_host ?? "0.0.0.0"}:${config()!.web_port ?? "—"}`,
                ],
                [
                  "auth",
                  authInfo().auth_required ? "session (cookie / bearer)" : "none",
                ],
              ]}
            />
          </Card>

          <Card>
            <CardHeader
              title="sync endpoint"
              subtitle="where ehco POSTs traffic stats"
              right={
                <button
                  class="inline-flex items-center gap-1 text-xs text-zinc-500 hover:text-emerald-600 dark:hover:text-emerald-400"
                  onClick={copySync}
                  disabled={!config()!.sync_traffic_endpoint}
                >
                  {copied() ? <Check size={12} /> : <Copy size={12} />}
                  {copied() ? "copied" : "copy"}
                </button>
              }
            />
            <p class="break-all rounded-md border border-zinc-200 bg-zinc-50 p-2.5 font-mono text-xs text-zinc-700 dark:border-zinc-800 dark:bg-zinc-950 dark:text-zinc-300">
              {String(config()!.sync_traffic_endpoint ?? "—")}
            </p>
          </Card>
        </Show>
      </div>

      <Show when={health()}>
        <SectionTitle
          title="database"
          subtitle="local SQLite metrics store"
        />
        <div class="grid gap-3 lg:grid-cols-2">
          <StorageCard h={health()!} />
          <LatencyCard h={health()!} onReset={onResetStats} busy={busyOp()} />
          <MaintenanceCard
            cleanupDays={cleanupDays()}
            setCleanupDays={setCleanupDays}
            busyOp={busyOp()}
            status={maintStatus()}
            onCleanup={onCleanup}
            onVacuum={onVacuum}
            onTruncate={onTruncate}
          />
        </div>
      </Show>

      <SectionTitle
        title="updates"
        subtitle="check for new ehco builds and apply them in place"
      />
      <UpdatesPanel />
    </>
  );
}

function SectionTitle(props: { title: string; subtitle?: string }) {
  return (
    <div class="mb-2 mt-6">
      <h2 class="text-[12px] font-semibold uppercase tracking-[0.14em] text-zinc-500">
        {props.title}
      </h2>
      <Show when={props.subtitle}>
        <p class="mt-0.5 text-[11px] text-zinc-500">{props.subtitle}</p>
      </Show>
    </div>
  );
}

function StorageCard(props: { h: DBHealth }) {
  const fragPct = () => {
    const pc = props.h.db_page_count;
    return pc > 0 ? (props.h.db_freelist_pages / pc) * 100 : 0;
  };
  const lastWriteText = () => {
    if (!props.h.last_rule_write_ts) return "never";
    const ageSec = Math.max(0, Date.now() / 1000 - props.h.last_rule_write_ts);
    if (ageSec < 60) return `${Math.round(ageSec)}s ago`;
    if (ageSec < 3600) return `${Math.round(ageSec / 60)}m ago`;
    if (ageSec < 86400) return `${Math.round(ageSec / 3600)}h ago`;
    return `${Math.round(ageSec / 86400)}d ago`;
  };
  return (
    <Card>
      <CardHeader title="storage" subtitle="file size · pages · row counts" />
      <DescList
        items={[
          ["db file", bytes(props.h.db_file_bytes)],
          [
            "pages",
            `${props.h.db_page_count.toLocaleString()} × ${props.h.db_page_size}B`,
          ],
          [
            "freelist",
            `${props.h.db_freelist_pages.toLocaleString()} (${fragPct().toFixed(1)}%)${fragPct() > 30 ? " — VACUUM recommended" : ""}`,
          ],
          ["node_metrics", `${props.h.node_metrics_rows.toLocaleString()} rows`],
          [
            "rule_metrics",
            `${props.h.rule_metrics_rows.toLocaleString()} rows${props.h.rule_metrics_rows === 0 ? " — no data, check sync pipeline" : ""}`,
          ],
          ["last rule write", lastWriteText()],
        ]}
      />
    </Card>
  );
}

function LatencyCard(props: {
  h: DBHealth;
  onReset: () => void;
  busy: string | null;
}) {
  const rows = () =>
    Object.entries(props.h.stats).map(([name, s]) => ({ name, ...s }));
  return (
    <Card>
      <CardHeader
        title="query latency"
        subtitle="since process start"
        right={
          <button
            class="text-xs text-zinc-500 hover:text-emerald-600 disabled:opacity-50 dark:hover:text-emerald-400"
            onClick={props.onReset}
            disabled={props.busy != null}
          >
            reset
          </button>
        }
      />
      <table class="w-full table-fixed font-mono text-xs">
        <colgroup>
          <col class="w-[40%]" />
          <col class="w-[20%]" />
          <col class="w-[20%]" />
          <col class="w-[20%]" />
        </colgroup>
        <thead class="text-zinc-500">
          <tr>
            <th class="text-left">op</th>
            <th class="text-right">count</th>
            <th class="text-right">avg</th>
            <th class="text-right">max</th>
          </tr>
        </thead>
        <tbody>
          <For each={rows()}>
            {(r) => (
              <tr class="border-t border-zinc-100 dark:border-zinc-900">
                <td class="py-1 truncate">{r.name}</td>
                <td class="text-right">{r.count.toLocaleString()}</td>
                <td class="text-right">{r.count ? `${r.avg_ms.toFixed(2)}ms` : "—"}</td>
                <td class="text-right">{r.count ? `${r.max_ms.toFixed(2)}ms` : "—"}</td>
              </tr>
            )}
          </For>
        </tbody>
      </table>
    </Card>
  );
}

function MaintenanceCard(props: {
  cleanupDays: number;
  setCleanupDays: (n: number) => void;
  busyOp: string | null;
  status: ToneStatus;
  onCleanup: () => void;
  onVacuum: () => void;
  onTruncate: () => void;
}) {
  return (
    <Card class="lg:col-span-2">
      <CardHeader
        title="maintenance"
        subtitle="prune · compact · wipe"
        right={
          <Show when={props.status}>
            <Pill tone={props.status!.tone} dot>
              {props.status!.text}
            </Pill>
          </Show>
        }
      />
      <div class="flex flex-wrap items-end gap-3">
        <label class="flex items-end gap-2">
          <span class="flex flex-col text-xs text-zinc-500">
            <span class="mb-1">older than (days)</span>
            <input
              type="number"
              min="1"
              class="w-20 rounded-md border border-zinc-200 bg-white px-2 py-1 text-sm dark:border-zinc-800 dark:bg-zinc-950"
              value={props.cleanupDays}
              onInput={(e) =>
                props.setCleanupDays(
                  Math.max(1, Number(e.currentTarget.value) || 1),
                )
              }
            />
          </span>
          <Button
            leadingIcon={<Trash2 size={13} />}
            onClick={props.onCleanup}
            disabled={props.busyOp != null}
          >
            Clean
          </Button>
        </label>
        <Button
          leadingIcon={<HardDrive size={13} />}
          onClick={props.onVacuum}
          disabled={props.busyOp != null}
        >
          VACUUM
        </Button>
        <Button
          variant="danger"
          leadingIcon={<AlertTriangle size={13} />}
          onClick={props.onTruncate}
          disabled={props.busyOp != null}
        >
          Truncate all…
        </Button>
      </div>
    </Card>
  );
}
