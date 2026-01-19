// Package lcd provides communication with Turing Smart Screen USB-C LCD displays.
package lcd

import (
	"fmt"
	"image"
	"io"
	"time"

	"go.bug.st/serial"
)

// Orientation defines screen orientation.
type Orientation byte

const (
	Portrait         Orientation = 0
	Landscape        Orientation = 1
	ReversePortrait  Orientation = 2
	ReverseLandscape Orientation = 3
)

// Command bytes for Rev A protocol.
const (
	cmdReset         byte = 101
	cmdClear         byte = 102
	cmdScreenOff     byte = 108
	cmdScreenOn      byte = 109
	cmdSetBrightness byte = 110
	cmdSetOrientation byte = 121
	cmdDisplayBitmap byte = 197
)

// Display represents a connection to a Turing Smart Screen LCD (Rev A).
type Display struct {
	port        serial.Port
	portName    string
	width       int
	height      int
	orientation Orientation
}

// Config holds display configuration.
type Config struct {
	Port        string
	Width       int
	Height      int
	Brightness  int
	Orientation Orientation
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		Port:        "/dev/ttyACM0",
		Width:       320,
		Height:      480,
		Brightness:  30,
		Orientation: ReverseLandscape,
	}
}

// New creates a new Display connection.
func New(cfg Config) (*Display, error) {
	d := &Display{
		portName: cfg.Port,
		width:    cfg.Width,
		height:   cfg.Height,
	}

	// Open serial port
	if err := d.openSerial(); err != nil {
		return nil, err
	}

	// Send HELLO to initialize communication
	if err := d.hello(); err != nil {
		d.Close()
		return nil, err
	}

	// Reset display (this reopens serial)
	if err := d.Reset(); err != nil {
		d.Close()
		return nil, err
	}

	// Send HELLO again after reset
	if err := d.hello(); err != nil {
		d.Close()
		return nil, err
	}

	if err := d.SetOrientation(cfg.Orientation); err != nil {
		d.Close()
		return nil, err
	}

	if err := d.SetBrightness(cfg.Brightness); err != nil {
		d.Close()
		return nil, err
	}

	if err := d.ScreenOn(); err != nil {
		d.Close()
		return nil, err
	}

	return d, nil
}

func (d *Display) openSerial() error {
	mode := &serial.Mode{
		BaudRate: 115200,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(d.portName, mode)
	if err != nil {
		return fmt.Errorf("open serial port %s: %w", d.portName, err)
	}

	if err := port.SetReadTimeout(time.Second); err != nil {
		port.Close()
		return fmt.Errorf("set read timeout: %w", err)
	}

	d.port = port
	return nil
}

// hello sends the HELLO command to initialize communication.
func (d *Display) hello() error {
	// Send 6 bytes of 0x45 (HELLO command)
	hello := []byte{0x45, 0x45, 0x45, 0x45, 0x45, 0x45}
	_, err := d.port.Write(hello)
	if err != nil {
		return fmt.Errorf("hello: %w", err)
	}
	// Read response (ignore it, just need to send hello)
	buf := make([]byte, 32)
	d.port.Read(buf)
	return nil
}

// Close closes the display connection.
func (d *Display) Close() error {
	if d.port != nil {
		d.ScreenOff()
		return d.port.Close()
	}
	return nil
}

// Width returns the display width (after orientation).
func (d *Display) Width() int {
	if d.orientation == Landscape || d.orientation == ReverseLandscape {
		return d.height // Swapped for landscape
	}
	return d.width
}

// Height returns the display height (after orientation).
func (d *Display) Height() int {
	if d.orientation == Landscape || d.orientation == ReverseLandscape {
		return d.width // Swapped for landscape
	}
	return d.height
}

// sendCommand sends a command using Rev A 6-byte packed format.
// Format: x, y, ex, ey packed into 5 bytes + command byte
func (d *Display) sendCommand(cmd byte, x, y, ex, ey int) error {
	buf := make([]byte, 6)
	buf[0] = byte(x >> 2)
	buf[1] = byte(((x & 3) << 6) + (y >> 4))
	buf[2] = byte(((y & 15) << 4) + (ex >> 6))
	buf[3] = byte(((ex & 63) << 2) + (ey >> 8))
	buf[4] = byte(ey & 255)
	buf[5] = cmd

	_, err := d.port.Write(buf)
	return err
}

// Reset resets the display.
func (d *Display) Reset() error {
	if err := d.sendCommand(cmdReset, 0, 0, 0, 0); err != nil {
		return fmt.Errorf("reset: %w", err)
	}
	
	// Close serial and wait for display to reset
	if d.port != nil {
		d.port.Close()
		d.port = nil
	}
	time.Sleep(5 * time.Second)
	
	// Reopen serial
	return d.openSerial()
}

// Clear clears the display to black.
func (d *Display) Clear() error {
	return d.sendCommand(cmdClear, 0, 0, 0, 0)
}

// ScreenOn turns on the display.
func (d *Display) ScreenOn() error {
	return d.sendCommand(cmdScreenOn, 0, 0, 0, 0)
}

// ScreenOff turns off the display.
func (d *Display) ScreenOff() error {
	return d.sendCommand(cmdScreenOff, 0, 0, 0, 0)
}

// SetBrightness sets the display brightness (0-100).
func (d *Display) SetBrightness(level int) error {
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	// Display uses inverted scale: 0 = brightest, 255 = darkest
	levelAbsolute := 255 - ((level * 255) / 100)
	return d.sendCommand(cmdSetBrightness, levelAbsolute, 0, 0, 0)
}

// SetOrientation sets the display orientation.
func (d *Display) SetOrientation(o Orientation) error {
	d.orientation = o
	// Orientation is encoded in x position
	return d.sendCommand(cmdSetOrientation, int(o), 0, 0, 0)
}

// DrawImage draws an image at the specified position.
func (d *Display) DrawImage(img image.Image, x, y int) error {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	if w == 0 || h == 0 {
		return nil
	}

	// Send command header with coordinates
	ex := x + w - 1
	ey := y + h - 1
	if err := d.sendCommand(cmdDisplayBitmap, x, y, ex, ey); err != nil {
		return fmt.Errorf("send bitmap header: %w", err)
	}

	// Convert to RGB565 little-endian format
	pixels := make([]byte, w*h*2)
	idx := 0
	for py := bounds.Min.Y; py < bounds.Max.Y; py++ {
		for px := bounds.Min.X; px < bounds.Max.X; px++ {
			r, g, b, _ := img.At(px, py).RGBA()
			// RGB565: 5 bits R, 6 bits G, 5 bits B (little-endian)
			r5 := (r >> 11) & 0x1F
			g6 := (g >> 10) & 0x3F
			b5 := (b >> 11) & 0x1F
			rgb565 := (r5 << 11) | (g6 << 5) | b5
			// Little-endian
			pixels[idx] = byte(rgb565 & 0xFF)
			pixels[idx+1] = byte(rgb565 >> 8)
			idx += 2
		}
	}

	// Send pixel data
	if _, err := d.port.Write(pixels); err != nil {
		return fmt.Errorf("write pixels: %w", err)
	}

	return nil
}

// Simulated display for testing without hardware.

// SimulatedDisplay is a display that does nothing (for testing).
type SimulatedDisplay struct {
	width       int
	height      int
	orientation Orientation
}

// NewSimulated creates a simulated display.
func NewSimulated(width, height int) *SimulatedDisplay {
	return &SimulatedDisplay{
		width:       width,
		height:      height,
		orientation: ReverseLandscape,
	}
}

func (d *SimulatedDisplay) Close() error { return nil }

func (d *SimulatedDisplay) Width() int {
	if d.orientation == Landscape || d.orientation == ReverseLandscape {
		return d.height
	}
	return d.width
}

func (d *SimulatedDisplay) Height() int {
	if d.orientation == Landscape || d.orientation == ReverseLandscape {
		return d.width
	}
	return d.height
}

func (d *SimulatedDisplay) DrawImage(img image.Image, x, y int) error { return nil }

// Screen interface for both real and simulated displays.
type Screen interface {
	io.Closer
	Width() int
	Height() int
	DrawImage(img image.Image, x, y int) error
}

// Ensure both types implement Screen.
var _ Screen = (*Display)(nil)
var _ Screen = (*SimulatedDisplay)(nil)
