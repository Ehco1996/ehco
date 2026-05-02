import { For } from "solid-js";

export default function DescList(props: { items: [string, string][] }) {
  return (
    <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
      <For each={props.items}>
        {([k, v]) => (
          <>
            <dt class="text-xs uppercase tracking-wider text-zinc-500">{k}</dt>
            <dd class="truncate font-mono">{v}</dd>
          </>
        )}
      </For>
    </dl>
  );
}
