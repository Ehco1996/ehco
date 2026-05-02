import { createResource, createSignal, Show } from "solid-js";
import { RefreshCcw, Activity } from "lucide-solid";
import PageHeader from "../ui/PageHeader";
import Button from "../ui/Button";
import { Card, CardHeader } from "../ui/Card";
import EmptyState from "../ui/EmptyState";
import Chart from "../ui/Chart";
import Segmented from "../ui/Segmented";
import { api } from "../api/client";
import { bytes, pct } from "../util/format";

const WINDOWS = [
  { value: 5 * 60, label: "5m" },
  { value: 60 * 60, label: "1h" },
  { value: 6 * 60 * 60, label: "6h" },
  { value: 24 * 60 * 60, label: "24h" },
] as const;

export default function NodeMetrics() {
  const [windowSec, setWindowSec] = createSignal<number>(WINDOWS[1].value);

  const [data, { refetch }] = createResource(windowSec, async (sec) => {
    const end = Math.floor(Date.now() / 1000);
    return api.nodeMetrics({ start_ts: end - sec, end_ts: end });
  });

  const last = () => data()?.data?.[data()!.data.length - 1];

  return (
    <>
      <PageHeader
        title="Host"
        subtitle="System resource utilisation reported by the embedded collector."
        actions={
          <>
            <Segmented
              options={WINDOWS.map((w) => ({ value: w.value, label: w.label }))}
              value={windowSec()}
              onChange={setWindowSec}
            />
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
