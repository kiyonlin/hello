package util

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/bitly/go-simplejson"
	"github.com/pkg/errors"
	"io/ioutil"
	"math"
	"net/url"
	"strings"
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

func GetCurrencyFromSymbol(symbol string) (currency string, err error) {
	index := strings.Index(symbol, `_`)
	if index < 0 {
		return ``, errors.New(`wrong symbol format`)
	}
	return symbol[0:index], nil
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

func GetPrecision(num float64) int {
	for i := 0; true; i++ {
		temp := num * math.Pow(10, float64(i))
		if temp == math.Floor(temp) {
			return i
		}
	}
	return 0
}
