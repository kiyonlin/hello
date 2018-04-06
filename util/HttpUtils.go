package util

import (
	"strings"
	"io/ioutil"
	"fmt"
	"net/http"
)

//	method: GET, POST, DELETE
func HttpRequest(method string, reqUrl string, postData string, requestHeaders map[string]string) ([]byte, error) {
	//return []byte("Testing"), nil
	req, _ := http.NewRequest(method, reqUrl, strings.NewReader(postData))
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
