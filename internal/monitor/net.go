package monitor

import (
	"context"
	"fmt"
	"image/color"
	"log/slog"
	"time"

	"github.com/aleksclark/go-turing-smart-screen/internal/lcd"
	"github.com/aleksclark/go-turing-smart-screen/internal/netprobe"
)

// NetMonitor displays internet health information.
type NetMonitor struct {
	*Base
	prober     *netprobe.Prober
	ctx        context.Context
	cancel     context.CancelFunc
	pingY      int
	dnsY       int
	pingRows   int
	dividerX   int
}

// NewNetMonitor creates a new internet health monitor.
func NewNetMonitor(screen lcd.Screen, brightness int, interval time.Duration, logger *slog.Logger) *NetMonitor {
	fonts := DefaultFontConfig()
	fonts.Small = 14
	fonts.Normal = 16
	fonts.Large = 20

	base := NewBase(Config{
		Screen:   screen,
		Colors:   DefaultColors(),
		Fonts:    fonts,
		Interval: interval,
		Logger:   logger,
	})

	ctx, cancel := context.WithCancel(context.Background())

	return &NetMonitor{
		Base:   base,
		prober: netprobe.New(),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Name returns the monitor name.
func (m *NetMonitor) Name() string { return "Net" }

// Run starts the internet health monitor loop.
func (m *NetMonitor) Run() error {
	m.SetRunning(true)

	m.setupLayout()

	m.ClearBuffer()
	m.drawStatic()
	if err := m.DrawFullBuffer(); err != nil {
		return fmt.Errorf("initial draw: %w", err)
	}

	m.Logger().Info("started", "monitor", m.Name())

	ticker := time.NewTicker(m.Interval())
	defer ticker.Stop()

	for m.Running() {
		<-ticker.C
		m.prober.Probe(m.ctx)
		if err := m.update(); err != nil {
			m.Logger().Error("update failed", "error", err)
		}
	}

	return nil
}

// Stop stops the monitor.
func (m *NetMonitor) Stop() {
	m.cancel()
	m.SetRunning(false)
}

func (m *NetMonitor) setupLayout() {
	m.dividerX = m.Width() * 55 / 100
	m.pingY = 58
	m.dnsY = 58
	m.pingRows = 8
}

func (m *NetMonitor) drawStatic() {
	dc := m.NewContext(Region{0, 0, m.Width(), m.Height()})
	r := NewRenderer(dc, m.Colors(), m.fonts)

	r.DrawLine(0, 32, float64(m.Width()))
	r.DrawLine(0, 55, float64(m.Width()))
	r.DrawVerticalLine(float64(m.dividerX), 55, float64(m.Height()))
}

func (m *NetMonitor) update() error {
	dc := m.NewContext(Region{0, 0, m.Width(), m.Height()})
	r := NewRenderer(dc, m.Colors(), m.fonts)

	var updates []Region

	// Header
	iface := m.prober.IfaceStats()
	header := "Internet Health"
	if m.Changed("header", header) {
		reg := Region{5, 8, m.Width() - 10, 24}
		r.Clear(reg)
		r.DrawText(float64(reg.X), float64(reg.Y), header, m.fonts.Large, m.Colors().Header)
		if iface.Name != "" {
			r.DrawTextRight(float64(reg.X), float64(reg.Y), float64(reg.W), iface.Name, m.fonts.Normal, m.Colors().TextDim)
		}
		updates = append(updates, reg)
	}

	// Throughput row
	throughput := m.formatThroughput(iface)
	if m.Changed("throughput", throughput) {
		reg := Region{5, 36, m.Width() - 10, 17}
		r.Clear(reg)

		inRate := fmt.Sprintf("↓ %s  %s", netprobe.FormatRate(iface.RateIn), netprobe.FormatPktRate(iface.PktRateIn))
		outRate := fmt.Sprintf("↑ %s  %s", netprobe.FormatRate(iface.RateOut), netprobe.FormatPktRate(iface.PktRateOut))

		inColor := m.Colors().BarLow
		if iface.RateIn > 10e6 {
			inColor = m.Colors().BarMed
		}
		outColor := m.Colors().BarLow
		if iface.RateOut > 10e6 {
			outColor = m.Colors().BarMed
		}

		r.DrawText(5, float64(reg.Y), inRate, m.fonts.Small, inColor)
		r.DrawText(float64(m.Width()/2-20), float64(reg.Y), outRate, m.fonts.Small, outColor)

		totals := fmt.Sprintf("Σ %s/%s", formatTotalBytes(iface.BytesIn), formatTotalBytes(iface.BytesOut))
		r.DrawTextRight(float64(reg.X), float64(reg.Y), float64(reg.W), totals, m.fonts.Small, m.Colors().TextDim)

		updates = append(updates, reg)
	}

	// Ping section (left side)
	pingResults := m.prober.PingResults()
	sectionHeaderY := m.pingY
	if m.Changed("ping_header", true) {
		reg := Region{5, sectionHeaderY, m.dividerX - 10, 16}
		r.Clear(reg)
		r.DrawText(float64(reg.X), float64(reg.Y), "Connectivity", m.fonts.Normal, m.Colors().Header)
		updates = append(updates, reg)
	}

	rowH := 30
	startY := sectionHeaderY + 20
	for i := 0; i < m.pingRows; i++ {
		rowY := startY + i*rowH
		reg := Region{2, rowY, m.dividerX - 4, rowH}

		if i < len(pingResults) {
			pr := pingResults[i]
			key := fmt.Sprintf("ping_%d", i)
			hash := fmt.Sprintf("%s_%s_%v_%.2f", pr.Label, pr.Target, pr.Latency, pr.Loss)

			if m.Changed(key, hash) {
				r.Clear(reg)
				m.renderPingRow(r, reg, &pr)
				updates = append(updates, reg)
			}
		} else {
			key := fmt.Sprintf("ping_%d", i)
			if m.Changed(key, "empty") {
				r.Clear(reg)
				updates = append(updates, reg)
			}
		}
	}

	// DNS section (right side)
	dnsResults := m.prober.DNSResults()
	dnsX := m.dividerX + 5
	dnsW := m.Width() - dnsX - 5

	if m.Changed("dns_header", true) {
		reg := Region{dnsX, m.dnsY, dnsW, 16}
		r.Clear(reg)
		r.DrawText(float64(dnsX), float64(m.dnsY), "DNS Resolution", m.fonts.Normal, m.Colors().Header)
		updates = append(updates, reg)
	}

	dnsStartY := m.dnsY + 20
	dnsRowH := 36
	for i, dr := range dnsResults {
		rowY := dnsStartY + i*dnsRowH
		reg := Region{dnsX, rowY, dnsW, dnsRowH}
		key := fmt.Sprintf("dns_%d", i)
		hash := fmt.Sprintf("%s_%v_%s", dr.Domain, dr.Latency, dr.Error)

		if m.Changed(key, hash) {
			r.Clear(reg)
			m.renderDNSRow(r, reg, &dr)
			updates = append(updates, reg)
		}
	}

	// Send updates
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

func (m *NetMonitor) renderPingRow(r *Renderer, reg Region, pr *netprobe.PingResult) {
	orbRadius := 6.0
	orbX := float64(reg.X + 12)
	orbY := float64(reg.Y) + float64(reg.H)/2

	orbColor := m.orbColor(pr.Loss, pr.Alive)
	r.DrawCircle(orbX, orbY, orbRadius, orbColor)

	textX := float64(reg.X + 26)

	// Line 1: Label + target IP
	label := pr.Label
	if len(label) > 12 {
		label = label[:12]
	}
	r.DrawText(textX, float64(reg.Y), label, m.fonts.Normal, m.Colors().Text)

	target := pr.Target
	r.DrawTextRight(float64(reg.X), float64(reg.Y), float64(reg.W), target, m.fonts.Small, m.Colors().TextDim)

	// Line 2: Latency + loss
	var latText string
	if pr.Alive {
		latText = netprobe.FormatLatency(pr.Latency)
	} else {
		latText = "timeout"
	}
	latColor := m.Colors().BarLow
	if pr.Latency > 100*time.Millisecond {
		latColor = m.Colors().BarHigh
	} else if pr.Latency > 30*time.Millisecond {
		latColor = m.Colors().BarMed
	}
	if !pr.Alive {
		latColor = m.Colors().BarHigh
	}
	r.DrawText(textX, float64(reg.Y+14), latText, m.fonts.Small, latColor)

	lossText := netprobe.FormatLoss(pr.Loss)
	lossColor := m.Colors().BarLow
	if pr.Loss > 0.1 {
		lossColor = m.Colors().BarHigh
	} else if pr.Loss > 0 {
		lossColor = m.Colors().BarMed
	}
	r.DrawTextRight(float64(reg.X), float64(reg.Y+14), float64(reg.W), lossText, m.fonts.Small, lossColor)
}

func (m *NetMonitor) renderDNSRow(r *Renderer, reg Region, dr *netprobe.DNSResult) {
	orbRadius := 5.0
	orbX := float64(reg.X + 8)
	orbY := float64(reg.Y) + float64(reg.H)/2

	isErr := dr.Error != ""
	var orbColor color.Color
	if isErr {
		orbColor = color.RGBA{255, 0, 0, 255}
	} else if dr.Latency > 200*time.Millisecond {
		orbColor = color.RGBA{255, 255, 0, 255}
	} else {
		orbColor = color.RGBA{0, 255, 0, 255}
	}
	r.DrawCircle(orbX, orbY, orbRadius, orbColor)

	textX := float64(reg.X + 20)

	// Domain name
	r.DrawText(textX, float64(reg.Y), dr.Domain, m.fonts.Normal, m.Colors().Text)

	// Latency or error
	if isErr {
		errText := dr.Error
		if len(errText) > 25 {
			errText = errText[:25]
		}
		r.DrawText(textX, float64(reg.Y+16), errText, m.fonts.Small, m.Colors().BarHigh)
	} else {
		latText := netprobe.FormatLatency(dr.Latency)
		latColor := m.Colors().BarLow
		if dr.Latency > 200*time.Millisecond {
			latColor = m.Colors().BarHigh
		} else if dr.Latency > 50*time.Millisecond {
			latColor = m.Colors().BarMed
		}
		r.DrawTextRight(float64(reg.X), float64(reg.Y), float64(reg.W), latText, m.fonts.Small, latColor)
	}
}

func (m *NetMonitor) orbColor(loss float64, alive bool) color.Color {
	if !alive {
		return color.RGBA{255, 0, 0, 255}
	}
	if loss > 0.1 {
		return color.RGBA{255, 0, 0, 255}
	}
	if loss > 0.03 {
		return color.RGBA{255, 150, 0, 255}
	}
	if loss > 0 {
		return color.RGBA{255, 255, 0, 255}
	}
	return color.RGBA{0, 255, 0, 255}
}

func (m *NetMonitor) formatThroughput(s netprobe.InterfaceStats) string {
	return fmt.Sprintf("%.0f_%.0f_%.0f_%.0f_%d_%d",
		s.RateIn, s.RateOut, s.PktRateIn, s.PktRateOut, s.BytesIn, s.BytesOut)
}

func formatTotalBytes(b uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case b >= TB:
		return fmt.Sprintf("%.1fT", float64(b)/TB)
	case b >= GB:
		return fmt.Sprintf("%.1fG", float64(b)/GB)
	case b >= MB:
		return fmt.Sprintf("%.0fM", float64(b)/MB)
	default:
		return fmt.Sprintf("%.0fK", float64(b)/KB)
	}
}
