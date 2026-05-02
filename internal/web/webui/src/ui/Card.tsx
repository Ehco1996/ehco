import { JSX } from "solid-js";

export function Card(props: {
  children?: JSX.Element;
  class?: string;
  padded?: boolean;
}) {
  const padded = props.padded ?? true;
  return (
    <div
      class={`rounded-xl border border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900 ${padded ? "p-4 sm:p-5" : ""} ${props.class ?? ""}`}
    >
      {props.children}
    </div>
  );
}

export function CardHeader(props: {
  title: string;
  subtitle?: string;
  right?: JSX.Element;
}) {
  return (
    <div class="mb-3 flex items-baseline justify-between gap-3">
      <div class="min-w-0">
        <h3 class="truncate text-sm font-semibold tracking-tight">
          {props.title}
        </h3>
        {props.subtitle && (
          <p class="mt-0.5 truncate text-xs text-zinc-500">{props.subtitle}</p>
        )}
      </div>
      {props.right && (
        <div class="shrink-0 text-xs text-zinc-500">{props.right}</div>
      )}
    </div>
  );
}
