package metric_reader

import (
	"math"

	dto "github.com/prometheus/client_model/go"
)

func calculatePercentile(histogram *dto.Histogram, percentile float64) float64 {
	if histogram == nil {
		return 0
	}
	totalSamples := histogram.GetSampleCount()
	targetSample := percentile * float64(totalSamples)
	cumulativeCount := uint64(0)
	var lastBucketBound float64

	for _, bucket := range histogram.Bucket {
		cumulativeCount += bucket.GetCumulativeCount()
		if float64(cumulativeCount) >= targetSample {
			// Linear interpolation between bucket boundaries
			if bucket.GetCumulativeCount() > 0 && lastBucketBound != bucket.GetUpperBound() {
				return lastBucketBound + (float64(targetSample-float64(cumulativeCount-bucket.GetCumulativeCount()))/float64(bucket.GetCumulativeCount()))*(bucket.GetUpperBound()-lastBucketBound)
			} else {
				return bucket.GetUpperBound()
			}
		}
		lastBucketBound = bucket.GetUpperBound()
	}
	return math.NaN()
}

func getMetricValue(metric *dto.Metric, metricType dto.MetricType) float64 {
	switch metricType {
	case dto.MetricType_COUNTER:
		return metric.Counter.GetValue()
	case dto.MetricType_GAUGE:
		return metric.Gauge.GetValue()
	case dto.MetricType_HISTOGRAM:
		histogram := metric.Histogram
		if histogram != nil {
			return calculatePercentile(histogram, 0.9)
		}
	}
	return 0
}
