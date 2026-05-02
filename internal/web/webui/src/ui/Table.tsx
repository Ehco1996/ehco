import { JSX } from "solid-js";

/**
 * TableScroll wraps a `<table>` with a horizontal scroll container and a
 * card-style border. Use on desktop; pages typically pair this with a
 * stacked card list for mobile (`md:hidden` / `hidden md:block`).
 */
export function TableScroll(props: { children: JSX.Element; class?: string }) {
  return (
    <div
      class={`scroll-pretty overflow-x-auto rounded-xl border border-zinc-200 dark:border-zinc-800 ${props.class ?? ""}`}
    >
      {props.children}
    </div>
  );
}

export const tableClasses = {
  table: "w-full text-left text-sm",
  thead:
    "bg-zinc-50 text-[11px] font-semibold uppercase tracking-wider text-zinc-500 dark:bg-zinc-900",
  th: "px-3 py-2.5 font-semibold whitespace-nowrap",
  tbody: "divide-y divide-zinc-100 dark:divide-zinc-800",
  tr: "hover:bg-zinc-50/70 dark:hover:bg-zinc-900/50",
  td: "px-3 py-2.5 align-middle",
};
