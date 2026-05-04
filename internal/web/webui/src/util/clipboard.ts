// copyText writes `text` to the system clipboard, transparently
// degrading to the legacy execCommand path on plain-HTTP origins where
// `navigator.clipboard` is unavailable.
//
// Background: ehco's admin SPA frequently runs over plain HTTP on a
// LAN IP (e.g. http://192.168.x.x), which is not a secure context.
// Browsers expose `navigator.clipboard` only on HTTPS or localhost, so
// the modern API silently rejects on the most common deployment shape.
// `document.execCommand("copy")` is deprecated but still implemented
// everywhere and works in non-secure contexts.
export async function copyText(text: string): Promise<boolean> {
  if (!text) return false;
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fall through to the legacy path; some browsers reject even
      // in a secure context (permissions, headless, etc).
    }
  }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    ta.style.pointerEvents = "none";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}
