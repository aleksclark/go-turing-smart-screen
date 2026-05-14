package main

import (
	"fmt"

	"github.com/aleksclark/go-turing-smart-screen/internal/config"
	"github.com/aleksclark/go-turing-smart-screen/internal/signoz"
)

func main() {
	cfg, _ := config.Load()
	c := signoz.New(cfg.SignozURL, cfg.SignozAPIKey)

	fmt.Println("--- FetchMooseFS ---")
	moose, err := c.FetchMooseFS()
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
	} else {
		fmt.Printf("  Total: %s\n", signoz.FormatBytes(moose.TotalSpace))
		fmt.Printf("  Avail: %s\n", signoz.FormatBytes(moose.AvailSpace))
		fmt.Printf("  Used%%: %.1f%%\n", moose.UsedPct)
	}

	fmt.Println("\n--- FetchNodeStats ---")
	stats, err := c.FetchNodeStats()
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
	} else {
		fmt.Println("  Memory %:")
		for h, v := range stats.MemoryPct {
			fmt.Printf("    %s: %.1f%%\n", h, v)
		}
		fmt.Println("  CPU Load:")
		for h, v := range stats.CPULoad {
			fmt.Printf("    %s: %.2f\n", h, v)
		}
	}

	fmt.Println("\n--- FetchAlerts ---")
	alerts, err := c.FetchAlerts()
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
	} else {
		fmt.Printf("  Firing: %d\n", len(alerts))
		for _, a := range alerts {
			fmt.Printf("    - %s [%s]\n", a.Name, a.Severity)
		}
	}
}
