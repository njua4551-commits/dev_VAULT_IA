package api

import (
	"context"
	"os"
	"testing"
)

func TestLiveVaultQuota(t *testing.T) {
	if os.Getenv("NOSECODE_LIVE_QUOTA") == "" {
		t.Skip("define NOSECODE_LIVE_QUOTA=1")
	}
	script, err := FindScript("")
	if err != nil {
		t.Fatalf("vault.py: %v", err)
	}
	py, err := FindPython()
	if err != nil {
		t.Fatalf("python: %v", err)
	}
	q, err := RunVaultQuota(context.Background(), py, script)
	if err != nil {
		t.Fatalf("cuota: %v", err)
	}
	t.Logf("GPU usadas=%.2f total=%.2f reservadas=%.2f quedan=%.2f",
		q.UsedHours, q.TotalHours, q.ReservedHours, q.RemainingHours)
	if q.TotalHours <= 0 {
		t.Fatalf("cuota total invalida: %+v", q)
	}
}
