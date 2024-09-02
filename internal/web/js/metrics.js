const MetricsModule = (function () {
  // Constants
  const API_BASE_URL = '/api/v1';
  const NODE_METRICS_PATH = '/node_metrics/';
  const BYTE_TO_MB = 1024 * 1024;

  const handleError = (error) => {
    console.error('Error:', error);
  };

  // API functions
  const fetchData = async (path, params = {}) => {
    const url = new URL(API_BASE_URL + path, window.location.origin);
    Object.entries(params).forEach(([key, value]) => url.searchParams.append(key, value));
    try {
      const response = await fetch(url.toString());
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      return await response.json();
    } catch (error) {
      handleError(error);
      return null;
    }
  };

  const fetchLatestMetric = () => fetchData(NODE_METRICS_PATH, { latest: true }).then((data) => data?.data[0]);
  const fetchMetrics = (startTs, endTs) => fetchData(NODE_METRICS_PATH, { start_ts: startTs, end_ts: endTs }).then((data) => data?.data);

  // Chart functions
  const initChart = (canvasId, type, datasets, legendPosition = '', yDisplayText = '', title = '', unit = '') => {
    const ctx = $(`#${canvasId}`)[0].getContext('2d');
    const colors = {
      cpu: 'rgba(255, 99, 132, 1)',
      memory: 'rgba(54, 162, 235, 1)',
      disk: 'rgba(255, 206, 86, 1)',
      receive: 'rgba(0, 150, 255, 1)',
      transmit: 'rgba(255, 140, 0, 1)',
    };

    const getDatasetConfig = (label) => {
      const color = colors[label.toLowerCase()] || 'rgba(0, 0, 0, 1)';
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
    };

    const data = {
      labels: [],
      datasets: $.isArray(datasets) ? datasets.map((dataset) => getDatasetConfig(dataset.label)) : [getDatasetConfig(datasets.label)],
    };

    return new Chart(ctx, {
      type,
      data,
      options: {
        line: {
          spanGaps: false, // 设置为 false，不连接空值
        },
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
              title: function (tooltipItems) {
                return new Date(tooltipItems[0].label).toLocaleString();
              },
              label: function (context) {
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
        },
        scales: {
          x: {
            type: 'time',
            time: {
              unit: 'minute',
              displayFormats: {
                minute: 'HH:mm',
              },
            },
            ticks: {
              maxRotation: 0,
              autoSkip: true,
              maxTicksLimit: 10,
            },
            adapters: {
              date: {
                locale: 'en',
              },
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
      },
    });
  };

  const updateChart = (chart, newData, labels) => {
    if (!newData || !labels) {
      console.error('Invalid data or labels provided');
      return;
    }

    if ($.isArray(newData) && $.isArray(newData[0])) {
      $.each(chart.data.datasets, (index, dataset) => {
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
  };

  const updateCharts = (charts, metrics, startTs, endTs) => {
    console.log('Raw metrics data:', metrics);

    const generateTimestamps = (start, end) => {
      const timestamps = [];
      let current = moment.unix(start);
      const endMoment = moment.unix(end);
      while (current.isSameOrBefore(endMoment)) {
        timestamps.push(current.toISOString());
        current.add(1, 'minute');
      }
      return timestamps;
    };

    const timestamps = generateTimestamps(startTs, endTs);

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

    updateChart(charts.cpu, processData('cpu_usage'), timestamps);
    updateChart(charts.memory, processData('memory_usage'), timestamps);
    updateChart(charts.disk, processData('disk_usage'), timestamps);
    updateChart(
      charts.network,
      [
        processData('network_in').map((v) => (v === null ? null : v / BYTE_TO_MB)),
        processData('network_out').map((v) => (v === null ? null : v / BYTE_TO_MB)),
      ],
      timestamps
    );
  };

  const addLatestDataToCharts = (charts, latestMetric) => {
    console.log('Raw latestMetric data:', latestMetric);
    const timestamp = moment.unix(latestMetric.timestamp);

    $.each(charts, (key, chart) => {
      // 检查是否已经有这个时间戳的数据
      const existingDataIndex = chart.data.labels.findIndex((label) => label.isSame(timestamp));

      if (existingDataIndex === -1) {
        // 如果是新数据，添加到末尾
        chart.data.labels.push(timestamp);
        if (key === 'network') {
          chart.data.datasets[0].data.push({ x: timestamp, y: latestMetric.network_in / BYTE_TO_MB });
          chart.data.datasets[1].data.push({ x: timestamp, y: latestMetric.network_out / BYTE_TO_MB });
        } else {
          chart.data.datasets[0].data.push({ x: timestamp, y: latestMetric[`${key}_usage`] });
        }

        // 更新x轴范围，但保持一定的时间窗口
        const timeWindow = moment.duration(30, 'minutes'); // 设置显示的时间窗口，例如30分钟
        const oldestAllowedTime = moment(timestamp).subtract(timeWindow);

        chart.options.scales.x.min = oldestAllowedTime;
        chart.options.scales.x.max = timestamp;

        // 开启图表的平移和缩放功能
        chart.options.plugins.zoom = {
          pan: {
            enabled: true,
            mode: 'x',
          },
          zoom: {
            wheel: {
              enabled: true,
            },
            pinch: {
              enabled: true,
            },
            mode: 'x',
          },
        };

        chart.update();
      }
      // 如果数据已存在，我们不做任何操作，保持现有数据
    });
  };

  // Chart initialization
  const initializeCharts = async () => {
    const metric = await fetchLatestMetric();
    if (!metric) return null;
    return {
      cpu: initChart('cpuChart', 'line', { label: 'CPU' }, 'top', 'Usage (%)', `CPU`, '%'),
      memory: initChart('memoryChart', 'line', { label: 'Memory' }, 'top', 'Usage (%)', `Memory`, '%'),
      disk: initChart('diskChart', 'line', { label: 'Disk' }, 'top', 'Usage (%)', `Disk`, '%'),
      network: initChart(
        'networkChart',
        'line',
        [{ label: 'Receive' }, { label: 'Transmit' }],
        'top',
        'Rate (MB/s)',
        'Network Rate',
        'MB/s'
      ),
    };
  };

  // Date range functions
  const setupDateRangeDropdown = (charts) => {
    const $dateRangeDropdown = $('#dateRangeDropdown');
    const $dateRangeButton = $('#dateRangeButton');
    const $dateRangeText = $('#dateRangeText');
    const $dateRangeInput = $('#dateRangeInput');

    $dateRangeDropdown.find('.dropdown-item[data-range]').on('click', function (e) {
      e.preventDefault();
      const range = $(this).data('range');
      const now = new Date();
      let start, end;
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
      end = now;

      const startTs = Math.floor(start.getTime() / 1000);
      const endTs = Math.floor(end.getTime() / 1000);
      fetchDataForRange(charts, startTs, endTs);
      $dateRangeText.text($(this).text());
      $dateRangeDropdown.removeClass('is-active');
    });

    $dateRangeButton.on('click', (event) => {
      event.stopPropagation();
      $dateRangeDropdown.toggleClass('is-active');
    });

    $(document).on('click', (event) => {
      if (!$dateRangeDropdown.has(event.target).length) {
        $dateRangeDropdown.removeClass('is-active');
      }
    });

    const picker = flatpickr($dateRangeInput[0], {
      mode: 'range',
      enableTime: true,
      dateFormat: 'Y-m-d H:i',
      onChange: function (selectedDates) {
        if (selectedDates.length === 2) {
          const startTs = Math.floor(selectedDates[0].getTime() / 1000);
          const endTs = Math.floor(selectedDates[1].getTime() / 1000);
          fetchDataForRange(charts, startTs, endTs);

          const formattedStart = selectedDates[0].toLocaleString();
          const formattedEnd = selectedDates[1].toLocaleString();
          $dateRangeText.text(`${formattedStart} - ${formattedEnd}`);

          // 关闭下拉菜单
          $dateRangeDropdown.removeClass('is-active');
        }
      },
      onClose: function () {
        // 确保在日期选择器关闭时也关闭下拉菜单
        $dateRangeDropdown.removeClass('is-active');
      },
    });

    // 防止点击日期选择器时关闭下拉菜单
    $dateRangeInput.on('click', (event) => {
      event.stopPropagation();
    });
  };

  const fetchDataForRange = async (charts, startTs, endTs) => {
    const metrics = await fetchMetrics(startTs, endTs);
    if (metrics) {
      console.log('Raw metrics data:', metrics);
      updateCharts(charts, metrics, startTs, endTs);
    }
  };

  // Auto refresh functions
  const setupAutoRefresh = (charts) => {
    let autoRefreshInterval;
    let isAutoRefreshing = false;
    $('#refreshButton').click(function () {
      if (isAutoRefreshing) {
        clearInterval(autoRefreshInterval);
        $(this).removeClass('is-info');
        $(this).find('span:last').text('Auto Refresh');
        isAutoRefreshing = false;
      } else {
        $(this).addClass('is-info');
        $(this).find('span:last').text('Stop Refresh');
        isAutoRefreshing = true;
        refreshData(charts);
        autoRefreshInterval = setInterval(() => refreshData(charts), 5000);
      }
    });
  };

  const refreshData = async (charts) => {
    const latestMetric = await fetchLatestMetric();
    if (latestMetric) {
      addLatestDataToCharts(charts, latestMetric);
    }
  };

  // Main initialization function
  const init = async () => {
    const charts = await initializeCharts();
    if (charts) {
      setupDateRangeDropdown(charts);
      setupAutoRefresh(charts);
    }
  };

  // Public API
  return {
    init: init,
  };
})();

// Initialize when the DOM is ready
document.addEventListener('DOMContentLoaded', MetricsModule.init);
