// Package signoz provides a client for querying SigNoz metrics and alerts.
package signoz

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client queries SigNoz for metrics and alert state.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client

	mu     sync.RWMutex
	cache  map[string]*cacheEntry
	alerts []Alert
}

type cacheEntry struct {
	value     float64
	series    map[string]float64
	fetchedAt time.Time
}

// MetricResult holds a single metric value or grouped values.
type MetricResult struct {
	Value  float64
	Series map[string]float64 // host -> value for grouped queries
}

// Alert represents a firing SigNoz alert.
type Alert struct {
	Name     string
	State    string
	Severity string
	Labels   map[string]string
}

// MooseFSStats holds MooseFS cluster space information.
type MooseFSStats struct {
	TotalSpace float64
	AvailSpace float64
	UsedPct    float64
}

// NodeStats holds per-node CPU and memory metrics.
type NodeStats struct {
	MemoryPct map[string]float64 // host -> memory usage percent
	CPULoad   map[string]float64 // host -> 1m load average
}

// New creates a new SigNoz client.
func New(baseURL, apiKey string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:3301"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: make(map[string]*cacheEntry),
	}
}

// FetchMooseFS queries MooseFS cluster space metrics.
func (c *Client) FetchMooseFS() (*MooseFSStats, error) {
	now := time.Now().UnixMilli()
	start := now - 300000 // 5 min

	total, err := c.queryMetricLast("moosefs.cluster.total_space", start, now)
	if err != nil {
		return nil, fmt.Errorf("total_space: %w", err)
	}

	avail, err := c.queryMetricLast("moosefs.cluster.avail_space", start, now)
	if err != nil {
		return nil, fmt.Errorf("avail_space: %w", err)
	}

	var pct float64
	if total > 0 {
		pct = (total - avail) / total * 100
	}

	return &MooseFSStats{
		TotalSpace: total,
		AvailSpace: avail,
		UsedPct:    pct,
	}, nil
}

// FetchNodeStats queries per-node memory usage and CPU load.
func (c *Client) FetchNodeStats() (*NodeStats, error) {
	now := time.Now().UnixMilli()
	start := now - 300000

	// Query memory: used bytes and total bytes per host
	memUsed, err := c.queryMetricGrouped("system.memory.usage", "host.name", "avg", "state = 'used'", start, now)
	if err != nil {
		memUsed = make(map[string]float64)
	}

	// Total = used + free + cached + buffered, but we can get it from avg of all states summed
	// Simpler: query each node's total as (used + free + buffered + cached) via sum without filter
	// Actually on OTel collector, system.memory.usage grouped by host with no state filter and sum agg
	// gives total per state per host — we need a different approach.
	// Best: hardcode known total or use system.memory.limit if available.
	// Practical: assume 16GB per node (common), let user see raw GB if wrong.
	// Better: compute percentage from used / (used + free)
	memFree, _ := c.queryMetricGrouped("system.memory.usage", "host.name", "avg", "state = 'free'", start, now)
	memCached, _ := c.queryMetricGrouped("system.memory.usage", "host.name", "avg", "state = 'cached'", start, now)
	memBuffered, _ := c.queryMetricGrouped("system.memory.usage", "host.name", "avg", "state = 'buffered'", start, now)

	memPct := make(map[string]float64)
	for host, used := range memUsed {
		total := used
		if v, ok := memFree[host]; ok {
			total += v
		}
		if v, ok := memCached[host]; ok {
			total += v
		}
		if v, ok := memBuffered[host]; ok {
			total += v
		}
		if total > 0 {
			memPct[host] = used / total * 100
		}
	}

	cpuLoad, err := c.queryMetricGrouped("system.cpu.load_average.1m", "host.name", "avg", "", start, now)
	if err != nil {
		cpuLoad = make(map[string]float64)
	}

	return &NodeStats{
		MemoryPct: memPct,
		CPULoad:   cpuLoad,
	}, nil
}

// FetchAlerts returns currently firing alerts.
func (c *Client) FetchAlerts() ([]Alert, error) {
	url := c.baseURL + "/api/v1/rules"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Status string `json:"status"`
		Data   struct {
			Rules []struct {
				Alert  string            `json:"alert"`
				State  string            `json:"state"`
				Labels map[string]string `json:"labels"`
			} `json:"rules"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		// Try alternate format (SigNoz native)
		return c.parseNativeAlerts(body)
	}

	var alerts []Alert
	for _, r := range result.Data.Rules {
		if r.State == "firing" {
			alerts = append(alerts, Alert{
				Name:     r.Alert,
				State:    r.State,
				Severity: r.Labels["severity"],
				Labels:   r.Labels,
			})
		}
	}
	return alerts, nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("SIGNOZ-API-KEY", c.apiKey)
	}
}

func (c *Client) parseNativeAlerts(body []byte) ([]Alert, error) {
	var result struct {
		Rules []struct {
			Alert  string            `json:"alert"`
			State  string            `json:"state"`
			Labels map[string]string `json:"labels"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	var alerts []Alert
	for _, r := range result.Rules {
		if r.State == "firing" {
			alerts = append(alerts, Alert{
				Name:     r.Alert,
				State:    r.State,
				Severity: r.Labels["severity"],
				Labels:   r.Labels,
			})
		}
	}
	return alerts, nil
}

func (c *Client) queryMetricLast(metricName string, start, end int64) (float64, error) {
	payload := fmt.Sprintf(`{
		"start": %d,
		"end": %d,
		"compositeQuery": {
			"queryType": "builder",
			"panelType": "value",
			"builderQueries": {
				"A": {
					"dataSource": "metrics",
					"queryName": "A",
					"aggregateOperator": "avg",
					"aggregateAttribute": {
						"key": "%s",
						"dataType": "float64",
						"type": "Gauge",
						"isMonotonic": false
					},
					"timeAggregation": "avg",
					"spaceAggregation": "avg",
					"disabled": false,
					"expression": "A",
					"reduceTo": "last",
					"stepInterval": 60
				}
			}
		}
	}`, start, end, metricName)

	url := c.baseURL + "/api/v3/query_range"
	req, err := http.NewRequest("POST", url, strings.NewReader(payload))
	if err != nil {
		return 0, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("signoz returned %d: %s", resp.StatusCode, string(body))
	}

	return parseLastValue(body)
}

func (c *Client) queryMetricGrouped(metricName, groupBy, aggOp, filter string, start, end int64) (map[string]float64, error) {
	filterClause := ""
	if filter != "" {
		filterClause = fmt.Sprintf(`,"filters": {"items": [{"key": {"key": "%s"}, "op": "=", "value": "%s"}], "op": "AND"}`,
			strings.Split(filter, " = ")[0],
			strings.Trim(strings.Split(filter, " = ")[1], "'"))
	}

	payload := fmt.Sprintf(`{
		"start": %d,
		"end": %d,
		"compositeQuery": {
			"queryType": "builder",
			"panelType": "graph",
			"builderQueries": {
				"A": {
					"dataSource": "metrics",
					"queryName": "A",
					"aggregateOperator": "%s",
					"aggregateAttribute": {
						"key": "%s",
						"dataType": "float64",
						"type": "Gauge",
						"isMonotonic": false
					},
					"timeAggregation": "%s",
					"spaceAggregation": "%s",
					"disabled": false,
					"expression": "A",
					"groupBy": [{"key": "%s", "dataType": "string", "type": "tag", "isColumn": false}],
					"stepInterval": 60
					%s
				}
			}
		}
	}`, start, end, aggOp, metricName, aggOp, aggOp, groupBy, filterClause)

	url := c.baseURL + "/api/v3/query_range"
	req, err := http.NewRequest("POST", url, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("signoz returned %d: %s", resp.StatusCode, string(body))
	}

	return parseGroupedValues(body)
}

func parseLastValue(body []byte) (float64, error) {
	var result struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				QueryName string `json:"queryName"`
				Series    []struct {
					Values []struct {
						Timestamp int64  `json:"timestamp"`
						Value     string `json:"value"`
					} `json:"values"`
				} `json:"series"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("parse response: %w", err)
	}

	for _, r := range result.Data.Result {
		for _, s := range r.Series {
			if len(s.Values) > 0 {
				v, err := strconv.ParseFloat(s.Values[len(s.Values)-1].Value, 64)
				if err != nil {
					return 0, nil
				}
				return v, nil
			}
		}
	}
	return 0, nil
}

func parseGroupedValues(body []byte) (map[string]float64, error) {
	var result struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				QueryName string `json:"queryName"`
				Series    []struct {
					Labels map[string]string `json:"labels"`
					Values []struct {
						Timestamp int64  `json:"timestamp"`
						Value     string `json:"value"`
					} `json:"values"`
				} `json:"series"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	values := make(map[string]float64)
	for _, r := range result.Data.Result {
		for _, s := range r.Series {
			var key string
			for _, v := range s.Labels {
				key = v
				break
			}
			if key != "" && len(s.Values) > 0 {
				v, err := strconv.ParseFloat(s.Values[len(s.Values)-1].Value, 64)
				if err == nil {
					values[key] = v
				}
			}
		}
	}
	return values, nil
}

// FormatBytes formats bytes to human-readable form.
func FormatBytes(bytes float64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
		PB = TB * 1024
	)
	switch {
	case bytes >= PB:
		return fmt.Sprintf("%.1f PB", bytes/PB)
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", bytes/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", bytes/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.0f MB", bytes/MB)
	default:
		return fmt.Sprintf("%.0f KB", bytes/KB)
	}
}
