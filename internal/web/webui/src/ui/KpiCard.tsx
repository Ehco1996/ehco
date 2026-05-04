import { JSX } from "solid-js";

export default function KpiCard(props: {
  label: string;
  value: JSX.Element;
  hint?: JSX.Element;
  icon?: JSX.Element;
}) {
  return (
    <div class="rounded-md border border-zinc-200 bg-white p-3 sm:p-4 dark:border-zinc-800 dark:bg-zinc-900">
      <div class="flex items-center justify-between text-[10px] font-semibold uppercase tracking-[0.16em] text-zinc-500">
        <span>{props.label}</span>
        {props.icon && <span class="text-zinc-400">{props.icon}</span>}
      </div>
      <div class="mt-2 font-mono text-[20px] font-semibold tabular-nums tracking-tight sm:text-[24px]">
        {props.value}
      </div>
      {props.hint && (
        <div class="mt-1 truncate text-[11px] text-zinc-500">{props.hint}</div>
      )}
    </div>
  );
}
