import { Show, onMount } from "solid-js";
import { HashRouter, Route, Navigate } from "@solidjs/router";
import { authState, probeAuth } from "./store/auth";
import LoginGate from "./components/LoginGate";
import Layout from "./components/Layout";
import Home from "./pages/Home";
import Rules from "./pages/Rules";
import XrayConns from "./pages/XrayConns";
import XrayUsers from "./pages/XrayUsers";
import Logs from "./pages/Logs";
import Settings from "./pages/Settings";

export default function App() {
  onMount(probeAuth);

  return (
    <Show
      when={authState() !== "checking"}
      fallback={
        <div class="grid h-full place-items-center text-sm text-zinc-500">
          loading…
        </div>
      }
    >
      <Show when={authState() === "ok"} fallback={<LoginGate />}>
        <HashRouter root={Layout}>
          <Route path="/" component={Home} />
          <Route path="/users" component={XrayUsers} />
          <Route path="/conns" component={XrayConns} />
          <Route path="/rules" component={Rules} />
          <Route path="/logs" component={Logs} />
          <Route path="/settings" component={Settings} />
          {/* Legacy paths kept so bookmarks survive the IA shuffle. */}
          <Route path="/xray/users" component={() => <Navigate href="/users" />} />
          <Route path="/xray/conns" component={() => <Navigate href="/conns" />} />
          <Route path="/node" component={() => <Navigate href="/" />} />
          <Route path="/updates" component={() => <Navigate href="/settings" />} />
          <Route path="*" component={() => <Navigate href="/" />} />
        </HashRouter>
      </Show>
    </Show>
  );
}
