import { JSX, createSignal, Show } from "solid-js";
import { A } from "@solidjs/router";
import {
  LayoutDashboard,
  ServerCog,
  Users,
  Cable,
  ScrollText,
  Settings,
  Menu,
  X,
  Sun,
  Moon,
  LogOut,
} from "lucide-solid";
import { theme, toggleTheme } from "../store/theme";
import { authInfo, signOut } from "../store/auth";
import Logo from "../ui/Logo";

const authConfigured = () => authInfo().token || authInfo().basic;

interface NavItem {
  href: string;
  label: string;
  icon: typeof LayoutDashboard;
  end?: boolean;
}

const liveNav: NavItem[] = [
  { href: "/", label: "Overview", icon: LayoutDashboard, end: true },
  { href: "/xray/users", label: "Users", icon: Users },
  { href: "/xray/conns", label: "Conns", icon: Cable },
  { href: "/logs", label: "Logs", icon: ScrollText },
];

const configNav: NavItem[] = [
  { href: "/rules", label: "Rules", icon: ServerCog },
  { href: "/settings", label: "Settings", icon: Settings },
];

// Mobile bottom-bar — five items max for usable touch targets.
const primaryMobile: NavItem[] = [
  { href: "/", label: "Overview", icon: LayoutDashboard, end: true },
  { href: "/xray/users", label: "Users", icon: Users },
  { href: "/xray/conns", label: "Conns", icon: Cable },
  { href: "/logs", label: "Logs", icon: ScrollText },
];
const moreMobile: NavItem[] = configNav;

export default function Layout(props: { children?: JSX.Element }) {
  const [moreOpen, setMoreOpen] = createSignal(false);

  return (
    <div class="flex h-full flex-col md:flex-row">
      {/* ===== Desktop sidebar ===== */}
      <aside class="hidden w-56 shrink-0 flex-col border-r border-zinc-200 bg-white px-3 py-4 md:flex dark:border-zinc-800 dark:bg-zinc-900">
        <Brand />
        <nav class="mt-6 flex flex-1 flex-col gap-4">
          <NavGroup label="Live" items={liveNav} />
          <NavGroup label="Config" items={configNav} />
        </nav>
        <div class="mt-3 flex flex-col gap-0.5 border-t border-zinc-200 pt-3 dark:border-zinc-800">
          <NavButton onClick={toggleTheme} icon={theme() === "dark" ? Sun : Moon}>
            {theme() === "dark" ? "Light mode" : "Dark mode"}
          </NavButton>
          <Show when={authConfigured()}>
            <NavButton onClick={signOut} icon={LogOut}>
              Sign out
            </NavButton>
          </Show>
        </div>
      </aside>

      {/* ===== Mobile top bar ===== */}
      <header class="sticky top-0 z-30 flex h-12 items-center justify-between border-b border-zinc-200 bg-white/95 px-4 backdrop-blur md:hidden dark:border-zinc-800 dark:bg-zinc-900/95">
        <Brand compact />
        <button
          aria-label="theme"
          class="grid h-9 w-9 place-items-center rounded-md text-zinc-600 hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-zinc-800"
          onClick={toggleTheme}
        >
          {theme() === "dark" ? <Sun size={18} /> : <Moon size={18} />}
        </button>
      </header>

      {/* ===== Main content ===== */}
      <main class="scroll-pretty min-w-0 flex-1 overflow-y-auto px-4 pb-24 pt-4 md:px-8 md:pb-8 md:pt-8">
        {props.children}
      </main>

      {/* ===== Mobile bottom tab bar ===== */}
      <nav class="safe-bottom fixed inset-x-0 bottom-0 z-30 flex h-14 items-stretch border-t border-zinc-200 bg-white/95 backdrop-blur md:hidden dark:border-zinc-800 dark:bg-zinc-900/95">
        {primaryMobile.map((l) => (
          <TabLink {...l} />
        ))}
        <button
          class="flex flex-1 flex-col items-center justify-center gap-1 text-[11px] font-medium text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100"
          onClick={() => setMoreOpen(true)}
        >
          <Menu size={18} />
          More
        </button>
      </nav>

      {/* ===== Mobile "more" sheet ===== */}
      <Show when={moreOpen()}>
        <div class="fixed inset-0 z-40 md:hidden">
          <button
            aria-label="close"
            class="absolute inset-0 bg-black/50"
            onClick={() => setMoreOpen(false)}
          />
          <div class="safe-bottom absolute inset-x-0 bottom-0 rounded-t-2xl border-t border-zinc-200 bg-white p-2 dark:border-zinc-800 dark:bg-zinc-900">
            <div class="flex items-center justify-between px-3 pb-2 pt-1">
              <div class="text-sm font-semibold">More</div>
              <button
                aria-label="close"
                class="-m-1 grid h-8 w-8 place-items-center rounded-md text-zinc-500 hover:bg-zinc-100 dark:hover:bg-zinc-800"
                onClick={() => setMoreOpen(false)}
              >
                <X size={18} />
              </button>
            </div>
            <div class="grid grid-cols-3 gap-2 px-2 pb-3 pt-1">
              {moreMobile.map((l) => (
                <SheetTile {...l} onClose={() => setMoreOpen(false)} />
              ))}
              <Show when={authConfigured()}>
                <button
                  class="flex flex-col items-center justify-center gap-1 rounded-xl border border-zinc-200 p-3 text-xs font-medium text-rose-600 hover:bg-rose-50 dark:border-zinc-800 dark:text-rose-400 dark:hover:bg-rose-950/40"
                  onClick={() => {
                    setMoreOpen(false);
                    signOut();
                  }}
                >
                  <LogOut size={20} />
                  Sign out
                </button>
              </Show>
            </div>
          </div>
        </div>
      </Show>
    </div>
  );
}

function NavGroup(props: { label: string; items: NavItem[] }) {
  return (
    <div class="flex flex-col gap-0.5">
      <div class="px-3 pb-1 text-[10px] font-semibold uppercase tracking-wider text-zinc-400">
        {props.label}
      </div>
      {props.items.map((l) => (
        <NavLink {...l} />
      ))}
    </div>
  );
}

function Brand(props: { compact?: boolean }) {
  return (
    <div class="flex items-center gap-2.5">
      <Logo size={props.compact ? 28 : 32} />
      <div class="leading-tight">
        <div class="text-sm font-semibold">ehco</div>
        {!props.compact && (
          <div class="text-[11px] text-zinc-500">admin</div>
        )}
      </div>
    </div>
  );
}

function NavLink(props: NavItem) {
  const Icon = props.icon;
  return (
    <A
      href={props.href}
      end={props.end}
      class="flex items-center gap-2.5 rounded-md px-3 py-1.5 text-sm text-zinc-600 hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-zinc-800"
      activeClass="!bg-emerald-50 !text-emerald-700 dark:!bg-emerald-500/10 dark:!text-emerald-400"
    >
      <Icon size={16} />
      {props.label}
    </A>
  );
}

function NavButton(props: {
  onClick: () => void;
  icon: typeof LayoutDashboard;
  children: JSX.Element;
}) {
  const Icon = props.icon;
  return (
    <button
      onClick={props.onClick}
      class="flex items-center gap-2.5 rounded-md px-3 py-1.5 text-left text-sm text-zinc-600 hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-zinc-800"
    >
      <Icon size={16} />
      {props.children}
    </button>
  );
}

function TabLink(props: NavItem) {
  const Icon = props.icon;
  return (
    <A
      href={props.href}
      end={props.end}
      class="flex flex-1 flex-col items-center justify-center gap-1 text-[11px] font-medium text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100"
      activeClass="!text-emerald-600 dark:!text-emerald-400"
    >
      <Icon size={18} />
      {props.label}
    </A>
  );
}

function SheetTile(props: NavItem & { onClose: () => void }) {
  const Icon = props.icon;
  return (
    <A
      href={props.href}
      onClick={() => props.onClose()}
      class="flex flex-col items-center justify-center gap-1 rounded-xl border border-zinc-200 p-3 text-xs font-medium text-zinc-700 hover:bg-zinc-50 dark:border-zinc-800 dark:text-zinc-300 dark:hover:bg-zinc-800"
      activeClass="!border-emerald-500/50 !text-emerald-700 dark:!text-emerald-400"
    >
      <Icon size={20} />
      {props.label}
    </A>
  );
}
