const Config = {
  API_BASE_URL: '/api/v1',
  RULE_METRICS_PATH: '/rule_metrics/',
  BYTE_TO_MB: 1024 * 1024,
  CHART_COLORS: {
    connectionCount: 'rgba(255, 99, 132, 1)',
    handshakeDuration: 'rgba(54, 162, 235, 1)',
    pingLatency: 'rgba(255, 206, 86, 1)',
    networkTransmitBytes: 'rgba(75, 192, 192, 1)',
  },
  TIME_WINDOW: 30, // minutes
  AUTO_REFRESH_INTERVAL: 5000, // milliseconds
};

class ApiService {
  static async fetchData(path, params = {}) {
    const url = new URL(Config.API_BASE_URL + path, window.location.origin);
    Object.entries(params).forEach(([key, value]) => url.searchParams.append(key, value));
    try {
      const response = await fetch(url.toString());
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      return await response.json();
    } catch (error) {
      console.error('Error:', error);
      return null;
    }
  }

  static async fetchRuleMetrics(startTs, endTs, label = '', remote = '') {
    const params = { start_ts: startTs, end_ts: endTs };
    if (label) params.label = label;
    if (remote) params.remote = remote;
    return await this.fetchData(Config.RULE_METRICS_PATH, params);
  }

  static async fetchConfig() {
    return await this.fetchData('/config/');
  }
  static async fetchLabelsAndRemotes() {
    const config = await this.fetchConfig();
    if (!config || !config.relay_configs) {
      return { labels: [], remotes: [] };
    }

    const labels = new Set();
    const remotes = new Set();

    config.relay_configs.forEach((relayConfig) => {
      if (relayConfig.label) labels.add(relayConfig.label);
      if (relayConfig.remotes) {
        relayConfig.remotes.forEach((remote) => remotes.add(remote));
      }
    });

    return {
      labels: Array.from(labels),
      remotes: Array.from(remotes),
    };
  }
}

class ChartManager {
  constructor() {
    this.charts = {};
  }

  initializeCharts() {
    this.charts = {
      connectionCount: this.initChart('connectionCountChart', 'line', 'Connection Count', 'Count'),
      handshakeDuration: this.initChart('handshakeDurationChart', 'line', 'Handshake Duration', 'ms'),
      pingLatency: this.initChart('pingLatencyChart', 'line', 'Ping Latency', 'ms'),
      networkTransmitBytes: this.initChart('networkTransmitBytesChart', 'line', 'Network Transmit', 'MB'),
    };
  }

  initChart(canvasId, type, title, unit) {
    const ctx = $(`#${canvasId}`)[0].getContext('2d');
    const color = Config.CHART_COLORS[canvasId.replace('Chart', '')];

    return new Chart(ctx, {
      type: type,
      data: {
        labels: [],
        datasets: [
          {
            label: title,
            borderColor: color,
            backgroundColor: color.replace('1)', '0.2)'),
            borderWidth: 2,
            data: [],
          },
        ],
      },
      options: this.getChartOptions(title, unit),
    });
  }

  getChartOptions(title, unit) {
    return {
      responsive: true,
      plugins: {
        title: {
          display: true,
          text: title,
          font: { size: 16, weight: 'bold' },
        },
        tooltip: {
          callbacks: {
            label: (context) => `${context.dataset.label}: ${context.parsed.y.toFixed(2)} ${unit}`,
          },
        },
      },
      scales: {
        x: {
          type: 'time',
          time: { unit: 'minute', displayFormats: { minute: 'HH:mm' } },
          title: { display: true, text: 'Time' },
        },
        y: {
          beginAtZero: true,
          title: { display: true, text: unit },
        },
      },
    };
  }

  fillMissingDataPoints(data, startTime, endTime) {
    const filledData = [];
    let currentTime = startTime;
    let dataIndex = 0;

    while (currentTime <= endTime) {
      if (dataIndex < data.length && data[dataIndex].x.getTime() === currentTime.getTime()) {
        filledData.push(data[dataIndex]);
        dataIndex++;
      } else {
        filledData.push({ x: new Date(currentTime), y: null });
      }
      currentTime.setMinutes(currentTime.getMinutes() + 1);
    }

    return filledData;
  }

  updateCharts(metrics, startTime, endTime) {
    Object.entries(this.charts).forEach(([key, chart]) => {
      const data = metrics.map((m) => {
        let value;
        switch (key) {
          case 'connectionCount':
            value = m.tcp_connection_count + m.udp_connection_count;
            break;
          case 'handshakeDuration':
            value = Math.max(m.tcp_handshake_duration, m.udp_handshake_duration);
            break;
          case 'pingLatency':
            value = m.ping_latency;
            break;
          case 'networkTransmitBytes':
            value = (m.tcp_network_transmit_bytes + m.udp_network_transmit_bytes) / Config.BYTE_TO_MB;
            break;
          default:
            value = 0;
        }
        return {
          x: new Date(m.timestamp * 1000),
          y: value,
        };
      });

      const filledData = this.fillMissingDataPoints(data, startTime, endTime);
      chart.data.datasets[0].data = filledData;
      chart.update();
    });
  }
}

class FilterManager {
  constructor(chartManager) {
    this.chartManager = chartManager;
    this.$labelFilter = $('#labelFilter');
    this.$remoteFilter = $('#remoteFilter');
    this.relayConfigs = [];
    this.setupEventListeners();
    this.loadFilters();
  }

  setupEventListeners() {
    this.$labelFilter.on('change', () => this.onLabelChange());
    this.$remoteFilter.on('change', () => this.applyFilters());
  }

  async loadFilters() {
    const config = await ApiService.fetchConfig();
    if (config && config.relay_configs) {
      this.relayConfigs = config.relay_configs;
      this.populateLabelFilter();
      this.onLabelChange(); // Initialize remotes for the first label
    }
  }

  populateLabelFilter() {
    const labels = [...new Set(this.relayConfigs.map((config) => config.label))];
    this.populateFilter(this.$labelFilter, labels);
  }

  onLabelChange() {
    const selectedLabel = this.$labelFilter.val();
    const remotes = this.getRemotesForLabel(selectedLabel);
    this.populateFilter(this.$remoteFilter, remotes);
    this.applyFilters();
  }

  getRemotesForLabel(label) {
    const config = this.relayConfigs.find((c) => c.label === label);
    return config ? config.remotes : [];
  }

  populateFilter($select, options) {
    $select.empty().append($('<option>', { value: '', text: 'All' }));
    options.forEach((option) => {
      $select.append($('<option>', { value: option, text: option }));
    });
  }

  async applyFilters() {
    const label = this.$labelFilter.val();
    const remote = this.$remoteFilter.val();
    const endDate = new Date();
    const startDate = new Date(endDate - Config.TIME_WINDOW * 60 * 1000);

    const metrics = await ApiService.fetchRuleMetrics(
      Math.floor(startDate.getTime() / 1000),
      Math.floor(endDate.getTime() / 1000),
      label,
      remote
    );

    if (metrics && metrics.data) {
      this.chartManager.updateCharts(metrics.data);
    }
  }
}

class DateRangeManager {
  constructor(chartManager, filterManager) {
    this.chartManager = chartManager;
    this.filterManager = filterManager;
    this.$dateRangeDropdown = $('#dateRangeDropdown');
    this.$dateRangeButton = $('#dateRangeButton');
    this.$dateRangeText = $('#dateRangeText');
    this.$dateRangeInput = $('#dateRangeInput');
    this.setupEventListeners();
  }

  setupEventListeners() {
    this.$dateRangeDropdown.find('.dropdown-item[data-range]').on('click', (e) => this.handlePresetDateRange(e));
    this.$dateRangeButton.on('click', () => this.$dateRangeDropdown.toggleClass('is-active'));
    $(document).on('click', (e) => {
      if (!this.$dateRangeDropdown.has(e.target).length) {
        this.$dateRangeDropdown.removeClass('is-active');
      }
    });
    this.initializeDatePicker();
  }

  handlePresetDateRange(e) {
    e.preventDefault();
    const range = $(e.currentTarget).data('range');
    const [start, end] = this.calculateDateRange(range);
    this.fetchAndUpdateCharts(start, end);
    this.$dateRangeText.text($(e.currentTarget).text());
    this.$dateRangeDropdown.removeClass('is-active');
  }

  calculateDateRange(range) {
    const now = new Date();
    const start = new Date(now - this.getMillisecondsFromRange(range));
    return [start, now];
  }

  getMillisecondsFromRange(range) {
    const rangeMap = {
      '30m': 30 * 60 * 1000,
      '1h': 60 * 60 * 1000,
      '3h': 3 * 60 * 60 * 1000,
      '6h': 6 * 60 * 60 * 1000,
      '12h': 12 * 60 * 60 * 1000,
      '24h': 24 * 60 * 60 * 1000,
      '7d': 7 * 24 * 60 * 60 * 1000,
    };
    return rangeMap[range] || 30 * 60 * 1000; // Default to 30 minutes
  }

  initializeDatePicker() {
    flatpickr(this.$dateRangeInput[0], {
      mode: 'range',
      enableTime: true,
      dateFormat: 'Y-m-d H:i',
      onChange: (selectedDates) => this.handleDatePickerChange(selectedDates),
    });
  }

  handleDatePickerChange(selectedDates) {
    if (selectedDates.length === 2) {
      const [start, end] = selectedDates;
      this.fetchAndUpdateCharts(start, end);
      this.$dateRangeText.text(`${start.toLocaleString()} - ${end.toLocaleString()}`);
      this.$dateRangeDropdown.removeClass('is-active');
    }
  }

  async fetchAndUpdateCharts(start, end) {
    const metrics = await ApiService.fetchRuleMetrics(
      Math.floor(start.getTime() / 1000),
      Math.floor(end.getTime() / 1000),
      this.filterManager.$labelFilter.val(),
      this.filterManager.$remoteFilter.val()
    );

    if (metrics && metrics.data) {
      this.chartManager.updateCharts(metrics.data, start, end);
    }
  }
}

class RuleMetricsModule {
  constructor() {
    this.chartManager = new ChartManager();
    this.filterManager = new FilterManager(this.chartManager);
    this.dateRangeManager = new DateRangeManager(this.chartManager, this.filterManager);
  }

  init() {
    this.chartManager.initializeCharts();
    this.filterManager.applyFilters();
  }
}

// Initialize when the DOM is ready
$(document).ready(() => {
  const ruleMetricsModule = new RuleMetricsModule();
  ruleMetricsModule.init();
});
