package api

import (
	"encoding/json"
	"fmt"
	"hello/model"
	"hello/util"
	"time"
)

var subscribeHandlerBitmex = func(subscribes []interface{}, subType string) error {
	var err error = nil
	step := 8
	stepSubscribes := make([]interface{}, 0)
	for i := 0; i*step < len(subscribes); i++ {
		//subBook := fmt.Sprintf(`{"op": "subscribe", "args": ["orderBookL2:%s"]}`, v)
		//subOrder := fmt.Sprintf(`{"op": "subscribe", "args": ["order:%s"]}`, v)
		subscribeMap := make(map[string]interface{})
		subscribeMap[`op`] = `subscribe`
		if (i+1)*step < len(subscribes) {
			stepSubscribes = subscribes[i*step : (i+1)*step]
		} else {
			stepSubscribes = subscribes[i*step:]
		}
		subscribeMap[`args`] = stepSubscribes
		subscribeMessage := util.JsonEncodeMapToByte(subscribeMap)
		if err := sendToWs(model.Bitmex, subscribeMessage); err != nil {
			util.SocketInfo("bitmex can not subscribe " + err.Error())
			return err
		}
	}
	return err

}

func WsDepthServeBitmex(markets *model.Markets, errHandler ErrHandler) (chan struct{}, error) {
	lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte) {
		if util.GetNow().Unix()-lastPingTime > 5 { // ping bitmex server every 5 seconds
			lastPingTime = util.GetNow().Unix()
			if err := sendToWs(model.Bitmex, []byte(`ping`)); err != nil {
				util.SocketInfo("bitmex server ping client error " + err.Error())
			}
		}
		if len(event) == 0 {
			return
		}
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
			//bidAsks := model.BidAsk{Ts: int(util.GetNowUnixMillion())}
			//bidAsks.Asks = model.Ticks{}
			//bidAsks.Bids = model.Ticks{}
			//for _, value := range data {
			//	symbol, _ := value.(map[string]interface{})[`symbol`].(string)
			//	id, _ := value.(map[string]interface{})[`id`].(json.Number).Int64()
			//	side, _ := value.(map[string]interface{})[`side`].(string)
			//	size, _ := value.(map[string]interface{})[`size`].(json.Number).Float64()
			//	price, _ := value.(map[string]interface{})[`price`].(json.Number).Float64()
			//	bidAsk := model.Tick{Id: fmt.Sprintf(`%d`, id), Price: price, Amount: size, Symbol: symbol}
			//	if side == `Buy` {
			//		bidAsks.Bids = append(bidAsks.Bids, bidAsk)
			//	} else if side == `Sell` {
			//		bidAsks.Asks = append(bidAsks.Asks, bidAsk)
			//	}
			//}
			//sort.Sort(bidAsks.Asks)
			//sort.Sort(sort.Reverse(bidAsks.Bids))
		case `update`:
		case `insert`:
			for _, value := range data {
				item := value.(map[string]interface{})
				if item == nil {
					continue
				}
				deal := &model.Deal{Market: model.Bitmex}
				if item[`timestamp`] != nil {
					dealTime, err := time.Parse(time.RFC3339, item[`timestamp`].(string))
					if err == nil {
						deal.Ts = dealTime.Unix() * 1000
					}
				}
				if item[`size`] != nil {
					amount, err := item[`size`].(json.Number).Float64()
					if err == nil {
						deal.Amount = amount
					}
				}
				if item[`price`] != nil {
					price, err := item[`price`].(json.Number).Float64()
					if err == nil {
						deal.Price = price
					}
				}
				if item[`symbol`] != nil {
					switch item[`symbol`].(string) {
					case `XBTUSD`:
						deal.Symbol = `btcusd_p`

					}
				}
				if item[`side`] != nil {
					deal.Side = item[`side`].(string)
				}
				markets.SetTrade(deal)
				//{"table":"trade","action":"insert","data":
				// [{"timestamp":"2019-10-28T11:12:18.272Z","symbol":"XBTUSD","side":"Buy","size":600,"price":9380,
				// "tickDirection":"PlusTick",
				// "trdMatchID":"385093ac-5474-65ae-4868-c6b87366968b",
				// "grossValue":6396600,"homeNotional":0.063966,"foreignNotional":600}]}
			}
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
	return WebSocketServe(model.Bitmex, model.AppConfig.WSUrls[model.Bitmex], model.SubscribeDepth,
		model.GetWSSubscribes(model.Bitmex, model.SubscribeDeal), subscribeHandlerBitmex, wsHandler, errHandler)
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
