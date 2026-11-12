package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// Línea legible que imprime vault.py quota en versiones antiguas:
	// «GPU: usadas 0.8 h de 30.0 h (reservadas por sesiones activas: 0.0 h)».
	humanQuota    = regexp.MustCompile(`(?i)GPU:\s*usadas\s+([0-9]+(?:[.,][0-9]+)?)\s*h\s+de\s+([0-9]+(?:[.,][0-9]+)?)\s*h`)
	humanReserved = regexp.MustCompile(`(?i)reservad[oa]s?[^:]*:\s*([0-9]+(?:[.,][0-9]+)?)\s*h`)
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

// ParseVaultQuota lee la cuota de `vault.py quota`. Acepta las lineas
// maquina-legibles QUOTA_* y la linea legible «GPU: usadas X h de Y h».
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
		m := humanQuota.FindStringSubmatch(out)
		if len(m) != 3 {
			return VaultQuota{}, errors.New("vault.py quota no devolvio la cuota de GPU")
		}
		used, err := parseQuotaFloat(m[1])
		if err != nil {
			return VaultQuota{}, err
		}
		total, err := parseQuotaFloat(m[2])
		if err != nil {
			return VaultQuota{}, err
		}
		q.UsedHours, q.TotalHours = used, total
		if r := humanReserved.FindStringSubmatch(out); len(r) == 2 {
			q.ReservedHours, _ = parseQuotaFloat(r[1])
		}
	}
	if q.RemainingHours == 0 && q.TotalHours > q.UsedHours {
		q.RemainingHours = q.TotalHours - q.UsedHours
	}
	if q.RemainingHours < 0 {
		q.RemainingHours = 0
	}
	return q, nil
}

func parseQuotaFloat(s string) (float64, error) {
	n, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", "."), 64)
	if err != nil {
		return 0, fmt.Errorf("cuota invalida %q", s)
	}
	return n, nil
}
