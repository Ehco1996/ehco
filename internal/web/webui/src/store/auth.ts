import { createSignal } from "solid-js";

export type AuthState = "checking" | "needed" | "ok";

export interface AuthInfo {
  /** Server has a dashboard password configured. When false the SPA
   *  loads straight into the dashboard with no LoginGate. */
  auth_required: boolean;
  /** Whether the request that fetched /auth/info already carried a
   *  valid session cookie or bearer token. */
  authenticated: boolean;
}

const [authState, setAuthState] = createSignal<AuthState>("checking");
const [authInfo, setAuthInfo] = createSignal<AuthInfo>({
  auth_required: false,
  authenticated: false,
});

export { authState, authInfo };

const fetchAuthInfo = async (): Promise<AuthInfo> => {
  try {
    const res = await fetch("/api/v1/auth/info", {
      headers: { Accept: "application/json" },
      credentials: "same-origin",
    });
    if (!res.ok) return { auth_required: false, authenticated: false };
    return (await res.json()) as AuthInfo;
  } catch {
    return { auth_required: false, authenticated: false };
  }
};

/**
 * Boot probe. Hits /auth/info, which both reports whether auth is
 * needed AND whether the current cookie still authenticates. Always
 * lands in "ok" or "needed" so the UI never sticks on "checking".
 */
export async function probeAuth(): Promise<void> {
  setAuthState("checking");
  const info = await fetchAuthInfo();
  setAuthInfo(info);
  if (!info.auth_required || info.authenticated) {
    setAuthState("ok");
  } else {
    setAuthState("needed");
  }
}

export async function signIn(password: string): Promise<string | null> {
  try {
    const res = await fetch("/api/v1/auth/login", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      credentials: "same-origin",
      body: JSON.stringify({ password }),
    });
    if (res.status === 401) {
      return "Wrong password.";
    }
    if (!res.ok) {
      return `HTTP ${res.status}: ${(await res.text().catch(() => "")) || res.statusText}`;
    }
    setAuthInfo({ ...authInfo(), authenticated: true });
    setAuthState("ok");
    return null;
  } catch (e) {
    return String(e);
  }
}

export async function signOut(): Promise<void> {
  try {
    await fetch("/api/v1/auth/logout", {
      method: "POST",
      credentials: "same-origin",
    });
  } catch {
    /* server may already have cleared the session — fall through */
  }
  setAuthInfo({ ...authInfo(), authenticated: false });
  setAuthState("needed");
}
