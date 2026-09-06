package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/Ehco1996/ehco/internal/store"
)

func main() {
	targetDir := filepath.Join(os.Getenv("HOME"), ".ehco", "metrics_ts")
	_ = os.RemoveAll(targetDir)

	fmt.Println("==========================================================")
	fmt.Println("🚀 ehco Time-Series Storage PoC: tstorage Benchmark & 7-Day Ingestion")
	fmt.Println("==========================================================")
	fmt.Printf("Data storage path: %s\n", targetDir)

	st, err := store.NewStore(targetDir)
	if err != nil {
		fmt.Printf("Failed to initialize Store: %v\n", err)
		return
	}

	ctx := context.Background()

	// 1 week = 7 days
	// Sampling interval: 5 seconds
	totalSeconds := 7 * 24 * 3600
	stepSec := 5
	totalSamples := totalSeconds / stepSec // 120,960 sample groups
	totalPoints := totalSamples * 5        // 5 metrics per sample = 604,800 data points

	now := time.Now()
	startTime := now.Add(-time.Duration(totalSeconds) * time.Second)

	fmt.Printf("\n[1/4] Generating and ingesting 7 days of historical metrics...\n")
	fmt.Printf("Time range: %s ~ %s\n", startTime.Format("2006-01-02 15:04:05"), now.Format("2006-01-02 15:04:05"))
	fmt.Printf("Sample count: %d groups (%d data points covering CPU/Mem/Disk/NetIn/NetOut)\n", totalSamples, totalPoints)

	insertStart := time.Now()

	// Simulate realistic diurnal traffic curve
	r := rand.New(rand.NewSource(42))
	for i := 0; i < totalSamples; i++ {
		curTime := startTime.Add(time.Duration(i*stepSec) * time.Second)
		hourOfDay := float64(curTime.Hour()) + float64(curTime.Minute())/60.0

		// Diurnal factor (0.3 ~ 1.0)
		diurnal := 0.65 + 0.35*math.Sin((hourOfDay-8)/24.0*2*math.Pi)
		jitter := (r.Float64() - 0.5) * 5.0

		cpu := math.Max(5, math.Min(95, 25.0*diurnal+jitter+10.0))
		mem := math.Max(20, math.Min(80, 45.0+5.0*diurnal+jitter*0.2))
		disk := 32.5 + float64(i)/float64(totalSamples)*2.0 // Slight growth over time: 32.5% -> 34.5%
		netIn := math.Max(1024, (5*1024*1024)*diurnal+r.Float64()*1024*1024)
		netOut := math.Max(1024, (8*1024*1024)*diurnal+r.Float64()*1024*1024)

		s := &store.Sample{
			Timestamp:                curTime.Unix(),
			CpuUsagePercent:          cpu,
			MemoryUsagePercent:       mem,
			DiskUsagePercent:         disk,
			NetworkReceiveBytesRate:  netIn,
			NetworkTransmitBytesRate: netOut,
		}

		if err := st.AddNodeMetric(ctx, s); err != nil {
			fmt.Printf("Ingestion failed (i=%d): %v\n", i, err)
			return
		}

		if (i+1)%(totalSamples/5) == 0 || i == totalSamples-1 {
			pct := float64(i+1) / float64(totalSamples) * 100
			fmt.Printf("  -> Ingestion progress: %5.1f%% (%d / %d samples)\n", pct, i+1, totalSamples)
		}
	}

	insertDuration := time.Since(insertStart)
	throughput := float64(totalPoints) / insertDuration.Seconds()

	fmt.Printf("\n✅ Ingestion completed! Total duration: %v\n", insertDuration)
	fmt.Printf("Throughput: %.2f points/sec (%.2f sample groups/sec)\n", throughput, float64(totalSamples)/insertDuration.Seconds())

	// Flush persistence and inspect disk usage
	fmt.Println("\n[2/4] Inspecting disk persistence and space usage...")
	_ = st.Close()

	var totalBytes int64
	var fileCount int
	var partitionCount int
	_ = filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			if info.IsDir() && path != targetDir {
				partitionCount++
			} else if !info.IsDir() {
				fileCount++
				totalBytes += info.Size()
			}
		}
		return nil
	})

	sizeMB := float64(totalBytes) / (1024 * 1024)
	bytesPerPoint := float64(totalBytes) / float64(totalPoints)

	fmt.Printf("  Data directory size: %.2f MB (%d bytes)\n", sizeMB, totalBytes)
	fmt.Printf("  Partitions: %d directories\n", partitionCount)
	fmt.Printf("  Data files: %d files\n", fileCount)
	fmt.Printf("  Average bytes per point: %.2f bytes/point (timestamp + float64 + index)\n", bytesPerPoint)

	// Re-open store and execute query benchmark
	fmt.Println("\n[3/4] Reopening store and benchmarking frontend queries...")
	st2, err := store.NewStore(targetDir)
	if err != nil {
		fmt.Printf("Failed to reopen store: %v\n", err)
		return
	}
	defer func() { _ = st2.Close() }()

	// Scenario 1: Poll latest status (overview 15s polling, Num=1)
	t1 := time.Now()
	rLatest, err := st2.QueryNodeMetric(ctx, &store.QueryNodeMetricsReq{
		StartTimestamp: now.Add(-5 * time.Minute).Unix(),
		EndTimestamp:   now.Unix(),
		Num:            1,
	})
	durLatest := time.Since(t1)
	if err != nil {
		fmt.Printf("Latest point query failed: %v\n", err)
	} else {
		fmt.Printf("  1. Real-time query (last 5m, top 1 point): %v (%d returned)\n", durLatest, rLatest.TOTAL)
		if len(rLatest.Data) > 0 {
			p := rLatest.Data[0]
			fmt.Printf("     [Preview] CPU: %.1f%%, Mem: %.1f%%, NetIn: %.2f MB/s, NetOut: %.2f MB/s\n",
				p.CPUUsage, p.MemoryUsage, p.NetworkIn/1024/1024, p.NetworkOut/1024/1024)
		}
	}

	// Scenario 2: Last 1 hour raw samples (720 raw points)
	t2 := time.Now()
	r1h, err := st2.QueryNodeMetric(ctx, &store.QueryNodeMetricsReq{
		StartTimestamp: now.Add(-1 * time.Hour).Unix(),
		EndTimestamp:   now.Unix(),
		Num:            -1,
		Step:           0,
	})
	dur1h := time.Since(t2)
	if err != nil {
		fmt.Printf("1-hour query failed: %v\n", err)
	} else {
		fmt.Printf("  2. Last 1 hour raw data (no downsampling, expected 720 points): %v (%d points)\n", dur1h, r1h.TOTAL)
	}

	// Scenario 3: Last 24 hours downsampled (step=60s, expected 1440 points)
	t3 := time.Now()
	r24h, err := st2.QueryNodeMetric(ctx, &store.QueryNodeMetricsReq{
		StartTimestamp: now.Add(-24 * time.Hour).Unix(),
		EndTimestamp:   now.Unix(),
		Num:            -1,
		Step:           60,
	})
	dur24h := time.Since(t3)
	if err != nil {
		fmt.Printf("24-hour query failed: %v\n", err)
	} else {
		fmt.Printf("  3. Last 24 hours (step=60s downsampled, expected 1440 buckets): %v (%d buckets)\n", dur24h, r24h.TOTAL)
	}

	// Scenario 4: Last 7 days span (step=300s, expected 2016 points)
	t4 := time.Now()
	r7d, err := st2.QueryNodeMetric(ctx, &store.QueryNodeMetricsReq{
		StartTimestamp: now.Add(-7 * 24 * time.Hour).Unix(),
		EndTimestamp:   now.Unix(),
		Num:            -1,
		Step:           300,
	})
	dur7d := time.Since(t4)
	if err != nil {
		fmt.Printf("7-day query failed: %v\n", err)
	} else {
		fmt.Printf("  4. Last 7 days overview (step=300s downsampled, expected 2016 buckets): %v (%d buckets)\n", dur7d, r7d.TOTAL)
	}

	// Scenario 5: Settings health check API
	fmt.Println("\n[4/4] Verifying Settings DBHealth API...")
	health, err := st2.Health(ctx)
	if err != nil {
		fmt.Printf("Health check failed: %v\n", err)
	} else {
		fmt.Printf("  DBHealth: disk size = %.2f MB\n", float64(health.FileBytes)/(1024*1024))
		fmt.Printf("  DBHealth: partitions = %d\n", health.Partitions)
		fmt.Printf("  DBHealth: sample count = %d\n", health.NodeMetricsRows)
		for k, v := range health.Stats {
			fmt.Printf("  Stat [%s]: count=%d, last=%.2fms, max=%.2fms\n", k, v.Count, v.LastMs, v.MaxMs)
		}
	}

	fmt.Println("\n==========================================================")
	fmt.Println("🎉 PoC & 7-Day Ingestion Verification Complete!")
	fmt.Println("==========================================================")
}
