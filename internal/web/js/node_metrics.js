const Config = {
  API_BASE_URL: '/api/v1',
  NODE_METRICS_PATH: '/node_metrics/',
  BYTE_TO_MB: 1024 * 1024,
  CHART_COLORS: {
    cpu: 'rgba(255, 99, 132, 1)',
    memory: 'rgba(54, 162, 235, 1)',
    disk: 'rgba(255, 206, 86, 1)',
    receive: 'rgba(0, 150, 255, 1)',
    transmit: 'rgba(255, 140, 0, 1)',
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

  static async fetchLatestMetric() {
    const data = await this.fetchData(Config.NODE_METRICS_PATH, { latest: true });
    return data?.data[0];
  }

  static async fetchMetrics(startTs, endTs) {
    const data = await this.fetchData(Config.NODE_METRICS_PATH, { start_ts: startTs, end_ts: endTs });
    return data?.data;
  }
}

class ChartManager {
  constructor() {
    this.charts = {};
  }

  initializeCharts() {
    this.charts = {
      cpu: this.initChart('cpuChart', 'line', { label: 'CPU' }, 'top', 'Usage (%)', 'CPU', '%'),
      memory: this.initChart('memoryChart', 'line', { label: 'Memory' }, 'top', 'Usage (%)', 'Memory', '%'),
      disk: this.initChart('diskChart', 'line', { label: 'Disk' }, 'top', 'Usage (%)', 'Disk', '%'),
      network: this.initChart(
        'networkChart',
        'line',
        [{ label: 'Receive' }, { label: 'Transmit' }],
        'top',
        'Rate (MB/s)',
        'Network Rate',
        'MB/s'
      ),
    };
  }

  initChart(canvasId, type, datasets, legendPosition, yDisplayText, title, unit) {
    const ctx = $(`#${canvasId}`)[0].getContext('2d');
    const data = {
      labels: [],
      datasets: Array.isArray(datasets)
        ? datasets.map((dataset) => this.getDatasetConfig(dataset.label))
        : [this.getDatasetConfig(datasets.label)],
    };

    return new Chart(ctx, {
      type,
      data,
      options: this.getChartOptions(legendPosition, yDisplayText, title, unit),
    });
  }

  getDatasetConfig(label) {
    const color = Config.CHART_COLORS[label.toLowerCase()] || 'rgba(0, 0, 0, 1)';
    return {
      label,
      borderColor: color,
      backgroundColor: color.replace('1)', '0.2)'),
      borderWidth: 2,
      pointRadius: 2,
      pointHoverRadius: 2,
      fill: true,
      data: [],
    };
  }

  getChartOptions(legendPosition, yDisplayText, title, unit) {
    return {
      line: { spanGaps: false },
      responsive: true,
      plugins: {
        legend: { position: legendPosition },
        title: {
          display: !!title,
          text: title,
          position: 'bottom',
          font: { size: 14, weight: 'bold' },
        },
        tooltip: {
          callbacks: {
            title: (tooltipItems) => new Date(tooltipItems[0].label).toLocaleString(),
            label: (context) => {
              let label = context.dataset.label || '';
              if (label) {
                label += ': ';
              }
              if (context.parsed.y !== null) {
                label += context.parsed.y.toFixed(2) + ' ' + unit;
              }
              return label;
            },
          },
        },
        zoom: {
          pan: { enabled: true, mode: 'x' },
          zoom: {
            wheel: { enabled: true },
            pinch: { enabled: true },
            mode: 'x',
          },
        },
      },
      scales: {
        x: {
          type: 'time',
          time: {
            unit: 'minute',
            displayFormats: { minute: 'HH:mm' },
          },
          ticks: {
            maxRotation: 0,
            autoSkip: true,
            maxTicksLimit: 10,
          },
          adapters: {
            date: { locale: 'en' },
          },
        },
        y: {
          beginAtZero: true,
          title: { display: true, text: yDisplayText, font: { weight: 'bold' } },
        },
      },
      elements: { line: { tension: 0.4 } },
      downsample: {
        enabled: true,
        threshold: 200,
      },
    };
  }

  updateCharts(metrics, startTs, endTs) {
    const timestamps = this.generateTimestamps(startTs, endTs);
    const processData = (dataKey) => {
      const data = new Array(timestamps.length).fill(null);
      metrics.forEach((metric) => {
        const index = Math.floor((metric.timestamp - startTs) / 60);
        if (index >= 0 && index < data.length) {
          data[index] = metric[dataKey];
        }
      });
      return data;
    };

    this.updateChart(this.charts.cpu, processData('cpu_usage'), timestamps);
    this.updateChart(this.charts.memory, processData('memory_usage'), timestamps);
    this.updateChart(this.charts.disk, processData('disk_usage'), timestamps);
    this.updateChart(
      this.charts.network,
      [
        processData('network_in').map((v) => (v === null ? null : v / Config.BYTE_TO_MB)),
        processData('network_out').map((v) => (v === null ? null : v / Config.BYTE_TO_MB)),
      ],
      timestamps
    );
  }

  updateChart(chart, newData, labels) {
    if (!newData || !labels) {
      console.error('Invalid data or labels provided');
      return;
    }

    if (Array.isArray(newData) && Array.isArray(newData[0])) {
      chart.data.datasets.forEach((dataset, index) => {
        if (newData[index]) {
          dataset.data = newData[index].map((value, i) => ({ x: moment(labels[i]), y: value }));
        }
      });
    } else {
      chart.data.datasets[0].data = newData.map((value, i) => ({ x: moment(labels[i]), y: value }));
    }

    chart.options.scales.x.min = moment(labels[0]);
    chart.options.scales.x.max = moment(labels[labels.length - 1]);
    chart.update();
  }

  addLatestDataToCharts(latestMetric) {
    const timestamp = moment.unix(latestMetric.timestamp);

    Object.entries(this.charts).forEach(([key, chart]) => {
      const existingDataIndex = chart.data.labels.findIndex((label) => label.isSame(timestamp));

      if (existingDataIndex === -1) {
        chart.data.labels.push(timestamp);
        if (key === 'network') {
          chart.data.datasets[0].data.push({ x: timestamp, y: latestMetric.network_in / Config.BYTE_TO_MB });
          chart.data.datasets[1].data.push({ x: timestamp, y: latestMetric.network_out / Config.BYTE_TO_MB });
        } else {
          chart.data.datasets[0].data.push({ x: timestamp, y: latestMetric[`${key}_usage`] });
        }

        const timeWindow = moment.duration(Config.TIME_WINDOW, 'minutes');
        const oldestAllowedTime = moment(timestamp).subtract(timeWindow);

        chart.options.scales.x.min = oldestAllowedTime;
        chart.options.scales.x.max = timestamp;

        chart.update();
      }
    });
  }

  generateTimestamps(start, end) {
    const timestamps = [];
    let current = moment.unix(start);
    const endMoment = moment.unix(end);
    while (current.isSameOrBefore(endMoment)) {
      timestamps.push(current.toISOString());
      current.add(1, 'minute');
    }
    return timestamps;
  }
}

class DateRangeManager {
  constructor(chartManager) {
    this.chartManager = chartManager;
    this.$dateRangeDropdown = $('#dateRangeDropdown');
    this.$dateRangeButton = $('#dateRangeButton');
    this.$dateRangeText = $('#dateRangeText');
    this.$dateRangeInput = $('#dateRangeInput');
    this.setupEventListeners();
  }

  setupEventListeners() {
    this.$dateRangeDropdown.find('.dropdown-item[data-range]').on('click', (e) => this.handlePresetDateRange(e));
    this.$dateRangeButton.on('click', (event) => this.toggleDropdown(event));
    $(document).on('click', (event) => this.closeDropdownOnOutsideClick(event));
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
    let start;
    switch (range) {
      case '30m':
        start = new Date(now - 30 * 60 * 1000);
        break;
      case '1h':
        start = new Date(now - 60 * 60 * 1000);
        break;
      case '3h':
        start = new Date(now - 3 * 60 * 60 * 1000);
        break;
      case '6h':
        start = new Date(now - 6 * 60 * 60 * 1000);
        break;
      case '12h':
        start = new Date(now - 12 * 60 * 60 * 1000);
        break;
      case '24h':
        start = new Date(now - 24 * 60 * 60 * 1000);
        break;
      case '7d':
        start = new Date(now - 7 * 24 * 60 * 60 * 1000);
        break;
    }
    return [start, now];
  }

  toggleDropdown(event) {
    event.stopPropagation();
    this.$dateRangeDropdown.toggleClass('is-active');
  }

  closeDropdownOnOutsideClick(event) {
    if (!this.$dateRangeDropdown.has(event.target).length) {
      this.$dateRangeDropdown.removeClass('is-active');
    }
  }

  initializeDatePicker() {
    flatpickr(this.$dateRangeInput[0], {
      mode: 'range',
      enableTime: true,
      dateFormat: 'Y-m-d H:i',
      onChange: (selectedDates) => this.handleDatePickerChange(selectedDates),
      onClose: () => this.$dateRangeDropdown.removeClass('is-active'),
    });

    this.$dateRangeInput.on('click', (event) => event.stopPropagation());
  }

  handleDatePickerChange(selectedDates) {
    if (selectedDates.length === 2) {
      const [start, end] = selectedDates;
      this.fetchAndUpdateCharts(start, end);
      const formattedStart = start.toLocaleString();
      const formattedEnd = end.toLocaleString();
      this.$dateRangeText.text(`${formattedStart} - ${formattedEnd}`);
      this.$dateRangeDropdown.removeClass('is-active');
    }
  }

  async fetchAndUpdateCharts(start, end) {
    const startTs = Math.floor(start.getTime() / 1000);
    const endTs = Math.floor(end.getTime() / 1000);
    const metrics = await ApiService.fetchMetrics(startTs, endTs);
    if (metrics) {
      this.chartManager.updateCharts(metrics, startTs, endTs);
    }
  }
}

class AutoRefreshManager {
  constructor(chartManager) {
    this.chartManager = chartManager;
    this.autoRefreshInterval = null;
    this.isAutoRefreshing = false;
    this.$refreshButton = $('#refreshButton');
    this.setupEventListeners();
  }

  setupEventListeners() {
    this.$refreshButton.click(() => this.toggleAutoRefresh());
  }

  toggleAutoRefresh() {
    if (this.isAutoRefreshing) {
      this.stopAutoRefresh();
    } else {
      this.startAutoRefresh();
    }
  }

  startAutoRefresh() {
    this.isAutoRefreshing = true;
    this.$refreshButton.addClass('is-info');
    this.$refreshButton.find('span:last').text('Stop Refresh');
    this.refreshData();
    this.autoRefreshInterval = setInterval(() => this.refreshData(), Config.AUTO_REFRESH_INTERVAL);
  }

  stopAutoRefresh() {
    this.isAutoRefreshing = false;
    clearInterval(this.autoRefreshInterval);
    this.$refreshButton.removeClass('is-info');
    this.$refreshButton.find('span:last').text('Auto Refresh');
  }

  async refreshData() {
    const latestMetric = await ApiService.fetchLatestMetric();
    if (latestMetric) {
      this.chartManager.addLatestDataToCharts(latestMetric);
    }
  }
}

class MetricsModule {
  constructor() {
    this.chartManager = new ChartManager();
    this.dateRangeManager = new DateRangeManager(this.chartManager);
    this.autoRefreshManager = new AutoRefreshManager(this.chartManager);
  }

  async init() {
    this.chartManager.initializeCharts();
  }
}

// Initialize when the DOM is ready
document.addEventListener('DOMContentLoaded', () => {
  const metricsModule = new MetricsModule();
  metricsModule.init();
});
