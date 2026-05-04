import { Show, onMount } from "solid-js";
import { HashRouter, Route } from "@solidjs/router";
import { authState, probeAuth } from "./store/auth";
import LoginGate from "./components/LoginGate";
import Layout from "./components/Layout";
import Overview from "./pages/Overview";
import Rules from "./pages/Rules";
import XrayConns from "./pages/XrayConns";
import XrayUsers from "./pages/XrayUsers";
import Logs from "./pages/Logs";
import Settings from "./pages/Settings";
import Updates from "./pages/Updates";
import NodeMetricsPage from "./pages/NodeMetrics";

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
          <Route path="/" component={Overview} />
          <Route path="/rules" component={Rules} />
          <Route path="/xray/conns" component={XrayConns} />
          <Route path="/xray/users" component={XrayUsers} />
          <Route path="/logs" component={Logs} />
          <Route path="/settings" component={Settings} />
          <Route path="/updates" component={Updates} />
          <Route path="/node" component={NodeMetricsPage} />
          <Route path="*" component={Overview} />
        </HashRouter>
      </Show>
    </Show>
  );
}
