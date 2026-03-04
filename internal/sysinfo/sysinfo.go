// Package sysinfo provides system information gathering.
package sysinfo

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

// CPUInfo holds CPU information.
type CPUInfo struct {
	PerCPU    []float64
	Overall   float64
	Freq      float64 // GHz
	Load1     float64
	Load5     float64
	Load15    float64
	Temp      float64 // Celsius, 0 if unavailable
	CoreCount int
}

// GetCPUInfo returns current CPU information.
func GetCPUInfo() (*CPUInfo, error) {
	info := &CPUInfo{}

	// Per-CPU percentages
	perCPU, err := cpu.Percent(0, true)
	if err != nil {
		return nil, err
	}
	info.PerCPU = perCPU
	info.CoreCount = len(perCPU)

	// Overall percentage
	overall, err := cpu.Percent(0, false)
	if err == nil && len(overall) > 0 {
		info.Overall = overall[0]
	}

	// Frequency
	freqs, err := cpu.Info()
	if err == nil && len(freqs) > 0 {
		info.Freq = freqs[0].Mhz / 1000.0
	}

	// Load averages
	loadAvg, err := load.Avg()
	if err == nil {
		info.Load1 = loadAvg.Load1
		info.Load5 = loadAvg.Load5
		info.Load15 = loadAvg.Load15
	}

	// Temperature
	temps, err := host.SensorsTemperatures()
	if err == nil {
		for _, t := range temps {
			// Look for CPU temp sensors (use prefix matching - gopsutil returns keys like "k10temp_tctl")
			if hasPrefixAny(t.SensorKey, "coretemp", "k10temp", "cpu_thermal", "zenpower", "cpu-thermal") {
				info.Temp = t.Temperature
				break
			}
		}
		// Fallback: skip ACPI thermal zones (often ambient temp), use first non-ACPI sensor
		if info.Temp == 0 {
			for _, t := range temps {
				if !hasPrefixAny(t.SensorKey, "acpitz", "thermal_zone") {
					info.Temp = t.Temperature
					break
				}
			}
		}
	}

	return info, nil
}

// MemInfo holds memory information.
type MemInfo struct {
	Total       uint64
	Used        uint64
	Available   uint64
	UsedPercent float64
	SwapTotal   uint64
	SwapUsed    uint64
	SwapPercent float64
	ZswapUsed   uint64 // Zswap compressed pool size (like htop shows)
}

// GetMemInfo returns current memory information.
func GetMemInfo() (*MemInfo, error) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	info := &MemInfo{
		Total:       vm.Total,
		Used:        vm.Used,
		Available:   vm.Available,
		UsedPercent: vm.UsedPercent,
	}

	swap, err := mem.SwapMemory()
	if err == nil {
		info.SwapTotal = swap.Total
		info.SwapUsed = swap.Used
		if swap.Total > 0 {
			info.SwapPercent = swap.UsedPercent
		}
	}

	// Read zswap usage (like htop 3.3+ shows)
	info.ZswapUsed = readZswapUsed()

	return info, nil
}

// readZswapUsed reads the zswap compressed pool size from /proc/meminfo
func readZswapUsed() uint64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "Zswap:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.ParseUint(fields[1], 10, 64)
				return kb * 1024 // Convert KB to bytes
			}
		}
	}
	return 0
}

// ProcessMemInfo holds memory info for a process group.
type ProcessMemInfo struct {
	Name    string
	RSS     uint64
	Percent float64
	Count   int
}

// Process grouping patterns
var processGroups = map[string]*regexp.Regexp{
	"chrome":        regexp.MustCompile(`^(chrome|chromium|Chrome|Chromium)$`),
	"firefox":       regexp.MustCompile(`^(firefox|Firefox|firefox-esr)$`),
	"code":          regexp.MustCompile(`^(code|Code|code-oss)$`),
	"electron":      regexp.MustCompile(`^(electron|Electron)$`),
	"slack":         regexp.MustCompile(`^(slack|Slack)$`),
	"discord":       regexp.MustCompile(`^(discord|Discord)$`),
	"spotify":       regexp.MustCompile(`^(spotify|Spotify)$`),
	"cursor":        regexp.MustCompile(`^(cursor|Cursor)$`),
	"crush":         regexp.MustCompile(`^(crush|Crush)$`),
	"node":          regexp.MustCompile(`^(node|nodejs|Node)$`),
	"python":        regexp.MustCompile(`^(python|python3|Python)$`),
	"java":          regexp.MustCompile(`^(java|Java)$`),
	"rust-analyzer": regexp.MustCompile(`^rust-analyzer$`),
	"gopls":         regexp.MustCompile(`^gopls$`),
	"docker":        regexp.MustCompile(`^(docker|dockerd|containerd)$`),
	"gnome":         regexp.MustCompile(`^gnome-`),
	"systemd":       regexp.MustCompile(`^systemd`),
}

func getProcessGroup(name string) string {
	for group, pattern := range processGroups {
		if pattern.MatchString(name) {
			return group
		}
	}
	return name
}

// GetTopProcesses returns the top N processes by memory usage.
// Reads /proc directly to avoid expensive gopsutil calls.
func GetTopProcesses(n int) ([]ProcessMemInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}
	totalMem := memInfo.Total

	// Aggregate by group
	groups := make(map[string]*ProcessMemInfo)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid := 0
		for _, c := range entry.Name() {
			if c < '0' || c > '9' {
				pid = -1
				break
			}
			pid = pid*10 + int(c-'0')
		}
		if pid <= 0 {
			continue
		}

		// Read /proc/[pid]/statm for memory (much faster than /proc/[pid]/status)
		name, rss, err := readProcStatm(pid)
		if err != nil {
			continue
		}

		group := getProcessGroup(name)
		if g, ok := groups[group]; ok {
			g.RSS += rss
			g.Count++
		} else {
			groups[group] = &ProcessMemInfo{
				Name:  group,
				RSS:   rss,
				Count: 1,
			}
		}
	}

	// Calculate percentages and convert to slice
	result := make([]ProcessMemInfo, 0, len(groups))
	for _, g := range groups {
		g.Percent = float64(g.RSS) / float64(totalMem) * 100
		result = append(result, *g)
	}

	// Sort by RSS
	sort.Slice(result, func(i, j int) bool {
		return result[i].RSS > result[j].RSS
	})

	if len(result) > n {
		result = result[:n]
	}

	return result, nil
}

// readProcStatm reads /proc/[pid]/statm and /proc/[pid]/comm for memory info
func readProcStatm(pid int) (name string, rss uint64, err error) {
	// Read name from comm (simpler than parsing stat)
	nameData, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return "", 0, err
	}
	name = strings.TrimSpace(string(nameData))

	// Read statm: size resident shared text lib data dt
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return "", 0, err
	}

	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return "", 0, fmt.Errorf("invalid statm")
	}

	// resident is in pages, convert to bytes (page size is typically 4096)
	pages, _ := strconv.ParseUint(fields[1], 10, 64)
	rss = pages * 4096

	return name, rss, nil
}

// FormatBytes formats bytes to human-readable string.
func FormatBytes(b uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return formatFloat1(float64(b)/GB) + "G"
	case b >= MB:
		return itoa(int(float64(b)/MB+0.5)) + "M"
	case b >= KB:
		return itoa(int(float64(b)/KB+0.5)) + "K"
	default:
		return itoa(int(b)) + "B"
	}
}

// formatFloat1 formats a float with 1 decimal place.
func formatFloat1(f float64) string {
	// Round to 1 decimal
	i := int(f*10 + 0.5)
	whole := i / 10
	frac := i % 10
	return itoa(whole) + "." + string(rune('0'+frac))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

// hasPrefixAny returns true if s has any of the given prefixes.
func hasPrefixAny(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if len(s) >= len(p) && s[:len(p)] == p {
			return true
		}
	}
	return false
}

// ProcessCPUInfo holds CPU usage info for a process group.
type ProcessCPUInfo struct {
	Name    string
	Percent float64
	Count   int
}

// cpuTimesCache caches process CPU times for delta calculation
var (
	cpuTimesCache     = make(map[int]procStat) // pid -> stat
	cpuTimesCacheMu   sync.Mutex
	lastCPUSampleTime time.Time
	numCPU            = float64(runtime.NumCPU())
	clockTicks        = float64(100) // Linux default, could read from sysconf
)

type procStat struct {
	name  string
	utime uint64
	stime uint64
}

// GetTopProcessesByCPU returns the top N processes by CPU usage.
// Reads /proc directly to avoid expensive gopsutil calls.
func GetTopProcessesByCPU(n int) ([]ProcessCPUInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	now := time.Now()
	cpuTimesCacheMu.Lock()
	defer cpuTimesCacheMu.Unlock()

	elapsed := now.Sub(lastCPUSampleTime).Seconds()
	if elapsed < 0.1 {
		elapsed = 0.1
	}

	groups := make(map[string]*ProcessCPUInfo)
	newCache := make(map[int]procStat)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid := 0
		for _, c := range entry.Name() {
			if c < '0' || c > '9' {
				pid = -1
				break
			}
			pid = pid*10 + int(c-'0')
		}
		if pid <= 0 {
			continue
		}

		// Read /proc/[pid]/stat
		stat, err := readProcStat(pid)
		if err != nil {
			continue
		}
		newCache[pid] = stat

		// Calculate CPU percent from delta
		var cpuPct float64
		if prev, ok := cpuTimesCache[pid]; ok && elapsed > 0 {
			udelta := stat.utime - prev.utime
			sdelta := stat.stime - prev.stime
			totalDelta := float64(udelta+sdelta) / clockTicks
			cpuPct = (totalDelta / elapsed) * 100.0
		}

		if cpuPct < 0.1 {
			continue
		}

		group := getProcessGroup(stat.name)
		if g, ok := groups[group]; ok {
			g.Percent += cpuPct
			g.Count++
		} else {
			groups[group] = &ProcessCPUInfo{
				Name:    group,
				Percent: cpuPct,
				Count:   1,
			}
		}
	}

	cpuTimesCache = newCache
	lastCPUSampleTime = now

	result := make([]ProcessCPUInfo, 0, len(groups))
	for _, g := range groups {
		result = append(result, *g)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Percent > result[j].Percent
	})

	if len(result) > n {
		result = result[:n]
	}

	return result, nil
}

// readProcStat reads /proc/[pid]/stat and extracts name, utime, stime
func readProcStat(pid int) (procStat, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return procStat{}, err
	}

	// Format: pid (name) state ppid pgrp session tty_nr tpgid flags
	//         minflt cminflt majflt cmajflt utime stime ...
	// Find the name between ( and )
	start := -1
	end := -1
	for i, c := range data {
		if c == '(' && start == -1 {
			start = i + 1
		}
		if c == ')' {
			end = i
		}
	}
	if start == -1 || end == -1 || end <= start {
		return procStat{}, fmt.Errorf("invalid stat format")
	}

	name := string(data[start:end])

	// Parse fields after the name
	fields := strings.Fields(string(data[end+2:])) // skip ") "
	if len(fields) < 13 {
		return procStat{}, fmt.Errorf("not enough fields")
	}

	// utime is field 11 (0-indexed), stime is field 12 (after the name)
	// But we skipped "pid (name) ", so utime is index 11, stime is 12
	utime, _ := strconv.ParseUint(fields[11], 10, 64)
	stime, _ := strconv.ParseUint(fields[12], 10, 64)

	return procStat{name: name, utime: utime, stime: stime}, nil
}

// TempInfo holds temperature sensor information.
type TempInfo struct {
	Label string
	Temp  float64
}

// GetTemperatures returns categorized temperature readings.
func GetTemperatures() ([]TempInfo, error) {
	temps, err := host.SensorsTemperatures()
	if err != nil {
		return nil, err
	}

	var result []TempInfo

	// Map of sensor prefixes to friendly labels
	sensorLabels := map[string]string{
		"k10temp_tctl":    "CPU",
		"k10temp_tccd":    "CCD",
		"coretemp_core":   "Core",
		"coretemp_pack":   "CPU",
		"nvme_composite":  "NVMe",
		"nvme_sensor":     "NVMe",
		"amdgpu_edge":     "GPU",
		"amdgpu_junction": "GPU Hot",
		"amdgpu_mem":      "VRAM",
		"nouveau":         "GPU",
		"radeon":          "GPU",
		"iwlwifi":         "WiFi",
		"mt7921":          "WiFi",
		"r8169":           "NIC",
	}

	seen := make(map[string]bool)

	for _, t := range temps {
		if t.Temperature <= 0 || t.Temperature > 150 {
			continue // Skip invalid readings
		}

		// Skip ACPI thermal zones (often inaccurate)
		if hasPrefixAny(t.SensorKey, "acpitz", "thermal_zone") {
			continue
		}

		// Find matching label
		label := ""
		for prefix, lbl := range sensorLabels {
			if hasPrefixAny(t.SensorKey, prefix) {
				label = lbl
				break
			}
		}

		if label == "" {
			continue // Skip unknown sensors
		}

		// Deduplicate by label (take first/hottest)
		if seen[label] {
			continue
		}
		seen[label] = true

		result = append(result, TempInfo{
			Label: label,
			Temp:  t.Temperature,
		})
	}

	// Sort by label alphabetically
	sort.Slice(result, func(i, j int) bool {
		return result[i].Label < result[j].Label
	})

	return result, nil
}
