package util

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"github.com/bitly/go-simplejson"
	"io/ioutil"
	"time"
	"net/url"
	"math"
	"fmt"
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
	json.Unmarshal(bytes, &jsonMap)
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
	return GetNow().Unix() * 1000
}

func GetPrecision(num float64) int {
	for i := 0; true; i++ {
		temp := num * math.Pow(10, float64(i))
		if temp == math.Floor(temp) {
			return i
		}
	}
	return 0
}