package bytes

import (
	"fmt"
	"math"
)

func PrettyByteSize(bf float64) string {
	for _, unit := range []string{"", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi"} {
		if math.Abs(bf) < 1024.0 {
			return fmt.Sprintf(" %3.1f%sB ", bf, unit)
		}
		bf /= 1024.0
	}
	return fmt.Sprintf(" %.1fYiB ", bf)
}

func PrettyBitRate(bps float64) string {
	for _, unit := range []string{"bps", "Kbps", "Mbps", "Gbps", "Tbps", "Pbps", "Ebps", "Zbps"} {
		if math.Abs(bps) < 1000.0 {
			return fmt.Sprintf(" %3.1f %s ", bps, unit)
		}
		bps /= 1000.0
	}
	return fmt.Sprintf(" %.1f Ybps ", bps)
}
