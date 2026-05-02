import { createSignal } from "solid-js";
import { KeyRound, Palette, RotateCw, Plug } from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import Button from "../ui/Button";
import { Input } from "../ui/Input";
import { Card, CardHeader } from "../ui/Card";
import { Pill } from "../ui/Pill";
import { api } from "../api/client";
import { saveToken, token } from "../store/auth";
import { theme, toggleTheme } from "../store/theme";

export default function Settings() {
  const [tokenInput, setTokenInput] = createSignal(token());
  const [reloadStatus, setReloadStatus] = createSignal<{
    tone: "ok" | "error" | "neutral";
    text: string;
  } | null>(null);

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

  return (
    <>
      <PageHeader
        title="Settings"
        subtitle="Local UI preferences and a few admin actions."
      />

      <div class="grid gap-3 lg:grid-cols-2">
        <Card>
          <CardHeader
            title="Access token"
            subtitle="Appended to every API request as ?token=…"
          />
          <p class="mb-3 text-sm text-zinc-500">
            Stored in <code class="font-mono text-xs">sessionStorage</code>.
            Leave empty if your ehco instance has no{" "}
            <code class="font-mono text-xs">web_token</code>.
          </p>
          <div class="flex gap-2">
            <Input
              type="password"
              mono
              placeholder="paste token"
              value={tokenInput()}
              onInput={(e) => setTokenInput(e.currentTarget.value)}
            />
            <Button
              variant="primary"
              leadingIcon={<KeyRound size={13} />}
              onClick={() => saveToken(tokenInput().trim())}
            >
              Save
            </Button>
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
          <CardHeader
            title="API surface"
            subtitle="Endpoints the UI consumes"
          />
          <ul class="space-y-1 font-mono text-xs text-zinc-600 dark:text-zinc-400">
            <Endpoint method="GET" path="/api/v1/config/" />
            <Endpoint method="POST" path="/api/v1/config/reload/" />
            <Endpoint method="GET" path="/api/v1/health_check/" />
            <Endpoint method="GET" path="/api/v1/node_metrics/" />
            <Endpoint method="GET" path="/api/v1/rule_metrics/" />
            <Endpoint method="GET" path="/api/v1/xray/conns" />
            <Endpoint method="DELETE" path="/api/v1/xray/conns/:id" />
            <Endpoint method="DELETE" path="/api/v1/xray/conns?user=…" />
            <Endpoint method="GET" path="/api/v1/xray/users" />
            <Endpoint method="GET" path="/metrics/" />
            <Endpoint method="WS" path="/ws/logs" />
          </ul>
          <div class="mt-3 inline-flex items-center gap-1 text-xs text-zinc-500">
            <Plug size={12} /> All endpoints require{" "}
            <code class="font-mono">?token=</code> when web_token is set.
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
