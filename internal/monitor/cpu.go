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
	cpuCount  int
	overallY  int
	cols      int
	barHeight int
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
	// Determine columns based on core count
	switch {
	case m.cpuCount <= 8:
		m.cols = 1
	case m.cpuCount <= 16:
		m.cols = 2
	default:
		m.cols = 4
	}

	// Calculate bar height
	yOffset := 68
	availableHeight := m.Height() - yOffset - 40
	rows := (m.cpuCount + m.cols - 1) / m.cols
	barSpacing := 3
	m.barHeight = (availableHeight - 30) / rows
	if m.barHeight < 12 {
		m.barHeight = 12
	}
	if m.barHeight > 35 {
		m.barHeight = 35
	}
	_ = barSpacing

	m.overallY = m.Height() - 35
}

func (m *CPUMonitor) drawStatic() {
	dc := m.NewContext(Region{0, 0, m.Width(), m.Height()})
	r := NewRenderer(dc, m.Colors(), m.fonts)

	// Separator lines
	r.DrawLine(0, 35, float64(m.Width()))
	r.DrawLine(0, float64(m.overallY-5), float64(m.Width()))

	// "ALL" label
	r.DrawText(5, float64(m.overallY), "ALL", m.fonts.Normal, m.Colors().Header)
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
		header += fmt.Sprintf(" | %.0f°C", info.Temp)
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

	// Per-CPU bars
	yOffset := 68
	colWidth := (m.Width() - 10) / m.cols
	pctWidth := 38
	barWidth := colWidth - pctWidth - 8
	barSpacing := 3

	for i, pct := range info.PerCPU {
		col := i % m.cols
		row := i / m.cols
		x := 5 + col*colWidth
		y := yOffset + row*(m.barHeight+barSpacing)

		key := fmt.Sprintf("cpu_%d", i)
		if m.ChangedFloat(key, pct, 2.0) {
			// Percentage text
			pctReg := Region{x, y + (m.barHeight-18)/2, pctWidth, 20}
			r.Clear(pctReg)
			r.DrawTextRight(float64(pctReg.X), float64(pctReg.Y), float64(pctReg.W),
				fmt.Sprintf("%3.0f%%", pct), m.fonts.Small, m.Colors().Text)
			updates = append(updates, pctReg)

			// Bar
			barReg := Region{x + pctWidth + 4, y, barWidth, m.barHeight - 1}
			r.DrawBar(barReg, pct, 0, 100, true)
			updates = append(updates, barReg)
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
