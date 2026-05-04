import { createResource, createSignal, For, onCleanup, Show } from "solid-js";
import { Download, RefreshCw, CircleCheck, CircleAlert } from "lucide-solid";
import { Card, CardHeader } from "../ui/Card";
import Button from "../ui/Button";
import { Pill } from "../ui/Pill";
import Segmented from "../ui/Segmented";
import DescList from "../ui/DescList";
import { api, ApiError } from "../api/client";
import { relTime } from "../util/format";
import type {
  UpdateCheck,
  UpdateState,
  UpdateStatus,
  VersionInfo,
} from "../api/types";

type Channel = "auto" | "stable" | "nightly";

const STEPS: UpdateState[] = [
  "checking",
  "downloading",
  "installing",
  "restarting",
  "done",
];
const cap = (s: string) => s[0].toUpperCase() + s.slice(1);

export default function UpdatesPanel() {
  const [version, { refetch: rcVersion }] = createResource<VersionInfo>(() =>
    api.version(),
  );
  const [channel, setChannel] = createSignal<Channel>("auto");
  const [checking, setChecking] = createSignal(false);
  const [check, setCheck] = createSignal<UpdateCheck | null>(null);
  const [checkErr, setCheckErr] = createSignal("");
  const [status, setStatus] = createSignal<UpdateStatus | null>(null);
  const [applyErr, setApplyErr] = createSignal("");

  let timer: number | null = null;
  const stopTimer = () => {
    if (timer != null) {
      window.clearInterval(timer);
      timer = null;
    }
  };
  onCleanup(stopTimer);

  // Single polling loop: while a job is in progress, hit /update/status.
  // Once state==restarting the relay may go down mid-poll; we then probe
  // /version and stop when the running version differs from the one we
  // started from (i.e. the new binary booted).
  const tick = async () => {
    const before = status()?.from;
    try {
      const s = await api.updateStatus();
      if (s.state !== "idle") {
        setStatus(s);
        if (s.state === "done" || s.state === "failed") {
          stopTimer();
          rcVersion();
          return;
        }
      } else if (status()?.state === "restarting") {
        // New process booted; status reset to idle. Probe /version to
        // confirm and land on done.
        const v = await api.version();
        if (before && v.version !== before) {
          setStatus({ ...(status() as UpdateStatus), state: "done" });
          rcVersion();
          stopTimer();
        }
      }
    } catch {
      // Relay is restarting — keep polling, the new process will answer.
    }
  };

  const startPolling = () => {
    if (timer != null) return;
    timer = window.setInterval(tick, 1500) as unknown as number;
  };

  // Hydrate any in-flight job on mount so a refresh during update doesn't
  // lose the indicator.
  api.updateStatus().then((s) => {
    if (s.state !== "idle") {
      setStatus(s);
      if (s.state !== "done" && s.state !== "failed") startPolling();
    }
  }).catch(() => {});

  const runCheck = async () => {
    setChecking(true);
    setCheckErr("");
    try {
      setCheck(await api.updateCheck(channel()));
    } catch (e) {
      setCheckErr(e instanceof ApiError ? e.message : String(e));
    } finally {
      setChecking(false);
    }
  };

  const applyUpdate = async () => {
    const c = check();
    if (!c) return;
    if (!confirm(
      `Update to ${c.latest_version}? This replaces the running binary and restarts ehco. Active connections will drop.`,
    )) return;
    setApplyErr("");
    try {
      await api.updateApply({ channel: channel(), force: false, restart: true });
      setStatus({
        state: "checking",
        channel: channel(),
        from: version()?.version ?? "",
        to: c.latest_version,
        started_at: new Date().toISOString(),
      });
      startPolling();
    } catch (e) {
      setApplyErr(e instanceof ApiError ? e.message : String(e));
    }
  };

  const inProgress = () => {
    const s = status()?.state;
    return s === "checking" || s === "downloading" || s === "installing" || s === "restarting";
  };
  const isNightly = () => version()?.version.includes("-") ?? false;
  const linuxOnly = () => version()?.go_os === "linux";

  return (
    <>
      <div class="mb-3 flex items-center justify-between">
        <div>
          <h2 class="text-[12px] font-semibold uppercase tracking-[0.14em] text-zinc-500">
            updates
          </h2>
          <p class="mt-0.5 text-[11px] text-zinc-500">
            check for new ehco builds and apply them in place
          </p>
        </div>
        <Pill tone={isNightly() ? "warn" : "info"}>
          {isNightly() ? "nightly build" : "stable build"}
        </Pill>
      </div>

      <Card>
        <CardHeader title="current build" subtitle="reported by this binary" />
        <DescList
          items={[
            ["Version", version()?.version ?? "—"],
            ["Branch", version()?.git_branch || "—"],
            ["Commit", (version()?.git_revision ?? "").slice(0, 7) || "—"],
            ["Built", version()?.build_time || "—"],
            ["Started", version()?.start_time ? relTime(version()!.start_time) : "—"],
            ["Platform", version() ? `${version()!.go_os}/${version()!.go_arch}` : "—"],
          ]}
        />
        <Show when={version() && !linuxOnly()}>
          <div class="mt-3 rounded-md border border-amber-200 bg-amber-50 p-2 text-xs text-amber-800 dark:border-amber-900/40 dark:bg-amber-950/30 dark:text-amber-300">
            Self-update is only supported on linux. On {version()!.go_os} you'll need to rebuild from source.
          </div>
        </Show>
      </Card>

      <Card class="mt-4">
        <div class="flex flex-wrap items-center gap-3">
          <CardHeader title="check for updates" subtitle="github releases (Ehco1996/ehco)" />
          <div class="ml-auto flex items-center gap-2">
            <Segmented<Channel>
              options={[
                { value: "auto", label: "Auto" },
                { value: "stable", label: "Stable" },
                { value: "nightly", label: "Nightly" },
              ]}
              value={channel()}
              onChange={setChannel}
              size="sm"
            />
            <Button
              size="sm"
              variant="primary"
              loading={checking()}
              leadingIcon={<RefreshCw size={13} />}
              onClick={runCheck}
            >
              Check
            </Button>
          </div>
        </div>

        <Show when={checkErr()}>
          <div class="mt-3 rounded-md border border-rose-200 bg-rose-50 p-2 text-xs text-rose-700 dark:border-rose-900/50 dark:bg-rose-950/30 dark:text-rose-400">
            {checkErr()}
          </div>
        </Show>

        <Show when={check()}>
          {(c) => (
            <div class="mt-4">
              <Show
                when={c().update_available}
                fallback={
                  <div class="flex items-center gap-2 rounded-md border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-800 dark:border-emerald-900/40 dark:bg-emerald-950/30 dark:text-emerald-300">
                    <CircleCheck size={15} />
                    <span>
                      Up to date — already on{" "}
                      <span class="font-mono">{c().current_version}</span>{" "}
                      ({c().channel} channel).
                    </span>
                  </div>
                }
              >
                <div class="rounded-md border border-amber-200 bg-amber-50 p-3 dark:border-amber-900/40 dark:bg-amber-950/30">
                  <div class="flex flex-wrap items-center gap-2 text-sm text-amber-900 dark:text-amber-200">
                    <CircleAlert size={15} />
                    <span>
                      New version available:{" "}
                      <span class="font-mono">{c().latest_version}</span>
                    </span>
                    <span class="text-xs text-amber-700 dark:text-amber-400">
                      published {relTime(c().published_at)}
                    </span>
                    <a
                      href={c().release_url}
                      target="_blank"
                      rel="noreferrer"
                      class="ml-auto text-xs underline"
                    >
                      release notes
                    </a>
                  </div>
                  <Show when={c().release_body}>
                    <pre class="scroll-pretty mt-2 max-h-40 overflow-y-auto whitespace-pre-wrap break-words text-xs text-amber-900/80 dark:text-amber-200/80">
                      {c().release_body}
                    </pre>
                  </Show>
                  <div class="mt-3 flex items-center gap-2">
                    <Button
                      variant="primary"
                      size="sm"
                      leadingIcon={<Download size={13} />}
                      disabled={!linuxOnly() || inProgress()}
                      onClick={applyUpdate}
                    >
                      Update now
                    </Button>
                    <span class="text-xs text-amber-800/70 dark:text-amber-300/70">
                      Asset: <span class="font-mono">{c().asset_name || "n/a"}</span>
                    </span>
                  </div>
                </div>
              </Show>
            </div>
          )}
        </Show>
      </Card>

      <Show when={status() && status()!.state !== "idle"}>
        <Card class="mt-4">
          <CardHeader
            title="update progress"
            subtitle={status()!.from ? `${status()!.from} → ${status()!.to || "?"}` : undefined}
          />
          <Show when={applyErr()}>
            <div class="mb-2 rounded-md border border-rose-200 bg-rose-50 p-2 text-xs text-rose-700 dark:border-rose-900/50 dark:bg-rose-950/30 dark:text-rose-400">
              {applyErr()}
            </div>
          </Show>
          <StepIndicator state={status()!.state} />
          <Show when={status()!.state === "restarting"}>
            <div class="mt-3 text-xs text-zinc-500">
              Waiting for the relay to come back online…
            </div>
          </Show>
          <Show when={status()!.error}>
            <div class="mt-3 rounded-md border border-rose-200 bg-rose-50 p-2 text-xs text-rose-700 dark:border-rose-900/50 dark:bg-rose-950/30 dark:text-rose-400">
              {status()!.error}
            </div>
          </Show>
        </Card>
      </Show>
    </>
  );
}

function StepIndicator(props: { state: UpdateState }) {
  const cur = () => STEPS.indexOf(props.state);
  return (
    <div class="flex flex-wrap items-center gap-2 text-xs">
      <For each={STEPS}>
        {(label, i) => {
          const done = () => cur() > i();
          const active = () => cur() === i();
          return (
            <>
              <Show when={i() > 0}>
                <span class={done() ? "h-px w-6 bg-emerald-400" : "h-px w-6 bg-zinc-300 dark:bg-zinc-700"} />
              </Show>
              <span
                class={
                  done() || active()
                    ? "inline-flex items-center gap-1 rounded-full border border-emerald-300 bg-emerald-50 px-2 py-0.5 font-medium text-emerald-800 dark:border-emerald-700/60 dark:bg-emerald-950/40 dark:text-emerald-300"
                    : "inline-flex items-center gap-1 rounded-full border border-zinc-200 px-2 py-0.5 text-zinc-500 dark:border-zinc-800"
                }
              >
                <span
                  class={
                    "h-1.5 w-1.5 rounded-full " +
                    (active() ? "animate-pulse bg-emerald-500" : done() ? "bg-emerald-500" : "bg-zinc-400")
                  }
                />
                {cap(label)}
              </span>
            </>
          );
        }}
      </For>
    </div>
  );
}
