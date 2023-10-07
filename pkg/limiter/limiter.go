package limiter

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	GCInterval = time.Minute
)

type IPRateLimiter struct {
	sync.RWMutex

	// key: ip
	previousRateM map[string]*rate.Limiter
	currentRateM  map[string]*rate.Limiter

	limit rate.Limit // 表示每秒可以放入多少个token到桶中
	burst int        // 表示桶容量大小,即同一时刻能取到的最大token数量

	lastGcTime time.Time // 上次gc的时间
}

// NewIPRateLimiter .
func NewIPRateLimiter(limit rate.Limit, burst int) *IPRateLimiter {
	i := &IPRateLimiter{
		previousRateM: make(map[string]*rate.Limiter),
		currentRateM:  make(map[string]*rate.Limiter),
		limit:         limit,
		burst:         burst,
		lastGcTime:    time.Now(),
	}
	return i
}

func (i *IPRateLimiter) GetOreCreateLimiter(ip string) *rate.Limiter {
	i.RLock()
	limiter, exists := i.currentRateM[ip]
	if exists {
		i.RUnlock()
		return limiter
	}
	i.RUnlock()

	i.Lock()
	defer i.Unlock()
	// check again maybe race by another thread
	if limiter, exists := i.currentRateM[ip]; exists {
		return limiter
	}

	// for gc
	if limiter, exists := i.previousRateM[ip]; exists {
		i.currentRateM[ip] = limiter
		delete(i.previousRateM, ip)
		return limiter
	}

	// init new one
	limiter = rate.NewLimiter(i.limit, i.burst)
	i.currentRateM[ip] = limiter
	return limiter
}

func (i *IPRateLimiter) gc() {
	i.Lock()
	defer i.Unlock()
	now := time.Now()
	// todo refine this logger
	fmt.Printf("[IPRateLimiter] gc alive count=%d\n", len(i.currentRateM))
	i.lastGcTime = now
	i.previousRateM = i.currentRateM
	i.currentRateM = make(map[string]*rate.Limiter)
}

func (i *IPRateLimiter) CanServe(ip string) bool {
	ipl := i.GetOreCreateLimiter(ip)
	if time.Since(i.lastGcTime) > GCInterval {
		i.gc()
	}
	return ipl.Allow()
}
