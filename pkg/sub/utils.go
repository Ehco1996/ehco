package sub

import (
	"fmt"
	"net"
)

func getFreePortInBatch(host string, count int) ([]int, error) {
	res := make([]int, 0, count)
	listenerList := make([]net.Listener, 0, count)
	for i := 0; i < count; i++ {
		listener, err := net.Listen("tcp", fmt.Sprintf("%s:0", host))
		if err != nil {
			return res, err
		}
		listenerList = append(listenerList, listener)
		address := listener.Addr().(*net.TCPAddr)
		res = append(res, address.Port)
	}
	for _, listener := range listenerList {
		_ = listener.Close()
	}
	return res, nil
}
