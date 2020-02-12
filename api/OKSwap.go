package api

import (
	"bytes"
	"compress/flate"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hello/model"
	"hello/util"
	"io"
	"time"
)

var subscribeHandlerOKSwap = func(subscribes []interface{}, subType string) error {
	var err error = nil
	step := 5
	epoch := time.Now().UnixNano() / int64(time.Millisecond)
	timestamp := fmt.Sprintf(`%d.%d`, epoch/1000, epoch%1000)
	toBeSign := fmt.Sprintf(`%s%s%s`, timestamp, `GET`, `/users/self/verify`)
	hash := hmac.New(sha256.New, []byte(model.AppConfig.OkexSecret))
	hash.Write([]byte(toBeSign))
	sign := base64.StdEncoding.EncodeToString([]byte(hash.Sum(nil)))
	authCmd := fmt.Sprintf(`{"op":"login","args":["%s","%s","%s","%s"]}`,
		model.AppConfig.OkexKey, `OKSwap`, timestamp, sign)
	if err = sendToWs(model.OKSwap, []byte(authCmd)); err != nil {
		util.SocketInfo("okswap can not auth " + err.Error())
	}
	stepSubscribes := make([]interface{}, 0)
	for i := 0; i*step < len(subscribes); i++ {
		subscribeMap := make(map[string]interface{})
		subscribeMap[`op`] = `subscribe`
		if (i+1)*step < len(subscribes) {
			stepSubscribes = subscribes[i*step : (i+1)*step]
		} else {
			stepSubscribes = subscribes[i*step:]
		}
		subscribeMap[`args`] = stepSubscribes
		subscribeMessage := util.JsonEncodeMapToByte(subscribeMap)
		if err = sendToWs(model.OKSwap, subscribeMessage); err != nil {
			util.SocketInfo("okswap can not subscribe " + err.Error())
			return err
		}
	}
	return err
}

func WsDepthServeOKSwap(markets *model.Markets, errHandler ErrHandler) (chan struct{}, error) {
	lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte) {
		if util.GetNow().Unix()-lastPingTime > 30 { // ping ws server every 5 seconds
			lastPingTime = util.GetNow().Unix()
			if err := sendToWs(model.OKSwap, []byte(`"ping"`)); err != nil {
				util.SocketInfo("okswap server ping client error " + err.Error())
			}
		}
		if len(event) == 0 {
			return
		}
		//messages := make([]OKEXMessage, 1)
		var out bytes.Buffer
		reader := flate.NewReader(bytes.NewReader(event))
		_, _ = io.Copy(&out, reader)
		event = out.Bytes()
		fmt.Println(string(event))
		markets.GetBmPendingOrders()
	}
	return WebSocketServe(model.OKSwap, model.AppConfig.WSUrls[model.OKSwap], model.SubscribeDepth,
		model.GetWSSubscribes(model.OKSwap, model.SubscribeDepth),
		subscribeHandlerOKSwap, wsHandler, errHandler)
}
