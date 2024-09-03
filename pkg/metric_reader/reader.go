package metric_reader

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
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
		return nil, nil, errors.Wrap(err, "failed to fetch metrics")
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
		return nil, errors.Wrap(err, "failed to create request")
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send request")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	var parser expfmt.TextParser
	return parser.TextToMetricFamilies(strings.NewReader(string(body)))
}
