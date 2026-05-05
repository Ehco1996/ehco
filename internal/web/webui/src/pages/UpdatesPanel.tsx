import { createResource, createSignal, onCleanup, Show } from "solid-js";
import { Download, RefreshCw, CircleCheck, CircleAlert, Loader2 } from "lucide-solid";
import { Card, CardHeader } from "../ui/Card";
import Button from "../ui/Button";
import { Pill } from "../ui/Pill";
import Segmented from "../ui/Segmented";
import DescList from "../ui/DescList";
import { api, ApiError } from "../api/client";
import { relTime } from "../util/format";
import type { UpdateCheck, VersionInfo } from "../api/types";

type Channel = "auto" | "stable" | "nightly";

// /version is polled every POLL_MS while an update is in flight; if the
// commit hasn't changed within TIMEOUT_MS we give up and surface a hint.
const POLL_MS = 2000;
const TIMEOUT_MS = 60_000;

export default function UpdatesPanel() {
  const [version, { refetch: rcVersion }] = createResource<VersionInfo>(() =>
    api.version(),
  );
  const [channel, setChannel] = createSignal<Channel>("auto");
  const [checking, setChecking] = createSignal(false);
  const [check, setCheck] = createSignal<UpdateCheck | null>(null);
  const [checkErr, setCheckErr] = createSignal("");

  // Updating state. `startCommit` is captured at click time; we treat
  // /version returning a different commit as "done".
  const [updating, setUpdating] = createSignal(false);
  const [startCommit, setStartCommit] = createSignal("");
  const [updateMsg, setUpdateMsg] = createSignal("");
  const [updateErr, setUpdateErr] = createSignal("");

  let timer: number | null = null;
  let timeoutHandle: number | null = null;
  const stopPolling = () => {
    if (timer != null) {
      window.clearInterval(timer);
      timer = null;
    }
    if (timeoutHandle != null) {
      window.clearTimeout(timeoutHandle);
      timeoutHandle = null;
    }
  };
  onCleanup(stopPolling);

  const tick = async () => {
    try {
      const v = await api.version();
      if (v.git_revision && v.git_revision !== startCommit()) {
        setUpdating(false);
        setUpdateMsg(`Updated to ${v.git_revision.slice(0, 7)}.`);
        rcVersion();
        stopPolling();
      }
    } catch {
      // Relay restarting; keep polling.
    }
  };

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
    if (
      !confirm(
        `Update to ${c.latest_version}? This replaces the running binary and restarts ehco. Active connections will drop.`,
      )
    )
      return;
    setUpdateErr("");
    setUpdateMsg("");
    try {
      await api.updateApply({
        channel: channel(),
        force: false,
        restart: true,
      });
    } catch (e) {
      setUpdateErr(e instanceof ApiError ? e.message : String(e));
      return;
    }
    setStartCommit(version()?.git_revision ?? "");
    setUpdating(true);
    timer = window.setInterval(tick, POLL_MS) as unknown as number;
    timeoutHandle = window.setTimeout(() => {
      stopPolling();
      setUpdating(false);
      setUpdateErr(
        "Timed out waiting for the relay to come back. Check journalctl -u ehco for details.",
      );
    }, TIMEOUT_MS) as unknown as number;
  };

  const isNightly = () => version()?.version.includes("-") ?? false;
  const linuxOnly = () => version()?.go_os === "linux";

  return (
    <Card>
      <CardHeader
        title="build & self-update"
        subtitle="github releases · Ehco1996/ehco"
        right={
          <div class="flex items-center gap-2">
            <Show when={version()}>
              <Pill tone={isNightly() ? "warn" : "info"}>
                {isNightly() ? "nightly" : "stable"}
              </Pill>
            </Show>
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
        }
      />

      <DescList
        items={[
          ["version", version()?.version ?? "—"],
          ["branch", version()?.git_branch || "—"],
          ["commit", (version()?.git_revision ?? "").slice(0, 7) || "—"],
          ["built", version()?.build_time || "—"],
          [
            "started",
            version()?.start_time ? relTime(version()!.start_time) : "—",
          ],
          [
            "platform",
            version() ? `${version()!.go_os}/${version()!.go_arch}` : "—",
          ],
        ]}
      />

      <Show when={version() && !linuxOnly()}>
        <div class="mt-3 rounded-md border border-amber-200 bg-amber-50 p-2 text-xs text-amber-800 dark:border-amber-900/40 dark:bg-amber-950/30 dark:text-amber-300">
          Self-update is only supported on linux. On {version()!.go_os} you'll
          need to rebuild from source.
        </div>
      </Show>

      <Show when={checkErr()}>
        <div class="mt-3 rounded-md border border-rose-200 bg-rose-50 p-2 text-xs text-rose-700 dark:border-rose-900/50 dark:bg-rose-950/30 dark:text-rose-400">
          {checkErr()}
        </div>
      </Show>

      <Show when={check()}>
        {(c) => (
          <div class="mt-3">
            <Show
              when={c().update_available}
              fallback={
                <div class="flex items-center gap-2 rounded-md border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-800 dark:border-emerald-900/40 dark:bg-emerald-950/30 dark:text-emerald-300">
                  <CircleCheck size={15} />
                  <span>
                    Up to date — already on{" "}
                    <span class="font-mono">{c().current_version}</span> (
                    {c().channel} channel).
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
                    disabled={!linuxOnly() || updating()}
                    onClick={applyUpdate}
                  >
                    Update now
                  </Button>
                  <span class="text-xs text-amber-800/70 dark:text-amber-300/70">
                    Asset:{" "}
                    <span class="font-mono">{c().asset_name || "n/a"}</span>
                  </span>
                </div>
              </div>
            </Show>
          </div>
        )}
      </Show>

      <Show when={updating()}>
        <div class="mt-3 flex items-center gap-2 rounded-md border border-sky-200 bg-sky-50 p-3 text-sm text-sky-800 dark:border-sky-900/40 dark:bg-sky-950/30 dark:text-sky-300">
          <Loader2 size={15} class="animate-spin" />
          <span>Updating… waiting for the relay to come back online.</span>
        </div>
      </Show>

      <Show when={!updating() && updateMsg()}>
        <div class="mt-3 flex items-center gap-2 rounded-md border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-800 dark:border-emerald-900/40 dark:bg-emerald-950/30 dark:text-emerald-300">
          <CircleCheck size={15} />
          <span>{updateMsg()}</span>
        </div>
      </Show>

      <Show when={updateErr()}>
        <div class="mt-3 rounded-md border border-rose-200 bg-rose-50 p-2 text-xs text-rose-700 dark:border-rose-900/50 dark:bg-rose-950/30 dark:text-rose-400">
          {updateErr()}
        </div>
      </Show>
    </Card>
  );
}
