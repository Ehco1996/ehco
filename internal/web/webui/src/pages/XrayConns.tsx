import { createMemo, createResource, createSignal, For, Show } from "solid-js";
import { useSearchParams } from "@solidjs/router";
import { Cable, X as XIcon } from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import Toolbar from "../ui/Toolbar";
import Button from "../ui/Button";
import { Input, Select } from "../ui/Input";
import { Pill } from "../ui/Pill";
import EmptyState from "../ui/EmptyState";
import Segmented from "../ui/Segmented";
import DataTable, { Column } from "../ui/DataTable";
import RefreshPicker from "../ui/RefreshPicker";
import { api } from "../api/client";
import { relTime } from "../util/format";
import { ipKind, ipKindLabel, ipKindTone } from "../util/ip";
import { usePolling } from "../util/polling";
import type { XrayConn } from "../api/types";

type GroupMode = "flat" | "user" | "target";

export default function XrayConns() {
  const [params, setParams] = useSearchParams<{
    user?: string;
    net?: string;
    group?: string;
  }>();
  const [busy, setBusy] = createSignal<number | "user" | null>(null);

  const [data, { refetch }] = createResource<XrayConn[]>(() => api.xrayConns());

  const poll = usePolling(() => refetch(), { defaultSec: 5 });

  const filterUser = () => params.user ?? "";
  const filterNet = () => params.net ?? "";
  const groupMode = (): GroupMode => {
    const g = params.group;
    return g === "user" || g === "target" ? g : "flat";
  };

  const filtered = createMemo(() => {
    const all = data() ?? [];
    const u = filterUser().trim();
    const n = filterNet().trim().toLowerCase();
    return all
      .filter((c) => !u || String(c.user_id) === u)
      .filter((c) => !n || c.network.toLowerCase() === n);
  });

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

  const targetHost = (target: string) => {
    // strip :port from "host:port" — keep IPv6 brackets
    const idx = target.lastIndexOf(":");
    if (idx <= 0) return target;
    return target.slice(0, idx);
  };

  const groupedRows = createMemo(() => {
    const mode = groupMode();
    if (mode === "flat") return null;
    const map = new Map<string, XrayConn[]>();
    for (const c of filtered()) {
      const key = mode === "user" ? String(c.user_id) : targetHost(c.target);
      const arr = map.get(key);
      if (arr) arr.push(c);
      else map.set(key, [c]);
    }
    return Array.from(map.entries())
      .map(([key, conns]) => ({ key, conns }))
      .sort((a, b) => b.conns.length - a.conns.length);
  });

  const columns: Column<XrayConn>[] = [
    {
      key: "id",
      header: "id",
      cell: (c) => (
        <span class="font-mono text-xs text-zinc-500">{c.id}</span>
      ),
      sortable: true,
      sortBy: (c) => c.id,
      width: "80px",
      mdOnly: true,
    },
    {
      key: "user",
      header: "user",
      cell: (c) => <span class="font-mono">{c.user_id}</span>,
      sortable: true,
      sortBy: (c) => c.user_id,
      width: "90px",
    },
    {
      key: "net",
      header: "net",
      cell: (c) => (
        <Pill tone={c.network === "tcp" ? "info" : "accent"}>{c.network}</Pill>
      ),
      sortable: true,
      sortBy: (c) => c.network,
      width: "70px",
    },
    {
      key: "source",
      header: "source",
      cell: (c) => (
        <div class="flex items-center gap-1.5">
          <Pill tone={ipKindTone[ipKind(c.source_ip)]}>
            {ipKindLabel[ipKind(c.source_ip)]}
          </Pill>
          <span class="font-mono text-xs">{c.source_ip}</span>
        </div>
      ),
      mdOnly: true,
    },
    {
      key: "target",
      header: "target",
      cell: (c) => (
        <span class="block max-w-[320px] truncate font-mono text-xs">
          {c.target}
        </span>
      ),
      sortable: true,
      sortBy: (c) => c.target,
    },
    {
      key: "age",
      header: "age",
      cell: (c) => (
        <span class="text-xs text-zinc-500">{relTime(c.since)}</span>
      ),
      sortable: true,
      sortBy: (c) => -new Date(c.since).getTime(),
      width: "80px",
    },
    {
      key: "actions",
      header: "",
      align: "right",
      width: "70px",
      cell: (c) => (
        <Button
          variant="danger"
          size="sm"
          disabled={busy() === c.id}
          onClick={(e) => {
            e.stopPropagation();
            killOne(c.id);
          }}
        >
          kill
        </Button>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        title="Xray Connections"
        subtitle="Live conns from the in-process xray outbound."
        actions={<RefreshPicker handle={poll} />}
      />

      <Toolbar>
        <Segmented<GroupMode>
          options={[
            { value: "flat", label: "Flat" },
            { value: "user", label: "By user" },
            { value: "target", label: "By target" },
          ]}
          value={groupMode()}
          onChange={(g) => setParams({ ...params, group: g === "flat" ? undefined : g })}
        />
        <div class="flex items-center gap-1">
          <Input
            mono
            placeholder="user_id"
            class="w-28"
            value={filterUser()}
            onInput={(e) =>
              setParams({ ...params, user: e.currentTarget.value || undefined })
            }
          />
          <Show when={filterUser()}>
            <button
              class="grid h-7 w-7 place-items-center rounded-md text-zinc-500 hover:bg-zinc-100 dark:hover:bg-zinc-800"
              onClick={() => setParams({ ...params, user: undefined })}
              title="clear"
            >
              <XIcon size={12} />
            </button>
          </Show>
        </div>
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
        <span class="ml-auto text-xs text-zinc-500 tabular-nums">
          {filtered().length} of {data()?.length ?? 0}
        </span>
      </Toolbar>

      <Show
        when={groupMode() === "flat"}
        fallback={
          <Show
            when={groupedRows()!.length}
            fallback={
              <div class="rounded-xl border border-zinc-200 bg-white p-6 dark:border-zinc-800 dark:bg-zinc-900">
                <EmptyState
                  icon={<Cable size={28} />}
                  title="No connections match"
                />
              </div>
            }
          >
            <div class="flex flex-col gap-3">
              <For each={groupedRows()!}>
                {(g) => (
                  <div class="rounded-xl border border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
                    <div class="flex items-center justify-between gap-3 border-b border-zinc-200 px-4 py-2 dark:border-zinc-800">
                      <div class="flex items-center gap-2">
                        <span class="text-[10px] font-semibold uppercase tracking-wider text-zinc-500">
                          {groupMode() === "user" ? "user" : "target"}
                        </span>
                        <span class="font-mono text-sm">{g.key}</span>
                        <Pill>{g.conns.length} conns</Pill>
                      </div>
                      <Show when={groupMode() === "user"}>
                        <Button
                          variant="danger"
                          size="sm"
                          disabled={busy() === "user"}
                          onClick={async () => {
                            if (!confirm(`Kill all ${g.conns.length} conns for user_id=${g.key}?`)) return;
                            setBusy("user");
                            try {
                              await api.killUser(Number(g.key));
                              await refetch();
                            } finally {
                              setBusy(null);
                            }
                          }}
                        >
                          kill user
                        </Button>
                      </Show>
                    </div>
                    <DataTable<XrayConn>
                      rows={g.conns}
                      columns={columns}
                      rowKey={(c) => c.id}
                      pageSize={20}
                      defaultSort={{ key: "age", dir: "asc" }}
                      density="compact"
                      empty={<EmptyState title="empty" />}
                    />
                  </div>
                )}
              </For>
            </div>
          </Show>
        }
      >
        <DataTable<XrayConn>
          rows={filtered()}
          columns={columns}
          rowKey={(c) => c.id}
          pageSize={50}
          defaultSort={{ key: "age", dir: "asc" }}
          empty={
            <EmptyState
              icon={<Cable size={28} />}
              title="No live connections"
              hint="The table will populate when xray accepts inbound traffic."
            />
          }
        />
      </Show>
    </>
  );
}
