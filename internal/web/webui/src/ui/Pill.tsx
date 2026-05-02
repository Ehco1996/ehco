import { JSX } from "solid-js";

type Tone = "neutral" | "ok" | "info" | "warn" | "error" | "accent";

const tones: Record<Tone, string> = {
  neutral:
    "bg-zinc-100 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-300",
  ok: "bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400",
  info: "bg-sky-50 text-sky-700 dark:bg-sky-500/10 dark:text-sky-400",
  warn: "bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-400",
  error: "bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-400",
  accent:
    "bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400",
};

export function Pill(props: {
  tone?: Tone;
  dot?: boolean;
  pulse?: boolean;
  children: JSX.Element;
}) {
  const tone = () => props.tone ?? "neutral";
  return (
    <span
      class={`inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[11px] font-medium leading-none ${tones[tone()]}`}
    >
      {props.dot && (
        <span
          class={`h-1.5 w-1.5 rounded-full bg-current ${props.pulse ? "pulse-dot" : ""}`}
        />
      )}
      {props.children}
    </span>
  );
}
