import { createResource, createSignal, Show } from "solid-js";
import { Palette, RotateCw, Plug, Copy, Check } from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import Button from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import { Pill } from "../ui/Pill";
import DescList from "../ui/DescList";
import { api } from "../api/client";
import { authInfo } from "../store/auth";
import { theme, toggleTheme } from "../store/theme";
import UpdatesPanel from "./UpdatesPanel";

export default function Settings() {
  const [config] = createResource(() => api.config());
  const [reloadStatus, setReloadStatus] = createSignal<{
    tone: "ok" | "error" | "neutral";
    text: string;
  } | null>(null);
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

  return (
    <>
      <PageHeader
        title="settings"
        subtitle="local ui preferences · runtime config · admin actions"
      />

      <div class="grid gap-3 lg:grid-cols-2">
        <Show when={config()}>
          <Card>
            <CardHeader title="Runtime configuration" subtitle="Read-only snapshot" />
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
              title="Sync endpoint"
              subtitle="Where ehco POSTs traffic stats"
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
            title="Reload configuration"
            subtitle="Re-fetch from upstream"
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
          <CardHeader title="Theme" subtitle="Light / dark mode override" />
          <div class="flex items-center gap-3">
            <Pill tone="neutral">{theme()}</Pill>
            <Button leadingIcon={<Palette size={13} />} onClick={toggleTheme}>
              Toggle
            </Button>
          </div>
        </Card>

        <div class="lg:col-span-2">
          <UpdatesPanel />
        </div>

        <Card class="lg:col-span-2">
          <CardHeader
            title="API surface"
            subtitle="Endpoints the UI consumes"
          />
          <ul class="grid grid-cols-1 gap-y-1 font-mono text-xs text-zinc-600 sm:grid-cols-2 dark:text-zinc-400">
            <Endpoint method="GET" path="/api/v1/config/" />
            <Endpoint method="POST" path="/api/v1/config/reload/" />
            <Endpoint method="GET" path="/api/v1/health_check/" />
            <Endpoint method="GET" path="/api/v1/overview" />
            <Endpoint method="GET" path="/api/v1/node_metrics/" />
            <Endpoint method="GET" path="/api/v1/rule_metrics/" />
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
