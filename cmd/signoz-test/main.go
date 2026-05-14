package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aleksclark/go-turing-smart-screen/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Config loaded:\n  URL: %s\n  Key: %s...%s\n\n", cfg.SignozURL, cfg.SignozAPIKey[:8], cfg.SignozAPIKey[len(cfg.SignozAPIKey)-4:])

	client := &http.Client{Timeout: 15 * time.Second}

	// Test 1: List alerts
	fmt.Println("=== Test 1: List Alerts ===")
	testAlerts(client, cfg)

	// Test 2-4: Try different timestamp formats
	fmt.Println("\n=== Test 2: moosefs - start/end as millis ===")
	testMetricWithTimes(client, cfg, "moosefs.cluster.total_space", "millis")

	fmt.Println("\n=== Test 3: moosefs - start/end as nanos ===")
	testMetricWithTimes(client, cfg, "moosefs.cluster.total_space", "nanos")

	fmt.Println("\n=== Test 4: moosefs - start/end as seconds ===")
	testMetricWithTimes(client, cfg, "moosefs.cluster.total_space", "seconds")

	// Test 5: panelType=graph (time_series)
	fmt.Println("\n=== Test 5: moosefs - panelType=graph ===")
	testMetricGraph(client, cfg, "moosefs.cluster.total_space")

	// Test 6: system.cpu with graph
	fmt.Println("\n=== Test 6: system.cpu.load_average.1m - graph + group_by ===")
	testMetricGrouped(client, cfg)
}

func doRequest(client *http.Client, cfg *config.Config, method, url string, body string) (int, []byte) {
	var req *http.Request
	if body != "" {
		req, _ = http.NewRequest(method, url, strings.NewReader(body))
	} else {
		req, _ = http.NewRequest(method, url, nil)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("SIGNOZ-API-KEY", cfg.SignozAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return 0, nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func printBody(status int, body []byte) {
	fmt.Printf("  Status: %d\n", status)
	if body == nil {
		return
	}
	s := string(body)
	if len(s) > 1500 {
		s = s[:1500] + "..."
	}
	// Try pretty print
	var obj interface{}
	if json.Unmarshal(body, &obj) == nil {
		pretty, _ := json.MarshalIndent(obj, "  ", "  ")
		if len(pretty) < 2000 {
			fmt.Printf("  %s\n", pretty)
			return
		}
	}
	fmt.Printf("  %s\n", s)
}

func testAlerts(client *http.Client, cfg *config.Config) {
	status, body := doRequest(client, cfg, "GET", cfg.SignozURL+"/api/v1/rules", "")
	fmt.Printf("  Status: %d\n", status)
	if body != nil {
		// Just show count
		var result struct {
			Data struct {
				Rules []struct {
					Alert string `json:"alert"`
					State string `json:"state"`
				} `json:"rules"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &result) == nil {
			fmt.Printf("  Found %d rules:\n", len(result.Data.Rules))
			for _, r := range result.Data.Rules {
				fmt.Printf("    - %s [%s]\n", r.Alert, r.State)
			}
		}
	}
}

func testMetricWithTimes(client *http.Client, cfg *config.Config, metric, mode string) {
	now := time.Now()
	var start, end int64
	switch mode {
	case "millis":
		start = now.Add(-5*time.Minute).UnixMilli()
		end = now.UnixMilli()
	case "nanos":
		start = now.Add(-5*time.Minute).UnixNano()
		end = now.UnixNano()
	case "seconds":
		start = now.Add(-5*time.Minute).Unix()
		end = now.Unix()
	}

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
	}`, start, end, metric)

	status, body := doRequest(client, cfg, "POST", cfg.SignozURL+"/api/v3/query_range", payload)
	printBody(status, body)
}

func testMetricGraph(client *http.Client, cfg *config.Config, metric string) {
	now := time.Now()
	start := now.Add(-5 * time.Minute).UnixMilli()
	end := now.UnixMilli()

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
					"stepInterval": 60
				}
			}
		}
	}`, start, end, metric)

	status, body := doRequest(client, cfg, "POST", cfg.SignozURL+"/api/v3/query_range", payload)
	printBody(status, body)
}

func testMetricGrouped(client *http.Client, cfg *config.Config) {
	now := time.Now()
	start := now.Add(-5 * time.Minute).UnixMilli()
	end := now.UnixMilli()

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
					"aggregateOperator": "avg",
					"aggregateAttribute": {
						"key": "system.cpu.load_average.1m",
						"dataType": "float64",
						"type": "Gauge",
						"isMonotonic": false
					},
					"timeAggregation": "avg",
					"spaceAggregation": "avg",
					"disabled": false,
					"expression": "A",
					"groupBy": [{"key": "host.name", "dataType": "string", "type": "tag", "isColumn": false}],
					"stepInterval": 60
				}
			}
		}
	}`, start, end)

	status, body := doRequest(client, cfg, "POST", cfg.SignozURL+"/api/v3/query_range", payload)
	printBody(status, body)
}
