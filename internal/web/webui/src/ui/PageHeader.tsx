import { JSX } from "solid-js";

export default function PageHeader(props: {
  title: string;
  subtitle?: string;
  actions?: JSX.Element;
}) {
  return (
    <div class="mb-4 flex flex-col gap-3 border-b border-zinc-200 pb-3 sm:mb-5 sm:flex-row sm:items-end sm:justify-between sm:gap-4 dark:border-zinc-800">
      <div class="min-w-0">
        <h1 class="text-[18px] font-semibold tracking-tight sm:text-[20px]">
          <span class="text-emerald-600 dark:text-emerald-400">{">"}</span>{" "}
          {props.title}
        </h1>
        {props.subtitle && (
          <p class="mt-1 text-[12px] text-zinc-500">{props.subtitle}</p>
        )}
      </div>
      {props.actions && (
        <div class="-mx-1 flex flex-wrap items-center gap-2 px-1 sm:mx-0 sm:px-0">
          {props.actions}
        </div>
      )}
    </div>
  );
}
