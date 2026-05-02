import { JSX, splitProps } from "solid-js";

type Variant = "primary" | "secondary" | "ghost" | "danger";
type Size = "sm" | "md";

interface ButtonProps extends JSX.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
  loading?: boolean;
  leadingIcon?: JSX.Element;
}

const base =
  "inline-flex items-center justify-center gap-1.5 rounded-md font-medium whitespace-nowrap transition-colors disabled:cursor-not-allowed disabled:opacity-50";

const sizes: Record<Size, string> = {
  sm: "h-7 px-2.5 text-xs",
  md: "h-8 px-3 text-sm",
};

const variants: Record<Variant, string> = {
  primary:
    "bg-emerald-600 text-white hover:bg-emerald-500 dark:bg-emerald-500 dark:hover:bg-emerald-400 dark:text-zinc-950",
  secondary:
    "border border-zinc-200 bg-white text-zinc-900 hover:bg-zinc-50 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-100 dark:hover:bg-zinc-800",
  ghost:
    "text-zinc-700 hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800",
  danger:
    "border border-rose-300 bg-white text-rose-700 hover:bg-rose-50 dark:border-rose-800/60 dark:bg-zinc-900 dark:text-rose-400 dark:hover:bg-rose-950/40",
};

export default function Button(props: ButtonProps) {
  const [local, rest] = splitProps(props, [
    "variant",
    "size",
    "loading",
    "leadingIcon",
    "class",
    "children",
    "disabled",
  ]);
  const variant = () => local.variant ?? "secondary";
  const size = () => local.size ?? "md";
  return (
    <button
      type="button"
      {...rest}
      disabled={local.disabled || local.loading}
      class={`${base} ${sizes[size()]} ${variants[variant()]} ${local.class ?? ""}`}
    >
      {local.loading ? (
        <span class="h-3 w-3 animate-spin rounded-full border-2 border-current border-r-transparent" />
      ) : (
        local.leadingIcon
      )}
      {local.children}
    </button>
  );
}
