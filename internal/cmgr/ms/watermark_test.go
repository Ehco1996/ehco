package ms

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvictUntilBelow_OldestFirst(t *testing.T) {
	dir := t.TempDir()
	for i, name := range []string{"old", "mid", "new"} {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(p, 0o755))
		mt := time.Now().Add(time.Duration(i) * time.Hour)
		require.NoError(t, os.Chtimes(p, mt, mt))
	}

	calls := 0
	prober := func(string) (float64, error) {
		calls++
		if calls <= 2 {
			return 80, nil
		}
		return 30, nil
	}

	ms := &MetricsStore{dir: dir}
	ms.evictUntilBelow(50, prober)

	_, err := os.Stat(filepath.Join(dir, "old"))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, "mid"))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, "new"))
	require.NoError(t, err)
}
