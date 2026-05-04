import { createResource, createSignal, For, Show } from "solid-js";
import {
  Palette,
  RotateCw,
  Plug,
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
import { theme, toggleTheme } from "../store/theme";
import { bytes } from "../util/format";
import UpdatesPanel from "./UpdatesPanel";

// truncateConfirm must match the literal in internal/cmgr/ms/health.go.
// Wire shape, not user copy — the prompt label can change freely, but
// what we POST cannot.
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
    try {
      await navigator.clipboard.writeText(v);
      setCopied(true);
      setTimeout(() => setCopied(false), 1200);
    } catch {
      /* ignore */
    }
  };

  // runMaint funnels every maintenance op through the same loading +
  // toast-style status pipeline. Keeps the four buttons free of
  // try/catch boilerplate and ensures the health card always refreshes
  // on completion.
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
        subtitle="local ui preferences · runtime config · admin actions"
      />

      <div class="grid gap-3 lg:grid-cols-2">
        <Show when={config()}>
          <Card>
            <CardHeader title="runtime configuration" subtitle="read-only snapshot" />
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

        <Card>
          <CardHeader
            title="reload configuration"
            subtitle="re-fetch from upstream"
          />
          <p class="mb-3 text-sm text-zinc-500">
            A listener change reloads xray and drops active conns.
          </p>
          <div class="flex items-center gap-3">
            <Button
              variant="primary"
              leadingIcon={<RotateCw size={13} />}
              onClick={triggerReload}
            >
              Reload
            </Button>
            {reloadStatus() && (
              <Pill tone={reloadStatus()!.tone} dot>
                {reloadStatus()!.text}
              </Pill>
            )}
          </div>
        </Card>

        <Card>
          <CardHeader title="theme" subtitle="light / dark mode override" />
          <div class="flex items-center gap-3">
            <Pill tone="neutral">{theme()}</Pill>
            <Button leadingIcon={<Palette size={13} />} onClick={toggleTheme}>
              Toggle
            </Button>
          </div>
        </Card>

        <Show when={health()}>
          <StorageCard h={health()!} />
          <LatencyCard h={health()!} onReset={onResetStats} busy={busyOp()} />

          <Card class="lg:col-span-2">
            <CardHeader
              title="maintenance"
              subtitle="prune · compact · wipe · reset stats"
            />
            <div class="grid gap-3 md:grid-cols-3">
              <div class="flex items-end gap-2">
                <label class="flex flex-col text-xs text-zinc-500">
                  <span class="mb-1">older than (days)</span>
                  <input
                    type="number"
                    min="1"
                    class="w-24 rounded-md border border-zinc-200 bg-white px-2 py-1 text-sm dark:border-zinc-800 dark:bg-zinc-950"
                    value={cleanupDays()}
                    onInput={(e) =>
                      setCleanupDays(Math.max(1, Number(e.currentTarget.value) || 1))
                    }
                  />
                </label>
                <Button
                  leadingIcon={<Trash2 size={13} />}
                  onClick={onCleanup}
                  disabled={busyOp() != null}
                >
                  Clean
                </Button>
              </div>
              <div class="flex items-end">
                <Button
                  leadingIcon={<HardDrive size={13} />}
                  onClick={onVacuum}
                  disabled={busyOp() != null}
                >
                  VACUUM
                </Button>
              </div>
              <div class="flex items-end">
                <Button
                  variant="danger"
                  leadingIcon={<AlertTriangle size={13} />}
                  onClick={onTruncate}
                  disabled={busyOp() != null}
                >
                  Truncate all…
                </Button>
              </div>
            </div>
            <Show when={maintStatus()}>
              <div class="mt-3">
                <Pill tone={maintStatus()!.tone} dot>
                  {maintStatus()!.text}
                </Pill>
              </div>
            </Show>
          </Card>
        </Show>

        <div class="lg:col-span-2">
          <UpdatesPanel />
        </div>

        <Card class="lg:col-span-2">
          <CardHeader
            title="api surface"
            subtitle="endpoints the ui consumes"
          />
          <ul class="grid grid-cols-1 gap-y-1 font-mono text-xs text-zinc-600 sm:grid-cols-2 dark:text-zinc-400">
            <Endpoint method="GET" path="/api/v1/config/" />
            <Endpoint method="POST" path="/api/v1/config/reload/" />
            <Endpoint method="GET" path="/api/v1/health_check/" />
            <Endpoint method="GET" path="/api/v1/overview" />
            <Endpoint method="GET" path="/api/v1/node_metrics/" />
            <Endpoint method="GET" path="/api/v1/rule_metrics/" />
            <Endpoint method="GET" path="/api/v1/db/health" />
            <Endpoint method="POST" path="/api/v1/db/cleanup" />
            <Endpoint method="POST" path="/api/v1/db/vacuum" />
            <Endpoint method="POST" path="/api/v1/db/truncate" />
            <Endpoint method="POST" path="/api/v1/db/reset_stats" />
            <Endpoint method="GET" path="/api/v1/xray/conns" />
            <Endpoint method="DELETE" path="/api/v1/xray/conns/:id" />
            <Endpoint method="DELETE" path="/api/v1/xray/conns?user=…" />
            <Endpoint method="GET" path="/api/v1/xray/users" />
            <Endpoint method="GET" path="/metrics/" />
            <Endpoint method="WS" path="/ws/logs" />
          </ul>
          <div class="mt-3 inline-flex flex-wrap items-center gap-1 text-xs text-zinc-500">
            <Plug size={12} />
            <Show
              when={authInfo().auth_required}
              fallback={<span>No auth configured — all endpoints are open.</span>}
            >
              <span>
                Browsers authenticate via the session cookie set at login;
                machine clients send <code class="font-mono">Authorization: Bearer &lt;api_token&gt;</code>.
              </span>
            </Show>
          </div>
        </Card>
      </div>
    </>
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
      <CardHeader title="storage" subtitle="local SQLite metrics store" />
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
      <table class="w-full font-mono text-xs">
        <thead class="text-zinc-500">
          <tr>
            <th class="text-left">op</th>
            <th class="text-right">count</th>
            <th class="text-right">avg</th>
            <th class="text-right">max</th>
            <th class="text-right">last</th>
          </tr>
        </thead>
        <tbody>
          <For each={rows()}>
            {(r) => (
              <tr class="border-t border-zinc-100 dark:border-zinc-900">
                <td class="py-1">{r.name}</td>
                <td class="text-right">{r.count.toLocaleString()}</td>
                <td class="text-right">{r.count ? `${r.avg_ms.toFixed(2)}ms` : "—"}</td>
                <td class="text-right">{r.count ? `${r.max_ms.toFixed(2)}ms` : "—"}</td>
                <td class="text-right">{r.count ? `${r.last_ms.toFixed(2)}ms` : "—"}</td>
              </tr>
            )}
          </For>
        </tbody>
      </table>
    </Card>
  );
}

const methodTones: Record<string, "info" | "ok" | "error" | "warn"> = {
  GET: "info",
  POST: "ok",
  DELETE: "error",
  WS: "warn",
};

function Endpoint(props: { method: string; path: string }) {
  return (
    <li class="flex items-center gap-2">
      <span class="w-12">
        <Pill tone={methodTones[props.method]}>{props.method}</Pill>
      </span>
      <span>{props.path}</span>
    </li>
  );
}
