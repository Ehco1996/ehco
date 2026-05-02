import { creds } from "../store/auth";

export class ApiError extends Error {
  status: number;
  constructor(status: number, msg: string) {
    super(msg);
    this.status = status;
  }
}

function withAuth(path: string): { url: string; headers: Record<string, string> } {
  const c = creds();
  let url = path;
  if (c.token) {
    const u = new URL(path, window.location.origin);
    u.searchParams.set("token", c.token);
    url = u.pathname + u.search;
  }
  const headers: Record<string, string> = {};
  if (c.user || c.pass) {
    headers["Authorization"] = "Basic " + btoa(`${c.user}:${c.pass}`);
  }
  return { url, headers };
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const { url, headers } = withAuth(path);
  const res = await fetch(url, {
    ...init,
    headers: {
      Accept: "application/json",
      ...headers,
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
  QueryRuleMetricsResp,
} from "./types";

export const api = {
  config: () => request<EhcoConfig>("/api/v1/config/"),
  reload: () =>
    request<string>("/api/v1/config/reload/", { method: "POST" }),
  healthCheck: (label: string) =>
    request<HealthCheckResp>(
      `/api/v1/health_check/?relay_label=${encodeURIComponent(label)}`,
    ),
  nodeMetrics: (params: { start_ts?: number; end_ts?: number; latest?: boolean }) => {
    const q = new URLSearchParams();
    if (params.start_ts != null) q.set("start_ts", String(params.start_ts));
    if (params.end_ts != null) q.set("end_ts", String(params.end_ts));
    if (params.latest) q.set("latest", "true");
    return request<QueryNodeMetricsResp>(`/api/v1/node_metrics/?${q.toString()}`);
  },
  ruleMetrics: (params: {
    label?: string;
    remote?: string;
    start_ts?: number;
    end_ts?: number;
    latest?: boolean;
  }) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
      if (v != null && v !== "") q.set(k, String(v));
    }
    return request<QueryRuleMetricsResp>(`/api/v1/rule_metrics/?${q.toString()}`);
  },
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
};

export function wsURL(path: string): string {
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  const c = creds();
  const q = c.token ? `?token=${encodeURIComponent(c.token)}` : "";
  return `${proto}//${window.location.host}${path}${q}`;
}
