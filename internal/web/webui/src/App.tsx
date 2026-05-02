import { createSignal, Show, onMount } from "solid-js";
import { HashRouter, Route } from "@solidjs/router";
import { api, ApiError } from "./api/client";
import LoginGate from "./components/LoginGate";
import Layout from "./components/Layout";
import Overview from "./pages/Overview";
import Rules from "./pages/Rules";
import XrayConns from "./pages/XrayConns";
import XrayUsers from "./pages/XrayUsers";
import NodeMetrics from "./pages/NodeMetrics";
import Logs from "./pages/Logs";
import Settings from "./pages/Settings";

type AuthState = "checking" | "needed" | "ok";

export default function App() {
  const [auth, setAuth] = createSignal<AuthState>("checking");

  const probe = async () => {
    try {
      await api.config();
      setAuth("ok");
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        setAuth("needed");
      } else {
        // Other errors (network, 5xx) — let the user in; pages will surface
        // their own errors. Avoids hard-blocking if auth is fine but the
        // server is in a bad state.
        setAuth("ok");
      }
    }
  };

  onMount(probe);

  return (
    <Show
      when={auth() !== "checking"}
      fallback={
        <div class="grid h-full place-items-center text-sm text-zinc-500">
          loading…
        </div>
      }
    >
      <Show when={auth() === "ok"} fallback={<LoginGate onAuthed={() => setAuth("ok")} />}>
        <HashRouter root={Layout}>
          <Route path="/" component={Overview} />
          <Route path="/rules" component={Rules} />
          <Route path="/xray/conns" component={XrayConns} />
          <Route path="/xray/users" component={XrayUsers} />
          <Route path="/host" component={NodeMetrics} />
          <Route path="/logs" component={Logs} />
          <Route path="/settings" component={Settings} />
          <Route path="*" component={Overview} />
        </HashRouter>
      </Show>
    </Show>
  );
}
