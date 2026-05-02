import { createResource, createSignal, onCleanup, Show } from "solid-js";
import {
  Cable,
  Users as UsersIcon,
  ServerCog,
  ArrowDownUp,
  Cpu,
  MemoryStick,
  Download,
  Upload,
} from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import KpiCard from "../ui/KpiCard";
import { Card, CardHeader } from "../ui/Card";
import DescList from "../ui/DescList";
import { Pill } from "../ui/Pill";
import { api } from "../api/client";
import { bytes, pct } from "../util/format";

export default function Overview() {
  const [config] = createResource(() => api.config());
  const [conns, { refetch: rcConns }] = createResource(() => api.xrayConns());
  const [users, { refetch: rcUsers }] = createResource(() => api.xrayUsers());
  const [node, { refetch: rcNode }] = createResource(() =>
    api.nodeMetrics({ latest: true }),
  );

  const [tick, setTick] = createSignal(0);
  const iv = window.setInterval(() => {
    if (document.visibilityState !== "visible") return;
    setTick((t) => t + 1);
    rcConns();
    rcUsers();
    rcNode();
  }, 5000);
  onCleanup(() => window.clearInterval(iv));

  const totalUp = () =>
    (users() ?? []).reduce((a, u) => a + u.upload_total, 0);
  const totalDown = () =>
    (users() ?? []).reduce((a, u) => a + u.download_total, 0);
  const latestNode = () => node()?.data?.[node()!.data.length - 1];
  const ruleCount = () =>
    Array.isArray(config()?.relay_configs) ? config()!.relay_configs!.length : 0;
  const runningUsers = () => (users() ?? []).filter((u) => u.running).length;

  return (
    <>
      <PageHeader
        title="Overview"
        subtitle="Live snapshot of the relay frontend and embedded xray-core."
        actions={
          <Pill tone="ok" dot pulse>
            live · {tick()}
          </Pill>
        }
      />

      <div class="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
        <KpiCard
          label="Active conns"
          icon={<Cable size={14} />}
          value={conns()?.length ?? "—"}
          hint={`${runningUsers()} users running`}
        />
        <KpiCard
          label="Known users"
          icon={<UsersIcon size={14} />}
          value={users()?.length ?? "—"}
          hint={`${(users() ?? []).filter((u) => u.enable).length} enabled`}
        />
        <KpiCard
          label="Relay rules"
          icon={<ServerCog size={14} />}
          value={ruleCount()}
          hint={
            config()
              ? `:${config()!.web_port} · ${config()!.log_level ?? "info"}`
              : ""
          }
        />
        <KpiCard
          label="Total xfer"
          icon={<ArrowDownUp size={14} />}
          value={bytes(totalUp() + totalDown())}
          hint={
            <span class="inline-flex gap-3 font-mono">
              <span class="inline-flex items-center gap-1">
                <Upload size={11} /> {bytes(totalUp())}
              </span>
              <span class="inline-flex items-center gap-1">
                <Download size={11} /> {bytes(totalDown())}
              </span>
            </span>
          }
        />
      </div>

      <Show when={latestNode()}>
        <div class="mt-4 grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
          <KpiCard
            label="CPU"
            icon={<Cpu size={14} />}
            value={pct(latestNode()!.cpu_usage)}
          />
          <KpiCard
            label="Memory"
            icon={<MemoryStick size={14} />}
            value={pct(latestNode()!.memory_usage)}
          />
          <KpiCard
            label="Net in"
            icon={<Download size={14} />}
            value={bytes(latestNode()!.network_in)}
          />
          <KpiCard
            label="Net out"
            icon={<Upload size={14} />}
            value={bytes(latestNode()!.network_out)}
          />
        </div>
      </Show>

      <Show when={config()}>
        <div class="mt-4 grid grid-cols-1 gap-3 lg:grid-cols-2">
          <Card>
            <CardHeader title="Configuration" />
            <DescList
              items={[
                ["log level", String(config()!.log_level ?? "—")],
                ["reload interval", `${config()!.reload_interval ?? 0}s`],
                ["ping", config()!.enable_ping ? "enabled" : "disabled"],
                [
                  "web bind",
                  `${config()!.web_host ?? "0.0.0.0"}:${config()!.web_port ?? "—"}`,
                ],
              ]}
            />
          </Card>
          <Card>
            <CardHeader
              title="Sync endpoint"
              subtitle="Where ehco POSTs traffic stats"
            />
            <p class="break-all rounded-md border border-zinc-200 bg-zinc-50 p-2.5 font-mono text-xs text-zinc-700 dark:border-zinc-800 dark:bg-zinc-950 dark:text-zinc-300">
              {String(config()!.sync_traffic_endpoint ?? "—")}
            </p>
          </Card>
        </div>
      </Show>
    </>
  );
}
