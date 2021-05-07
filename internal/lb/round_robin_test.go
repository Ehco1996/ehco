package lb

import (
	"testing"
)

func Test_roundrobin_Next(t *testing.T) {
	remotes := []string{
		"127.0.0.1",
		"127.0.0.2",
		"127.0.0.3",
	}
	rb := NewRBRemotes(remotes)
	for i := 0; i < len(remotes); i++ {
		if res := rb.Next(); res != remotes[i] {
			t.Fatalf("need %s got %s", remotes[i], res)
		}
	}
}
