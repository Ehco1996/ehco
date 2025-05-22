/**
 * @file Manages the node metrics dashboard functionality.
 */

/**
 * Configuration for the Node Metrics module.
 * @typedef {object} NodeMetricsConfig
 * @property {string} API_BASE_URL - Base URL for the API.
 * @property {string} NODE_METRICS_PATH - Path for node metrics API endpoint.
 * @property {number} BYTE_TO_MB - Conversion factor from bytes to megabytes.
 * @property {object} CHART_COLORS - Colors for the charts.
 * @property {string} CHART_COLORS.cpu - CPU chart color.
 * @property {string} CHART_COLORS.memory - Memory chart color.
 * @property {string} CHART_COLORS.disk - Disk chart color.
 * @property {string} CHART_COLORS.networkReceive - Network receive color.
 * @property {string} CHART_COLORS.networkTransmit - Network transmit color.
 * @property {number} DEFAULT_TIME_WINDOW_MINS - Default time window in minutes for displaying metrics.
 * @property {number} AUTO_REFRESH_INTERVAL_MS - Auto-refresh interval in milliseconds.
 */

/** @type {NodeMetricsConfig} */
const Config = {
    API_BASE_URL: '/api/v1',
    NODE_METRICS_PATH: '/node_metrics/',
    BYTE_TO_MB: 1024 * 1024,
    CHART_COLORS: {
        cpu: 'rgba(255, 99, 132, 1)',    // Red
        memory: 'rgba(54, 162, 235, 1)', // Blue
        disk: 'rgba(255, 206, 86, 1)',   // Yellow
        networkReceive: 'rgba(75, 192, 192, 1)', // Teal
        networkTransmit: 'rgba(153, 102, 255, 1)' // Purple
    },
    DEFAULT_TIME_WINDOW_MINS: 30,
    AUTO_REFRESH_INTERVAL_MS: 5000,
};

/**
 * Service for fetching node metrics data from the API.
 */
class ApiService {
    /**
     * Generic method to fetch data from an API endpoint.
     * @param {string} path - The API path.
     * @param {object} [params={}] - Query parameters.
     * @returns {Promise<object|null>} The JSON response or null on error.
     */
    async fetchData(path, params = {}) {
        const url = new URL(Config.API_BASE_URL + path, window.location.origin);
        Object.entries(params).forEach(([key, value]) => url.searchParams.append(key, value));
        try {
            const response = await fetch(url.toString());
            if (!response.ok) {
                console.error(`HTTP error! status: ${response.status}, path: ${path}`);
                return null;
            }
            return await response.json();
        } catch (error) {
            console.error('Error fetching data:', error);
            return null;
        }
    }

    /**
     * Fetches the latest node metric.
     * @returns {Promise<object|null>} The latest metric data or null.
     */
    async fetchLatestMetric() {
        const response = await this.fetchData(Config.NODE_METRICS_PATH, { latest: 'true' });
        return response && response.data && response.data.length > 0 ? response.data[0] : null;
    }

    /**
     * Fetches node metrics within a specified time range.
     * @param {number} startTs - Start timestamp (Unix seconds).
     * @param {number} endTs - End timestamp (Unix seconds).
     * @returns {Promise<Array<object>|null>} An array of metric data or null.
     */
    async fetchMetrics(startTs, endTs) {
        const response = await this.fetchData(Config.NODE_METRICS_PATH, { start_ts: startTs, end_ts: endTs });
        return response ? response.data : null;
    }
}

/**
 * Manages Chart.js instances for displaying node metrics.
 */
class ChartManager {
    constructor() {
        /** @type {Object.<string, Chart>} */
        this.charts = {};
        this.timeWindowMs = Config.DEFAULT_TIME_WINDOW_MINS * 60 * 1000;
    }

    /**
     * Initializes all charts.
     */
    initializeCharts() {
        this.charts.cpu = this._initChart('cpuChart', [{ label: 'CPU Usage', colorKey: 'cpu' }], 'CPU Usage (%)', '%');
        this.charts.memory = this._initChart('memoryChart', [{ label: 'Memory Usage', colorKey: 'memory' }], 'Memory Usage (%)', '%');
        this.charts.disk = this._initChart('diskChart', [{ label: 'Disk Usage', colorKey: 'disk' }], 'Disk Usage (%)', '%');
        this.charts.network = this._initChart('networkChart', [
            { label: 'Receive', colorKey: 'networkReceive' },
            { label: 'Transmit', colorKey: 'networkTransmit' }
        ], 'Network Rate (MB/s)', 'MB/s');
    }
    
    /**
     * Creates a dataset configuration.
     * @param {string} label - Dataset label.
     * @param {string} colorKey - Key to look up color in Config.CHART_COLORS.
     * @returns {object} Chart.js dataset configuration.
     * @private
     */
    _getDatasetConfig(label, colorKey) {
        const color = Config.CHART_COLORS[colorKey] || 'rgba(0,0,0,1)';
        return {
            label: label,
            borderColor: color,
            backgroundColor: color.replace(/, ?1\)/, ', 0.2)'), // Make background semi-transparent
            data: [],
            fill: true,
            pointRadius: 2,
            tension: 0.4,
        };
    }

    /**
     * Creates Chart.js options.
     * @param {string} yLabel - Y-axis label.
     * @param {string} titleText - Chart title.
     * @param {string} unit - Unit for tooltips.
     * @returns {object} Chart.js options configuration.
     * @private
     */
    _getChartOptions(yLabel, titleText, unit) {
        return {
            responsive: true,
            maintainAspectRatio: false,
            scales: {
                x: {
                    type: 'time',
                    time: {
                        unit: 'minute',
                        tooltipFormat: 'MMM D, YYYY HH:mm:ss', // Full date and time for tooltip
                        displayFormats: {
                            minute: 'HH:mm' // Display only HH:mm on the axis
                        }
                    },
                    title: {
                        display: true,
                        text: 'Time'
                    }
                },
                y: {
                    beginAtZero: true,
                    title: {
                        display: true,
                        text: yLabel
                    }
                }
            },
            plugins: {
                title: {
                    display: true,
                    text: titleText,
                    font: { size: 16 }
                },
                tooltip: {
                    mode: 'index',
                    intersect: false,
                    callbacks: {
                        label: function(context) {
                            let label = context.dataset.label || '';
                            if (label) {
                                label += ': ';
                            }
                            if (context.parsed.y !== null) {
                                label += context.parsed.y.toFixed(2) + ' ' + unit;
                            }
                            return label;
                        }
                    }
                }
            }
        };
    }

    /**
     * Initializes a single chart.
     * @param {string} canvasId - ID of the canvas element.
     * @param {Array<{label: string, colorKey: string}>} datasetsMeta - Metadata for datasets.
     * @param {string} yLabel - Y-axis label.
     * @param {string} unit - Unit for tooltips.
     * @returns {Chart} The new Chart.js instance.
     * @private
     */
    _initChart(canvasId, datasetsMeta, yLabel, unit) {
        const ctx = document.getElementById(canvasId).getContext('2d');
        const datasets = datasetsMeta.map(meta => this._getDatasetConfig(meta.label, meta.colorKey));
        const titleText = datasetsMeta.map(m => m.label).join(' & ') + ' Over Time';
        
        return new Chart(ctx, {
            type: 'line',
            data: { datasets: datasets },
            options: this._getChartOptions(yLabel, titleText, unit)
        });
    }

    /**
     * Updates all charts with a new set of historical metrics.
     * @param {Array<object>} metrics - Array of metric objects from the API.
     * @param {number} startTs - Start timestamp of the metrics range (Unix seconds).
     * @param {number} endTs - End timestamp of the metrics range (Unix seconds).
     */
    updateAllCharts(metrics, startTs, endTs) {
        if (!metrics) metrics = [];
        
        const cpuData = metrics.map(m => ({ x: m.timestamp * 1000, y: m.cpu_usage }));
        const memoryData = metrics.map(m => ({ x: m.timestamp * 1000, y: m.memory_usage }));
        const diskData = metrics.map(m => ({ x: m.timestamp * 1000, y: m.disk_usage }));
        const networkReceiveData = metrics.map(m => ({ x: m.timestamp * 1000, y: m.network_in / Config.BYTE_TO_MB }));
        const networkTransmitData = metrics.map(m => ({ x: m.timestamp * 1000, y: m.network_out / Config.BYTE_TO_MB }));

        this.charts.cpu.data.datasets[0].data = cpuData;
        this.charts.memory.data.datasets[0].data = memoryData;
        this.charts.disk.data.datasets[0].data = diskData;
        this.charts.network.data.datasets[0].data = networkReceiveData;
        this.charts.network.data.datasets[1].data = networkTransmitData;

        const minTime = startTs * 1000;
        const maxTime = endTs * 1000;

        Object.values(this.charts).forEach(chart => {
            chart.options.scales.x.min = minTime;
            chart.options.scales.x.max = maxTime;
            chart.update('quiet'); // 'quiet' to prevent animations if not desired for range updates
        });
        this.timeWindowMs = maxTime - minTime; // Update current time window
    }

    /**
     * Adds a new latest data point to all charts and shifts old data if necessary.
     * @param {object} latestMetric - The latest metric object from the API.
     */
    addLatestDataPoint(latestMetric) {
        if (!latestMetric) return;

        const newTimestamp = latestMetric.timestamp * 1000;

        const dataPoints = {
            cpu: { x: newTimestamp, y: latestMetric.cpu_usage },
            memory: { x: newTimestamp, y: latestMetric.memory_usage },
            disk: { x: newTimestamp, y: latestMetric.disk_usage },
            network: [
                { x: newTimestamp, y: latestMetric.network_in / Config.BYTE_TO_MB },
                { x: newTimestamp, y: latestMetric.network_out / Config.BYTE_TO_MB }
            ]
        };

        Object.entries(this.charts).forEach(([key, chart]) => {
            if (key === 'network') {
                chart.data.datasets[0].data.push(dataPoints.network[0]);
                chart.data.datasets[1].data.push(dataPoints.network[1]);
            } else {
                chart.data.datasets[0].data.push(dataPoints[key]);
            }

            // Maintain the time window
            const oldestAllowedTimestamp = newTimestamp - this.timeWindowMs;
            chart.data.datasets.forEach(dataset => {
                dataset.data = dataset.data.filter(point => point.x >= oldestAllowedTimestamp);
            });
            
            chart.options.scales.x.min = oldestAllowedTimestamp;
            chart.options.scales.x.max = newTimestamp;
            chart.update('none'); // 'none' for smooth live update
        });
    }
}

/**
 * Manages date range selection for the node metrics.
 */
class DateRangeManagerNode {
    /**
     * @param {ApiService} apiService - Instance of ApiService.
     * @param {ChartManager} chartManager - Instance of ChartManager.
     */
    constructor(apiService, chartManager) {
        this.apiService = apiService;
        this.chartManager = chartManager;
        this.$dateRangeDropdown = document.getElementById('dateRangeDropdownNode');
        this.$dateRangeText = document.getElementById('dateRangeTextNode');
        this.$dateRangeInput = document.getElementById('dateRangeInputNode');
        this.flatpickrInstance = null;
    }

    /**
     * Initializes event listeners and Flatpickr.
     */
    init() {
        // Event listeners for preset ranges
        const presetLinks = this.$dateRangeDropdown.querySelectorAll('.dropdown-item[data-range]');
        presetLinks.forEach(link => {
            link.addEventListener('click', (e) => {
                e.preventDefault();
                const rangeKey = e.currentTarget.dataset.range;
                this.$dateRangeText.textContent = e.currentTarget.textContent;
                this.handlePresetRange(rangeKey);
                this.$dateRangeDropdown.classList.remove('is-active');
            });
        });

        // Toggle dropdown
        const triggerButton = this.$dateRangeDropdown.querySelector('.button');
        triggerButton.addEventListener('click', (e) => {
            e.stopPropagation();
            this.$dateRangeDropdown.classList.toggle('is-active');
        });
        document.addEventListener('click', (e) => { // Close on outside click
            if (!this.$dateRangeDropdown.contains(e.target)) {
                this.$dateRangeDropdown.classList.remove('is-active');
            }
        });
        
        // Flatpickr for custom range
        this.flatpickrInstance = flatpickr(this.$dateRangeInput, {
            mode: 'range',
            enableTime: true,
            dateFormat: 'Y-m-d H:i',
            onChange: (selectedDates) => {
                if (selectedDates.length === 2) {
                    this.handleCustomRange(selectedDates);
                    this.$dateRangeText.textContent = `${moment(selectedDates[0]).format('MMM D, HH:mm')} - ${moment(selectedDates[1]).format('MMM D, HH:mm')}`;
                    this.$dateRangeDropdown.classList.remove('is-active');
                }
            },
        });
         // Prevent dropdown from closing when clicking inside flatpickr input
        this.$dateRangeInput.addEventListener('click', e => e.stopPropagation());
    }

    /**
     * Handles selection of a preset date range.
     * @param {string} rangeKey - The key for the preset range (e.g., '30m', '1h').
     */
    handlePresetRange(rangeKey) {
        const { start, end } = this._calculateDateRange(rangeKey);
        this._fetchAndUpdate(start, end);
    }

    /**
     * Handles selection of a custom date range from Flatpickr.
     * @param {Array<Date>} selectedDates - Array with start and end dates.
     */
    handleCustomRange(selectedDates) {
        this._fetchAndUpdate(selectedDates[0], selectedDates[1]);
    }

    /**
     * Calculates start and end Date objects for a preset range key.
     * @param {string} rangeKey - The preset range key.
     * @returns {{start: Date, end: Date}} The calculated start and end dates.
     * @private
     */
    _calculateDateRange(rangeKey) {
        const end = new Date();
        let start = new Date();
        switch (rangeKey) {
            case '30m': start.setMinutes(end.getMinutes() - 30); break;
            case '1h': start.setHours(end.getHours() - 1); break;
            case '3h': start.setHours(end.getHours() - 3); break;
            case '6h': start.setHours(end.getHours() - 6); break;
            case '12h': start.setHours(end.getHours() - 12); break;
            case '24h': start.setDate(end.getDate() - 1); break;
            case '7d': start.setDate(end.getDate() - 7); break;
            default: start.setMinutes(end.getMinutes() - Config.DEFAULT_TIME_WINDOW_MINS);
        }
        return { start, end };
    }

    /**
     * Fetches metrics for the given range and updates charts.
     * @param {Date} start - Start date.
     * @param {Date} end - End date.
     * @private
     */
    async _fetchAndUpdate(start, end) {
        const startTs = Math.floor(start.getTime() / 1000);
        const endTs = Math.floor(end.getTime() / 1000);
        const metrics = await this.apiService.fetchMetrics(startTs, endTs);
        this.chartManager.updateAllCharts(metrics, startTs, endTs);
    }

    /**
     * Gets the current selected date range.
     * @returns {{start: Date, end: Date}}
     */
    getCurrentDateRange() {
        if (this.flatpickrInstance && this.flatpickrInstance.selectedDates.length === 2) {
            return { start: this.flatpickrInstance.selectedDates[0], end: this.flatpickrInstance.selectedDates[1] };
        }
        // Default to last 30 mins if no custom range selected or initial load
        const rangeKey = this.$dateRangeDropdown.querySelector('.dropdown-item.is-active')?.dataset.range || '30m';
        return this._calculateDateRange(rangeKey);
    }
}

/**
 * Manages auto-refresh functionality for node metrics.
 */
class AutoRefreshManagerNode {
    /**
     * @param {ApiService} apiService - Instance of ApiService.
     * @param {ChartManager} chartManager - Instance of ChartManager.
     */
    constructor(apiService, chartManager) {
        this.apiService = apiService;
        this.chartManager = chartManager;
        this.$refreshButton = document.getElementById('refreshButtonNode');
        this.isAutoRefreshing = false;
        this.refreshIntervalId = null;
    }

    /**
     * Initializes event listener for the refresh button.
     */
    init() {
        this.$refreshButton.addEventListener('click', () => this.toggleAutoRefresh());
    }

    /**
     * Toggles auto-refresh state.
     */
    toggleAutoRefresh() {
        if (this.isAutoRefreshing) {
            this._stopRefresh();
        } else {
            this._startRefresh();
        }
    }

    /**
     * Starts the auto-refresh process.
     * @private
     */
    _startRefresh() {
        this.isAutoRefreshing = true;
        this.$refreshButton.classList.add('is-success'); // Change color to indicate active
        this.$refreshButton.classList.remove('is-info');
        this.$refreshButton.querySelector('span:last-child').textContent = 'Stop Refresh';
        this.$refreshButton.querySelector('.fa-sync').classList.add('fa-spin');
        
        this._fetchAndAddData(); // Fetch immediately
        this.refreshIntervalId = setInterval(
            () => this._fetchAndAddData(),
            Config.AUTO_REFRESH_INTERVAL_MS
        );
    }

    /**
     * Stops the auto-refresh process.
     * @private
     */
    _stopRefresh() {
        this.isAutoRefreshing = false;
        if (this.refreshIntervalId) {
            clearInterval(this.refreshIntervalId);
            this.refreshIntervalId = null;
        }
        this.$refreshButton.classList.remove('is-success');
        this.$refreshButton.classList.add('is-info');
        this.$refreshButton.querySelector('span:last-child').textContent = 'Auto Refresh';
        this.$refreshButton.querySelector('.fa-sync').classList.remove('fa-spin');
    }

    /**
     * Fetches the latest metric and adds it to the charts.
     * @private
     */
    async _fetchAndAddData() {
        const latestMetric = await this.apiService.fetchLatestMetric();
        if (latestMetric) {
            this.chartManager.addLatestDataPoint(latestMetric);
        }
    }
}

/**
 * Main module for the Node Metrics dashboard.
 */
class NodeMetricsModule {
    constructor() {
        this.apiService = new ApiService();
        this.chartManager = new ChartManager();
        this.dateRangeManager = new DateRangeManagerNode(this.apiService, this.chartManager);
        this.autoRefreshManager = new AutoRefreshManagerNode(this.apiService, this.chartManager);
    }

    /**
     * Initializes the module, sets up components, and loads initial data.
     */
    async init() {
        this.chartManager.initializeCharts();
        this.dateRangeManager.init();
        this.autoRefreshManager.init();

        // Load initial data (e.g., last 30 minutes)
        const initialRange = this.dateRangeManager._calculateDateRange('30m');
        await this.dateRangeManager._fetchAndUpdate(initialRange.start, initialRange.end);
        // Set the text for the initially loaded range
        const initialRangeText = this.dateRangeManager.$dateRangeDropdown.querySelector('.dropdown-item[data-range="30m"]').textContent;
        this.dateRangeManager.$dateRangeText.textContent = initialRangeText;
    }
}

// Initialize the module when the DOM is fully loaded.
document.addEventListener('DOMContentLoaded', () => {
    const nodeMetricsModule = new NodeMetricsModule();
    nodeMetricsModule.init().catch(error => {
        console.error("Failed to initialize Node Metrics module:", error);
    });
});
