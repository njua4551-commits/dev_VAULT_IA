package ui

import "testing"

func TestFormatQuotaHours(t *testing.T) {
	cases := map[float64]string{
		26.834: "26.8",
		9.876:  "9.88",
		0.5:    "0.50",
	}
	for in, want := range cases {
		if got := formatQuotaHours(in); got != want {
			t.Fatalf("formatQuotaHours(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestOnQuotaConservaUltimaValida(t *testing.T) {
	m := Model{quota: quotaState{ok: true}}
	m.quota.q.RemainingHours = 12
	m = m.onQuota(quotaMsg{err: errQuotaTest{}})
	if !m.quota.ok || m.quota.q.RemainingHours != 12 || !m.quota.err {
		t.Fatalf("quota = %+v", m.quota)
	}
}

type errQuotaTest struct{}

func (errQuotaTest) Error() string { return "sin cuota" }
