import { createResource, createSignal, Show } from "solid-js";
import { Activity, Cpu, MemoryStick, HardDrive, Network } from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import KpiCard from "../ui/KpiCard";
import { Card, CardHeader } from "../ui/Card";
import Chart from "../ui/Chart";
import Segmented from "../ui/Segmented";
import RefreshPicker from "../ui/RefreshPicker";
import EmptyState from "../ui/EmptyState";
import { api } from "../api/client";
import { bytes, pct } from "../util/format";
import { usePolling } from "../util/polling";

const WINDOWS = [
  { value: 5 * 60, label: "5m" },
  { value: 60 * 60, label: "1h" },
  { value: 6 * 60 * 60, label: "6h" },
  { value: 24 * 60 * 60, label: "24h" },
] as const;

export default function NodeMetricsPage() {
  const [latest, { refetch: rcLatest }] = createResource(() =>
    api.nodeMetrics({ latest: true }),
  );
  const [windowSec, setWindow] = createSignal<number>(WINDOWS[1].value);
  const [series, { refetch: rcSeries }] = createResource(windowSec, async (sec) => {
    const end = Math.floor(Date.now() / 1000);
    return api.nodeMetrics({ start_ts: end - sec, end_ts: end });
  });

  const poll = usePolling(
    () => {
      rcLatest();
      rcSeries();
    },
    { defaultSec: 5 },
  );

  const last = () => {
    const d = latest()?.data;
    return d && d.length > 0 ? d[d.length - 1] : null;
  };
  const points = () => series()?.data ?? [];

  return (
    <>
      <PageHeader
        title="Node"
        subtitle="Host CPU, memory, disk, and network for this relay box."
        actions={<RefreshPicker handle={poll} />}
      />

      <Show
        when={last()}
        fallback={
          <Card class="mt-2">
            <EmptyState
              icon={<Activity size={28} />}
              title="No samples yet"
              hint="The metrics sampler runs every 5s. If this persists, check that the web server's /metrics/ endpoint is reachable."
            />
          </Card>
        }
      >
        <div class="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
          <KpiCard label="CPU" icon={<Cpu size={14} />} value={pct(last()!.cpu_usage)} />
          <KpiCard label="Memory" icon={<MemoryStick size={14} />} value={pct(last()!.memory_usage)} />
          <KpiCard label="Disk" icon={<HardDrive size={14} />} value={pct(last()!.disk_usage)} />
          <KpiCard
            label="Network ↓"
            icon={<Network size={14} />}
            value={`${bytes(last()!.network_in)}/s`}
            hint={<span class="font-mono">↑ {bytes(last()!.network_out)}/s</span>}
          />
        </div>

        <Card class="mt-4" padded={false}>
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-zinc-200 px-4 py-3 sm:px-5 dark:border-zinc-800">
            <CardHeader title="Time range" subtitle="Sampled every 5s." />
            <Segmented
              options={WINDOWS.map((w) => ({ value: w.value, label: w.label }))}
              value={windowSec()}
              onChange={setWindow}
              size="sm"
            />
          </div>
          <Show
            when={points().length > 1}
            fallback={
              <div class="px-4 py-6 text-center text-xs text-zinc-500 sm:px-5">
                Collecting samples… come back in a moment.
              </div>
            }
          >
            <div class="grid grid-cols-1 gap-3 px-4 py-4 sm:px-5 lg:grid-cols-2">
              <ChartCard title="CPU & Memory">
                <Chart
                  height={180}
                  timestamps={points().map((d) => d.timestamp)}
                  series={[
                    { label: "CPU %", stroke: "#10b981", values: points().map((d) => d.cpu_usage) },
                    { label: "Mem %", stroke: "#6366f1", values: points().map((d) => d.memory_usage) },
                  ]}
                  yFormat={(v) => pct(v)}
                />
              </ChartCard>
              <ChartCard title="Disk">
                <Chart
                  height={180}
                  timestamps={points().map((d) => d.timestamp)}
                  series={[{ label: "Disk %", stroke: "#f59e0b", values: points().map((d) => d.disk_usage) }]}
                  yFormat={(v) => pct(v)}
                />
              </ChartCard>
              <div class="lg:col-span-2">
                <ChartCard title="Network throughput">
                  <Chart
                    height={180}
                    timestamps={points().map((d) => d.timestamp)}
                    series={[
                      { label: "in", stroke: "#0ea5e9", values: points().map((d) => d.network_in) },
                      { label: "out", stroke: "#f97316", values: points().map((d) => d.network_out) },
                    ]}
                    yFormat={(v) => bytes(v) + "/s"}
                  />
                </ChartCard>
              </div>
            </div>
          </Show>
        </Card>
      </Show>
    </>
  );
}

function ChartCard(props: { title: string; children: any }) {
  return (
    <div class="rounded-lg border border-zinc-200 p-3 dark:border-zinc-800">
      <div class="mb-2 text-[11px] font-semibold uppercase tracking-wider text-zinc-500">
        {props.title}
      </div>
      {props.children}
    </div>
  );
}
