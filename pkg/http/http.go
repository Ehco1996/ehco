package http

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/Ehco1996/ehco/pkg/log"
	"github.com/hashicorp/go-retryablehttp"
)

func generateRetirableClient() *retryablehttp.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.Logger = log.NewZapLeveledLogger("http")
	return retryClient
}

func PostJSONWithRetry(url string, dataStruct interface{}) error {
	retryClient := generateRetirableClient()

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
	if err != nil {
		return err
	}
	return err
}

func GetJSONWithRetry(url string, dataStruct interface{}) error {
	retryClient := generateRetirableClient()
	resp, err := retryClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(&dataStruct)
}
