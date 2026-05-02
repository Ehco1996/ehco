import { createSignal } from "solid-js";

const STORAGE_KEY = "ehco.token";

const initial = (() => {
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
})();

const [token, setToken] = createSignal(initial);

export { token };

export function saveToken(t: string) {
  if (t) sessionStorage.setItem(STORAGE_KEY, t);
  else sessionStorage.removeItem(STORAGE_KEY);
  setToken(t);
}

export function clearToken() {
  saveToken("");
}
