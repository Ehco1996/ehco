import { JSX } from "solid-js";

export default function Toolbar(props: { children: JSX.Element }) {
  return (
    <div class="mb-3 flex flex-wrap items-center gap-2">{props.children}</div>
  );
}
