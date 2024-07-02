package web

type HealthCheckResp struct {
	ErrorCode int    `json:"error_code"` // code = 0 means success
	Message   string `json:"msg"`
	Latency   int64  `json:"latency"`
}
