package xray

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
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

func PrettyByteSize(bf float64) string {
	for _, unit := range []string{"", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi"} {
		if math.Abs(bf) < 1024.0 {
			return fmt.Sprintf(" %3.1f%sB ", bf, unit)
		}
		bf /= 1024.0
	}
	return fmt.Sprintf(" %.1fYiB ", bf)
}
