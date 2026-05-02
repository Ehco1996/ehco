import { JSX, For } from "solid-js";

export interface SegmentOption<V extends string | number> {
  value: V;
  label: JSX.Element;
  hint?: string;
}

export default function Segmented<V extends string | number>(props: {
  options: SegmentOption<V>[];
  value: V;
  onChange: (v: V) => void;
  size?: "sm" | "md";
}) {
  const size = () => props.size ?? "md";
  return (
    <div class="inline-flex overflow-hidden rounded-md border border-zinc-200 dark:border-zinc-800">
      <For each={props.options}>
        {(o) => (
          <button
            type="button"
            title={o.hint}
            class={
              (size() === "sm" ? "h-7 px-2 text-[11px]" : "h-8 px-2.5 text-xs") +
              " font-medium whitespace-nowrap transition-colors " +
              (props.value === o.value
                ? "bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-400"
                : "text-zinc-600 hover:bg-zinc-50 dark:text-zinc-400 dark:hover:bg-zinc-800")
            }
            onClick={() => props.onChange(o.value)}
          >
            {o.label}
          </button>
        )}
      </For>
    </div>
  );
}
