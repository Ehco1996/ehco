package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"syscall"
)

func ShouldRetry(err error) bool {
	if err == nil {
		// 没有错误，无需重试
		return false
	}

	// 如果错误实现了 net.Error 接口，我们可以进一步检查
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	// 对于某些特定的系统调用错误，也可以考虑重试
	var sysErr syscall.Errno
	if errors.As(err, &sysErr) {
		switch sysErr {
		case syscall.ECONNRESET, syscall.ECONNABORTED:
			// 例：连接被重置或中止
			return true
		}
	}
	return false
}

func PostJson(c *http.Client, url string, dataStruct interface{}) error {
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(dataStruct); err != nil {
		return err
	}
	r, err := http.Post(url, "application/json", buf)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	_, err = io.ReadAll(r.Body)
	return err
}
