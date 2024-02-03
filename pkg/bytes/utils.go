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
