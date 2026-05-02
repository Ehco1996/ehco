import { createResource, createSignal, Show } from "solid-js";
import { RefreshCcw, Activity } from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import Button from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import EmptyState from "../ui/EmptyState";
import Chart from "../ui/Chart";
import { api } from "../api/client";
import { bytes, pct } from "../util/format";

const WINDOWS = [
  { label: "5m", seconds: 5 * 60 },
  { label: "1h", seconds: 60 * 60 },
  { label: "6h", seconds: 6 * 60 * 60 },
  { label: "24h", seconds: 24 * 60 * 60 },
];

export default function NodeMetrics() {
  const [windowSec, setWindowSec] = createSignal(WINDOWS[1].seconds);

  const [data, { refetch }] = createResource(windowSec, async (sec) => {
    const end = Math.floor(Date.now() / 1000);
    return api.nodeMetrics({ start_ts: end - sec, end_ts: end });
  });

  const last = () => data()?.data?.[data()!.data.length - 1];

  return (
    <>
      <PageHeader
        title="Node Metrics"
        subtitle="System resource utilisation reported by the embedded collector."
        actions={
          <>
            <div class="inline-flex overflow-hidden rounded-md border border-zinc-200 dark:border-zinc-800">
              {WINDOWS.map((w) => (
                <button
                  class={
                    "h-8 px-2.5 text-xs font-medium transition-colors " +
                    (windowSec() === w.seconds
                      ? "bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-400"
                      : "text-zinc-600 hover:bg-zinc-50 dark:text-zinc-400 dark:hover:bg-zinc-800")
                  }
                  onClick={() => setWindowSec(w.seconds)}
                >
                  {w.label}
                </button>
              ))}
            </div>
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

      <Show
        when={data()?.data?.length}
        fallback={
          <Card>
            <EmptyState
              icon={<Activity size={28} />}
              title={data.loading ? "Loading…" : "No metrics in this window"}
              hint="Try widening the time range or check the collector."
            />
          </Card>
        }
      >
        <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
          <Card>
            <CardHeader
              title="CPU & Memory"
              right={
                <span class="font-mono">
                  CPU {pct(last()!.cpu_usage)} · Mem {pct(last()!.memory_usage)}
                </span>
              }
            />
            <Chart
              timestamps={data()!.data.map((d) => d.timestamp)}
              series={[
                {
                  label: "CPU %",
                  stroke: "#10b981",
                  values: data()!.data.map((d) => d.cpu_usage),
                },
                {
                  label: "Memory %",
                  stroke: "#6366f1",
                  values: data()!.data.map((d) => d.memory_usage),
                },
              ]}
              yFormat={(v) => pct(v)}
            />
          </Card>
          <Card>
            <CardHeader
              title="Network"
              right={
                <span class="font-mono">
                  in {bytes(last()!.network_in)} · out{" "}
                  {bytes(last()!.network_out)}
                </span>
              }
            />
            <Chart
              timestamps={data()!.data.map((d) => d.timestamp)}
              series={[
                {
                  label: "in",
                  stroke: "#0ea5e9",
                  values: data()!.data.map((d) => d.network_in),
                },
                {
                  label: "out",
                  stroke: "#f97316",
                  values: data()!.data.map((d) => d.network_out),
                },
              ]}
              yFormat={(v) => bytes(v)}
            />
          </Card>
        </div>
      </Show>
    </>
  );
}
