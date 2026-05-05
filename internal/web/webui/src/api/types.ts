// Shapes returned by ehco's admin API. Mirror the Go structs in
// pkg/xray, internal/cmgr/ms, internal/web. Update both sides when wire
// shapes change — there is no shared schema.

export interface XrayConn {
  id: number;
  user_id: number;
  email: string;
  network: string;
  target: string;
  source_ip: string;
  since: string; // RFC3339
}

export interface XrayUser {
  user_id: number;
  level: number;
  protocol: string;
  method: string;
  enable: boolean;
  running: boolean;
  upload_total: number;
  download_total: number;
  tcp_conn_count: number;
  recent_ips: string[];
}

export interface NodeMetric {
  timestamp: number; // unix seconds
  cpu_usage: number;
  memory_usage: number;
  disk_usage: number;
  network_in: number;  // bytes/sec
  network_out: number; // bytes/sec
}

export interface XraySnapshot {
  conns: number;
  users: number;
  enabled_users: number;
  running_users: number;
  upload_total: number;
  download_total: number;
}

export interface OverviewResp {
  xray?: XraySnapshot;
  host?: NodeMetric;
  rules: number;
  // RFC3339; zero-value omitted by the server. Time of last config
  // reload attempt (file or remote HTTP).
  last_reload_at?: string;
}

export interface QueryNodeMetricsResp {
  total: number;
  data: NodeMetric[];
}

export interface HealthCheckResp {
  error_code: number;
  msg: string;
  latency: number;
}

export interface RelayConfig {
  label?: string;
  listen?: string;
  listen_type?: string;
  transport_type?: string;
  tcp_remotes?: string[];
  udp_remotes?: string[];
  [k: string]: unknown;
}

export interface XrayConfig {
  inbounds?: unknown[];
  outbounds?: unknown[];
  [k: string]: unknown;
}

export interface EhcoConfig {
  web_port?: number;
  web_host?: string;
  // Server-side dashboard / machine credentials. Sent down with the
  // config response; the SPA never displays them, but the index
  // signature catch-all needs them named so consumers don't
  // accidentally treat them as ordinary settings.
  dashboard_pass?: string;
  api_token?: string;
  enable_ping?: boolean;
  log_level?: string;
  reload_interval?: number;
  relay_sync_url?: string;
  relay_configs?: RelayConfig[];
  xray_config?: XrayConfig;
  sync_traffic_endpoint?: string;
  [k: string]: unknown;
}

export interface VersionInfo {
  version: string;
  git_branch: string;
  git_revision: string;
  build_time: string;
  start_time: string; // RFC3339
  go_os: string;
  go_arch: string;
}

export interface UpdateCheck {
  channel: string;
  current_version: string;
  latest_version: string;
  latest_tag: string;
  release_name: string;
  release_body: string;
  release_url: string;
  published_at: string; // RFC3339
  update_available: boolean;
  asset_name: string;
  asset_url: string;
}

export interface UpdateApplyOptions {
  channel: string;
  force: boolean;
  restart: boolean;
}

export interface OpStatsSnapshot {
  count: number;
  avg_ms: number;
  max_ms: number;
  last_ms: number;
}

export interface DBHealth {
  db_file_bytes: number;
  db_page_count: number;
  db_page_size: number;
  db_freelist_pages: number;
  node_metrics_rows: number;
  stats: Record<string, OpStatsSnapshot>;
}

export interface DBMaintenanceResult {
  node_deleted?: number;
  bytes_before?: number;
  bytes_after?: number;
  duration_ms: number;
}

export interface LogFrame {
  level: string;
  ts?: string;
  logger?: string;
  caller?: string;
  msg: string;
  [k: string]: unknown;
}
