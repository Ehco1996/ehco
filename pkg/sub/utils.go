package sub

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

var client = http.Client{Timeout: time.Second * 10}

func getHttpBody(url string) ([]byte, error) {
	resp, err := client.Get(url)
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

func longestCommonPrefix(s string, t string) string {
	runeS := []rune(s)
	runeT := []rune(t)

	i := 0
	for i = 0; i < len(runeS) && i < len(runeT); i++ {
		if runeS[i] != runeT[i] {
			return string(runeS[:i])
		}
	}
	return string(runeS[:i])
}

func groupByLongestCommonPrefix(strList []string) map[string][]string {
	sort.Strings(strList)

	grouped := make(map[string][]string)

	// 先找出有相同前缀的字符串
	for i := 0; i < len(strList); i++ {
		for j := i + 1; j < len(strList); j++ {
			prefix := longestCommonPrefix(strList[i], strList[j])
			if prefix == "" {
				continue
			}
			if _, ok := grouped[prefix]; !ok {
				grouped[prefix] = []string{}
			}
		}
	}

	// 过滤掉有相同前缀的前缀中较短的
	for prefix := range grouped {
		for otherPrefix := range grouped {
			if prefix == otherPrefix {
				continue
			}
			if strings.HasPrefix(otherPrefix, prefix) {
				delete(grouped, prefix)
			}
		}
	}

	// 将字符串分组
	for _, proxy := range strList {
		foundPrefix := false
		for prefix := range grouped {
			if strings.HasPrefix(proxy, prefix) {
				grouped[prefix] = append(grouped[prefix], proxy)
				foundPrefix = true
				break
			}
		}
		// 处理没有相同前缀的字符串,自己是一个分组
		if !foundPrefix {
			grouped[proxy] = []string{proxy}
		}
	}
	return grouped
}

func strInArray(ele string, array []string) bool {
	for _, v := range array {
		if v == ele {
			return true
		}
	}
	return false
}
