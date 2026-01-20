// Package sysinfo provides system information gathering.
package sysinfo

import (
	"regexp"
	"sort"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
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
			// Look for CPU temp sensors
			if t.SensorKey == "coretemp" || t.SensorKey == "k10temp" ||
				t.SensorKey == "cpu_thermal" || t.SensorKey == "zenpower" {
				info.Temp = t.Temperature
				break
			}
		}
		// Fallback to first sensor if no CPU sensor found
		if info.Temp == 0 && len(temps) > 0 {
			info.Temp = temps[0].Temperature
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

	return info, nil
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
func GetTopProcesses(n int) ([]ProcessMemInfo, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}

	// Aggregate by group
	groups := make(map[string]*ProcessMemInfo)
	
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}
	totalMem := memInfo.Total

	for _, p := range procs {
		name, err := p.Name()
		if err != nil {
			continue
		}
		
		meminfo, err := p.MemoryInfo()
		if err != nil {
			continue
		}

		group := getProcessGroup(name)
		if g, ok := groups[group]; ok {
			g.RSS += meminfo.RSS
			g.Count++
		} else {
			groups[group] = &ProcessMemInfo{
				Name:  group,
				RSS:   meminfo.RSS,
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
