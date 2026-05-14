package monitor

import (
	"context"
	"fmt"
	"image/color"
	"log/slog"
	"sort"
	"time"

	"github.com/aleksclark/go-turing-smart-screen/internal/config"
	"github.com/aleksclark/go-turing-smart-screen/internal/lcd"
	"github.com/aleksclark/go-turing-smart-screen/internal/netprobe"
	"github.com/aleksclark/go-turing-smart-screen/internal/signoz"
)

// NetMonitor displays fleet health: condensed internet stats, MooseFS, node charts, and alerts.
type NetMonitor struct {
	*Base
	prober  *netprobe.Prober
	signoz  *signoz.Client
	ctx     context.Context
	cancel  context.CancelFunc

	// Layout Y positions
	inetY   int // Internet section (2 lines)
	mooseY  int // MooseFS section
	chartsY int // Node charts section
	alertsY int // Alerts section

	// Cached SigNoz data
	mooseFS    *signoz.MooseFSStats
	nodeStats  *signoz.NodeStats
	alerts     []signoz.Alert
	lastSigNoz time.Time
	signozErr  string // last error message, empty if OK
}

// NewNetMonitor creates a new fleet health monitor.
func NewNetMonitor(screen lcd.Screen, brightness int, interval time.Duration, logger *slog.Logger) *NetMonitor {
	fonts := DefaultFontConfig()
	fonts.Small = 12
	fonts.Normal = 14
	fonts.Large = 18

	base := NewBase(Config{
		Screen:   screen,
		Colors:   DefaultColors(),
		Fonts:    fonts,
		Interval: interval,
		Logger:   logger,
	})

	ctx, cancel := context.WithCancel(context.Background())

	cfg, _ := config.Load()
	var signozClient *signoz.Client
	if cfg != nil {
		signozClient = signoz.New(cfg.SignozURL, cfg.SignozAPIKey)
	} else {
		signozClient = signoz.New("", "")
	}

	return &NetMonitor{
		Base:   base,
		prober: netprobe.New(),
		signoz: signozClient,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Name returns the monitor name.
func (m *NetMonitor) Name() string { return "Fleet" }

// Run starts the fleet health monitor loop.
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
		m.fetchSigNoz()
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
	// Layout for 480x320 landscape:
	// 0-35:   Header (title + connectivity orb)
	// 38-70:  Internet stats (rates + issue)
	// 72-108: MooseFS
	// 110-240: Node charts (side by side)
	// 242-320: Alerts
	m.inetY = 38
	m.mooseY = 72
	m.chartsY = 110
	m.alertsY = 242
}

func (m *NetMonitor) drawStatic() {
	dc := m.NewContext(Region{0, 0, m.Width(), m.Height()})
	r := NewRenderer(dc, m.Colors(), m.fonts)

	// Section dividers
	r.DrawLine(0, 35, float64(m.Width()))
	r.DrawLine(0, float64(m.mooseY-2), float64(m.Width()))
	r.DrawLine(0, float64(m.chartsY-2), float64(m.Width()))
	r.DrawLine(0, float64(m.alertsY-2), float64(m.Width()))

	// Chart vertical divider
	r.DrawVerticalLine(float64(m.Width()/2), float64(m.chartsY-2), float64(m.alertsY-2))
}

func (m *NetMonitor) fetchSigNoz() {
	// Only fetch every 30s to avoid hammering
	if time.Since(m.lastSigNoz) < 30*time.Second {
		return
	}
	m.lastSigNoz = time.Now()

	var firstErr error

	moose, err := m.signoz.FetchMooseFS()
	if err != nil {
		m.Logger().Debug("moosefs fetch failed", "error", err)
		if firstErr == nil {
			firstErr = err
		}
	} else {
		m.mooseFS = moose
	}

	stats, err := m.signoz.FetchNodeStats()
	if err != nil {
		m.Logger().Debug("node stats fetch failed", "error", err)
		if firstErr == nil {
			firstErr = err
		}
	} else {
		m.nodeStats = stats
	}

	alerts, err := m.signoz.FetchAlerts()
	if err != nil {
		m.Logger().Debug("alerts fetch failed", "error", err)
		if firstErr == nil {
			firstErr = err
		}
	} else {
		m.alerts = alerts
	}

	if firstErr != nil {
		errMsg := firstErr.Error()
		if len(errMsg) > 50 {
			errMsg = errMsg[:50]
		}
		m.signozErr = errMsg
	} else {
		m.signozErr = ""
	}
}

func (m *NetMonitor) update() error {
	dc := m.NewContext(Region{0, 0, m.Width(), m.Height()})
	r := NewRenderer(dc, m.Colors(), m.fonts)

	var updates []Region

	// Header + Connectivity orb
	worstLevel, worstIssue := m.assessConnectivity(m.prober.PingResults(), m.prober.DNSResults())
	headerKey := fmt.Sprintf("%d_%s_%s", worstLevel, worstIssue, m.signozErr)
	if m.Changed("header", headerKey) {
		reg := Region{0, 0, m.Width(), 35}
		r.Clear(reg)
		r.DrawText(5, 8, "Fleet Health", m.fonts.Large, m.Colors().Header)

		// SigNoz error indicator
		if m.signozErr != "" {
			r.DrawText(float64(m.Width()/2-20), 10, "SigNoz ✗", m.fonts.Small, m.Colors().BarHigh)
		}

		// Connectivity orb + "Conn" on far right of title bar
		orbRadius := 7.0
		orbX := float64(m.Width()) - 55
		orbY := 18.0

		var orbColor color.Color
		switch worstLevel {
		case 2:
			orbColor = color.RGBA{255, 0, 0, 255}
		case 1:
			orbColor = color.RGBA{255, 255, 0, 255}
		default:
			orbColor = color.RGBA{0, 255, 0, 255}
		}
		r.DrawCircle(orbX, orbY, orbRadius, orbColor)
		r.DrawText(orbX+11, 8, "Conn", m.fonts.Normal, m.Colors().TextDim)

		updates = append(updates, reg)
	}

	// -- Internet section (2 lines) --
	updates = append(updates, m.updateInternet(r)...)

	// -- MooseFS section --
	updates = append(updates, m.updateMooseFS(r)...)

	// -- Node charts --
	updates = append(updates, m.updateNodeCharts(r)...)

	// -- Alerts --
	updates = append(updates, m.updateAlerts(r)...)

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

func (m *NetMonitor) updateInternet(r *Renderer) []Region {
	var updates []Region

	iface := m.prober.IfaceStats()
	pingResults := m.prober.PingResults()
	dnsResults := m.prober.DNSResults()

	// Single line: rates on left, issue detail on right if any
	_, worstIssue := m.assessConnectivity(pingResults, dnsResults)
	worstLevel, _ := m.assessConnectivity(pingResults, dnsResults)
	rateKey := fmt.Sprintf("%.0f_%.0f_%d_%s_%s", iface.RateIn, iface.RateOut, worstLevel, worstIssue, iface.Name)
	if m.Changed("inet_all", rateKey) {
		reg := Region{5, m.inetY, m.Width() - 10, 34}
		r.Clear(reg)

		// Line 1: ↓ rate  ↑ rate  [iface]
		inRate := fmt.Sprintf("↓ %s", netprobe.FormatRate(iface.RateIn))
		outRate := fmt.Sprintf("↑ %s", netprobe.FormatRate(iface.RateOut))

		inColor := m.Colors().BarLow
		if iface.RateIn > 50e6 {
			inColor = m.Colors().BarMed
		}
		outColor := m.Colors().BarLow
		if iface.RateOut > 50e6 {
			outColor = m.Colors().BarMed
		}

		r.DrawText(5, float64(m.inetY), inRate, m.fonts.Normal, inColor)
		r.DrawText(120, float64(m.inetY), outRate, m.fonts.Normal, outColor)

		if iface.Name != "" {
			r.DrawTextRight(5, float64(m.inetY), float64(m.Width()-10), iface.Name, m.fonts.Small, m.Colors().TextDim)
		}

		// Line 2: issue detail if connectivity is degraded
		if worstIssue != "" {
			issueColor := m.Colors().BarMed
			if worstLevel == 2 {
				issueColor = m.Colors().BarHigh
			}
			r.DrawText(5, float64(m.inetY+17), worstIssue, m.fonts.Small, issueColor)
		}

		updates = append(updates, reg)
	}

	return updates
}

// assessConnectivity returns (level, issue) where level: 0=green, 1=yellow, 2=red
func (m *NetMonitor) assessConnectivity(pings []netprobe.PingResult, dns []netprobe.DNSResult) (int, string) {
	worst := 0
	worstMsg := ""

	for _, pr := range pings {
		if !pr.Alive {
			worst = 2
			worstMsg = fmt.Sprintf("%s DOWN", pr.Label)
		} else if pr.Loss > 0.1 && worst < 2 {
			worst = 2
			worstMsg = fmt.Sprintf("%s %.0f%% loss", pr.Label, pr.Loss*100)
		} else if pr.Loss > 0.03 && worst < 1 {
			worst = 1
			worstMsg = fmt.Sprintf("%s %.0f%% loss", pr.Label, pr.Loss*100)
		} else if pr.Latency > 100*time.Millisecond && worst < 1 {
			worst = 1
			worstMsg = fmt.Sprintf("%s %s", pr.Label, netprobe.FormatLatency(pr.Latency))
		}
	}

	for _, dr := range dns {
		if dr.Error != "" {
			if worst < 2 {
				worst = 2
				worstMsg = fmt.Sprintf("DNS %s fail", dr.Domain)
			}
		} else if dr.Latency > 200*time.Millisecond && worst < 1 {
			worst = 1
			worstMsg = fmt.Sprintf("DNS %s %s", dr.Domain, netprobe.FormatLatency(dr.Latency))
		}
	}

	return worst, worstMsg
}

func (m *NetMonitor) updateMooseFS(r *Renderer) []Region {
	var updates []Region

	if m.mooseFS == nil {
		errText := "awaiting data..."
		if m.signozErr != "" {
			errText = m.signozErr
		}
		if m.Changed("moose", "none_"+errText) {
			reg := Region{5, m.mooseY, m.Width() - 10, 36}
			r.Clear(reg)
			r.DrawText(5, float64(m.mooseY), "MooseFS", m.fonts.Normal, m.Colors().Header)
			errColor := m.Colors().TextDim
			if m.signozErr != "" {
				errColor = m.Colors().BarHigh
			}
			r.DrawText(5, float64(m.mooseY+16), errText, m.fonts.Small, errColor)
			updates = append(updates, reg)
		}
		return updates
	}

	mooseKey := fmt.Sprintf("%.0f_%.0f_%.1f", m.mooseFS.TotalSpace, m.mooseFS.AvailSpace, m.mooseFS.UsedPct)
	if m.Changed("moose", mooseKey) {
		reg := Region{5, m.mooseY, m.Width() - 10, 36}
		r.Clear(reg)

		// Label
		label := fmt.Sprintf("MooseFS  %s / %s",
			signoz.FormatBytes(m.mooseFS.TotalSpace-m.mooseFS.AvailSpace),
			signoz.FormatBytes(m.mooseFS.TotalSpace))
		r.DrawText(5, float64(m.mooseY), label, m.fonts.Normal, m.Colors().Header)

		// Percentage on right
		pctText := fmt.Sprintf("%.1f%%", m.mooseFS.UsedPct)
		r.DrawTextRight(5, float64(m.mooseY), float64(m.Width()-10), pctText, m.fonts.Normal, m.Colors().Text)

		// Bar
		barReg := Region{5, m.mooseY + 18, m.Width() - 10, 14}
		r.DrawBar(barReg, m.mooseFS.UsedPct, 0, 100, true)

		updates = append(updates, reg)
	}

	return updates
}

func (m *NetMonitor) updateNodeCharts(r *Renderer) []Region {
	var updates []Region

	if m.nodeStats == nil {
		errText := "awaiting data..."
		if m.signozErr != "" {
			errText = m.signozErr
		}
		if m.Changed("charts", "none_"+errText) {
			reg := Region{5, m.chartsY, m.Width() - 10, m.alertsY - m.chartsY - 4}
			r.Clear(reg)
			r.DrawText(5, float64(m.chartsY), "Memory %", m.fonts.Small, m.Colors().Header)
			r.DrawText(float64(m.Width()/2+5), float64(m.chartsY), "CPU Load", m.fonts.Small, m.Colors().Header)
			errColor := m.Colors().TextDim
			if m.signozErr != "" {
				errColor = m.Colors().BarHigh
			}
			r.DrawText(5, float64(m.chartsY+16), errText, m.fonts.Small, errColor)
			updates = append(updates, reg)
		}
		return updates
	}

	// Build chart key for change detection
	chartKey := m.formatNodeKey()
	if !m.Changed("charts", chartKey) {
		return updates
	}

	halfW := m.Width() / 2
	chartH := m.alertsY - m.chartsY - 4

	// Left chart: Memory % by node
	leftReg := Region{0, m.chartsY, halfW - 2, chartH}
	r.Clear(leftReg)
	r.DrawText(5, float64(m.chartsY), "Memory %", m.fonts.Small, m.Colors().Header)

	hosts := m.sortedHosts(m.nodeStats.MemoryPct)
	barY := m.chartsY + 16
	barH := 12
	spacing := 2
	maxBars := (chartH - 18) / (barH + spacing)
	if len(hosts) > maxBars {
		hosts = hosts[:maxBars]
	}

	for i, host := range hosts {
		y := barY + i*(barH+spacing)
		pct := m.nodeStats.MemoryPct[host]

		// Short hostname
		shortHost := host
		if len(shortHost) > 8 {
			shortHost = shortHost[:8]
		}
		r.DrawText(5, float64(y), shortHost, m.fonts.Small, m.Colors().TextDim)

		// Bar
		bReg := Region{70, y, halfW - 110, barH}
		r.DrawBar(bReg, pct, 0, 100, false)

		// Percentage
		r.DrawTextRight(float64(halfW-40), float64(y), 35, fmt.Sprintf("%.0f%%", pct), m.fonts.Small, m.Colors().Text)
	}
	updates = append(updates, leftReg)

	// Right chart: CPU Load by node
	rightReg := Region{halfW + 2, m.chartsY, halfW - 4, chartH}
	r.Clear(rightReg)
	r.DrawText(float64(halfW+5), float64(m.chartsY), "CPU Load", m.fonts.Small, m.Colors().Header)

	cpuHosts := m.sortedHosts(m.nodeStats.CPULoad)
	if len(cpuHosts) > maxBars {
		cpuHosts = cpuHosts[:maxBars]
	}

	// Find max load for scaling
	maxLoad := 1.0
	for _, v := range m.nodeStats.CPULoad {
		if v > maxLoad {
			maxLoad = v
		}
	}
	// Round up to nearest sensible scale
	if maxLoad < 4 {
		maxLoad = 4
	} else if maxLoad < 8 {
		maxLoad = 8
	} else if maxLoad < 16 {
		maxLoad = 16
	} else if maxLoad < 32 {
		maxLoad = 32
	} else {
		maxLoad = 64
	}

	for i, host := range cpuHosts {
		y := barY + i*(barH+spacing)
		load := m.nodeStats.CPULoad[host]

		shortHost := host
		if len(shortHost) > 8 {
			shortHost = shortHost[:8]
		}
		r.DrawText(float64(halfW+5), float64(y), shortHost, m.fonts.Small, m.Colors().TextDim)

		// Bar scaled to max
		bReg := Region{halfW + 70, y, halfW - 112, barH}
		r.DrawBar(bReg, load, 0, maxLoad, false)

		// Value
		r.DrawTextRight(float64(m.Width()-40), float64(y), 35, fmt.Sprintf("%.1f", load), m.fonts.Small, m.Colors().Text)
	}
	updates = append(updates, rightReg)

	return updates
}

func (m *NetMonitor) updateAlerts(r *Renderer) []Region {
	var updates []Region

	alertKey := m.formatAlertKey()
	if !m.Changed("alerts", alertKey) {
		return updates
	}

	reg := Region{0, m.alertsY, m.Width(), m.Height() - m.alertsY}
	r.Clear(reg)

	// Header
	r.DrawText(5, float64(m.alertsY), "Alerts", m.fonts.Normal, m.Colors().Header)

	if len(m.alerts) == 0 {
		r.DrawText(5, float64(m.alertsY+16), "No firing alerts", m.fonts.Small, m.Colors().BarLow)
		updates = append(updates, reg)
		return updates
	}

	// Active alerts count
	countText := fmt.Sprintf("(%d firing)", len(m.alerts))
	r.DrawTextRight(5, float64(m.alertsY), float64(m.Width()-10), countText, m.fonts.Small, m.Colors().BarHigh)

	rowH := 16
	maxAlerts := (m.Height() - m.alertsY - 18) / rowH
	showing := m.alerts
	if len(showing) > maxAlerts {
		showing = showing[:maxAlerts]
	}

	for i, alert := range showing {
		y := m.alertsY + 18 + i*rowH

		// Severity color
		var sevColor color.Color
		switch alert.Severity {
		case "critical":
			sevColor = color.RGBA{255, 0, 0, 255}
		case "warning":
			sevColor = color.RGBA{255, 255, 0, 255}
		default:
			sevColor = color.RGBA{255, 150, 0, 255}
		}

		// Small orb
		r.DrawCircle(12, float64(y)+7, 4, sevColor)

		// Alert name
		name := alert.Name
		if len(name) > 45 {
			name = name[:45]
		}
		r.DrawText(22, float64(y), name, m.fonts.Small, m.Colors().Text)
	}

	updates = append(updates, reg)
	return updates
}

func (m *NetMonitor) sortedHosts(data map[string]float64) []string {
	hosts := make([]string, 0, len(data))
	for h := range data {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	return hosts
}

func (m *NetMonitor) formatNodeKey() string {
	if m.nodeStats == nil {
		return "nil"
	}
	key := "mem:"
	for h, v := range m.nodeStats.MemoryPct {
		key += fmt.Sprintf("%s=%.0f;", h, v)
	}
	key += "cpu:"
	for h, v := range m.nodeStats.CPULoad {
		key += fmt.Sprintf("%s=%.1f;", h, v)
	}
	return key
}

func (m *NetMonitor) formatAlertKey() string {
	if len(m.alerts) == 0 {
		return "none"
	}
	key := ""
	for _, a := range m.alerts {
		key += a.Name + ":" + a.State + ";"
	}
	return key
}
