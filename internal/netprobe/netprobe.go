// Package netprobe provides network health probing: ICMP ping, DNS resolution, and interface stats.
package netprobe

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"

	psnet "github.com/shirou/gopsutil/v3/net"
)

// PingResult holds the result of pinging a single target.
type PingResult struct {
	Target  string
	Label   string
	Latency time.Duration
	Loss    float64 // 0.0 to 1.0
	Alive   bool
}

// DNSResult holds the result of a DNS resolution test.
type DNSResult struct {
	Domain  string
	Latency time.Duration
	Error   string
}

// InterfaceStats holds throughput stats for a network interface.
type InterfaceStats struct {
	Name      string
	BytesIn   uint64
	BytesOut  uint64
	PktsIn    uint64
	PktsOut   uint64
	RateIn    float64 // bytes/sec
	RateOut   float64 // bytes/sec
	PktRateIn float64 // packets/sec
	PktRateOut float64
}

// Prober runs periodic network health checks.
type Prober struct {
	mu sync.RWMutex

	pingTargets []pingTarget
	dnsServers  []string
	dnsDomains  []string
	iface       string

	pingResults map[string]*pingRing
	dnsResults  []DNSResult
	ifaceStats  InterfaceStats

	prevCounters *psnet.IOCountersStat
	prevTime     time.Time

	seq int
}

type pingTarget struct {
	addr  string
	label string
}

// New creates a new Prober. It auto-detects the default gateway, DNS servers, and primary interface.
func New() *Prober {
	p := &Prober{
		dnsServers:  detectDNSServers(),
		dnsDomains:  []string{"google.com", "cloudflare.com", "github.com"},
		iface:       detectPrimaryInterface(),
		pingResults: make(map[string]*pingRing),
	}

	gw := detectDefaultGateway()

	var targets []pingTarget
	if gw != "" {
		targets = append(targets, pingTarget{gw, "Gateway"})
	}

	for _, dns := range p.dnsServers {
		targets = append(targets, pingTarget{dns, "DNS " + dns})
	}

	targets = append(targets,
		pingTarget{"1.1.1.1", "Cloudflare"},
		pingTarget{"8.8.8.8", "Google"},
		pingTarget{"208.67.222.222", "OpenDNS"},
		pingTarget{"9.9.9.9", "Quad9"},
	)

	seen := make(map[string]bool)
	for _, t := range targets {
		if !seen[t.addr] {
			seen[t.addr] = true
			p.pingTargets = append(p.pingTargets, t)
			p.pingResults[t.addr] = newPingRing(30)
		}
	}

	return p
}

// Probe runs one round of all probes.
func (p *Prober) Probe(ctx context.Context) {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		p.probePing(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		p.probeDNS(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		p.probeInterface()
	}()

	wg.Wait()
}

// PingResults returns the current ping results for all targets.
func (p *Prober) PingResults() []PingResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var results []PingResult
	for _, t := range p.pingTargets {
		ring := p.pingResults[t.addr]
		results = append(results, PingResult{
			Target:  t.addr,
			Label:   t.label,
			Latency: ring.avgLatency(),
			Loss:    ring.lossRate(),
			Alive:   ring.lastAlive(),
		})
	}
	return results
}

// DNSResults returns the current DNS resolution results.
func (p *Prober) DNSResults() []DNSResult {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]DNSResult, len(p.dnsResults))
	copy(out, p.dnsResults)
	return out
}

// IfaceStats returns the current interface throughput stats.
func (p *Prober) IfaceStats() InterfaceStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ifaceStats
}

// probePing sends ICMP echo to all targets.
func (p *Prober) probePing(ctx context.Context) {
	conn, err := icmp.ListenPacket("udp4", "")
	if err != nil {
		for _, t := range p.pingTargets {
			p.mu.Lock()
			p.pingResults[t.addr].add(0, false)
			p.mu.Unlock()
		}
		return
	}
	defer conn.Close()

	p.seq++

	for i, t := range p.pingTargets {
		if ctx.Err() != nil {
			return
		}

		dst, err := net.ResolveUDPAddr("udp4", t.addr+":0")
		if err != nil {
			p.mu.Lock()
			p.pingResults[t.addr].add(0, false)
			p.mu.Unlock()
			continue
		}

		msg := &icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				ID:   os.Getpid() & 0xffff,
				Seq:  p.seq*100 + i,
				Data: []byte("probe"),
			},
		}
		wb, _ := msg.Marshal(nil)

		start := time.Now()
		conn.SetDeadline(time.Now().Add(2 * time.Second))

		if _, err := conn.WriteTo(wb, dst); err != nil {
			p.mu.Lock()
			p.pingResults[t.addr].add(0, false)
			p.mu.Unlock()
			continue
		}

		rb := make([]byte, 1500)
		n, _, err := conn.ReadFrom(rb)
		latency := time.Since(start)

		if err != nil {
			p.mu.Lock()
			p.pingResults[t.addr].add(0, false)
			p.mu.Unlock()
			continue
		}

		rm, err := icmp.ParseMessage(1, rb[:n])
		if err != nil || rm.Type != ipv4.ICMPTypeEchoReply {
			p.mu.Lock()
			p.pingResults[t.addr].add(0, false)
			p.mu.Unlock()
			continue
		}

		p.mu.Lock()
		p.pingResults[t.addr].add(latency, true)
		p.mu.Unlock()
	}
}

// probeDNS tests DNS resolution for several domains, bypassing local cache
// by querying 1.1.1.1 directly.
func (p *Prober) probeDNS(ctx context.Context) {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "udp", "1.1.1.1:53")
		},
	}
	var results []DNSResult

	for _, domain := range p.dnsDomains {
		start := time.Now()
		_, err := resolver.LookupHost(ctx, domain)
		latency := time.Since(start)

		result := DNSResult{
			Domain:  domain,
			Latency: latency,
		}
		if err != nil {
			result.Error = err.Error()
			if len(result.Error) > 40 {
				result.Error = result.Error[:40]
			}
		}
		results = append(results, result)
	}

	p.mu.Lock()
	p.dnsResults = results
	p.mu.Unlock()
}

// probeInterface reads network interface counters and computes rates.
func (p *Prober) probeInterface() {
	counters, err := psnet.IOCounters(true)
	if err != nil {
		return
	}

	now := time.Now()

	var current *psnet.IOCountersStat
	for i := range counters {
		if counters[i].Name == p.iface {
			current = &counters[i]
			break
		}
	}

	if current == nil {
		// Try aggregate
		agg, err := psnet.IOCounters(false)
		if err == nil && len(agg) > 0 {
			current = &agg[0]
		}
	}

	if current == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	stats := InterfaceStats{
		Name:    current.Name,
		BytesIn: current.BytesRecv,
		BytesOut: current.BytesSent,
		PktsIn:  current.PacketsRecv,
		PktsOut: current.PacketsSent,
	}

	if p.prevCounters != nil {
		elapsed := now.Sub(p.prevTime).Seconds()
		if elapsed > 0 {
			stats.RateIn = float64(current.BytesRecv-p.prevCounters.BytesRecv) / elapsed
			stats.RateOut = float64(current.BytesSent-p.prevCounters.BytesSent) / elapsed
			stats.PktRateIn = float64(current.PacketsRecv-p.prevCounters.PacketsRecv) / elapsed
			stats.PktRateOut = float64(current.PacketsSent-p.prevCounters.PacketsSent) / elapsed
		}
	}

	p.ifaceStats = stats
	p.prevCounters = current
	p.prevTime = now
}

// pingRing is a circular buffer of ping results for computing rolling averages.
type pingRing struct {
	latencies []time.Duration
	alive     []bool
	pos       int
	size      int
	count     int
}

func newPingRing(size int) *pingRing {
	return &pingRing{
		latencies: make([]time.Duration, size),
		alive:     make([]bool, size),
		size:      size,
	}
}

func (r *pingRing) add(latency time.Duration, alive bool) {
	r.latencies[r.pos] = latency
	r.alive[r.pos] = alive
	r.pos = (r.pos + 1) % r.size
	if r.count < r.size {
		r.count++
	}
}

func (r *pingRing) avgLatency() time.Duration {
	if r.count == 0 {
		return 0
	}
	var sum time.Duration
	var n int
	for i := 0; i < r.count; i++ {
		if r.alive[i] {
			sum += r.latencies[i]
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / time.Duration(n)
}

func (r *pingRing) lossRate() float64 {
	if r.count == 0 {
		return 1.0
	}
	var lost int
	for i := 0; i < r.count; i++ {
		if !r.alive[i] {
			lost++
		}
	}
	return float64(lost) / float64(r.count)
}

func (r *pingRing) lastAlive() bool {
	if r.count == 0 {
		return false
	}
	idx := (r.pos - 1 + r.size) % r.size
	return r.alive[idx]
}

// detectDefaultGateway reads the default gateway from /proc/net/route.
func detectDefaultGateway() string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		// Default route: destination is 00000000
		if fields[1] == "00000000" {
			gw := fields[2]
			return parseHexIP(gw)
		}
	}
	return ""
}

// detectDNSServers reads DNS servers from /etc/resolv.conf.
func detectDNSServers() []string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return []string{"8.8.8.8", "1.1.1.1"}
	}
	var servers []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				ip := fields[1]
				if ip != "127.0.0.53" && ip != "127.0.0.1" && ip != "::1" {
					servers = append(servers, ip)
				}
			}
		}
	}
	if len(servers) == 0 {
		return []string{"8.8.8.8", "1.1.1.1"}
	}
	if len(servers) > 2 {
		servers = servers[:2]
	}
	return servers
}

// detectPrimaryInterface finds the interface used for the default route.
func detectPrimaryInterface() string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return "eth0"
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if fields[1] == "00000000" {
			return fields[0]
		}
	}
	return "eth0"
}

// parseHexIP converts a hex-encoded little-endian IPv4 address.
func parseHexIP(hex string) string {
	if len(hex) != 8 {
		return ""
	}
	var octets [4]byte
	for i := 0; i < 4; i++ {
		b := hexByte(hex[i*2], hex[i*2+1])
		octets[i] = b
	}
	// Linux /proc/net/route uses little-endian on little-endian systems
	return fmt.Sprintf("%d.%d.%d.%d", octets[3], octets[2], octets[1], octets[0])
}

func hexByte(hi, lo byte) byte {
	return hexVal(hi)<<4 | hexVal(lo)
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// FormatRate formats bytes/sec to human-readable throughput.
func FormatRate(bytesPerSec float64) string {
	switch {
	case bytesPerSec >= 1e9:
		return fmt.Sprintf("%.1f GB/s", bytesPerSec/1e9)
	case bytesPerSec >= 1e6:
		return fmt.Sprintf("%.1f MB/s", bytesPerSec/1e6)
	case bytesPerSec >= 1e3:
		return fmt.Sprintf("%.1f KB/s", bytesPerSec/1e3)
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
}

// FormatPktRate formats packets/sec.
func FormatPktRate(pktsPerSec float64) string {
	if pktsPerSec >= 1000 {
		return fmt.Sprintf("%.1fk/s", pktsPerSec/1000)
	}
	return fmt.Sprintf("%.0f/s", pktsPerSec)
}

// FormatLatency formats a duration for display.
func FormatLatency(d time.Duration) string {
	ms := float64(d.Microseconds()) / 1000.0
	if ms < 1 {
		return fmt.Sprintf("%.1fms", ms)
	}
	if ms < 100 {
		return fmt.Sprintf("%.1fms", ms)
	}
	return fmt.Sprintf("%.0fms", ms)
}

// FormatLoss formats loss percentage.
func FormatLoss(loss float64) string {
	pct := loss * 100
	if pct == 0 {
		return "0%"
	}
	if pct >= 100 {
		return "100%"
	}
	return fmt.Sprintf("%.0f%%", math.Ceil(pct))
}
