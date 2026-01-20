// Package monitor provides base types and rendering for LCD monitors.
package monitor

import (
	"image"
	"image/color"
	"image/draw"
	"log/slog"
	"os"
	"time"

	"github.com/fogleman/gg"
	"github.com/aleksclark/go-turing-smart-screen/internal/lcd"
)

// Colors defines the color palette for monitors.
type Colors struct {
	BG       color.Color
	Text     color.Color
	TextDim  color.Color
	Header   color.Color
	BarLow   color.Color
	BarMed   color.Color
	BarHigh  color.Color
	BarBG    color.Color
	Border   color.Color
}

// DefaultColors returns the default htop-style green palette.
func DefaultColors() Colors {
	return Colors{
		BG:       color.RGBA{0, 0, 0, 255},
		Text:     color.RGBA{0, 255, 0, 255},
		TextDim:  color.RGBA{0, 180, 0, 255},
		Header:   color.RGBA{0, 255, 255, 255},
		BarLow:   color.RGBA{0, 255, 0, 255},
		BarMed:   color.RGBA{255, 255, 0, 255},
		BarHigh:  color.RGBA{255, 0, 0, 255},
		BarBG:    color.RGBA{40, 40, 40, 255},
		Border:   color.RGBA{80, 80, 80, 255},
	}
}

// FontConfig holds font configuration.
type FontConfig struct {
	Path     string
	Small    float64
	Normal   float64
	Large    float64
}

// fontSearchPaths lists common font locations to search.
var fontSearchPaths = []string{
	// JetBrains Mono (preferred)
	"/usr/share/fonts/TTF/JetBrainsMono-Regular.ttf",
	"/usr/share/fonts/jetbrains-mono/JetBrainsMono-Regular.ttf",
	"/usr/share/fonts/truetype/jetbrains-mono/JetBrainsMono-Regular.ttf",
	// DejaVu Sans Mono (common fallback)
	"/usr/share/fonts/TTF/DejaVuSansMono.ttf",
	"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
	"/usr/share/fonts/dejavu/DejaVuSansMono.ttf",
	// Liberation Mono
	"/usr/share/fonts/TTF/LiberationMono-Regular.ttf",
	"/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
	// Noto Sans Mono
	"/usr/share/fonts/noto/NotoSansMono-Regular.ttf",
	"/usr/share/fonts/truetype/noto/NotoSansMono-Regular.ttf",
	// macOS
	"/System/Library/Fonts/Monaco.ttf",
	"/Library/Fonts/SF-Mono-Regular.otf",
	// Windows
	"C:/Windows/Fonts/consola.ttf",
	"C:/Windows/Fonts/cour.ttf",
}

// findFont searches for an available monospace font.
func findFont() string {
	for _, path := range fontSearchPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// DefaultFontConfig returns the default font configuration.
func DefaultFontConfig() FontConfig {
	path := findFont()
	if path == "" {
		path = "/usr/share/fonts/TTF/DejaVuSansMono.ttf" // Fallback
	}
	return FontConfig{
		Path:   path,
		Small:  14,
		Normal: 16,
		Large:  20,
	}
}

// Region represents a rectangular area on the display.
type Region struct {
	X, Y, W, H int
}

// Bounds returns the region as an image.Rectangle.
func (r Region) Bounds() image.Rectangle {
	return image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H)
}

// Monitor is the interface for all screen monitors.
type Monitor interface {
	Name() string
	Run() error
	Stop()
}

// Base provides common functionality for monitors.
type Base struct {
	screen   lcd.Screen
	width    int
	height   int
	colors   Colors
	fontPath string
	fonts    FontConfig
	running  bool
	interval time.Duration
	logger   *slog.Logger
	
	// Frame buffer
	buffer *image.RGBA
	
	// Value cache for change detection
	cache map[string]any
}

// Config holds base monitor configuration.
type Config struct {
	Screen   lcd.Screen
	Colors   Colors
	Fonts    FontConfig
	Interval time.Duration
	Logger   *slog.Logger
}

// NewBase creates a new base monitor.
func NewBase(cfg Config) *Base {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	
	w := cfg.Screen.Width()
	h := cfg.Screen.Height()
	
	return &Base{
		screen:   cfg.Screen,
		width:    w,
		height:   h,
		colors:   cfg.Colors,
		fonts:    cfg.Fonts,
		fontPath: cfg.Fonts.Path,
		interval: cfg.Interval,
		logger:   cfg.Logger,
		buffer:   image.NewRGBA(image.Rect(0, 0, w, h)),
		cache:    make(map[string]any),
	}
}

// Width returns the display width.
func (b *Base) Width() int { return b.width }

// Height returns the display height.
func (b *Base) Height() int { return b.height }

// Colors returns the color palette.
func (b *Base) Colors() Colors { return b.colors }

// Logger returns the logger.
func (b *Base) Logger() *slog.Logger { return b.logger }

// Interval returns the refresh interval.
func (b *Base) Interval() time.Duration { return b.interval }

// Running returns whether the monitor is running.
func (b *Base) Running() bool { return b.running }

// SetRunning sets the running state.
func (b *Base) SetRunning(r bool) { b.running = r }

// Screen returns the LCD screen.
func (b *Base) Screen() lcd.Screen { return b.screen }

// Buffer returns the frame buffer.
func (b *Base) Buffer() *image.RGBA { return b.buffer }

// ClearBuffer fills the buffer with background color.
func (b *Base) ClearBuffer() {
	draw.Draw(b.buffer, b.buffer.Bounds(), &image.Uniform{b.colors.BG}, image.Point{}, draw.Src)
}

// DrawFullBuffer sends the entire buffer to the display.
func (b *Base) DrawFullBuffer() error {
	return b.screen.DrawImage(b.buffer, 0, 0)
}

// DrawRegion sends a region of the buffer to the display.
func (b *Base) DrawRegion(r Region) error {
	sub := b.buffer.SubImage(r.Bounds())
	return b.screen.DrawImage(sub, r.X, r.Y)
}

// Changed checks if a value changed and updates cache.
func (b *Base) Changed(key string, value any) bool {
	if prev, ok := b.cache[key]; ok && prev == value {
		return false
	}
	b.cache[key] = value
	return true
}

// ChangedFloat checks if a float value changed beyond threshold.
func (b *Base) ChangedFloat(key string, value, threshold float64) bool {
	if prev, ok := b.cache[key].(float64); ok {
		if abs(value-prev) < threshold {
			return false
		}
	}
	b.cache[key] = value
	return true
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// NewContext creates a drawing context for a region.
func (b *Base) NewContext(r Region) *gg.Context {
	dc := gg.NewContextForRGBA(b.buffer)
	// Clip to region
	dc.DrawRectangle(float64(r.X), float64(r.Y), float64(r.W), float64(r.H))
	dc.Clip()
	return dc
}

// Renderer provides high-level drawing operations.
type Renderer struct {
	dc     *gg.Context
	colors Colors
	fonts  FontConfig
}

// NewRenderer creates a renderer for a context.
func NewRenderer(dc *gg.Context, colors Colors, fonts FontConfig) *Renderer {
	return &Renderer{dc: dc, colors: colors, fonts: fonts}
}

// Clear fills a region with background color.
func (r *Renderer) Clear(reg Region) {
	r.dc.SetColor(r.colors.BG)
	r.dc.DrawRectangle(float64(reg.X), float64(reg.Y), float64(reg.W), float64(reg.H))
	r.dc.Fill()
}

// DrawText draws text at a position.
func (r *Renderer) DrawText(x, y float64, text string, fontSize float64, c color.Color) {
	if err := r.dc.LoadFontFace(r.fonts.Path, fontSize); err != nil {
		return
	}
	r.dc.SetColor(c)
	r.dc.DrawString(text, x, y+fontSize)
}

// DrawTextRight draws right-aligned text.
func (r *Renderer) DrawTextRight(x, y, width float64, text string, fontSize float64, c color.Color) {
	if err := r.dc.LoadFontFace(r.fonts.Path, fontSize); err != nil {
		return
	}
	tw, _ := r.dc.MeasureString(text)
	r.dc.SetColor(c)
	r.dc.DrawString(text, x+width-tw, y+fontSize)
}

// DrawBar draws a progress bar.
func (r *Renderer) DrawBar(reg Region, value, min, max float64, showBorder bool) {
	// Background
	r.dc.SetColor(r.colors.BarBG)
	r.dc.DrawRectangle(float64(reg.X), float64(reg.Y), float64(reg.W), float64(reg.H))
	r.dc.Fill()
	
	// Border
	if showBorder {
		r.dc.SetColor(r.colors.Border)
		r.dc.DrawRectangle(float64(reg.X), float64(reg.Y), float64(reg.W), float64(reg.H))
		r.dc.Stroke()
	}
	
	if value <= min {
		return
	}
	
	// Calculate fill
	pct := (value - min) / (max - min)
	if pct > 1 {
		pct = 1
	}
	fillW := float64(reg.W-2) * pct
	
	// Color based on percentage
	var c color.Color
	switch {
	case pct < 0.5:
		c = r.colors.BarLow
	case pct < 0.8:
		c = r.colors.BarMed
	default:
		c = r.colors.BarHigh
	}
	
	r.dc.SetColor(c)
	r.dc.DrawRectangle(float64(reg.X+1), float64(reg.Y+1), fillW, float64(reg.H-2))
	r.dc.Fill()
}

// DrawLine draws a horizontal line.
func (r *Renderer) DrawLine(x1, y, x2 float64) {
	r.dc.SetColor(r.colors.Border)
	r.dc.DrawLine(x1, y, x2, y)
	r.dc.Stroke()
}

// DrawCircle draws a filled circle.
func (r *Renderer) DrawCircle(x, y, radius float64, c color.Color) {
	r.dc.SetColor(c)
	r.dc.DrawCircle(x, y, radius)
	r.dc.Fill()
}
