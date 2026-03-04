package sysinfo

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// GPUVendor identifies the GPU manufacturer.
type GPUVendor string

const (
	GPUVendorAMD    GPUVendor = "AMD"
	GPUVendorNVIDIA GPUVendor = "NVIDIA"
	GPUVendorNone   GPUVendor = ""
)

// GPUInfo holds GPU metrics.
type GPUInfo struct {
	Vendor     GPUVendor
	Name       string
	Temp       float64 // Celsius
	Load       float64 // Percentage (0-100)
	MemUsed    uint64  // Bytes
	MemTotal   uint64  // Bytes
	MemPercent float64 // Percentage (0-100)
	Power      float64 // Watts
}

// gpuDetection caches the detected GPU vendor and card path.
var (
	gpuDetectOnce sync.Once
	gpuVendor     GPUVendor
	gpuCardPath   string // e.g. /sys/class/drm/card0/device
	gpuHwmonPath  string // e.g. /sys/class/drm/card0/device/hwmon/hwmon5
)

func detectGPU() {
	gpuDetectOnce.Do(func() {
		entries, err := filepath.Glob("/sys/class/drm/card[0-9]*/device/vendor")
		if err != nil {
			return
		}
		for _, vendorFile := range entries {
			data, err := os.ReadFile(vendorFile)
			if err != nil {
				continue
			}
			vid := strings.TrimSpace(string(data))
			deviceDir := filepath.Dir(vendorFile)

			switch vid {
			case "0x1002":
				gpuVendor = GPUVendorAMD
				gpuCardPath = deviceDir
				gpuHwmonPath = findHwmon(deviceDir)
				return
			case "0x10de":
				gpuVendor = GPUVendorNVIDIA
				gpuCardPath = deviceDir
				gpuHwmonPath = findHwmon(deviceDir)
				return
			}
		}
	})
}

// findHwmon finds the hwmon subdirectory for a device.
func findHwmon(deviceDir string) string {
	hwmonDir := filepath.Join(deviceDir, "hwmon")
	entries, err := os.ReadDir(hwmonDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "hwmon") {
			return filepath.Join(hwmonDir, e.Name())
		}
	}
	return ""
}

// GetGPUInfo returns current GPU metrics.
// Returns nil if no supported GPU is detected.
func GetGPUInfo() *GPUInfo {
	detectGPU()

	switch gpuVendor {
	case GPUVendorAMD:
		return getAMDGPUInfo()
	case GPUVendorNVIDIA:
		return getNVIDIAGPUInfo()
	default:
		return nil
	}
}

// getAMDGPUInfo reads AMD GPU metrics from sysfs.
func getAMDGPUInfo() *GPUInfo {
	info := &GPUInfo{Vendor: GPUVendorAMD}

	info.Load = readSysfsFloat(filepath.Join(gpuCardPath, "gpu_busy_percent"))

	if gpuHwmonPath != "" {
		info.Temp = readSysfsFloat(filepath.Join(gpuHwmonPath, "temp1_input")) / 1000.0
		power := readSysfsFloat(filepath.Join(gpuHwmonPath, "power1_average"))
		if power == 0 {
			power = readSysfsFloat(filepath.Join(gpuHwmonPath, "power1_input"))
		}
		info.Power = power / 1_000_000.0 // microwatts to watts
	}

	info.MemUsed = readSysfsUint64(filepath.Join(gpuCardPath, "mem_info_vram_used"))
	info.MemTotal = readSysfsUint64(filepath.Join(gpuCardPath, "mem_info_vram_total"))
	if info.MemTotal > 0 {
		info.MemPercent = float64(info.MemUsed) / float64(info.MemTotal) * 100
	}

	return info
}

// getNVIDIAGPUInfo reads NVIDIA GPU metrics by shelling out to nvidia-smi.
func getNVIDIAGPUInfo() *GPUInfo {
	info := &GPUInfo{Vendor: GPUVendorNVIDIA}

	output, err := runNvidiaSmi()
	if err != nil {
		if gpuHwmonPath != "" {
			info.Temp = readSysfsFloat(filepath.Join(gpuHwmonPath, "temp1_input")) / 1000.0
		}
		return info
	}

	fields := strings.Split(strings.TrimSpace(output), ", ")
	if len(fields) >= 5 {
		info.Temp, _ = strconv.ParseFloat(strings.TrimSpace(fields[0]), 64)
		info.Load, _ = strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)

		memUsedMiB, _ := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
		memTotalMiB, _ := strconv.ParseFloat(strings.TrimSpace(fields[3]), 64)
		info.MemUsed = uint64(memUsedMiB) * 1024 * 1024
		info.MemTotal = uint64(memTotalMiB) * 1024 * 1024
		if info.MemTotal > 0 {
			info.MemPercent = float64(info.MemUsed) / float64(info.MemTotal) * 100
		}

		info.Power, _ = strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
	}

	return info
}

func readSysfsFloat(path string) float64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	return v
}

func readSysfsUint64(path string) uint64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	return v
}
