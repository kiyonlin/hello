package util

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/bitly/go-simplejson"
	"io/ioutil"
	"net/url"
	"strconv"
	"time"
)

func UnGzip(byte []byte) []byte {
	r, err := gzip.NewReader(bytes.NewBuffer(byte))
	defer r.Close()
	if err != nil {
		fmt.Println(err.Error())
		return nil
	}
	undatas, _ := ioutil.ReadAll(r)
	return undatas
}
func ToJson(params *url.Values) string {
	parammap := make(map[string]string)
	for k, v := range *params {
		parammap[k] = v[0]
	}
	jsonData, _ := json.Marshal(parammap)
	return string(jsonData)
}

func NewJSON(data []byte) (j *simplejson.Json, err error) {
	j, err = simplejson.NewJson(data)
	if err != nil {
		return nil, err
	}
	return j, nil
}
func JsonDecodeByte(bytes []byte) map[string]interface{} {
	jsonMap := make(map[string]interface{})
	_ = json.Unmarshal(bytes, &jsonMap)
	return jsonMap
}
func JsonEncodeMapToByte(stringMap map[string]interface{}) []byte {
	jsonBytes, err := json.Marshal(stringMap)
	if err != nil {
		return nil
	}
	return jsonBytes
}

func GetNow() time.Time {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err == nil {
		return time.Now().In(location)
	}
	return time.Now()
}

func GetNowUnixMillion() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func FormatNum(input float64, decimal int) (num float64, str string) {
	format := `%.` + strconv.Itoa(decimal) + `f`
	str = fmt.Sprintf(format, input)
	num, _ = strconv.ParseFloat(str, 64)
	return num, str
}

func StartMidNightTimer(f func()) {
	go func() {
		for {
			now := time.Now()
			next := now.Add(time.Hour * 24)
			next = time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, next.Location())
			t := time.NewTimer(next.Sub(now))
			<-t.C
			f()
		}
	}()
}
