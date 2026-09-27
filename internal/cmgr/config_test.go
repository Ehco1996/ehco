package cmgr

import "testing"

func TestNeedsStart(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			// The regression this method exists for: a web panel without relay
			// sync still has host metrics to export.
			name: "metrics only",
			cfg:  Config{EnableMetrics: true, SyncInterval: 60},
			want: true,
		},
		{
			name: "sync only",
			cfg:  Config{SyncURL: "http://upstream.test/config", SyncInterval: 60},
			want: true,
		},
		{
			name: "both",
			cfg: Config{
				SyncURL:       "http://upstream.test/config",
				SyncInterval:  60,
				EnableMetrics: true,
			},
			want: true,
		},
		{
			name: "neither",
			cfg:  Config{SyncInterval: 60},
			want: false,
		},
		{
			// Metrics requested but no tick cadence: Adjust() normally coerces
			// SyncInterval, this pins the pre-Adjust behaviour.
			name: "metrics requested without interval",
			cfg:  Config{EnableMetrics: true},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.NeedsStart(); got != tt.want {
				t.Fatalf("NeedsStart() = %v, want %v", got, tt.want)
			}
		})
	}
}
