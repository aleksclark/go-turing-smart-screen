package monitor

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/aleksclark/go-turing-smart-screen/internal/lcd"
	"github.com/aleksclark/go-turing-smart-screen/internal/sysinfo"
)

// RAMMonitor displays memory usage information.
type RAMMonitor struct {
	*Base
	processListY int
	rowHeight    int
	numRows      int
}

// NewRAMMonitor creates a new RAM monitor.
func NewRAMMonitor(screen lcd.Screen, brightness int, interval time.Duration, logger *slog.Logger) *RAMMonitor {
	fonts := FontConfig{
		Path:   "res/fonts/jetbrains-mono/JetBrainsMono-Regular.ttf",
		Small:  16,
		Normal: 18,
		Large:  22,
	}

	base := NewBase(Config{
		Screen:   screen,
		Colors:   DefaultColors(),
		Fonts:    fonts,
		Interval: interval,
		Logger:   logger,
	})

	return &RAMMonitor{
		Base:    base,
		numRows: 5,
	}
}

// Name returns the monitor name.
func (m *RAMMonitor) Name() string { return "RAM" }

// Run starts the RAM monitor loop.
func (m *RAMMonitor) Run() error {
	m.SetRunning(true)

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
func (m *RAMMonitor) Stop() {
	m.SetRunning(false)
}

func (m *RAMMonitor) setupLayout() {
	// Process list starts after RAM/Swap bars and header
	m.processListY = 138

	// Calculate row height
	availableHeight := m.Height() - m.processListY - 10
	m.rowHeight = availableHeight / m.numRows
	if m.rowHeight < 28 {
		m.rowHeight = 28
	}
}

func (m *RAMMonitor) drawStatic() {
	dc := m.NewContext(Region{0, 0, m.Width(), m.Height()})
	r := NewRenderer(dc, m.Colors(), m.fonts)

	// Separator lines
	r.DrawLine(0, 35, float64(m.Width()))
	r.DrawLine(0, float64(m.processListY-5), float64(m.Width()))
}

func (m *RAMMonitor) update() error {
	memInfo, err := sysinfo.GetMemInfo()
	if err != nil {
		return err
	}

	procs, err := sysinfo.GetTopProcesses(m.numRows)
	if err != nil {
		return err
	}

	dc := m.NewContext(Region{0, 0, m.Width(), m.Height()})
	r := NewRenderer(dc, m.Colors(), m.fonts)

	var updates []Region

	// Header
	header := fmt.Sprintf("RAM Monitor - %s total", sysinfo.FormatBytes(memInfo.Total))
	if m.Changed("header", header) {
		reg := Region{5, 8, m.Width() - 10, 24}
		r.Clear(reg)
		r.DrawText(float64(reg.X), float64(reg.Y), header, m.fonts.Large, m.Colors().Header)
		updates = append(updates, reg)
	}

	// RAM label
	if m.Changed("ram_label", true) {
		reg := Region{5, 40, 45, 20}
		r.Clear(reg)
		r.DrawText(float64(reg.X), float64(reg.Y), "RAM", m.fonts.Normal, m.Colors().Text)
		updates = append(updates, reg)
	}

	// RAM bar
	if m.ChangedFloat("ram_pct", memInfo.UsedPercent, 0.5) {
		reg := Region{55, 40, 250, 24}
		r.DrawBar(reg, memInfo.UsedPercent, 0, 100, true)
		updates = append(updates, reg)
	}

	// RAM text
	ramText := fmt.Sprintf("%s / %s", sysinfo.FormatBytes(memInfo.Used), sysinfo.FormatBytes(memInfo.Total))
	if m.Changed("ram_text", ramText) {
		reg := Region{310, 40, 165, 20}
		r.Clear(reg)
		r.DrawTextRight(float64(reg.X), float64(reg.Y), float64(reg.W), ramText, m.fonts.Normal, m.Colors().Text)
		updates = append(updates, reg)
	}

	// Swap label
	if m.Changed("swap_label", true) {
		reg := Region{5, 75, 45, 20}
		r.Clear(reg)
		r.DrawText(float64(reg.X), float64(reg.Y), "Swap", m.fonts.Normal, m.Colors().TextDim)
		updates = append(updates, reg)
	}

	// Swap bar
	if m.ChangedFloat("swap_pct", memInfo.SwapPercent, 0.5) {
		reg := Region{55, 75, m.Width() - 180, 20}
		r.DrawBar(reg, memInfo.SwapPercent, 0, 100, true)
		updates = append(updates, reg)
	}

	// Swap text
	var swapText string
	if memInfo.SwapTotal > 0 {
		swapText = fmt.Sprintf("%s / %s", sysinfo.FormatBytes(memInfo.SwapUsed), sysinfo.FormatBytes(memInfo.SwapTotal))
	} else {
		swapText = "No swap"
	}
	if m.Changed("swap_text", swapText) {
		reg := Region{m.Width() - 120, 75, 115, 20}
		r.Clear(reg)
		r.DrawTextRight(float64(reg.X), float64(reg.Y), float64(reg.W), swapText, m.fonts.Normal, m.Colors().TextDim)
		updates = append(updates, reg)
	}

	// Process header
	if m.Changed("proc_header", true) {
		reg := Region{5, 110, m.Width() - 10, 22}
		r.Clear(reg)
		r.DrawText(float64(reg.X), float64(reg.Y), "PROCESS                    MEM        %     #", m.fonts.Normal, m.Colors().Header)
		updates = append(updates, reg)
	}

	// Process rows
	barWidth := 100
	for i := 0; i < m.numRows; i++ {
		rowY := m.processListY + i*m.rowHeight

		if i < len(procs) {
			proc := procs[i]
			key := fmt.Sprintf("proc_%d", i)
			
			// Create a comparable value
			procVal := fmt.Sprintf("%s_%d_%.1f", proc.Name, proc.Count, proc.Percent)
			if m.Changed(key, procVal) {
				// Name
				nameReg := Region{5, rowY, 160, m.rowHeight}
				r.Clear(nameReg)
				name := proc.Name
				if len(name) > 18 {
					name = name[:18]
				}
				r.DrawText(float64(nameReg.X), float64(nameReg.Y), name, m.fonts.Normal, m.Colors().TextDim)
				updates = append(updates, nameReg)

				// Bar
				barReg := Region{170, rowY + 2, barWidth, m.rowHeight - 6}
				r.DrawBar(barReg, proc.Percent, 0, 100, true)
				updates = append(updates, barReg)

				// Memory amount
				memReg := Region{280, rowY, 70, m.rowHeight}
				r.Clear(memReg)
				r.DrawTextRight(float64(memReg.X), float64(memReg.Y), float64(memReg.W),
					sysinfo.FormatBytes(proc.RSS), m.fonts.Normal, m.Colors().Text)
				updates = append(updates, memReg)

				// Percentage
				pctReg := Region{355, rowY, 55, m.rowHeight}
				r.Clear(pctReg)
				r.DrawTextRight(float64(pctReg.X), float64(pctReg.Y), float64(pctReg.W),
					fmt.Sprintf("%.1f%%", proc.Percent), m.fonts.Normal, m.Colors().TextDim)
				updates = append(updates, pctReg)

				// Count
				countReg := Region{415, rowY, 60, m.rowHeight}
				r.Clear(countReg)
				if proc.Count > 1 {
					r.DrawTextRight(float64(countReg.X), float64(countReg.Y), float64(countReg.W),
						fmt.Sprintf("x%d", proc.Count), m.fonts.Small, m.Colors().TextDim)
				}
				updates = append(updates, countReg)
			}
		} else {
			// Clear empty row
			key := fmt.Sprintf("proc_%d", i)
			if m.Changed(key, "empty") {
				reg := Region{5, rowY, m.Width() - 10, m.rowHeight}
				r.Clear(reg)
				updates = append(updates, reg)
			}
		}
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
