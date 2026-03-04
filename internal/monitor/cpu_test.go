package monitor

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/aleksclark/go-turing-smart-screen/internal/sysinfo"
)

// MockScreen captures rendered images for testing.
type MockScreen struct {
	width      int
	height     int
	lastImage  image.Image
	lastX      int
	lastY      int
	drawCalls  []drawCall
	fullBuffer *image.RGBA
}

type drawCall struct {
	img image.Image
	x   int
	y   int
}

func NewMockScreen(width, height int) *MockScreen {
	return &MockScreen{
		width:      width,
		height:     height,
		fullBuffer: image.NewRGBA(image.Rect(0, 0, width, height)),
	}
}

func (m *MockScreen) Width() int  { return m.width }
func (m *MockScreen) Height() int { return m.height }
func (m *MockScreen) Close() error { return nil }
func (m *MockScreen) ScreenOn() error { return nil }
func (m *MockScreen) ScreenOff() error { return nil }

func (m *MockScreen) DrawImage(img image.Image, x, y int) error {
	m.lastImage = img
	m.lastX = x
	m.lastY = y
	m.drawCalls = append(m.drawCalls, drawCall{img: img, x: x, y: y})

	// Composite onto full buffer
	bounds := img.Bounds()
	for py := bounds.Min.Y; py < bounds.Max.Y; py++ {
		for px := bounds.Min.X; px < bounds.Max.X; px++ {
			m.fullBuffer.Set(px, py, img.At(px, py))
		}
	}
	return nil
}

func (m *MockScreen) Reset() {
	m.drawCalls = nil
	m.lastImage = nil
	m.fullBuffer = image.NewRGBA(image.Rect(0, 0, m.width, m.height))
}

// Snapshot returns current buffer as PNG bytes.
func (m *MockScreen) Snapshot() ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, m.fullBuffer); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MockCPUInfo provides controllable CPU data for testing.
type MockCPUInfo struct {
	PerCPU    []float64
	Overall   float64
	Freq      float64
	Load1     float64
	Load5     float64
	Load15    float64
	Temp      float64
	CoreCount int
}

func (m *MockCPUInfo) ToSysinfo() *sysinfo.CPUInfo {
	return &sysinfo.CPUInfo{
		PerCPU:    m.PerCPU,
		Overall:   m.Overall,
		Freq:      m.Freq,
		Load1:     m.Load1,
		Load5:     m.Load5,
		Load15:    m.Load15,
		Temp:      m.Temp,
		CoreCount: m.CoreCount,
	}
}

// snapshotDir is where test snapshots are stored.
const snapshotDir = "testdata/snapshots"

// updateSnapshots can be set to true to regenerate snapshots.
var updateSnapshots = os.Getenv("UPDATE_SNAPSHOTS") == "1"

func loadSnapshot(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join(snapshotDir, name+".png")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("Failed to load snapshot %s: %v", name, err)
	}
	return data
}

func saveSnapshot(t *testing.T, name string, data []byte) {
	t.Helper()
	path := filepath.Join(snapshotDir, name+".png")
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		t.Fatalf("Failed to create snapshot dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("Failed to save snapshot %s: %v", name, err)
	}
}

func compareSnapshots(t *testing.T, name string, actual []byte) {
	t.Helper()
	
	if updateSnapshots {
		saveSnapshot(t, name, actual)
		t.Logf("Updated snapshot: %s", name)
		return
	}

	expected := loadSnapshot(t, name)
	if expected == nil {
		saveSnapshot(t, name, actual)
		t.Logf("Created new snapshot: %s", name)
		return
	}

	if !bytes.Equal(expected, actual) {
		// Save actual for comparison
		actualPath := filepath.Join(snapshotDir, name+"_actual.png")
		os.WriteFile(actualPath, actual, 0644)
		t.Errorf("Snapshot mismatch for %s. Actual saved to %s. Run with UPDATE_SNAPSHOTS=1 to update.", name, actualPath)
	}
}

func TestCPUMonitor_Layout(t *testing.T) {
	screen := NewMockScreen(480, 320)
	mon := NewCPUMonitor(screen, 50, 0, nil)
	
	// Simulate 16 cores
	mon.cpuCount = 16
	mon.setupLayout()

	if mon.coreY != 62 {
		t.Errorf("Expected coreY=62, got %d", mon.coreY)
	}
	if mon.coreH != 45 {
		t.Errorf("Expected coreH=45, got %d", mon.coreH)
	}
	expectedOverallY := 62 + 45 + 3
	if mon.overallY != expectedOverallY {
		t.Errorf("Expected overallY=%d, got %d", expectedOverallY, mon.overallY)
	}
	expectedPanelsY := expectedOverallY + 30
	if mon.panelsY != expectedPanelsY {
		t.Errorf("Expected panelsY=%d, got %d", expectedPanelsY, mon.panelsY)
	}
	if mon.gpuY != 258 {
		t.Errorf("Expected gpuY=258, got %d", mon.gpuY)
	}
}

func TestCPUMonitor_VerticalBars(t *testing.T) {
	screen := NewMockScreen(480, 320)
	mon := NewCPUMonitor(screen, 50, 0, nil)
	
	mon.cpuCount = 16
	mon.setupLayout()
	
	margin := 5
	spacing := 2
	availWidth := mon.Width() - 2*margin
	expectedBarWidth := (availWidth - (mon.cpuCount-1)*spacing) / mon.cpuCount
	
	if expectedBarWidth < 25 || expectedBarWidth > 30 {
		t.Errorf("Expected bar width ~27, got %d", expectedBarWidth)
	}
}

func TestCPUMonitor_InitialDraw(t *testing.T) {
	screen := NewMockScreen(480, 320)
	mon := NewCPUMonitor(screen, 50, 0, nil)
	
	mon.cpuCount = 8
	mon.setupLayout()
	mon.ClearBuffer()
	mon.drawStatic()
	
	err := mon.DrawFullBuffer()
	if err != nil {
		t.Fatalf("DrawFullBuffer failed: %v", err)
	}
	
	if len(screen.drawCalls) == 0 {
		t.Error("Expected at least one draw call for initial render")
	}
	
	if screen.lastImage == nil {
		t.Fatal("No image was drawn")
	}
	
	bounds := screen.lastImage.Bounds()
	if bounds.Dx() != 480 || bounds.Dy() != 320 {
		t.Errorf("Expected 480x320 image, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestCPUMonitor_Snapshot_8Cores_Idle(t *testing.T) {
	screen := NewMockScreen(480, 320)
	mon := NewCPUMonitor(screen, 50, 0, nil)
	
	mon.cpuCount = 8
	mon.setupLayout()
	mon.ClearBuffer()
	mon.drawStatic()
	mon.DrawFullBuffer()
	
	dc := mon.NewContext(Region{0, 0, mon.Width(), mon.Height()})
	r := NewRenderer(dc, mon.Colors(), mon.fonts)
	
	r.DrawText(5, 8, "CPU Monitor - 8 cores | 45C", mon.fonts.Large, mon.Colors().Header)
	r.DrawText(5, 38, "Freq: 3.50 GHz", mon.fonts.Normal, mon.Colors().TextDim)
	r.DrawText(190, 38, "Load: 0.50 1.00 0.75", mon.fonts.Normal, mon.Colors().TextDim)
	
	margin := 5
	spacing := 2
	availWidth := mon.Width() - 2*margin
	barWidth := (availWidth - (mon.cpuCount-1)*spacing) / mon.cpuCount
	
	for i := 0; i < mon.cpuCount; i++ {
		x := margin + i*(barWidth+spacing)
		reg := Region{x, mon.coreY, barWidth, mon.coreH}
		r.DrawVerticalBar(reg, 5.0, 0, 100)
	}
	
	r.DrawBar(Region{45, mon.overallY, mon.Width() - 120, 24}, 5.0, 0, 100, true)
	r.DrawTextRight(float64(mon.Width()-70), float64(mon.overallY), 65, "  5.0%", mon.fonts.Normal, mon.Colors().Text)
	
	drawTestPanels(mon, r, []mockProcess{
		{"idle", 0.5},
		{"kworker", 0.2},
	}, []mockTemp{
		{"CPU", 45.0},
		{"NVMe", 35.0},
	})

	drawTestGPU(mon, r, nil)
	
	mon.DrawFullBuffer()
	
	snapshot, err := screen.Snapshot()
	if err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}
	
	compareSnapshots(t, "cpu_8cores_idle", snapshot)
}

func TestCPUMonitor_Snapshot_16Cores_Mixed(t *testing.T) {
	screen := NewMockScreen(480, 320)
	mon := NewCPUMonitor(screen, 50, 0, nil)
	
	mon.cpuCount = 16
	mon.setupLayout()
	mon.ClearBuffer()
	mon.drawStatic()
	mon.DrawFullBuffer()
	
	dc := mon.NewContext(Region{0, 0, mon.Width(), mon.Height()})
	r := NewRenderer(dc, mon.Colors(), mon.fonts)
	
	r.DrawText(5, 8, "CPU Monitor - 16 cores | 72C", mon.fonts.Large, mon.Colors().Header)
	r.DrawText(5, 38, "Freq: 4.20 GHz", mon.fonts.Normal, mon.Colors().TextDim)
	r.DrawText(190, 38, "Load: 8.50 6.00 4.25", mon.fonts.Normal, mon.Colors().TextDim)
	
	margin := 5
	spacing := 2
	availWidth := mon.Width() - 2*margin
	barWidth := (availWidth - (mon.cpuCount-1)*spacing) / mon.cpuCount
	
	usages := []float64{95, 87, 72, 65, 45, 38, 25, 15, 92, 80, 55, 40, 30, 20, 10, 5}
	for i := 0; i < mon.cpuCount; i++ {
		x := margin + i*(barWidth+spacing)
		reg := Region{x, mon.coreY, barWidth, mon.coreH}
		r.DrawVerticalBar(reg, usages[i], 0, 100)
	}
	
	overall := 48.0
	r.DrawBar(Region{45, mon.overallY, mon.Width() - 120, 24}, overall, 0, 100, true)
	r.DrawTextRight(float64(mon.Width()-70), float64(mon.overallY), 65, " 48.0%", mon.fonts.Normal, mon.Colors().Text)
	
	drawTestPanels(mon, r, []mockProcess{
		{"chrome", 45.2},
		{"code", 32.5},
		{"node", 18.3},
		{"firefox", 8.7},
		{"gopls", 5.1},
	}, []mockTemp{
		{"CPU", 72.0},
		{"GPU", 65.0},
		{"NVMe", 48.0},
		{"WiFi", 42.0},
	})

	drawTestGPU(mon, r, &sysinfo.GPUInfo{
		Vendor:     sysinfo.GPUVendorAMD,
		Load:       42,
		Temp:       65,
		MemUsed:    3 * 1024 * 1024 * 1024,
		MemTotal:   16 * 1024 * 1024 * 1024,
		MemPercent: 18.75,
		Power:      85,
	})
	
	mon.DrawFullBuffer()
	
	snapshot, err := screen.Snapshot()
	if err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}
	
	compareSnapshots(t, "cpu_16cores_mixed", snapshot)
}

func TestCPUMonitor_Snapshot_32Cores_HighLoad(t *testing.T) {
	screen := NewMockScreen(480, 320)
	mon := NewCPUMonitor(screen, 50, 0, nil)
	
	mon.cpuCount = 32
	mon.setupLayout()
	mon.ClearBuffer()
	mon.drawStatic()
	mon.DrawFullBuffer()
	
	dc := mon.NewContext(Region{0, 0, mon.Width(), mon.Height()})
	r := NewRenderer(dc, mon.Colors(), mon.fonts)
	
	r.DrawText(5, 8, "CPU Monitor - 32 cores | 89C", mon.fonts.Large, mon.Colors().Header)
	r.DrawText(5, 38, "Freq: 4.50 GHz", mon.fonts.Normal, mon.Colors().TextDim)
	r.DrawText(190, 38, "Load: 28.0 24.0 20.0", mon.fonts.Normal, mon.Colors().TextDim)
	
	margin := 5
	spacing := 2
	availWidth := mon.Width() - 2*margin
	barWidth := (availWidth - (mon.cpuCount-1)*spacing) / mon.cpuCount
	
	for i := 0; i < mon.cpuCount; i++ {
		x := margin + i*(barWidth+spacing)
		reg := Region{x, mon.coreY, barWidth, mon.coreH}
		usage := 75.0 + float64(i%8)*3.5
		r.DrawVerticalBar(reg, usage, 0, 100)
	}
	
	r.DrawBar(Region{45, mon.overallY, mon.Width() - 120, 24}, 92.0, 0, 100, true)
	r.DrawTextRight(float64(mon.Width()-70), float64(mon.overallY), 65, " 92.0%", mon.fonts.Normal, mon.Colors().Text)
	
	drawTestPanels(mon, r, []mockProcess{
		{"compile", 320.5},
		{"rust-analyz", 85.2},
		{"chrome", 62.1},
		{"code", 45.8},
		{"docker", 28.3},
	}, []mockTemp{
		{"CPU", 89.0},
		{"GPU", 78.0},
		{"VRAM", 72.0},
		{"NVMe", 55.0},
		{"WiFi", 45.0},
	})

	drawTestGPU(mon, r, &sysinfo.GPUInfo{
		Vendor:     sysinfo.GPUVendorNVIDIA,
		Load:       95,
		Temp:       82,
		MemUsed:    10 * 1024 * 1024 * 1024,
		MemTotal:   12 * 1024 * 1024 * 1024,
		MemPercent: 83.3,
		Power:      280,
	})
	
	mon.DrawFullBuffer()
	
	snapshot, err := screen.Snapshot()
	if err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}
	
	compareSnapshots(t, "cpu_32cores_highload", snapshot)
}

type mockProcess struct {
	name    string
	percent float64
}

type mockTemp struct {
	label string
	temp  float64
}

func drawTestPanels(mon *CPUMonitor, r *Renderer, procs []mockProcess, temps []mockTemp) {
	panelWidth := mon.Width() / 2
	lineHeight := 18
	maxRows := (mon.gpuY - 3 - (mon.panelsY + 20)) / lineHeight
	if maxRows > 5 {
		maxRows = 5
	}

	// Left panel: processes
	procY := mon.panelsY + 20
	for i, p := range procs {
		if i >= maxRows {
			break
		}
		y := procY + i*lineHeight
		name := p.name
		if len(name) > 12 {
			name = name[:12]
		}
		r.DrawText(5, float64(y), name, mon.fonts.Small, mon.Colors().Text)
		r.DrawTextRight(float64(panelWidth-60), float64(y), 50,
			fmt.Sprintf("%5.1f%%", p.percent), mon.fonts.Small, mon.Colors().TextDim)
	}

	// Right panel: temps
	tempY := mon.panelsY + 20
	for i, t := range temps {
		if i >= maxRows {
			break
		}
		y := tempY + i*lineHeight
		r.DrawText(float64(panelWidth+5), float64(y), t.label, mon.fonts.Small, mon.Colors().Text)
		tempColor := mon.Colors().BarLow
		if t.temp >= 90 {
			tempColor = mon.Colors().BarHigh
		} else if t.temp >= 80 {
			tempColor = mon.Colors().BarMed
		}
		r.DrawTextRight(float64(mon.Width()-55), float64(y), 50,
			fmt.Sprintf("%5.1fC", t.temp), mon.fonts.Small, tempColor)
	}
}

func drawTestGPU(mon *CPUMonitor, r *Renderer, gpu *sysinfo.GPUInfo) {
	if gpu == nil {
		r.DrawText(40, float64(mon.gpuY), "not detected", mon.fonts.Small, mon.Colors().TextDim)
		return
	}

	// Row 1: Load bar + percentage + temp + power
	barReg := Region{40, mon.gpuY + 2, 180, 16}
	r.DrawBar(barReg, gpu.Load, 0, 100, true)

	r.DrawText(225, float64(mon.gpuY), fmt.Sprintf("%4.0f%%", gpu.Load),
		mon.fonts.Small, mon.Colors().Text)

	if gpu.Temp > 0 {
		tempColor := mon.Colors().BarLow
		if gpu.Temp >= 90 {
			tempColor = mon.Colors().BarHigh
		} else if gpu.Temp >= 75 {
			tempColor = mon.Colors().BarMed
		}
		r.DrawText(280, float64(mon.gpuY), fmt.Sprintf("%.0fC", gpu.Temp),
			mon.fonts.Small, tempColor)
	}

	if gpu.Power > 0 {
		r.DrawTextRight(float64(mon.Width()-5-60), float64(mon.gpuY), 60,
			fmt.Sprintf("%.0fW", gpu.Power), mon.fonts.Small, mon.Colors().TextDim)
	}

	// Row 2: VRAM bar + percentage + usage
	row2Y := mon.gpuY + 22
	r.DrawText(5, float64(row2Y), "VRAM", mon.fonts.Small, mon.Colors().Header)

	if gpu.MemTotal > 0 {
		vramBarReg := Region{40, row2Y + 2, 180, 16}
		r.DrawBar(vramBarReg, gpu.MemPercent, 0, 100, true)

		r.DrawText(225, float64(row2Y), fmt.Sprintf("%4.0f%%", gpu.MemPercent),
			mon.fonts.Small, mon.Colors().Text)

		r.DrawTextRight(float64(mon.Width()-5-120), float64(row2Y), 120,
			fmt.Sprintf("%s / %s", sysinfo.FormatBytes(gpu.MemUsed), sysinfo.FormatBytes(gpu.MemTotal)),
			mon.fonts.Small, mon.Colors().TextDim)
	}
}

func TestRenderer_DrawVerticalBar(t *testing.T) {
	screen := NewMockScreen(480, 320)
	mon := NewCPUMonitor(screen, 50, 0, nil)
	mon.ClearBuffer()
	
	dc := mon.NewContext(Region{0, 0, mon.Width(), mon.Height()})
	r := NewRenderer(dc, mon.Colors(), mon.fonts)
	
	tests := []struct {
		name  string
		value float64
		color string
	}{
		{"empty", 0, "none"},
		{"low", 25, "green"},
		{"medium", 60, "yellow"},
		{"high", 90, "red"},
		{"full", 100, "red"},
	}
	
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := 10 + i*50
			reg := Region{x, 50, 40, 100}
			r.DrawVerticalBar(reg, tt.value, 0, 100)
		})
	}
	
	mon.DrawFullBuffer()
	
	snapshot, err := screen.Snapshot()
	if err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}
	
	compareSnapshots(t, "vertical_bars_gradient", snapshot)
}
