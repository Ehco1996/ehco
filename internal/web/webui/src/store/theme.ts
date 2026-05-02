import { createSignal, createEffect } from "solid-js";

type Theme = "light" | "dark";
const KEY = "ehco.theme";

const initial: Theme = (() => {
  const stored = localStorage.getItem(KEY) as Theme | null;
  if (stored === "light" || stored === "dark") return stored;
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
})();

const [theme, setTheme] = createSignal<Theme>(initial);

createEffect(() => {
  const t = theme();
  document.documentElement.classList.toggle("dark", t === "dark");
  localStorage.setItem(KEY, t);
});

export { theme, setTheme };

export function toggleTheme() {
  setTheme(theme() === "dark" ? "light" : "dark");
}
