package transporter

import (
	"fmt"

	"github.com/Ehco1996/ehco/pkg/bytes"
)

type Stats struct {
	up   int64
	down int64
}

func (s *Stats) String() string {
	return fmt.Sprintf("up: %s, down: %s", bytes.PrettyByteSize(float64(s.up)), bytes.PrettyByteSize(float64(s.down)))
}

func (s *Stats) Record(up, down int64) {
	s.up += up
	s.down += down
}
