import { createResource, createSignal, For, onCleanup, Show } from "solid-js";
import { useSearchParams } from "@solidjs/router";
import { Pause, Play, RefreshCcw, Cable } from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import Toolbar from "../ui/Toolbar";
import Button from "../ui/Button";
import { Input, Select } from "../ui/Input";
import { Pill } from "../ui/Pill";
import { TableScroll, tableClasses as t } from "../ui/Table";
import EmptyState from "../ui/EmptyState";
import { Card } from "../ui/Card";
import { api } from "../api/client";
import { relTime } from "../util/format";
import { ipKind, ipKindLabel, ipKindTone } from "../util/ip";
import type { XrayConn } from "../api/types";

const REFRESH_MS = 2000;

export default function XrayConns() {
  const [params, setParams] = useSearchParams<{ user?: string; net?: string }>();
  const [paused, setPaused] = createSignal(false);
  const [busy, setBusy] = createSignal<number | "user" | null>(null);
  const [tick, setTick] = createSignal(0);

  const [data, { refetch }] = createResource<XrayConn[]>(() => api.xrayConns());

  const iv = window.setInterval(() => {
    if (paused() || document.visibilityState !== "visible") return;
    setTick((t) => t + 1);
    refetch();
  }, REFRESH_MS);
  onCleanup(() => window.clearInterval(iv));

  const filterUser = () => params.user ?? "";
  const filterNet = () => params.net ?? "";

  const rows = () => {
    const all = data() ?? [];
    const u = filterUser().trim();
    const n = filterNet().trim().toLowerCase();
    return all
      .filter((c) => !u || String(c.user_id) === u)
      .filter((c) => !n || c.network.toLowerCase() === n)
      .sort((a, b) => (a.since < b.since ? 1 : -1));
  };

  const killOne = async (id: number) => {
    setBusy(id);
    try {
      await api.killConn(id);
      await refetch();
    } finally {
      setBusy(null);
    }
  };

  const killUser = async () => {
    const u = filterUser().trim();
    if (!u) return;
    if (!confirm(`Kill all conns for user_id=${u}?`)) return;
    setBusy("user");
    try {
      await api.killUser(Number(u));
      await refetch();
    } finally {
      setBusy(null);
    }
  };

  return (
    <>
      <PageHeader
        title="Xray Connections"
        subtitle="Live conns from the in-process xray outbound. Auto-refresh every 2s."
        actions={
          <>
            <Pill tone={paused() ? "neutral" : "ok"} dot pulse={!paused()}>
              {paused() ? "paused" : `live · ${tick()}`}
            </Pill>
            <Button
              size="sm"
              leadingIcon={paused() ? <Play size={13} /> : <Pause size={13} />}
              onClick={() => setPaused(!paused())}
            >
              {paused() ? "Resume" : "Pause"}
            </Button>
            <Button
              size="sm"
              leadingIcon={<RefreshCcw size={13} />}
              onClick={() => refetch()}
            >
              Refresh
            </Button>
          </>
        }
      />

      <Toolbar>
        <Input
          mono
          placeholder="user_id"
          class="w-28"
          value={filterUser()}
          onInput={(e) =>
            setParams({ ...params, user: e.currentTarget.value || undefined })
          }
        />
        <Select
          value={filterNet()}
          onChange={(e) =>
            setParams({ ...params, net: e.currentTarget.value || undefined })
          }
        >
          <option value="">all networks</option>
          <option value="tcp">tcp</option>
          <option value="udp">udp</option>
        </Select>
        <Button
          variant="danger"
          size="sm"
          disabled={!filterUser() || busy() === "user"}
          onClick={killUser}
        >
          Kill by user
        </Button>
        <span class="ml-auto text-xs text-zinc-500">
          {rows().length} of {data()?.length ?? 0}
        </span>
      </Toolbar>

      {/* Desktop table */}
      <TableScroll class="hidden md:block">
        <table class={t.table}>
          <thead class={t.thead}>
            <tr>
              <th class={t.th}>id</th>
              <th class={t.th}>user</th>
              <th class={t.th}>net</th>
              <th class={t.th}>source</th>
              <th class={t.th}>target</th>
              <th class={t.th}>age</th>
              <th class={t.th + " text-right"} />
            </tr>
          </thead>
          <tbody class={t.tbody}>
            <Show
              when={rows().length}
              fallback={
                <tr>
                  <td colspan={7}>
                    <EmptyState
                      icon={<Cable size={28} />}
                      title="No live connections"
                      hint="The table will populate when xray accepts inbound traffic."
                    />
                  </td>
                </tr>
              }
            >
              <For each={rows()}>
                {(c) => (
                  <tr class={t.tr}>
                    <td class={t.td + " font-mono text-xs text-zinc-500"}>
                      {c.id}
                    </td>
                    <td class={t.td + " font-mono"}>{c.user_id}</td>
                    <td class={t.td}>
                      <Pill tone={c.network === "tcp" ? "info" : "accent"}>
                        {c.network}
                      </Pill>
                    </td>
                    <td class={t.td}>
                      <div class="flex items-center gap-1.5">
                        <Pill tone={ipKindTone[ipKind(c.source_ip)]}>
                          {ipKindLabel[ipKind(c.source_ip)]}
                        </Pill>
                        <span class="font-mono text-xs">{c.source_ip}</span>
                      </div>
                    </td>
                    <td
                      class={t.td + " max-w-[280px] truncate font-mono text-xs"}
                    >
                      {c.target}
                    </td>
                    <td class={t.td + " text-xs text-zinc-500"}>
                      {relTime(c.since)}
                    </td>
                    <td class={t.td + " text-right"}>
                      <Button
                        variant="danger"
                        size="sm"
                        disabled={busy() === c.id}
                        onClick={() => killOne(c.id)}
                      >
                        kill
                      </Button>
                    </td>
                  </tr>
                )}
              </For>
            </Show>
          </tbody>
        </table>
      </TableScroll>

      {/* Mobile card list */}
      <div class="flex flex-col gap-2 md:hidden">
        <Show
          when={rows().length}
          fallback={
            <Card>
              <EmptyState
                icon={<Cable size={28} />}
                title="No live connections"
                hint="The list will populate when xray accepts inbound traffic."
              />
            </Card>
          }
        >
          <For each={rows()}>
            {(c) => (
              <div class="rounded-xl border border-zinc-200 bg-white p-3 dark:border-zinc-800 dark:bg-zinc-900">
                <div class="flex items-start justify-between gap-2">
                  <div class="min-w-0">
                    <div class="flex items-center gap-2">
                      <Pill tone={c.network === "tcp" ? "info" : "accent"}>
                        {c.network}
                      </Pill>
                      <span class="font-mono text-sm">{c.user_id}</span>
                      <span class="text-xs text-zinc-500">
                        #{c.id}
                      </span>
                    </div>
                    <div class="mt-1 truncate font-mono text-xs text-zinc-700 dark:text-zinc-300">
                      {c.target}
                    </div>
                    <div class="mt-0.5 flex items-center gap-1.5 text-xs text-zinc-500">
                      <Pill tone={ipKindTone[ipKind(c.source_ip)]}>
                        {ipKindLabel[ipKind(c.source_ip)]}
                      </Pill>
                      <span class="font-mono">{c.source_ip}</span>
                      <span>·</span>
                      <span>{relTime(c.since)}</span>
                    </div>
                  </div>
                  <Button
                    variant="danger"
                    size="sm"
                    disabled={busy() === c.id}
                    onClick={() => killOne(c.id)}
                  >
                    kill
                  </Button>
                </div>
              </div>
            )}
          </For>
        </Show>
      </div>
    </>
  );
}
