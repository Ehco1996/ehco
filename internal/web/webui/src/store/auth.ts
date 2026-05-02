import { createSignal } from "solid-js";
import { api, ApiError } from "../api/client";

const STORAGE = {
  token: "ehco.token",
  user: "ehco.user",
  pass: "ehco.pass",
};

export type AuthState = "checking" | "needed" | "ok";

export interface Credentials {
  token: string;
  user: string;
  pass: string;
}

export interface AuthInfo {
  /** Server requires `?token=…` on every request. */
  token: boolean;
  /** Server requires HTTP Basic Auth on every request. */
  basic: boolean;
}

const readInitial = (): Credentials => {
  const out: Credentials = { token: "", user: "", pass: "" };
  try {
    const url = new URL(window.location.href);
    const fromUrl = url.searchParams.get("token");
    if (fromUrl) {
      sessionStorage.setItem(STORAGE.token, fromUrl);
      url.searchParams.delete("token");
      window.history.replaceState({}, "", url.toString());
      out.token = fromUrl;
    } else {
      out.token = sessionStorage.getItem(STORAGE.token) ?? "";
    }
    out.user = sessionStorage.getItem(STORAGE.user) ?? "";
    out.pass = sessionStorage.getItem(STORAGE.pass) ?? "";
  } catch {
    /* sessionStorage unavailable — leave creds blank */
  }
  return out;
};

const persist = (c: Credentials) => {
  try {
    if (c.token) sessionStorage.setItem(STORAGE.token, c.token);
    else sessionStorage.removeItem(STORAGE.token);
    if (c.user) sessionStorage.setItem(STORAGE.user, c.user);
    else sessionStorage.removeItem(STORAGE.user);
    if (c.pass) sessionStorage.setItem(STORAGE.pass, c.pass);
    else sessionStorage.removeItem(STORAGE.pass);
  } catch {
    /* ignore */
  }
};

const [creds, setCredsSig] = createSignal<Credentials>(readInitial());
const [authState, setAuthState] = createSignal<AuthState>("checking");
const [authInfo, setAuthInfo] = createSignal<AuthInfo>({
  token: false,
  basic: false,
});

export { creds, authState, authInfo };

const fetchAuthInfo = async (): Promise<AuthInfo> => {
  try {
    const res = await fetch("/api/v1/auth/info", {
      headers: { Accept: "application/json" },
    });
    if (!res.ok) return { token: false, basic: false };
    return (await res.json()) as AuthInfo;
  } catch {
    return { token: false, basic: false };
  }
};

/**
 * Boot probe. Fetches the server's auth requirements and verifies that
 * the credentials we already have (URL/sessionStorage) actually work.
 * Always lands in either "ok" or "needed" so the UI never sticks.
 */
export async function probeAuth(): Promise<void> {
  setAuthState("checking");
  setAuthInfo(await fetchAuthInfo());
  try {
    await api.config();
    setAuthState("ok");
  } catch (e) {
    if (e instanceof ApiError && (e.status === 401 || e.status === 403)) {
      // Wipe stale creds so the LoginGate starts clean.
      const empty: Credentials = { token: "", user: "", pass: "" };
      persist(empty);
      setCredsSig(empty);
    }
    setAuthState("needed");
  }
}

export async function signIn(input: Partial<Credentials>): Promise<string | null> {
  const next: Credentials = {
    token: (input.token ?? "").trim(),
    user: (input.user ?? "").trim(),
    pass: input.pass ?? "",
  };
  persist(next);
  setCredsSig(next);
  try {
    await api.config();
    setAuthState("ok");
    return null;
  } catch (e) {
    if (e instanceof ApiError && (e.status === 401 || e.status === 403)) {
      return "Credentials rejected. Check ehco's web_token / web_auth_user / web_auth_pass.";
    }
    if (e instanceof ApiError) return `HTTP ${e.status}: ${e.message}`;
    return String(e);
  }
}

export function signOut() {
  const empty: Credentials = { token: "", user: "", pass: "" };
  persist(empty);
  setCredsSig(empty);
  setAuthState("needed");
}
