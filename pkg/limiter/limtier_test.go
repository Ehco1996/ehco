package limiter

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestIPRateLimiter_CanServe(t *testing.T) {
	ipr := NewIPRateLimiter(rate.Limit(1), 1) // 1/s 处理一个请求

	ip1 := "1.1.1.1"
	ip2 := "1.2.2.2"

	if !ipr.CanServe(ip1) {
		t.Errorf("IPRateLimiter can't server ip=%s", ip1)
	}

	if ipr.CanServe(ip1) {
		t.Errorf("IPRateLimiter can server ip=%s in limit time", ip1)
	}

	if !ipr.CanServe(ip2) {
		t.Errorf("IPRateLimiter can't server ip=%s different ip should not affects each other", ip1)
	}

	time.Sleep(time.Second)
	if !ipr.CanServe(ip1) {
		t.Errorf("IPRateLimiter can't server ip=%s after sleep", ip1)
	}
}
