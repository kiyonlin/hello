package util

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"sort"
	"strings"
	"time"
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

////	method: GET, POST, DELETE
//func HttpRequestBybit(method string, reqUrl string, body string, requestHeaders map[string]string, form url.Values) (
//	[]byte, error) {
//	req, _ := http.NewRequest(method, reqUrl, strings.NewReader(body))
//	buf := &bytes.Buffer{}
//	w := multipart.NewWriter(buf)
//	for k, v := range form {
//		for _, iv := range v {
//			_ := w.WriteField(k, iv)
//		}
//	}
//	_ = w.Close()
//	if requestHeaders != nil {
//		for k, v := range requestHeaders {
//			req.Header.Add(k, v)
//		}
//	}
//	resp, err := http.DefaultClient.Do(req)
//	if err != nil {
//		SocketInfo("can not process request " + err.Error())
//		return nil, err
//	}
//	defer resp.Body.Close()
//
//	bodyData, err := ioutil.ReadAll(resp.Body)
//	if err != nil {
//		SocketInfo("can not read message from request " + err.Error())
//		return nil, err
//	}
//	if resp.StatusCode != 200 {
//		SocketInfo(fmt.Sprintf("%sHttpStatusCode:%d ,Desc:%s", reqUrl, resp.StatusCode, string(bodyData)))
//	}
//	return bodyData, nil
//}

//	method: GET, POST, DELETE
func HttpRequest(method string, reqUrl string, body string, requestHeaders map[string]string, timeout int) ([]byte, error) {
	req, _ := http.NewRequest(method, reqUrl, strings.NewReader(body))
	//buf := &bytes.Buffer{}
	//w := multipart.NewWriter(buf)
	//for k, v := range requestHeaders {
	//	for _, iv := range v {
	//		w.WriteField(k, iv)
	//	}
	//}
	//w.Close()
	//req.MultipartForm =
	if requestHeaders != nil {
		for k, v := range requestHeaders {
			req.Header.Add(k, v)
		}
	}
	ctx, cncl := context.WithTimeout(context.Background(), time.Second*time.Duration(timeout))
	defer cncl()
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
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

func SendMail(toAddress, subject, body string) (err error) {
	fromMail := "94764906@qq.com"
	from := mail.Address{Address: fromMail}
	to := mail.Address{Address: toAddress}
	headers := make(map[string]string)
	headers["From"] = from.String()
	headers["To"] = to.String()
	headers["Subject"] = subject
	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body
	servername := "smtp.qq.com:465"
	host, _, _ := net.SplitHostPort(servername)
	auth := smtp.PlainAuth("", fromMail, "urszfnsnanxebjga", host)
	tlsconfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}
	conn, err := tls.Dial("tcp", servername, tlsconfig)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	// Auth
	if err = c.Auth(auth); err != nil {
		return err
	}
	// To && From
	if err = c.Mail(from.Address); err != nil {
		return err
	}
	if err = c.Rcpt(to.Address); err != nil {
		return err
	}
	// Data
	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(message))
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	_ = c.Quit()
	SocketInfo(fmt.Sprintf(`%s to %s %s %s`,
		from.String(), to.String(), subject, message))
	return err
}
