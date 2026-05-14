package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/aleksclark/go-turing-smart-screen/internal/lcd"
	"github.com/aleksclark/go-turing-smart-screen/internal/monitor"
)

func main() {
	var (
		cpuPort    = flag.String("cpu-port", "", "Serial port for CPU display")
		ramPort    = flag.String("ram-port", "", "Serial port for RAM display")
		agentPort  = flag.String("agent-port", "", "Serial port for Agent display")
		netPort    = flag.String("net-port", "", "Serial port for Fleet Health display")
		brightness = flag.Int("brightness", 50, "Display brightness (0-100)")
		simulated  = flag.Bool("simulated", false, "Use simulated displays (no hardware)")
		debug      = flag.Bool("debug", false, "Enable debug logging")

		cpuInterval   = flag.Duration("cpu-interval", 1*time.Second, "CPU monitor refresh interval")
		ramInterval   = flag.Duration("ram-interval", 2*time.Second, "RAM monitor refresh interval")
		agentInterval = flag.Duration("agent-interval", 2*time.Second, "Agent monitor refresh interval")
		netInterval   = flag.Duration("net-interval", 5*time.Second, "Fleet health refresh interval")

		noCPU   = flag.Bool("no-cpu", false, "Disable CPU monitor")
		noRAM   = flag.Bool("no-ram", false, "Disable RAM monitor")
		noAgent = flag.Bool("no-agent", false, "Disable Agent monitor")
		noNet   = flag.Bool("no-net", false, "Disable Fleet Health monitor")
	)
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	type monitorEntry struct {
		name   string
		port   string
		create func(lcd.Screen, int, time.Duration, *slog.Logger) monitor.Monitor
		intv   time.Duration
		skip   bool
	}

	entries := []monitorEntry{
		{"CPU", *cpuPort, func(s lcd.Screen, b int, i time.Duration, l *slog.Logger) monitor.Monitor {
			return monitor.NewCPUMonitor(s, b, i, l)
		}, *cpuInterval, *noCPU},
		{"RAM", *ramPort, func(s lcd.Screen, b int, i time.Duration, l *slog.Logger) monitor.Monitor {
			return monitor.NewRAMMonitor(s, b, i, l)
		}, *ramInterval, *noRAM},
		{"Agent", *agentPort, func(s lcd.Screen, b int, i time.Duration, l *slog.Logger) monitor.Monitor {
			return monitor.NewAgentMonitor(s, b, i, l)
		}, *agentInterval, *noAgent},
		{"Fleet", *netPort, func(s lcd.Screen, b int, i time.Duration, l *slog.Logger) monitor.Monitor {
			return monitor.NewNetMonitor(s, b, i, l)
		}, *netInterval, *noNet},
	}

	var monitors []monitor.Monitor
	for _, e := range entries {
		if e.skip {
			continue
		}
		if e.port == "" && !*simulated {
			continue
		}

		var screen lcd.Screen
		if *simulated {
			screen = lcd.NewSimulated(480, 320)
		} else {
			cfg := lcd.DefaultConfig()
			cfg.Port = e.port
			cfg.Brightness = *brightness
			d, err := lcd.New(cfg)
			if err != nil {
				logger.Error("failed to open display", "monitor", e.name, "port", e.port, "error", err)
				continue
			}
			screen = d
		}

		m := e.create(screen, *brightness, e.intv, logger)
		monitors = append(monitors, m)
	}

	if len(monitors) == 0 {
		fmt.Fprintln(os.Stderr, "No monitors configured. Use --simulated or specify ports.")
		os.Exit(1)
	}

	// Start all monitors
	var wg sync.WaitGroup
	for _, m := range monitors {
		wg.Add(1)
		go func(mon monitor.Monitor) {
			defer wg.Done()
			if err := mon.Run(); err != nil {
				logger.Error("monitor failed", "name", mon.Name(), "error", err)
			}
		}(m)
	}

	logger.Info("all monitors started", "count", len(monitors))

	// Wait for signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	logger.Info("shutting down...")
	for _, m := range monitors {
		m.Stop()
	}
	wg.Wait()
}
