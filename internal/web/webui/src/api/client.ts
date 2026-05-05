export class ApiError extends Error {
  status: number;
  constructor(status: number, msg: string) {
    super(msg);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  // credentials: same-origin lets the cookie ride. No headers, no
  // ?token= rewriting — auth is owned by the browser and the server
  // session store now.
  const res = await fetch(path, {
    ...init,
    credentials: "same-origin",
    headers: {
      Accept: "application/json",
      ...(init?.headers ?? {}),
    },
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new ApiError(res.status, text || res.statusText);
  }
  const ctype = res.headers.get("content-type") ?? "";
  if (ctype.includes("application/json")) return (await res.json()) as T;
  return (await res.text()) as unknown as T;
}

import type {
  XrayConn,
  XrayUser,
  EhcoConfig,
  HealthCheckResp,
  QueryNodeMetricsResp,
  VersionInfo,
  UpdateCheck,
  UpdateApplyOptions,
  OverviewResp,
  DBHealth,
  DBMaintenanceResult,
} from "./types";

export const api = {
  config: () => request<EhcoConfig>("/api/v1/config/"),
  reload: () =>
    request<string>("/api/v1/config/reload/", { method: "POST" }),
  healthCheck: (label: string) =>
    request<HealthCheckResp>(
      `/api/v1/health_check/?relay_label=${encodeURIComponent(label)}`,
    ),
  nodeMetrics: (params: { start_ts?: number; end_ts?: number; latest?: boolean; step?: number }) => {
    const q = new URLSearchParams();
    if (params.start_ts != null) q.set("start_ts", String(params.start_ts));
    if (params.end_ts != null) q.set("end_ts", String(params.end_ts));
    if (params.latest) q.set("latest", "true");
    if (params.step && params.step > 1) q.set("step", String(params.step));
    return request<QueryNodeMetricsResp>(`/api/v1/node_metrics/?${q.toString()}`);
  },
  overview: () => request<OverviewResp>("/api/v1/overview"),
  xrayConns: (userId?: number) => {
    const q = userId ? `?user=${userId}` : "";
    return request<XrayConn[]>(`/api/v1/xray/conns${q}`);
  },
  killConn: (id: number) =>
    request<{ killed: number; id: number }>(`/api/v1/xray/conns/${id}`, {
      method: "DELETE",
    }),
  killUser: (userId: number) =>
    request<{ killed: number; user_id: number }>(
      `/api/v1/xray/conns?user=${userId}`,
      { method: "DELETE" },
    ),
  xrayUsers: () => request<XrayUser[]>("/api/v1/xray/users"),
  version: () => request<VersionInfo>("/api/v1/version"),
  updateCheck: (channel: string) =>
    request<UpdateCheck>(`/api/v1/update/check?channel=${encodeURIComponent(channel)}`),
  updateApply: (opts: UpdateApplyOptions) =>
    request<void>("/api/v1/update/apply", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(opts),
    }),
  dbHealth: () => request<DBHealth>("/api/v1/db/health"),
  dbCleanup: (older_than_days: number) =>
    request<DBMaintenanceResult>("/api/v1/db/cleanup", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ older_than_days }),
    }),
  dbVacuum: () =>
    request<DBMaintenanceResult>("/api/v1/db/vacuum", { method: "POST" }),
  dbTruncate: (confirm: string) =>
    request<DBMaintenanceResult>("/api/v1/db/truncate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ confirm }),
    }),
  dbResetStats: () =>
    request<string>("/api/v1/db/reset_stats", { method: "POST" }),
};

export function wsURL(path: string): string {
  // WS upgrades carry the session cookie automatically — no query
  // params, no auth headers (which JS can't set on WS upgrade anyway).
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}${path}`;
}
