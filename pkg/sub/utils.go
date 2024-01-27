package sub

import (
	"fmt"
	"io"
	"net"
	"net/http"
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

func getHttpBody(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		msg := fmt.Sprintf("http get sub config url=%s meet err=%v", url, err)
		return nil, fmt.Errorf(msg)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("http get sub config url=%s meet status code=%d", url, resp.StatusCode)
		return nil, fmt.Errorf(msg)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		msg := fmt.Sprintf("read body meet err=%v", err)
		return nil, fmt.Errorf(msg)
	}
	return body, nil
}
