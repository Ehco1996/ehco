import { JSX } from "solid-js";

export default function KpiCard(props: {
  label: string;
  value: JSX.Element;
  hint?: JSX.Element;
  icon?: JSX.Element;
}) {
  return (
    <div class="rounded-xl border border-zinc-200 bg-white p-3 sm:p-4 dark:border-zinc-800 dark:bg-zinc-900">
      <div class="flex items-center justify-between text-[11px] font-medium uppercase tracking-wider text-zinc-500">
        <span>{props.label}</span>
        {props.icon && <span class="text-zinc-400">{props.icon}</span>}
      </div>
      <div class="mt-2 font-mono text-xl font-semibold tracking-tight sm:text-2xl">
        {props.value}
      </div>
      {props.hint && (
        <div class="mt-1 truncate text-xs text-zinc-500">{props.hint}</div>
      )}
    </div>
  );
}
