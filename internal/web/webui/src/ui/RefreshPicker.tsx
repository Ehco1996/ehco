import { RefreshCcw } from "lucide-solid";
import { Pill } from "./Pill";
import Button from "./Button";
import { REFRESH_OPTIONS, PollingHandle } from "../util/polling";

export default function RefreshPicker(props: {
  handle: PollingHandle;
  /** Optional label shown next to the interval picker. */
  liveLabel?: string;
}) {
  const off = () => props.handle.interval() === 0;
  return (
    <>
      <Pill tone={off() ? "neutral" : "ok"} dot pulse={!off()}>
        {off()
          ? "paused"
          : `${props.liveLabel ?? "live"} · ${props.handle.tick()}`}
      </Pill>
      <select
        class="h-8 rounded-md border border-zinc-200 bg-white px-2 text-xs focus:border-emerald-500 focus:outline-none dark:border-zinc-800 dark:bg-zinc-900"
        value={props.handle.interval()}
        onChange={(e) => props.handle.setInterval(Number(e.currentTarget.value))}
        title="Auto-refresh interval"
      >
        {REFRESH_OPTIONS.map((o) => (
          <option value={o.value}>{o.label}</option>
        ))}
      </select>
      <Button
        size="sm"
        leadingIcon={<RefreshCcw size={13} />}
        onClick={() => props.handle.trigger()}
      >
        Refresh
      </Button>
    </>
  );
}
