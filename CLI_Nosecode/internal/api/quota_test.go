package api

import (
	"context"
	"math"
	"testing"
)

func TestParseVaultQuota(t *testing.T) {
	out := "GPU: usadas 3.2 h de 30.0 h; quedan 26.8 h\n" +
		"QUOTA_GPU_USED_H=3.17\n" +
		"QUOTA_GPU_TOTAL_H=30.00\n" +
		"QUOTA_GPU_RESERVED_H=0.00\n" +
		"QUOTA_GPU_REMAINING_H=26.83\n" +
		"QUOTA_REFRESH=2026-11-07T00:00:00\n"
	q, err := ParseVaultQuota(out)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(q.UsedHours-3.17) > 0.001 || q.TotalHours != 30 ||
		math.Abs(q.RemainingHours-26.83) > 0.001 || q.Refresh != "2026-11-07T00:00:00" {
		t.Fatalf("cuota = %+v", q)
	}
}

func TestParseVaultQuotaSinDatos(t *testing.T) {
	if _, err := ParseVaultQuota("GPU: no disponible\n"); err == nil {
		t.Fatal("debia fallar sin marcadores QUOTA_*")
	}
}

func TestRunVaultQuota(t *testing.T) {
	py, script := fakeVault(t, "print('QUOTA_GPU_USED_H=1.00', flush=True)\n"+
		"print('QUOTA_GPU_TOTAL_H=30.00', flush=True)\n"+
		"print('QUOTA_GPU_RESERVED_H=0.00', flush=True)\n"+
		"print('QUOTA_GPU_REMAINING_H=29.00', flush=True)\n")
	q, err := RunVaultQuota(context.Background(), py, script)
	if err != nil || q.RemainingHours != 29 || q.TotalHours != 30 {
		t.Fatalf("cuota = %+v err = %v", q, err)
	}
}
