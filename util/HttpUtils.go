package util

import (
	"strings"
	"io/ioutil"
	"fmt"
	"net/http"
	"errors"
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
		panic(err)
		return nil, err
	}
	defer resp.Body.Close()

	bodyData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
		return nil, err
	}

	if resp.StatusCode != 200 {
		SocketInfo(fmt.Sprintf("HttpStatusCode:%d ,Desc:%s", resp.StatusCode, string(bodyData)))
		return nil, errors.New(fmt.Sprintf("HttpStatusCode:%d ,Desc:%s", resp.StatusCode, string(bodyData)))
	}
	return bodyData, nil
}
