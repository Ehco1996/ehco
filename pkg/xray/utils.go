package xray

import (
	"bytes"
	"encoding/json"
	"net/http"
)

func getJson(c *http.Client, url string, target interface{}) error {
	r, err := c.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

func postJson(c *http.Client, url string, dataStruct interface{}) error {
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(dataStruct); err != nil {
		return err
	}
	r, err := http.Post(url, "application/json", buf)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	return err
}
