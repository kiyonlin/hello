package api

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"hello/model"
	"hello/util"
	"sort"
)

var subscribeHandlerBitmex = func(subscribes []string, conn *websocket.Conn) error {
	var err error = nil
	for _, v := range subscribes {
		subBook := fmt.Sprintf(`{"op": "subscribe", "args": ["orderBookL2:%s"]}`, v)
		err = conn.WriteMessage(websocket.TextMessage, []byte(subBook))
		if err != nil {
			util.SocketInfo("bitmex can not subscribe " + err.Error())
			return err
		}
		//subOrder := fmt.Sprintf(`{"op": "subscribe", "args": ["order:%s"]}`, v)
		//err = conn.WriteMessage(websocket.TextMessage, []byte(subOrder))
		//if err != nil {
		//	util.SocketInfo("bitmex can not subscribe " + err.Error())
		//	return err
		//}
	}
	return err
}

func WsDepthServeBitmex(errHandler ErrHandler) (chan struct{}, error) {
	lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte, conn *websocket.Conn) {
		if util.GetNow().Unix()-lastPingTime > 5 { // ping bitmex server every 5 seconds
			lastPingTime = util.GetNow().Unix()
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`ping`)); err != nil {
				util.SocketInfo("bitmex server ping client error " + err.Error())
			}
		}
		if len(event) == 0 {
			return
		}
		fmt.Println(string(event))
		depthJson, depthErr := util.NewJSON(event)
		if depthJson == nil {
			return
		}
		action, actionErr := depthJson.Get(`action`).String()
		data, dataErr := depthJson.Get(`data`).Array()
		if depthErr != nil || actionErr != nil || dataErr != nil || data == nil {
			util.SocketInfo(`bitmex parse err` + string(event))
			return
		}
		switch action {
		case `partial`:
			bidAsks := model.BidAsk{Ts: int(util.GetNowUnixMillion())}
			bidAsks.Asks = model.Ticks{}
			bidAsks.Bids = model.Ticks{}
			for _, value := range data {
				symbol, _ := value.(map[string]interface{})[`symbol`].(string)
				id, _ := value.(map[string]interface{})[`id`].(json.Number).Int64()
				side, _ := value.(map[string]interface{})[`side`].(string)
				size, _ := value.(map[string]interface{})[`size`].(json.Number).Float64()
				price, _ := value.(map[string]interface{})[`price`].(json.Number).Float64()
				bidAsk := model.Tick{Id: fmt.Sprintf(`%d`, id), Price: price, Amount: size, Symbol: symbol}
				if side == `Buy` {
					bidAsks.Bids = append(bidAsks.Bids, bidAsk)
				} else if side == `Sell` {
					bidAsks.Asks = append(bidAsks.Asks, bidAsk)
				}
			}
			sort.Sort(bidAsks.Asks)
			sort.Sort(sort.Reverse(bidAsks.Bids))
		case `update`:
		case `insert`:
		case `delete`:
		}
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
		//				handler(model.Coinpark, symbol)
		//			}
		//		}
		//	}
		//}
	}
	return WebSocketServe(model.AppConfig.WSUrls[model.Bitmex],
		model.GetDepthSubscribes(model.Bitmex), subscribeHandlerBitmex, wsHandler, errHandler)
}

func CancelOrderBitmex(orderId string) (result bool, errCode, msg string) {
	fmt.Println(orderId)
	return true, ``, ``
}

func queryOrderBitmex(orderId string) (dealAmount, dealPrice float64, status string) {
	fmt.Println(orderId)
	return 0, 0, ``
}

func getAccountBitmex(accounts *model.Accounts) {
	fmt.Println(len(accounts.Data))
}

func placeOrderBitmex(orderSide, orderType, symbol, price, amount string) (orderId, errCode string) {
	fmt.Println(fmt.Sprintf(`%s%s%s%s%s`, orderSide, orderType, symbol, price, amount))
	return ``, ``
}
