import { JSX } from "solid-js";

export default function EmptyState(props: {
  icon?: JSX.Element;
  title: string;
  hint?: string;
}) {
  return (
    <div class="flex flex-col items-center justify-center gap-2 py-10 text-center">
      {props.icon && <div class="text-zinc-400">{props.icon}</div>}
      <div class="text-sm font-medium text-zinc-700 dark:text-zinc-300">
        {props.title}
      </div>
      {props.hint && (
        <div class="text-xs text-zinc-500">{props.hint}</div>
      )}
    </div>
  );
}
