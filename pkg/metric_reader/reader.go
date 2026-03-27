package metric_reader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"go.uber.org/zap"
)

type Reader interface {
	ReadOnce(ctx context.Context) (*NodeMetrics, map[string]*RuleMetrics, error)
}

type readerImpl struct {
	metricsURL string
	httpClient *http.Client

	lastMetrics     *NodeMetrics
	lastRuleMetrics map[string]*RuleMetrics // key: label value: RuleMetrics
	l               *zap.SugaredLogger
}

func NewReader(metricsURL string) *readerImpl {
	c := &http.Client{Timeout: 30 * time.Second}
	return &readerImpl{
		httpClient: c,
		metricsURL: metricsURL,
		l:          zap.S().Named("metric_reader"),
	}
}

func (b *readerImpl) ReadOnce(ctx context.Context) (*NodeMetrics, map[string]*RuleMetrics, error) {
	metricMap, err := b.fetchMetrics(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch metrics: %w", err)
	}
	nm := &NodeMetrics{SyncTime: time.Now()}
	if err := b.ParseNodeMetrics(metricMap, nm); err != nil {
		return nil, nil, err
	}

	rm := make(map[string]*RuleMetrics)
	if err := b.ParseRuleMetrics(metricMap, rm); err != nil {
		return nil, nil, err
	}

	b.lastMetrics = nm
	b.lastRuleMetrics = rm
	return nm, rm, nil
}

func (r *readerImpl) fetchMetrics(ctx context.Context) (map[string]*dto.MetricFamily, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", r.metricsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	// Use LegacyValidation for backward compatibility with older Prometheus metrics
	// This prevents the "Invalid name validation scheme requested: unset" panic
	parser := expfmt.NewTextParser(model.LegacyValidation)
	return parser.TextToMetricFamilies(strings.NewReader(string(body)))
}
