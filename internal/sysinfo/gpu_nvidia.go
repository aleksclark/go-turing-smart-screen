package sysinfo

import (
	"os/exec"
	"strings"
)

func runNvidiaSmi() (string, error) {
	cmd := exec.Command("nvidia-smi",
		"--query-gpu=temperature.gpu,utilization.gpu,memory.used,memory.total,power.draw",
		"--format=csv,noheader,nounits")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
