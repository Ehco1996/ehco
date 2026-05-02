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
  network_in: number;
  network_out: number;
}

export interface QueryNodeMetricsResp {
  total: number;
  data: NodeMetric[];
}

export interface RuleMetric {
  timestamp: number;
  label: string;
  remote: string;
  ping_latency: number;
  tcp_connection_count: number;
  tcp_handshake_duration: number;
  tcp_network_transmit_bytes: number;
  udp_connection_count: number;
  udp_handshake_duration: number;
  udp_network_transmit_bytes: number;
}

export interface QueryRuleMetricsResp {
  total: number;
  data: RuleMetric[];
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
  web_token?: string;
  enable_ping?: boolean;
  log_level?: string;
  reload_interval?: number;
  relay_sync_url?: string;
  relay_configs?: RelayConfig[];
  xray_config?: XrayConfig;
  sync_traffic_endpoint?: string;
  [k: string]: unknown;
}

export interface LogFrame {
  level: string;
  ts?: string;
  logger?: string;
  caller?: string;
  msg: string;
  [k: string]: unknown;
}
