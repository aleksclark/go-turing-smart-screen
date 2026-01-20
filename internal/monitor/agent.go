package monitor

import (
	"fmt"
	"image/color"
	"log/slog"
	"time"

	"github.com/aleksclark/go-turing-smart-screen/internal/lcd"
	"github.com/aleksclark/go-turing-smart-screen/pkg/agentstat"
)

// Status colors for indicator circles.
var statusColors = map[string]color.Color{
	"idle":     color.RGBA{100, 100, 100, 255}, // Gray
	"thinking": color.RGBA{255, 255, 0, 255},   // Yellow
	"working":  color.RGBA{0, 255, 0, 255},     // Green
	"waiting":  color.RGBA{0, 150, 255, 255},   // Blue
	"error":    color.RGBA{255, 0, 0, 255},     // Red
	"done":     color.RGBA{0, 255, 150, 255},   // Teal
	"paused":   color.RGBA{255, 150, 0, 255},   // Orange
}

var defaultStatusColor = color.RGBA{80, 80, 80, 255}

// AgentMonitor displays coding agent status.
type AgentMonitor struct {
	*Base
	rowHeight   int
	numRows     int
	lastCleanup time.Time
}

// NewAgentMonitor creates a new agent monitor.
func NewAgentMonitor(screen lcd.Screen, brightness int, interval time.Duration, logger *slog.Logger) *AgentMonitor {
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

	return &AgentMonitor{
		Base:    base,
		numRows: 5,
	}
}

// Name returns the monitor name.
func (m *AgentMonitor) Name() string { return "Agent" }

// Run starts the agent monitor loop.
func (m *AgentMonitor) Run() error {
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
func (m *AgentMonitor) Stop() {
	m.SetRunning(false)
}

func (m *AgentMonitor) setupLayout() {
	yStart := 60
	availableHeight := m.Height() - yStart - 15
	m.rowHeight = availableHeight / m.numRows
	if m.rowHeight < 45 {
		m.rowHeight = 45
	}
}

func (m *AgentMonitor) drawStatic() {
	dc := m.NewContext(Region{0, 0, m.Width(), m.Height()})
	r := NewRenderer(dc, m.Colors(), m.fonts)

	// Header separator
	r.DrawLine(0, 32, float64(m.Width()))

	// Row separators
	yStart := 60
	for i := 0; i <= m.numRows; i++ {
		y := float64(yStart + i*m.rowHeight - 2)
		r.DrawLine(0, y, float64(m.Width()))
	}
}

func (m *AgentMonitor) update() error {
	// Periodic cleanup
	if time.Since(m.lastCleanup) > 5*time.Minute {
		agentstat.Cleanup(time.Hour)
		m.lastCleanup = time.Now()
	}

	agents, err := agentstat.ReadAll(agentstat.StaleThreshold)
	if err != nil {
		return err
	}

	dc := m.NewContext(Region{0, 0, m.Width(), m.Height()})
	r := NewRenderer(dc, m.Colors(), m.fonts)

	var updates []Region

	// Header
	if m.Changed("header", true) {
		reg := Region{5, 8, m.Width() - 10, 24}
		r.Clear(reg)
		r.DrawText(float64(reg.X), float64(reg.Y), "Agent Status Monitor", m.fonts.Large, m.Colors().Header)
		updates = append(updates, reg)
	}

	// Summary
	var active, stale int
	for _, a := range agents {
		if a.Stale {
			stale++
		} else {
			active++
		}
	}
	var summary string
	if len(agents) > 0 {
		summary = fmt.Sprintf("%d active, %d stale", active, stale)
	} else {
		summary = "No agents reporting"
	}
	if m.Changed("summary", summary) {
		reg := Region{5, 35, m.Width() - 10, 20}
		r.Clear(reg)
		r.DrawText(float64(reg.X), float64(reg.Y), summary, m.fonts.Normal, m.Colors().TextDim)
		updates = append(updates, reg)
	}

	// Agent rows
	yStart := 60
	for i := 0; i < m.numRows; i++ {
		rowY := yStart + i*m.rowHeight
		reg := Region{0, rowY, m.Width(), m.rowHeight}

		if i < len(agents) {
			agent := agents[i]
			// Create hash for change detection
			key := fmt.Sprintf("agent_%d", i)
			hash := fmt.Sprintf("%s_%s_%s_%s_%v_%d_%.2f_%v",
				agent.Agent, agent.Status, agent.Task, agent.Model,
				agent.Tools != nil && agent.Tools.Active != "",
				func() int {
					if agent.Tokens != nil {
						return agent.Tokens.Input / 1000
					}
					return 0
				}(),
				agent.CostUSD, agent.Stale)

			if m.Changed(key, hash) {
				m.renderAgentRow(r, reg, &agent)
				updates = append(updates, reg)
			}
		} else {
			key := fmt.Sprintf("agent_%d", i)
			if m.Changed(key, "empty") {
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

func (m *AgentMonitor) renderAgentRow(r *Renderer, reg Region, agent *agentstat.Status) {
	r.Clear(reg)

	// Dim colors for stale agents
	textColor := m.Colors().Text
	if agent.Stale {
		textColor = m.Colors().TextDim
	}

	// Status indicator circle
	circleRadius := 8.0
	circleX := float64(reg.X + 15)
	circleY := float64(reg.Y) + float64(reg.H)/2

	statusColor, ok := statusColors[agent.Status]
	if !ok {
		statusColor = defaultStatusColor
	}
	if agent.Stale {
		// Dim the color
		if c, ok := statusColor.(color.RGBA); ok {
			statusColor = color.RGBA{c.R / 2, c.G / 2, c.B / 2, c.A}
		}
	}
	r.DrawCircle(circleX, circleY, circleRadius, statusColor)

	// Agent name and project (top row)
	xText := float64(reg.X + 35)
	title := agent.Agent
	if agent.Project != "" {
		proj := agent.Project
		if len(proj) > 20 {
			proj = proj[:20]
		}
		title = fmt.Sprintf("%s • %s", agent.Agent, proj)
	}
	if len(title) > 35 {
		title = title[:35]
	}
	r.DrawText(xText, float64(reg.Y), title, m.fonts.Normal, textColor)

	// Model (top right)
	if agent.Model != "" {
		model := agent.Model
		// Shorten model name
		for _, prefix := range []string{"claude-", "gpt-", "-20250514"} {
			model = removePrefix(model, prefix)
		}
		if len(model) > 20 {
			model = model[:20]
		}
		r.DrawTextRight(float64(m.Width()-5-100), float64(reg.Y), 100, model, m.fonts.Small, m.Colors().TextDim)
	}

	// Current task (middle row)
	if agent.Task != "" {
		task := agent.Task
		if len(task) > 50 {
			task = task[:50]
		}
		r.DrawText(xText, float64(reg.Y+16), task, m.fonts.Small, textColor)
	}

	// Tools info (bottom left)
	if agent.Tools != nil {
		if agent.Tools.Active != "" {
			r.DrawText(xText, float64(reg.Y+30), "▶ "+agent.Tools.Active, m.fonts.Small, m.Colors().Header)
		} else if len(agent.Tools.Recent) > 0 {
			r.DrawText(xText, float64(reg.Y+30), "◦ "+agent.Tools.Recent[len(agent.Tools.Recent)-1], m.fonts.Small, m.Colors().TextDim)
		}
	}

	// Token count (bottom middle)
	if agent.Tokens != nil && (agent.Tokens.Input > 0 || agent.Tokens.Output > 0) {
		tokText := fmt.Sprintf("↓%s ↑%s",
			agentstat.FormatTokens(agent.Tokens.Input),
			agentstat.FormatTokens(agent.Tokens.Output))
		r.DrawText(180, float64(reg.Y+30), tokText, m.fonts.Small, m.Colors().TextDim)
	}

	// Cost (bottom right)
	if agent.CostUSD > 0 {
		r.DrawTextRight(float64(m.Width()-5-60), float64(reg.Y+30), 60,
			agentstat.FormatCost(agent.CostUSD), m.fonts.Small, m.Colors().TextDim)
	}

	// Age indicator for stale
	if agent.Stale {
		ageText := fmt.Sprintf("%ds ago", int(agent.Age.Seconds()))
		r.DrawTextRight(float64(m.Width()-5-80), float64(reg.Y+16), 80, ageText, m.fonts.Small, m.Colors().TextDim)
	}
}

func removePrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	// Also try as suffix
	if len(s) >= len(prefix) && s[len(s)-len(prefix):] == prefix {
		return s[:len(s)-len(prefix)]
	}
	return s
}
