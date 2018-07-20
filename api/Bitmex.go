package api

import (
	"fmt"
	"github.com/gorilla/websocket"
	"hello/model"
	"hello/util"
)

var subscribeHandlerBitmex = func(subscribes []string, conn *websocket.Conn) error {
	var err error = nil
	subscribes = []string{`XBTU18`}
	for _, v := range subscribes {
		subscribeMsg := fmt.Sprintf(`{"op": "subscribe", "args": ["orderBookL2:%s"]}`, v)
		err = conn.WriteMessage(websocket.TextMessage, []byte(subscribeMsg))
		if err != nil {
			util.SocketInfo("bitmex can not subscribe " + err.Error())
			return err
		}
	}
	return err
}

func WsDepthServeBitmex(markets *model.Markets, carryHandlers []CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
	lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte, conn *websocket.Conn) {
		if util.GetNow().Unix()-lastPingTime > 5 { // ping bitmex server every 5 seconds
			lastPingTime = util.GetNow().Unix()
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`ping`)); err != nil {
				util.SocketInfo("bitmex server ping client error " + err.Error())
			}
		}
		fmt.Println(string(event))
		//depthJson, err := util.NewJSON(event)
		//if err != nil {
		//	errHandler(err)
		//	return
		//}
		//if depthJson == nil {
		//	return
		//}
		//depthArray, err := depthJson.Array()
		//if err == nil && len(depthArray) > 0 {
		//	data := depthArray[0].(map[string]interface{})[`data`].(map[string]interface{})
		//	if data != nil {
		//		if data[`pair`] == nil {
		//			return
		//		}
		//		symbol := strings.ToLower(data[`pair`].(string))
		//		time, _ := data[`update_time`].(json.Number).Int64()
		//		bidAsk := model.BidAsk{Ts: int(time)}
		//		askArray := data[`asks`].([]interface{})
		//		bidArray := data[`bids`].([]interface{})
		//		bidAsk.Asks = make([][]float64, len(askArray))
		//		bidAsk.Bids = make([][]float64, len(bidArray))
		//		for i, value := range bidArray {
		//			bidAsk.Bids[i] = make([]float64, 2)
		//			str := value.(map[string]interface{})["price"].(string)
		//			bidAsk.Bids[i][0], _ = strconv.ParseFloat(str, 10)
		//			str = value.(map[string]interface{})["volume"].(string)
		//			bidAsk.Bids[i][1], _ = strconv.ParseFloat(str, 10)
		//		}
		//		for i, value := range askArray {
		//			bidAsk.Asks[i] = make([]float64, 2)
		//			str := value.(map[string]interface{})["price"].(string)
		//			bidAsk.Asks[i][0], _ = strconv.ParseFloat(str, 10)
		//			str = value.(map[string]interface{})["volume"].(string)
		//			bidAsk.Asks[i][1], _ = strconv.ParseFloat(str, 10)
		//		}
		//		sort.Sort(bidAsk.Asks)
		//		sort.Reverse(bidAsk.Bids)
		//		if markets.SetBidAsk(symbol, model.Coinpark, &bidAsk) {
		//			for _, handler := range carryHandlers {
		//				handler(symbol, model.Coinpark)
		//			}
		//		}
		//	}
		//}
	}
	return WebSocketServe(model.ApplicationConfig.WSUrls[model.Bitmex],
		model.GetSubscribes(model.Bitmex), subscribeHandlerBitmex, wsHandler, errHandler)
}
