import { createSignal } from "solid-js";
import { api, ApiError } from "../api/client";

const STORAGE_KEY = "ehco.token";

export type AuthState = "checking" | "needed" | "ok";

const readInitialToken = (): string => {
  try {
    const url = new URL(window.location.href);
    const fromUrl = url.searchParams.get("token");
    if (fromUrl) {
      sessionStorage.setItem(STORAGE_KEY, fromUrl);
      url.searchParams.delete("token");
      window.history.replaceState({}, "", url.toString());
      return fromUrl;
    }
    return sessionStorage.getItem(STORAGE_KEY) ?? "";
  } catch {
    return "";
  }
};

const persist = (t: string) => {
  try {
    if (t) sessionStorage.setItem(STORAGE_KEY, t);
    else sessionStorage.removeItem(STORAGE_KEY);
  } catch {
    /* ignore — private mode etc. */
  }
};

const [token, setTokenSig] = createSignal(readInitialToken());
const [authState, setAuthState] = createSignal<AuthState>("checking");

export { token, authState };

/**
 * Run once at app boot. Probes /api/v1/config/ with whatever token we
 * already have (URL or sessionStorage) and decides between "ok" and
 * "needed". Network errors keep us in "checking" briefly and then fall
 * to "needed" so the user has a way back in instead of seeing a blank
 * dashboard.
 */
export async function probeAuth(): Promise<void> {
  setAuthState("checking");
  try {
    await api.config();
    setAuthState("ok");
  } catch (e) {
    if (e instanceof ApiError && (e.status === 401 || e.status === 403)) {
      // Bad/missing token. Clear it so the LoginGate input starts empty
      // and the user isn't fighting a stale value.
      persist("");
      setTokenSig("");
      setAuthState("needed");
    } else {
      // Server reachable but unhappy (5xx, network blip). Surface the
      // login screen rather than letting the user into a broken app —
      // they can retry from there.
      setAuthState("needed");
    }
  }
}

/**
 * Save the supplied token and verify it by probing /config/. Returns an
 * error message on failure; resolves to null on success and flips
 * authState to "ok".
 */
export async function signIn(raw: string): Promise<string | null> {
  const t = raw.trim();
  persist(t);
  setTokenSig(t);
  try {
    await api.config();
    setAuthState("ok");
    return null;
  } catch (e) {
    if (e instanceof ApiError && (e.status === 401 || e.status === 403)) {
      return "Token rejected. Verify ehco's web_token configuration.";
    }
    if (e instanceof ApiError) return `HTTP ${e.status}: ${e.message}`;
    return String(e);
  }
}

/** Clear the token and drop back to the LoginGate without reloading. */
export function signOut() {
  persist("");
  setTokenSig("");
  setAuthState("needed");
}
