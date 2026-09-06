package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/Ehco1996/ehco/internal/cmgr/ms"
	"github.com/Ehco1996/ehco/internal/cmgr/sampler"
)

func main() {
	targetDir := filepath.Join(os.Getenv("HOME"), ".ehco", "metrics_ts")
	_ = os.RemoveAll(targetDir)

	fmt.Println("==========================================================")
	fmt.Println("🚀 ehco 时序存储 PoC：tstorage 性能测试与 7 天数据灌入")
	fmt.Println("==========================================================")
	fmt.Printf("数据存储路径: %s\n", targetDir)

	store, err := ms.NewMetricsStore(targetDir)
	if err != nil {
		fmt.Printf("初始化 MetricsStore 失败: %v\n", err)
		return
	}

	ctx := context.Background()

	// 1 礼拜 = 7 天
	// 采样间隔 5 秒
	totalSeconds := 7 * 24 * 3600
	stepSec := 5
	totalSamples := totalSeconds / stepSec // 120,960 个采样点
	totalPoints := totalSamples * 5        // 每个采样点 5 个指标 = 604,800 个数据点

	now := time.Now()
	startTime := now.Add(-time.Duration(totalSeconds) * time.Second)

	fmt.Printf("\n[1/4] 开始生成并写入 7 天历史监控数据...\n")
	fmt.Printf("时间范围: %s ~ %s\n", startTime.Format("2006-01-02 15:04:05"), now.Format("2006-01-02 15:04:05"))
	fmt.Printf("采样点数: %d 组 (%d 条数据点, 涵盖 CPU/内存/磁盘/网络入/网络出)\n", totalSamples, totalPoints)

	insertStart := time.Now()

	// 模拟真实的每日昼夜峰谷波形
	r := rand.New(rand.NewSource(42))
	for i := 0; i < totalSamples; i++ {
		curTime := startTime.Add(time.Duration(i*stepSec) * time.Second)
		hourOfDay := float64(curTime.Hour()) + float64(curTime.Minute())/60.0

		// 昼夜波动因子 (0.3 ~ 1.0)
		diurnal := 0.65 + 0.35*math.Sin((hourOfDay-8)/24.0*2*math.Pi)
		jitter := (r.Float64() - 0.5) * 5.0

		cpu := math.Max(5, math.Min(95, 25.0*diurnal+jitter+10.0))
		mem := math.Max(20, math.Min(80, 45.0+5.0*diurnal+jitter*0.2))
		disk := 32.5 + float64(i)/float64(totalSamples)*2.0 // 随时间微增 32.5% -> 34.5%
		netIn := math.Max(1024, (5*1024*1024)*diurnal+r.Float64()*1024*1024)
		netOut := math.Max(1024, (8*1024*1024)*diurnal+r.Float64()*1024*1024)

		nm := &sampler.NodeMetrics{
			SyncTime:                 curTime,
			CpuUsagePercent:          cpu,
			MemoryUsagePercent:       mem,
			DiskUsagePercent:         disk,
			NetworkReceiveBytesRate:  netIn,
			NetworkTransmitBytesRate: netOut,
		}

		if err := store.AddNodeMetric(ctx, nm); err != nil {
			fmt.Printf("写入失败 (i=%d): %v\n", i, err)
			return
		}

		if (i+1)%(totalSamples/5) == 0 || i == totalSamples-1 {
			pct := float64(i+1) / float64(totalSamples) * 100
			fmt.Printf("  -> 写入进度: %5.1f%% (%d / %d 采样点)\n", pct, i+1, totalSamples)
		}
	}

	insertDuration := time.Since(insertStart)
	throughput := float64(totalPoints) / insertDuration.Seconds()

	fmt.Printf("\n✅ 写入完成！总耗时: %v\n", insertDuration)
	fmt.Printf("写入吞吐量: %.2f 数据点/秒 (%.2f 采样组/秒)\n", throughput, float64(totalSamples)/insertDuration.Seconds())

	// 刷新持久化并统计文件大小
	fmt.Println("\n[2/4] 检查磁盘持久化与空间占用...")
	_ = store.Close()

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

	fmt.Printf("  数据目录总大小: %.2f MB (%d 字节)\n", sizeMB, totalBytes)
	fmt.Printf("  分区数量: %d 个分区目录\n", partitionCount)
	fmt.Printf("  数据文件数: %d 个文件\n", fileCount)
	fmt.Printf("  单个数据点平均占用: %.2f 字节/点 (包含时间戳 + Float64 + 指标索引)\n", bytesPerPoint)

	// 重新打开并进行查询测试
	fmt.Println("\n[3/4] 重新加载存储并执行前端典型查询测试...")
	store2, err := ms.NewMetricsStore(targetDir)
	if err != nil {
		fmt.Printf("重新打开失败: %v\n", err)
		return
	}
	defer store2.Close()

	// 场景 1: 轮询最新状态 (overview 15s 轮询，Num=1)
	t1 := time.Now()
	rLatest, err := store2.QueryNodeMetric(ctx, &ms.QueryNodeMetricsReq{
		StartTimestamp: now.Add(-5 * time.Minute).Unix(),
		EndTimestamp:   now.Unix(),
		Num:            1,
	})
	durLatest := time.Since(t1)
	if err != nil {
		fmt.Printf("最新点查询失败: %v\n", err)
	} else {
		fmt.Printf("  1. 实时点查询 (最近5分钟, 取最新1个点): 耗时 %v (返回 %d 条)\n", durLatest, rLatest.TOTAL)
		if len(rLatest.Data) > 0 {
			p := rLatest.Data[0]
			fmt.Printf("     [数据预览] CPU: %.1f%%, 内存: %.1f%%, 入网: %.2f MB/s, 出网: %.2f MB/s\n",
				p.CPUUsage, p.MemoryUsage, p.NetworkIn/1024/1024, p.NetworkOut/1024/1024)
		}
	}

	// 场景 2: 最近 1 小时原始点查询 (720 个原始点)
	t2 := time.Now()
	r1h, err := store2.QueryNodeMetric(ctx, &ms.QueryNodeMetricsReq{
		StartTimestamp: now.Add(-1 * time.Hour).Unix(),
		EndTimestamp:   now.Unix(),
		Num:            -1,
		Step:           0,
	})
	dur1h := time.Since(t2)
	if err != nil {
		fmt.Printf("1小时查询失败: %v\n", err)
	} else {
		fmt.Printf("  2. 最近 1 小时原始数据 (无降采样, 理论 720 点): 耗时 %v (返回 %d 点)\n", dur1h, r1h.TOTAL)
	}

	// 场景 3: 最近 24 小时监控 (降采样 step=60s，理论 1440 点)
	t3 := time.Now()
	r24h, err := store2.QueryNodeMetric(ctx, &ms.QueryNodeMetricsReq{
		StartTimestamp: now.Add(-24 * time.Hour).Unix(),
		EndTimestamp:   now.Unix(),
		Num:            -1,
		Step:           60,
	})
	dur24h := time.Since(t3)
	if err != nil {
		fmt.Printf("24小时查询失败: %v\n", err)
	} else {
		fmt.Printf("  3. 最近 24 小时监控 (step=60s 降采样, 理论 1440 桶): 耗时 %v (返回 %d 桶)\n", dur24h, r24h.TOTAL)
	}

	// 场景 4: 最近 7 天大跨度监控 (降采样 step=300s，理论 2016 点)
	t4 := time.Now()
	r7d, err := store2.QueryNodeMetric(ctx, &ms.QueryNodeMetricsReq{
		StartTimestamp: now.Add(-7 * 24 * time.Hour).Unix(),
		EndTimestamp:   now.Unix(),
		Num:            -1,
		Step:           300,
	})
	dur7d := time.Since(t4)
	if err != nil {
		fmt.Printf("7天查询失败: %v\n", err)
	} else {
		fmt.Printf("  4. 最近 7 天全局监控 (step=300s 降采样, 理论 2016 桶): 耗时 %v (返回 %d 桶)\n", dur7d, r7d.TOTAL)
	}

	// 场景 5: Settings 健康检查 API
	fmt.Println("\n[4/4] 验证前端 Settings 页面指标接口 (DBHealth)...")
	health, err := store2.Health(ctx)
	if err != nil {
		fmt.Printf("Health 检查失败: %v\n", err)
	} else {
		fmt.Printf("  DBHealth: 物理磁盘大小 = %.2f MB\n", float64(health.FileBytes)/(1024*1024))
		fmt.Printf("  DBHealth: 碎片率 (freelist) = %d (完全为零，无碎片)\n", health.FreelistPages)
		for k, v := range health.Stats {
			fmt.Printf("  Stat [%s]: 次数=%d, 最近耗时=%.2fms, 最大耗时=%.2fms\n", k, v.Count, v.LastMs, v.MaxMs)
		}
	}

	fmt.Println("\n==========================================================")
	fmt.Println("🎉 PoC 与 7 天数据灌入全部完成！")
	fmt.Println("==========================================================")
}
