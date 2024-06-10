package http

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/hashicorp/go-retryablehttp"
)

func PostJSONWithRetry(url string, dataStruct interface{}) error {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3

	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(dataStruct); err != nil {
		return err
	}
	r, err := retryClient.Post(url, "application/json", buf)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	_, err = io.ReadAll(r.Body)
	return err
}

func GetJSONWithRetry(url string, dataStruct interface{}) error {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3

	resp, err := retryablehttp.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(&dataStruct)
}
