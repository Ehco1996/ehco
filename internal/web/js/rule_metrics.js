/**
 * @file Manages the rule metrics dashboard functionality.
 */

/**
 * Configuration for the Rule Metrics module.
 * @typedef {object} RuleMetricsConfig
 * @property {string} API_BASE_URL - Base URL for the API.
 * @property {string} RULE_METRICS_PATH - Path for rule metrics API endpoint.
 * @property {string} CONFIG_PATH - Path for the config API endpoint.
 * @property {number} BYTE_TO_MB - Conversion factor from bytes to megabytes.
 * @property {number} DEFAULT_TIME_WINDOW_MINS - Default time window in minutes.
 */

/** @type {RuleMetricsConfig} */
const Config = {
    API_BASE_URL: '/api/v1',
    RULE_METRICS_PATH: '/rule_metrics/',
    CONFIG_PATH: '/config/',
    BYTE_TO_MB: 1024 * 1024,
    DEFAULT_TIME_WINDOW_MINS: 30,
};

/**
 * Service for fetching rule metrics and configuration data from the API.
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
     * Fetches rule metrics within a specified time range and for given filters.
     * @param {number} startTs - Start timestamp (Unix seconds).
     * @param {number} endTs - End timestamp (Unix seconds).
     * @param {string} [label=''] - Label filter.
     * @param {string} [remote=''] - Remote filter.
     * @returns {Promise<Array<object>|null>} An array of metric data or null.
     */
    async fetchRuleMetrics(startTs, endTs, label = '', remote = '') {
        const params = { start_ts: startTs, end_ts: endTs };
        if (label) params.label = label;
        if (remote) params.remote = remote;
        const response = await this.fetchData(Config.RULE_METRICS_PATH, params);
        return response ? response.data : null; // Assuming API returns { data: [...] }
    }

    /**
     * Fetches the application configuration.
     * @returns {Promise<object|null>} The configuration object or null.
     */
    async fetchConfig() {
        const response = await this.fetchData(Config.CONFIG_PATH);
        return response ? response.data : null; // Assuming API returns { data: { relay_configs: [...] } }
    }
}

/**
 * Manages Chart.js instances for displaying rule metrics.
 */
class ChartManager {
    constructor() {
        /** @type {Object.<string, Chart>} */
        this.charts = {};
        this.colorIndex = 0;
        this.groupColors = new Map(); // To store colors for label-remote groups
    }
    
    /**
     * Predefined list of base colors for chart lines/areas.
     * @returns {Array<string>}
     * @private
     */
    _getBaseColors() {
        return [
            'rgba(255, 99, 132, 1)', 'rgba(54, 162, 235, 1)', 'rgba(255, 206, 86, 1)',
            'rgba(75, 192, 192, 1)', 'rgba(153, 102, 255, 1)', 'rgba(255, 159, 64, 1)',
            'rgba(199, 199, 199, 1)', 'rgba(83, 102, 255, 1)', 'rgba(100, 255, 100, 1)',
            'rgba(255, 100, 100, 1)'
        ];
    }

    /**
     * Gets a color for a group, ensuring consistency for the same group.
     * @param {string} groupKey - Unique key for the group (e.g., "label-remote").
     * @returns {string} RGBA color string.
     * @private
     */
    _getColorForGroup(groupKey) {
        if (!this.groupColors.has(groupKey)) {
            const baseColors = this._getBaseColors();
            this.groupColors.set(groupKey, baseColors[this.colorIndex % baseColors.length]);
            this.colorIndex++;
        }
        return this.groupColors.get(groupKey);
    }
    
    /** Resets color index for new data updates to try and reuse colors from the start */
    _resetColorAssignments() {
        this.colorIndex = 0;
        this.groupColors.clear();
    }


    /**
     * Initializes all charts.
     */
    initializeCharts() {
        this.charts.connectionCount = this._initChart('connectionCountChart', 'Connection Count', 'Count');
        this.charts.handshakeDuration = this._initChart('handshakeDurationChart', 'Avg Handshake Duration', 'ms');
        this.charts.pingLatency = this._initChart('pingLatencyChart', 'Avg Ping Latency', 'ms');
        this.charts.networkTransmitBytes = this._initChart('networkTransmitBytesChart', 'Network Traffic', 'MB', true); // isStacked = true
    }

    /**
     * Creates Chart.js options.
     * @param {string} yLabel - Y-axis label.
     * @param {string} titleText - Chart title.
     * @param {string} unit - Unit for tooltips.
     * @param {boolean} [isStacked=false] - Whether the Y-axis should be stacked.
     * @returns {object} Chart.js options configuration.
     * @private
     */
    _getChartOptions(yLabel, titleText, unit, isStacked = false) {
        return {
            responsive: true,
            maintainAspectRatio: false,
            scales: {
                x: {
                    type: 'time',
                    time: { unit: 'minute', tooltipFormat: 'MMM D, YYYY HH:mm:ss', displayFormats: { minute: 'HH:mm' } },
                    title: { display: true, text: 'Time' }
                },
                y: {
                    beginAtZero: true,
                    stacked: isStacked,
                    title: { display: true, text: yLabel }
                }
            },
            plugins: {
                title: { display: true, text: titleText, font: { size: 16 } },
                tooltip: {
                    mode: 'index',
                    intersect: false,
                    callbacks: {
                        label: function(context) {
                            let label = context.dataset.label || '';
                            if (label) label += ': ';
                            if (context.parsed.y !== null) {
                                label += context.parsed.y.toFixed(2) + ' ' + unit;
                            }
                            return label;
                        }
                    }
                },
                legend: { display: true, position: 'top' }
            },
            interaction: { mode: 'index', intersect: false },
            elements: { point: { radius: 0 }, line: { tension: 0.1 } } // Hide points, slight curve
        };
    }
    
    /**
     * Creates a dataset configuration.
     * @param {string} label - Dataset label.
     * @param {string} color - RGBA color string.
     * @param {boolean} [isFilledArea=false] - Whether to fill the area under the line.
     * @returns {object} Chart.js dataset configuration.
     * @private
     */
    _getDatasetConfig(label, color, isFilledArea = false) {
        return {
            label: label,
            borderColor: color,
            backgroundColor: color.replace(/, ?1\)/, isFilledArea ? ', 0.3)' : ', 0)'), // Transparent or semi-transparent fill
            borderWidth: isFilledArea ? 1 : 2,
            data: [],
            fill: isFilledArea,
        };
    }

    /**
     * Initializes a single chart.
     * @param {string} canvasId - ID of the canvas element.
     * @param {string} titleBase - Base title for the chart.
     * @param {string} unit - Unit for tooltips and Y-axis.
     * @param {boolean} [isStacked=false] - Whether the chart is stacked.
     * @returns {Chart} The new Chart.js instance.
     * @private
     */
    _initChart(canvasId, titleBase, unit, isStacked = false) {
        const ctx = document.getElementById(canvasId).getContext('2d');
        return new Chart(ctx, {
            type: 'line',
            data: { datasets: [] }, // Datasets are added dynamically
            options: this._getChartOptions(unit, titleBase, unit, isStacked)
        });
    }

    /**
     * Generates an array of time labels (moment objects) for the X-axis.
     * @param {number} startTs - Start timestamp (Unix seconds).
     * @param {number} endTs - End timestamp (Unix seconds).
     * @param {number} [intervalMinutes=1] - Interval in minutes.
     * @returns {Array<moment>} Array of moment objects.
     * @private
     */
    _generateTimeLabels(startTs, endTs, intervalMinutes = 1) {
        const labels = [];
        let current = moment.unix(startTs);
        const end = moment.unix(endTs);
        while (current.isSameOrBefore(end)) {
            labels.push(current.clone());
            current.add(intervalMinutes, 'minutes');
        }
        return labels;
    }

    /**
     * Fills missing data points in a time series with nulls.
     * @param {Array<{x: number, y: number}>} data - Sorted array of data points (x is timestamp in ms).
     * @param {Array<moment>} timeLabels - Array of moment objects representing the complete time scale.
     * @returns {Array<{x: number, y: number|null}>} Data series with nulls for missing points.
     */
    fillMissingDataPoints(data, timeLabels) {
        if (!data || data.length === 0) {
            return timeLabels.map(label => ({ x: label.valueOf(), y: null }));
        }
        const filledData = [];
        let dataIndex = 0;
        timeLabels.forEach(labelMoment => {
            const labelTs = labelMoment.valueOf();
            if (dataIndex < data.length && data[dataIndex].x === labelTs) {
                filledData.push(data[dataIndex]);
                dataIndex++;
            } else {
                filledData.push({ x: labelTs, y: null });
            }
        });
        return filledData;
    }
    
    /**
     * Updates all charts with new metrics data.
     * @param {Array<object>|null} metricsData - Array of metric objects from the API.
     * @param {number} startTs - Start timestamp of the metrics range (Unix seconds).
     * @param {number} endTs - End timestamp of the metrics range (Unix seconds).
     */
    updateAllCharts(metricsData, startTs, endTs) {
        this._resetColorAssignments(); // Reset colors for a new update
        if (!metricsData) metricsData = [];

        const timeLabels = this._generateTimeLabels(startTs, endTs);

        // Group metrics by label and remote
        const groupedMetrics = metricsData.reduce((acc, m) => {
            const key = `${m.label}-${m.remote}`;
            if (!acc[key]) acc[key] = [];
            acc[key].push(m);
            return acc;
        }, {});

        // Clear existing datasets from all charts
        Object.values(this.charts).forEach(chart => chart.data.datasets = []);
        
        // Populate datasets for each group
        for (const groupKey in groupedMetrics) {
            const groupData = groupedMetrics[groupKey].sort((a,b) => a.timestamp - b.timestamp); // Sort by time
            const [label, remote] = groupKey.split('-');
            const color = this._getColorForGroup(groupKey);

            // Connection Count
            const connCountData = groupData.map(m => ({ x: m.timestamp * 1000, y: (m.tcp_connection_count || 0) + (m.udp_connection_count || 0) }));
            this.charts.connectionCount.data.datasets.push(this._getDatasetConfig(`${label} (${remote})`, color, false));
            this.charts.connectionCount.data.datasets.slice(-1)[0].data = this.fillMissingDataPoints(connCountData, timeLabels);
            
            // Handshake Duration
            const handshakeData = groupData.map(m => ({ x: m.timestamp * 1000, y: Math.max(m.tcp_handshake_duration || 0, m.udp_handshake_duration || 0) }));
            this.charts.handshakeDuration.data.datasets.push(this._getDatasetConfig(`${label} (${remote})`, color, false));
            this.charts.handshakeDuration.data.datasets.slice(-1)[0].data = this.fillMissingDataPoints(handshakeData, timeLabels);

            // Ping Latency
            const pingData = groupData.map(m => ({ x: m.timestamp * 1000, y: m.ping_latency || 0 }));
            this.charts.pingLatency.data.datasets.push(this._getDatasetConfig(`${label} (${remote})`, color, false));
            this.charts.pingLatency.data.datasets.slice(-1)[0].data = this.fillMissingDataPoints(pingData, timeLabels);
            
            // Network Transmit Bytes (Stacked Area)
            const tcpNetData = groupData.map(m => ({ x: m.timestamp * 1000, y: (m.tcp_network_transmit_bytes || 0) / Config.BYTE_TO_MB }));
            const udpNetData = groupData.map(m => ({ x: m.timestamp * 1000, y: (m.udp_network_transmit_bytes || 0) / Config.BYTE_TO_MB }));
            
            const tcpDataset = this._getDatasetConfig(`${label} (${remote}) TCP`, color, true); // isFilledArea = true
            tcpDataset.data = this.fillMissingDataPoints(tcpNetData, timeLabels);
            this.charts.networkTransmitBytes.data.datasets.push(tcpDataset);
            
            // Slightly different color for UDP of the same group
            const udpColor = color.replace(/, ?1\)/, ', 0.7)').replace('rgba', 'rgb'); // Make it a bit different and less opaque
            const udpDataset = this._getDatasetConfig(`${label} (${remote}) UDP`, udpColor, true);
            udpDataset.data = this.fillMissingDataPoints(udpNetData, timeLabels);
            this.charts.networkTransmitBytes.data.datasets.push(udpDataset);
        }

        // Update all charts
        Object.values(this.charts).forEach(chart => {
            chart.data.labels = timeLabels.map(m => m.valueOf()); // Pass timestamps for X-axis
            chart.options.scales.x.min = startTs * 1000;
            chart.options.scales.x.max = endTs * 1000;
            chart.update('quiet');
        });
    }
}


/**
 * Manages filter selection for rule metrics.
 */
class FilterManagerRule {
    /**
     * @param {ApiService} apiService
     * @param {ChartManager} chartManager
     * @param {DateRangeManagerRule} dateRangeManager
     */
    constructor(apiService, chartManager, dateRangeManager) {
        this.apiService = apiService;
        this.chartManager = chartManager;
        this.dateRangeManager = dateRangeManager;
        this.$labelFilter = document.getElementById('labelFilter');
        this.$remoteFilter = document.getElementById('remoteFilter');
        this.configData = null; // To store fetched config
    }

    async init() {
        await this._populateLabelFilter();
        this.$labelFilter.addEventListener('change', () => this._onLabelChange());
        this.$remoteFilter.addEventListener('change', () => this._triggerDataRefresh());
        // Initial data refresh will be triggered by DateRangeManager after it's ready
    }

    async _populateLabelFilter() {
        this.configData = await this.apiService.fetchConfig();
        if (!this.configData || !this.configData.relay_configs) {
            console.error('Failed to fetch or parse config for filters.');
            return;
        }
        const labels = [...new Set(this.configData.relay_configs.map(rc => rc.label))].sort();
        this.$labelFilter.innerHTML = '<option value="">All</option>'; // Reset
        labels.forEach(label => {
            const option = document.createElement('option');
            option.value = label;
            option.textContent = label;
            this.$labelFilter.appendChild(option);
        });
        this._updateRemoteFilter(); // Populate remotes for "All" or first label
    }

    _updateRemoteFilter() {
        const selectedLabel = this.$labelFilter.value;
        let remotes = new Set();
        if (this.configData && this.configData.relay_configs) {
            this.configData.relay_configs.forEach(rc => {
                if (!selectedLabel || rc.label === selectedLabel) {
                    (rc.remotes || []).forEach(remote => remotes.add(remote.address || remote)); // Assuming remote can be obj or string
                }
            });
        }
        this.$remoteFilter.innerHTML = '<option value="">All</option>'; // Reset
        Array.from(remotes).sort().forEach(remote => {
            const option = document.createElement('option');
            option.value = remote;
            option.textContent = remote;
            this.$remoteFilter.appendChild(option);
        });
    }

    _onLabelChange() {
        this._updateRemoteFilter();
        this._triggerDataRefresh();
    }

    async _triggerDataRefresh() {
        const filters = this.getCurrentFilters();
        const range = this.dateRangeManager.getCurrentDateRange();
        if (!range.start || !range.end) {
            console.warn("Date range not yet available for data refresh.");
            return;
        }
        const startTs = Math.floor(range.start.getTime() / 1000);
        const endTs = Math.floor(range.end.getTime() / 1000);
        
        const metricsData = await this.apiService.fetchRuleMetrics(startTs, endTs, filters.label, filters.remote);
        this.chartManager.updateAllCharts(metricsData, startTs, endTs);
    }

    getCurrentFilters() {
        return {
            label: this.$labelFilter.value,
            remote: this.$remoteFilter.value
        };
    }
}

/**
 * Manages date range selection for rule metrics.
 */
class DateRangeManagerRule {
    constructor(apiService, chartManager, filterManager) { // FilterManager needed to trigger refresh
        this.apiService = apiService; // May not be used directly if FilterManager handles API calls
        this.chartManager = chartManager; // May not be used directly
        this.filterManager = filterManager;
        this.$dateRangeDropdown = document.getElementById('dateRangeDropdownRule');
        this.$dateRangeText = document.getElementById('dateRangeTextRule');
        this.$dateRangeInput = document.getElementById('dateRangeInputRule');
        this.flatpickrInstance = null;
        this.currentRange = this._calculateDateRange(Config.DEFAULT_TIME_WINDOW_MINS + 'm'); // Initial default
    }

    init() {
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

        const triggerButton = this.$dateRangeDropdown.querySelector('.button');
        triggerButton.addEventListener('click', (e) => {
            e.stopPropagation();
            this.$dateRangeDropdown.classList.toggle('is-active');
        });
         document.addEventListener('click', (e) => {
            if (!this.$dateRangeDropdown.contains(e.target)) {
                this.$dateRangeDropdown.classList.remove('is-active');
            }
        });

        this.flatpickrInstance = flatpickr(this.$dateRangeInput, {
            mode: 'range', enableTime: true, dateFormat: 'Y-m-d H:i',
            onChange: (selectedDates) => {
                if (selectedDates.length === 2) {
                    this.handleCustomRange(selectedDates);
                    this.$dateRangeText.textContent = `${moment(selectedDates[0]).format('MMM D, HH:mm')} - ${moment(selectedDates[1]).format('MMM D, HH:mm')}`;
                    this.$dateRangeDropdown.classList.remove('is-active');
                }
            },
        });
        this.$dateRangeInput.addEventListener('click', e => e.stopPropagation());
        
        // Set initial text for default range
        const defaultRangeKey = Config.DEFAULT_TIME_WINDOW_MINS === 30 ? '30m' : (Config.DEFAULT_TIME_WINDOW_MINS + 'm');
        const defaultRangeLink = this.$dateRangeDropdown.querySelector(`.dropdown-item[data-range="${defaultRangeKey}"]`);
        if (defaultRangeLink) this.$dateRangeText.textContent = defaultRangeLink.textContent;

    }

    handlePresetRange(rangeKey) {
        this.currentRange = this._calculateDateRange(rangeKey);
        this.filterManager._triggerDataRefresh();
    }

    handleCustomRange(selectedDates) {
        this.currentRange = { start: selectedDates[0], end: selectedDates[1] };
        this.filterManager._triggerDataRefresh();
    }

    _calculateDateRange(rangeKey) {
        const end = new Date();
        let start = new Date();
        const durationMatch = rangeKey.match(/^(\d+)([mhd])$/); // 30m, 1h, 7d
        if (durationMatch) {
            const value = parseInt(durationMatch[1]);
            const unit = durationMatch[2];
            if (unit === 'm') start.setMinutes(end.getMinutes() - value);
            else if (unit === 'h') start.setHours(end.getHours() - value);
            else if (unit === 'd') start.setDate(end.getDate() - value);
        } else { // Default if parse fails
            start.setMinutes(end.getMinutes() - Config.DEFAULT_TIME_WINDOW_MINS);
        }
        return { start, end };
    }
    
    getCurrentDateRange() {
        return this.currentRange;
    }
}

/**
 * Main module for the Rule Metrics dashboard.
 */
class RuleMetricsModule {
    constructor() {
        this.apiService = new ApiService();
        this.chartManager = new ChartManager();
        // Order of instantiation matters for dependencies
        this.dateRangeManager = new DateRangeManagerRule(this.apiService, this.chartManager, null); // filterManager is set later
        this.filterManager = new FilterManagerRule(this.apiService, this.chartManager, this.dateRangeManager);
        this.dateRangeManager.filterManager = this.filterManager; // Circular dependency resolution
    }

    async init() {
        this.chartManager.initializeCharts();
        this.dateRangeManager.init(); // Sets up UI and default range
        await this.filterManager.init(); // Populates filters
        
        // Initial data load after everything is set up
        await this.filterManager._triggerDataRefresh(); 
    }
}

// Initialize the module when the DOM is fully loaded.
document.addEventListener('DOMContentLoaded', () => {
    const ruleMetricsModule = new RuleMetricsModule();
    ruleMetricsModule.init().catch(error => {
        console.error("Failed to initialize Rule Metrics module:", error);
    });
});
