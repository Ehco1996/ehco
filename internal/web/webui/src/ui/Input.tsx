import { JSX, splitProps } from "solid-js";

const cls =
  "h-8 w-full rounded-md border border-zinc-200 bg-white px-2.5 text-sm placeholder:text-zinc-400 focus:border-emerald-500 focus:outline-none focus:ring-2 focus:ring-emerald-500/30 disabled:opacity-60 dark:border-zinc-800 dark:bg-zinc-900 dark:placeholder:text-zinc-600";

export function Input(
  props: JSX.InputHTMLAttributes<HTMLInputElement> & { mono?: boolean },
) {
  const [local, rest] = splitProps(props, ["class", "mono"]);
  return (
    <input
      {...rest}
      class={`${cls} ${local.mono ? "font-mono" : ""} ${local.class ?? ""}`}
    />
  );
}

export function Select(props: JSX.SelectHTMLAttributes<HTMLSelectElement>) {
  const [local, rest] = splitProps(props, ["class", "children"]);
  return (
    <select {...rest} class={`${cls} pr-7 ${local.class ?? ""}`}>
      {local.children}
    </select>
  );
}
