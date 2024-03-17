package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

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
