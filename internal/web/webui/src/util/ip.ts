export type IpKind = "lo" | "lan" | "v6" | "wan" | "unknown";

/**
 * Categorise a source IP into a coarse "kind" so the UI can hint at
 * locality without doing a third-party geoip lookup. Privacy-respecting
 * by design: no fields about the IP leave the browser.
 *
 * - lo  : loopback (127.0.0.0/8 or ::1)
 * - lan : RFC1918 private (10/8, 172.16/12, 192.168/16) or IPv6 ULA fc00::/7
 * - v6  : any other IPv6
 * - wan : public IPv4
 */
export function ipKind(ip: string): IpKind {
  if (!ip) return "unknown";

  if (ip.includes(":")) {
    if (ip === "::1") return "lo";
    const lc = ip.toLowerCase();
    if (lc.startsWith("fc") || lc.startsWith("fd")) return "lan";
    return "v6";
  }

  const parts = ip.split(".");
  if (parts.length !== 4) return "unknown";
  const a = +parts[0];
  const b = +parts[1];
  if (a === 127) return "lo";
  if (a === 10) return "lan";
  if (a === 192 && b === 168) return "lan";
  if (a === 172 && b >= 16 && b <= 31) return "lan";
  if (a === 169 && b === 254) return "lan"; // link-local
  return "wan";
}

export type Tone = "neutral" | "ok" | "info" | "warn" | "error" | "accent";

export const ipKindTone: Record<IpKind, Tone> = {
  lo: "neutral",
  lan: "info",
  v6: "accent",
  wan: "ok",
  unknown: "neutral",
};

/** Short label shown inside the IP-kind pill. */
export const ipKindLabel: Record<IpKind, string> = {
  lo: "lo",
  lan: "lan",
  v6: "v6",
  wan: "wan",
  unknown: "?",
};
