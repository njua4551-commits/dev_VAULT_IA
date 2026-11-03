package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// VaultQuota es la cuota semanal de GPU que reporta Kaggle.
type VaultQuota struct {
	UsedHours      float64
	TotalHours     float64
	ReservedHours  float64
	RemainingHours float64
	Refresh        string
}

// RunVaultQuota ejecuta `vault.py quota` y devuelve la cuota de GPU.
func RunVaultQuota(parent context.Context, python, script string) (VaultQuota, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, python, "-u", script, "quota")
	cmd.Dir = filepath.Dir(filepath.Dir(script))
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
	out, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return VaultQuota{}, errors.New("tiempo agotado consultando la cuota de Kaggle")
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 220 {
			msg = msg[:220] + "…"
		}
		return VaultQuota{}, fmt.Errorf("vault.py quota: %v: %s", err, msg)
	}
	return ParseVaultQuota(string(out))
}

// ParseVaultQuota lee las lineas QUOTA_* que imprime vault.py quota.
func ParseVaultQuota(out string) (VaultQuota, error) {
	var q VaultQuota
	var seen bool
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		parse := func() (float64, error) {
			n, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return 0, fmt.Errorf("cuota invalida %s=%q", key, val)
			}
			return n, nil
		}

		var err error
		switch key {
		case "QUOTA_GPU_USED_H":
			q.UsedHours, err = parse()
			seen = true
		case "QUOTA_GPU_TOTAL_H":
			q.TotalHours, err = parse()
			seen = true
		case "QUOTA_GPU_RESERVED_H":
			q.ReservedHours, err = parse()
			seen = true
		case "QUOTA_GPU_REMAINING_H":
			q.RemainingHours, err = parse()
			seen = true
		case "QUOTA_REFRESH":
			q.Refresh = val
		}
		if err != nil {
			return VaultQuota{}, err
		}
	}
	if !seen {
		return VaultQuota{}, errors.New("vault.py quota no devolvio la cuota de GPU")
	}
	return q, nil
}
