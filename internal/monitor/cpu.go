package monitor

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/aleksclark/go-turing-smart-screen/internal/lcd"
	"github.com/aleksclark/go-turing-smart-screen/internal/sysinfo"
)

// CPUMonitor displays CPU usage information.
type CPUMonitor struct {
	*Base
	cpuCount    int
	coreY       int // Y position of per-core bars
	coreH       int // Height of per-core section
	overallY    int // Y position of overall bar
	panelsY     int // Y position of bottom panels
	gpuY        int // Y position of GPU section
}

// NewCPUMonitor creates a new CPU monitor.
func NewCPUMonitor(screen lcd.Screen, brightness int, interval time.Duration, logger *slog.Logger) *CPUMonitor {
	fonts := DefaultFontConfig()
	fonts.Small = 16
	fonts.Normal = 18
	fonts.Large = 22

	base := NewBase(Config{
		Screen:   screen,
		Colors:   DefaultColors(),
		Fonts:    fonts,
		Interval: interval,
		Logger:   logger,
	})

	return &CPUMonitor{Base: base}
}

// Name returns the monitor name.
func (m *CPUMonitor) Name() string { return "CPU" }

// Run starts the CPU monitor loop.
func (m *CPUMonitor) Run() error {
	m.SetRunning(true)
	
	// Get initial CPU info
	info, err := sysinfo.GetCPUInfo()
	if err != nil {
		return fmt.Errorf("get cpu info: %w", err)
	}
	m.cpuCount = info.CoreCount
	
	// Calculate layout
	m.setupLayout()
	
	// Initial draw
	m.ClearBuffer()
	m.drawStatic()
	if err := m.DrawFullBuffer(); err != nil {
		return fmt.Errorf("initial draw: %w", err)
	}

	m.Logger().Info("started", "monitor", m.Name())

	ticker := time.NewTicker(m.Interval())
	defer ticker.Stop()

	for m.Running() {
		select {
		case <-ticker.C:
			if err := m.update(); err != nil {
				m.Logger().Error("update failed", "error", err)
			}
		}
	}

	return nil
}

// Stop stops the monitor.
func (m *CPUMonitor) Stop() {
	m.SetRunning(false)
}

func (m *CPUMonitor) setupLayout() {
	// Layout (320px height):
	// 0-35: Header (35px)
	// 35-62: Freq/Load info (27px)
	// 62-107: Per-core vertical bars (45px, reduced 25% from 60px)
	// 110-140: Overall bar section (30px)
	// 143-255: Bottom panels - processes & temps (~112px)
	// 258-320: GPU stats (62px)
	m.coreY = 62
	m.coreH = 45
	m.overallY = m.coreY + m.coreH + 3
	m.panelsY = m.overallY + 30
	m.gpuY = 258
}

func (m *CPUMonitor) drawStatic() {
	dc := m.NewContext(Region{0, 0, m.Width(), m.Height()})
	r := NewRenderer(dc, m.Colors(), m.fonts)

	// Separator lines
	r.DrawLine(0, 35, float64(m.Width()))
	r.DrawLine(0, float64(m.coreY-3), float64(m.Width()))
	r.DrawLine(0, float64(m.overallY-5), float64(m.Width()))
	r.DrawLine(0, float64(m.panelsY-3), float64(m.Width()))
	r.DrawLine(0, float64(m.gpuY-3), float64(m.Width()))

	// "ALL" label
	r.DrawText(5, float64(m.overallY), "ALL", m.fonts.Normal, m.Colors().Header)

	// Panel headers
	panelWidth := m.Width() / 2
	r.DrawText(5, float64(m.panelsY), "Top Processes", m.fonts.Small, m.Colors().Header)
	r.DrawText(float64(panelWidth+5), float64(m.panelsY), "Temperatures", m.fonts.Small, m.Colors().Header)

	// Vertical divider between panels
	r.DrawVerticalLine(float64(panelWidth), float64(m.panelsY-3), float64(m.gpuY-3))

	// GPU header
	r.DrawText(5, float64(m.gpuY), "GPU", m.fonts.Small, m.Colors().Header)
}

func (m *CPUMonitor) update() error {
	info, err := sysinfo.GetCPUInfo()
	if err != nil {
		return err
	}

	dc := m.NewContext(Region{0, 0, m.Width(), m.Height()})
	r := NewRenderer(dc, m.Colors(), m.fonts)

	var updates []Region

	// Header
	header := fmt.Sprintf("CPU Monitor - %d cores", info.CoreCount)
	if info.Temp > 0 {
		header += fmt.Sprintf(" | %.0fC", info.Temp)
	}
	if m.Changed("header", header) {
		reg := Region{5, 8, m.Width() - 10, 24}
		r.Clear(reg)
		r.DrawText(float64(reg.X), float64(reg.Y), header, m.fonts.Large, m.Colors().Header)
		updates = append(updates, reg)
	}

	// Frequency
	freqStr := fmt.Sprintf("Freq: %.2f GHz", info.Freq)
	if m.ChangedFloat("freq", info.Freq, 0.05) {
		reg := Region{5, 38, 180, 20}
		r.Clear(reg)
		r.DrawText(float64(reg.X), float64(reg.Y), freqStr, m.fonts.Normal, m.Colors().TextDim)
		updates = append(updates, reg)
	}

	// Load
	loadStr := fmt.Sprintf("Load: %.2f %.2f %.2f", info.Load1, info.Load5, info.Load15)
	if m.Changed("load", loadStr) {
		reg := Region{190, 38, 280, 20}
		r.Clear(reg)
		r.DrawText(float64(reg.X), float64(reg.Y), loadStr, m.fonts.Normal, m.Colors().TextDim)
		updates = append(updates, reg)
	}

	// Per-CPU vertical bars
	margin := 5
	spacing := 2
	availWidth := m.Width() - 2*margin
	barWidth := (availWidth - (m.cpuCount-1)*spacing) / m.cpuCount
	if barWidth < 2 {
		barWidth = 2
	}

	for i, pct := range info.PerCPU {
		key := fmt.Sprintf("cpu_%d", i)
		if m.ChangedFloat(key, pct, 2.0) {
			x := margin + i*(barWidth+spacing)
			reg := Region{x, m.coreY, barWidth, m.coreH}
			r.DrawVerticalBar(reg, pct, 0, 100)
			updates = append(updates, reg)
		}
	}

	// Overall bar
	if m.ChangedFloat("overall", info.Overall, 1.0) {
		barReg := Region{45, m.overallY, m.Width() - 120, 24}
		r.DrawBar(barReg, info.Overall, 0, 100, true)
		updates = append(updates, barReg)

		pctReg := Region{m.Width() - 70, m.overallY, 65, 24}
		r.Clear(pctReg)
		r.DrawTextRight(float64(pctReg.X), float64(pctReg.Y), float64(pctReg.W),
			fmt.Sprintf("%5.1f%%", info.Overall), m.fonts.Normal, m.Colors().Text)
		updates = append(updates, pctReg)
	}

	// Bottom panels
	panelWidth := m.Width() / 2
	lineHeight := 18
	maxPanelRows := (m.gpuY - 3 - (m.panelsY + 20)) / lineHeight
	if maxPanelRows > 5 {
		maxPanelRows = 5
	}

	// Left panel: Top processes by CPU
	procs, _ := sysinfo.GetTopProcessesByCPU(maxPanelRows)
	procsKey := m.formatProcsKey(procs)
	if m.Changed("procs", procsKey) {
		procY := m.panelsY + 20
		procReg := Region{5, procY, panelWidth - 10, maxPanelRows * lineHeight}
		r.Clear(procReg)

		for i, p := range procs {
			y := procY + i*lineHeight
			name := p.Name
			if len(name) > 12 {
				name = name[:12]
			}
			r.DrawText(5, float64(y), name, m.fonts.Small, m.Colors().Text)
			r.DrawTextRight(float64(panelWidth-60), float64(y), 50,
				fmt.Sprintf("%5.1f%%", p.Percent), m.fonts.Small, m.Colors().TextDim)
		}
		updates = append(updates, procReg)
	}

	// Right panel: Temperatures
	temps, _ := sysinfo.GetTemperatures()
	tempsKey := m.formatTempsKey(temps)
	if m.Changed("temps", tempsKey) {
		tempY := m.panelsY + 20
		tempReg := Region{panelWidth + 5, tempY, panelWidth - 10, maxPanelRows * lineHeight}
		r.Clear(tempReg)

		maxTemps := maxPanelRows
		if len(temps) > maxTemps {
			temps = temps[:maxTemps]
		}

		for i, t := range temps {
			y := tempY + i*lineHeight
			r.DrawText(float64(panelWidth+5), float64(y), t.Label, m.fonts.Small, m.Colors().Text)
			tempColor := m.Colors().BarLow
			if t.Temp >= 90 {
				tempColor = m.Colors().BarHigh
			} else if t.Temp >= 80 {
				tempColor = m.Colors().BarMed
			}
			r.DrawTextRight(float64(m.Width()-55), float64(y), 50,
				fmt.Sprintf("%5.1fC", t.Temp), m.fonts.Small, tempColor)
		}
		updates = append(updates, tempReg)
	}

	// GPU section
	gpuUpdates := m.updateGPU(r)
	updates = append(updates, gpuUpdates...)

	// Send updates to display
	for _, reg := range updates {
		if err := m.DrawRegion(reg); err != nil {
			return err
		}
	}

	if len(updates) > 0 {
		m.Logger().Debug("updated regions", "count", len(updates), "monitor", m.Name())
	}

	return nil
}

func (m *CPUMonitor) updateGPU(r *Renderer) []Region {
	var updates []Region

	gpu := sysinfo.GetGPUInfo()
	if gpu == nil {
		if m.Changed("gpu", "none") {
			reg := Region{40, m.gpuY, m.Width() - 45, 18}
			r.Clear(reg)
			r.DrawText(40, float64(m.gpuY), "not detected", m.fonts.Small, m.Colors().TextDim)
			updates = append(updates, reg)
		}
		return updates
	}

	// GPU stats line 1: Load bar + temp
	gpuKey := fmt.Sprintf("%.0f_%.0f_%.0f_%.1f", gpu.Load, gpu.Temp, gpu.MemPercent, gpu.Power)
	if m.Changed("gpu", gpuKey) {
		// Row 1: Load bar with percentage and temperature
		row1 := Region{40, m.gpuY, m.Width() - 45, 20}
		r.Clear(row1)

		barReg := Region{40, m.gpuY + 2, 180, 16}
		r.DrawBar(barReg, gpu.Load, 0, 100, true)

		r.DrawText(225, float64(m.gpuY), fmt.Sprintf("%4.0f%%", gpu.Load),
			m.fonts.Small, m.Colors().Text)

		if gpu.Temp > 0 {
			tempColor := m.Colors().BarLow
			if gpu.Temp >= 90 {
				tempColor = m.Colors().BarHigh
			} else if gpu.Temp >= 75 {
				tempColor = m.Colors().BarMed
			}
			r.DrawText(280, float64(m.gpuY), fmt.Sprintf("%.0fC", gpu.Temp),
				m.fonts.Small, tempColor)
		}

		if gpu.Power > 0 {
			r.DrawTextRight(float64(m.Width()-5-60), float64(m.gpuY), 60,
				fmt.Sprintf("%.0fW", gpu.Power), m.fonts.Small, m.Colors().TextDim)
		}
		updates = append(updates, row1)

		// Row 2: VRAM bar with usage
		row2Y := m.gpuY + 22
		row2 := Region{5, row2Y, m.Width() - 10, 20}
		r.Clear(row2)

		r.DrawText(5, float64(row2Y), "VRAM", m.fonts.Small, m.Colors().Header)

		if gpu.MemTotal > 0 {
			vramBarReg := Region{40, row2Y + 2, 180, 16}
			r.DrawBar(vramBarReg, gpu.MemPercent, 0, 100, true)

			r.DrawText(225, float64(row2Y), fmt.Sprintf("%4.0f%%", gpu.MemPercent),
				m.fonts.Small, m.Colors().Text)

			r.DrawTextRight(float64(m.Width()-5-120), float64(row2Y), 120,
				fmt.Sprintf("%s / %s", sysinfo.FormatBytes(gpu.MemUsed), sysinfo.FormatBytes(gpu.MemTotal)),
				m.fonts.Small, m.Colors().TextDim)
		}
		updates = append(updates, row2)
	}

	return updates
}

// formatProcsKey creates a cache key for process list.
func (m *CPUMonitor) formatProcsKey(procs []sysinfo.ProcessCPUInfo) string {
	key := ""
	for _, p := range procs {
		key += fmt.Sprintf("%s:%.0f;", p.Name, p.Percent)
	}
	return key
}

// formatTempsKey creates a cache key for temperature list.
func (m *CPUMonitor) formatTempsKey(temps []sysinfo.TempInfo) string {
	key := ""
	for _, t := range temps {
		key += fmt.Sprintf("%s:%.0f;", t.Label, t.Temp)
	}
	return key
}
