package util

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
)

func ComposeParams(body map[string]interface{}) (params string) {
	keys := make([]string, 0, len(body))
	var buf strings.Builder
	for key := range body {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if buf.Len() > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(key)
		buf.WriteByte('=')
		buf.WriteString(body[key].(string))
	}
	return buf.String()
}

//	method: GET, POST, DELETE
func HttpRequest(method string, reqUrl string, body string, requestHeaders map[string]string) ([]byte, error) {
	//return []byte("Testing"), nil
	req, _ := http.NewRequest(method, reqUrl, strings.NewReader(body))
	if requestHeaders != nil {
		for k, v := range requestHeaders {
			req.Header.Add(k, v)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		SocketInfo("can not process request " + err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	bodyData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		SocketInfo("can not read message from request " + err.Error())
		return nil, err
	}
	if resp.StatusCode != 200 {
		SocketInfo(fmt.Sprintf("%sHttpStatusCode:%d ,Desc:%s", reqUrl, resp.StatusCode, string(bodyData)))
	}
	return bodyData, nil
}
