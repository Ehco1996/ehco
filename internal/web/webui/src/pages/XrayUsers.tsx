import { createEffect, createResource, createSignal, For, onCleanup, Show } from "solid-js";
import { useNavigate } from "@solidjs/router";
import { RefreshCcw, Users as UsersIcon, ArrowRight } from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import Toolbar from "../ui/Toolbar";
import Button from "../ui/Button";
import { Input } from "../ui/Input";
import { Pill } from "../ui/Pill";
import { TableScroll, tableClasses as t } from "../ui/Table";
import EmptyState from "../ui/EmptyState";
import { Card } from "../ui/Card";
import Sparkline from "../ui/Sparkline";
import { api } from "../api/client";
import { bytes } from "../util/format";
import { recordUserSnapshot, userSamples } from "../store/userTrafficHistory";

export default function XrayUsers() {
  const [data, { refetch }] = createResource(() => api.xrayUsers());
  const [filter, setFilter] = createSignal("");
  const nav = useNavigate();

  const iv = window.setInterval(() => {
    if (document.visibilityState !== "visible") return;
    refetch();
  }, 5000);
  onCleanup(() => window.clearInterval(iv));

  // Feed each fetched snapshot into the rolling-window store so the
  // sparkline column gains another data point per tick.
  createEffect(() => {
    const d = data();
    if (d) recordUserSnapshot(d);
  });

  const rows = () => {
    const all = data() ?? [];
    const f = filter().trim();
    const filtered = f ? all.filter((u) => String(u.user_id).includes(f)) : all;
    return filtered
      .slice()
      .sort(
        (a, b) =>
          b.upload_total + b.download_total - (a.upload_total + a.download_total),
      );
  };

  const userStatus = (
    enable: boolean,
    running: boolean,
  ): { tone: "ok" | "warn" | "neutral"; label: string } => {
    if (!enable) return { tone: "neutral", label: "disabled" };
    if (running) return { tone: "ok", label: "running" };
    return { tone: "warn", label: "enabled" };
  };

  return (
    <>
      <PageHeader
        title="Xray Users"
        subtitle="Cumulative byte counters since process boot. Reset on restart."
        actions={
          <Button
            size="sm"
            leadingIcon={<RefreshCcw size={13} />}
            onClick={() => refetch()}
          >
            Refresh
          </Button>
        }
      />

      <Toolbar>
        <Input
          mono
          placeholder="filter user_id"
          class="w-40"
          value={filter()}
          onInput={(e) => setFilter(e.currentTarget.value)}
        />
        <span class="ml-auto text-xs text-zinc-500">
          {rows().length} of {data()?.length ?? 0}
        </span>
      </Toolbar>

      {/* Desktop table */}
      <TableScroll class="hidden md:block">
        <table class={t.table}>
          <thead class={t.thead}>
            <tr>
              <th class={t.th}>user</th>
              <th class={t.th}>protocol</th>
              <th class={t.th}>status</th>
              <th class={t.th + " text-right"}>↑ up</th>
              <th class={t.th + " text-right"}>↓ down</th>
              <th class={t.th + " text-right"}>tcp</th>
              <th class={t.th}>recent throughput</th>
              <th class={t.th}>recent ips</th>
              <th class={t.th + " text-right"} />
            </tr>
          </thead>
          <tbody class={t.tbody}>
            <Show
              when={rows().length}
              fallback={
                <tr>
                  <td colspan={9}>
                    <EmptyState
                      icon={<UsersIcon size={28} />}
                      title="No users registered"
                      hint="Users appear here after the upstream config sync runs."
                    />
                  </td>
                </tr>
              }
            >
              <For each={rows()}>
                {(u) => {
                  const s = userStatus(u.enable, u.running);
                  return (
                    <tr class={t.tr}>
                      <td class={t.td + " font-mono"}>{u.user_id}</td>
                      <td class={t.td}>
                        <Pill>{u.protocol || "—"}</Pill>
                      </td>
                      <td class={t.td}>
                        <Pill tone={s.tone} dot pulse={s.tone === "ok"}>
                          {s.label}
                        </Pill>
                      </td>
                      <td class={t.td + " text-right font-mono"}>
                        {bytes(u.upload_total)}
                      </td>
                      <td class={t.td + " text-right font-mono"}>
                        {bytes(u.download_total)}
                      </td>
                      <td class={t.td + " text-right font-mono"}>
                        {u.tcp_conn_count}
                      </td>
                      <td class={t.td + " text-emerald-600 dark:text-emerald-400"}>
                        <Sparkline
                          values={userSamples(u.user_id)}
                          width={80}
                          height={20}
                        />
                      </td>
                      <td class={t.td + " max-w-[220px] truncate font-mono text-xs text-zinc-500"}>
                        {u.recent_ips.length ? u.recent_ips.join(", ") : "—"}
                      </td>
                      <td class={t.td + " text-right"}>
                        <Button
                          size="sm"
                          onClick={() => nav(`/xray/conns?user=${u.user_id}`)}
                        >
                          conns <ArrowRight size={13} />
                        </Button>
                      </td>
                    </tr>
                  );
                }}
              </For>
            </Show>
          </tbody>
        </table>
      </TableScroll>

      {/* Mobile cards */}
      <div class="flex flex-col gap-2 md:hidden">
        <Show
          when={rows().length}
          fallback={
            <Card>
              <EmptyState
                icon={<UsersIcon size={28} />}
                title="No users registered"
                hint="Users appear here after the upstream config sync runs."
              />
            </Card>
          }
        >
          <For each={rows()}>
            {(u) => {
              const s = userStatus(u.enable, u.running);
              return (
                <div
                  class="rounded-xl border border-zinc-200 bg-white p-3 dark:border-zinc-800 dark:bg-zinc-900"
                  onClick={() => nav(`/xray/conns?user=${u.user_id}`)}
                >
                  <div class="flex items-center gap-2">
                    <span class="font-mono text-sm font-semibold">
                      {u.user_id}
                    </span>
                    <Pill>{u.protocol || "—"}</Pill>
                    <Pill tone={s.tone} dot pulse={s.tone === "ok"}>
                      {s.label}
                    </Pill>
                    <span class="ml-auto font-mono text-xs text-zinc-500">
                      tcp {u.tcp_conn_count}
                    </span>
                  </div>
                  <div class="mt-2 flex items-baseline gap-4 font-mono text-xs">
                    <span>↑ {bytes(u.upload_total)}</span>
                    <span>↓ {bytes(u.download_total)}</span>
                  </div>
                  <div class="mt-2 text-emerald-600 dark:text-emerald-400">
                    <Sparkline
                      values={userSamples(u.user_id)}
                      width={300}
                      height={28}
                    />
                  </div>
                  <Show when={u.recent_ips.length}>
                    <div class="mt-1 truncate font-mono text-[11px] text-zinc-500">
                      {u.recent_ips.join(", ")}
                    </div>
                  </Show>
                </div>
              );
            }}
          </For>
        </Show>
      </div>
    </>
  );
}
