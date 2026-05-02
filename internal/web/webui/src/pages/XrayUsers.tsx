import { createEffect, createMemo, createResource, createSignal } from "solid-js";
import { useNavigate } from "@solidjs/router";
import { Users as UsersIcon, ArrowRight } from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import Toolbar from "../ui/Toolbar";
import Button from "../ui/Button";
import { Input } from "../ui/Input";
import { Pill } from "../ui/Pill";
import EmptyState from "../ui/EmptyState";
import Sparkline from "../ui/Sparkline";
import Segmented from "../ui/Segmented";
import DataTable, { Column } from "../ui/DataTable";
import RefreshPicker from "../ui/RefreshPicker";
import { api } from "../api/client";
import { bytes } from "../util/format";
import { usePolling } from "../util/polling";
import { recordUserSnapshot, userSamples } from "../store/userTrafficHistory";
import type { XrayUser } from "../api/types";

type View = "active" | "running" | "all" | "disabled";

const IDLE_BYTES = 1024; // ≤1KB up+down counts as idle (probe noise)

export default function XrayUsers() {
  const [data, { refetch }] = createResource(() => api.xrayUsers());
  const [filter, setFilter] = createSignal("");
  const [view, setView] = createSignal<View>("active");
  const nav = useNavigate();

  const poll = usePolling(() => refetch(), { defaultSec: 15 });

  createEffect(() => {
    const d = data();
    if (d) recordUserSnapshot(d);
  });

  const counts = createMemo(() => {
    const all = data() ?? [];
    let active = 0;
    let running = 0;
    let disabled = 0;
    for (const u of all) {
      if (!u.enable) {
        disabled++;
        continue;
      }
      if (u.running) running++;
      const recent = userSamples(u.user_id).reduce((a, b) => a + b, 0);
      if (
        recent > 0 ||
        u.tcp_conn_count > 0 ||
        u.upload_total + u.download_total > IDLE_BYTES
      ) {
        active++;
      }
    }
    return { all: all.length, active, running, disabled };
  });

  const rows = createMemo(() => {
    const all = data() ?? [];
    const f = filter().trim();
    const v = view();
    let r = all;
    if (f) r = r.filter((u) => String(u.user_id).includes(f));
    if (v === "running") {
      r = r.filter((u) => u.running);
    } else if (v === "disabled") {
      r = r.filter((u) => !u.enable);
    } else if (v === "active") {
      r = r.filter((u) => {
        if (!u.enable) return false;
        const recent = userSamples(u.user_id).reduce((a, b) => a + b, 0);
        return (
          recent > 0 ||
          u.tcp_conn_count > 0 ||
          u.upload_total + u.download_total > IDLE_BYTES
        );
      });
    }
    return r;
  });

  const userStatus = (
    enable: boolean,
    running: boolean,
  ): { tone: "ok" | "warn" | "neutral"; label: string } => {
    if (!enable) return { tone: "neutral", label: "disabled" };
    if (running) return { tone: "ok", label: "running" };
    return { tone: "warn", label: "enabled" };
  };

  const columns: Column<XrayUser>[] = [
    {
      key: "user_id",
      header: "user",
      cell: (u) => <span class="font-mono">{u.user_id}</span>,
      sortable: true,
      sortBy: (u) => u.user_id,
      width: "90px",
    },
    {
      key: "protocol",
      header: "proto",
      cell: (u) => <Pill>{u.protocol || "—"}</Pill>,
      sortable: true,
      sortBy: (u) => u.protocol ?? "",
      mdOnly: true,
    },
    {
      key: "status",
      header: "status",
      cell: (u) => {
        const s = userStatus(u.enable, u.running);
        return (
          <Pill tone={s.tone} dot pulse={s.tone === "ok"}>
            {s.label}
          </Pill>
        );
      },
      sortable: true,
      sortBy: (u) => (!u.enable ? 0 : u.running ? 2 : 1),
    },
    {
      key: "up",
      header: "↑ up",
      align: "right",
      cell: (u) => <span class="font-mono">{bytes(u.upload_total)}</span>,
      sortable: true,
      sortBy: (u) => u.upload_total,
    },
    {
      key: "down",
      header: "↓ down",
      align: "right",
      cell: (u) => <span class="font-mono">{bytes(u.download_total)}</span>,
      sortable: true,
      sortBy: (u) => u.download_total,
    },
    {
      key: "tcp",
      header: "tcp",
      align: "right",
      cell: (u) => <span class="font-mono">{u.tcp_conn_count}</span>,
      sortable: true,
      sortBy: (u) => u.tcp_conn_count,
      width: "60px",
    },
    {
      key: "spark",
      header: "5m trend",
      cell: (u) => (
        <span class="text-emerald-600 dark:text-emerald-400">
          <Sparkline values={userSamples(u.user_id)} width={80} height={20} />
        </span>
      ),
      mdOnly: true,
    },
    {
      key: "ips",
      header: "recent ips",
      mdOnly: true,
      cell: (u) => (
        <span class="block max-w-[220px] truncate font-mono text-xs text-zinc-500">
          {u.recent_ips.length ? u.recent_ips.join(", ") : "—"}
        </span>
      ),
    },
    {
      key: "actions",
      header: "",
      align: "right",
      width: "90px",
      cell: (u) => (
        <Button
          size="sm"
          onClick={(e) => {
            e.stopPropagation();
            nav(`/xray/conns?user=${u.user_id}`);
          }}
        >
          conns <ArrowRight size={13} />
        </Button>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        title="Xray Users"
        subtitle="Cumulative byte counters since process boot. Reset on restart."
        actions={<RefreshPicker handle={poll} />}
      />

      <Toolbar>
        <Segmented<View>
          options={[
            { value: "active", label: `Active · ${counts().active}` },
            { value: "running", label: `Running · ${counts().running}` },
            { value: "all", label: `All · ${counts().all}` },
            { value: "disabled", label: `Disabled · ${counts().disabled}` },
          ]}
          value={view()}
          onChange={setView}
        />
        <Input
          mono
          placeholder="filter user_id"
          class="w-40"
          value={filter()}
          onInput={(e) => setFilter(e.currentTarget.value)}
        />
        <span class="ml-auto text-xs text-zinc-500 tabular-nums">
          {rows().length} of {data()?.length ?? 0}
        </span>
      </Toolbar>

      <DataTable<XrayUser>
        rows={rows()}
        columns={columns}
        rowKey={(u) => u.user_id}
        pageSize={50}
        defaultSort={{ key: "down", dir: "desc" }}
        onRowClick={(u) => nav(`/xray/conns?user=${u.user_id}`)}
        empty={
          <EmptyState
            icon={<UsersIcon size={28} />}
            title={
              view() === "active"
                ? "No users with traffic right now"
                : "No users match the current filter"
            }
            hint={
              view() === "active"
                ? `Switch to "All" to see the ${data()?.length ?? 0} known users.`
                : "Try clearing the user_id filter."
            }
          />
        }
      />
    </>
  );
}
