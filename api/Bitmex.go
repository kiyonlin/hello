package api

import (
	"encoding/json"
	"fmt"
	"hello/model"
	"hello/util"
	"sort"
	"strconv"
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
		if util.GetNow().Unix()-lastPingTime > 30 { // ping bitmex server every 5 seconds
			lastPingTime = util.GetNow().Unix()
			if err := sendToWs(model.Bitmex, []byte(`ping`)); err != nil {
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
		table := depthJson.Get(`table`).MustString()
		if depthErr != nil || actionErr != nil || dataErr != nil || data == nil {
			util.SocketInfo(`bitmex parse err` + string(event))
			return
		}
		switch table {
		case `trade`:
			handleTrade(markets, action, data)
		case `orderBookL2_25`:
			handleOrderBook(markets, action, data)
		}
	}
	return WebSocketServe(model.Bitmex, model.AppConfig.WSUrls[model.Bitmex], model.SubscribeDepth,
		model.GetWSSubscribes(model.Bitmex, model.SubscribeDeal+`,`+model.SubscribeDepth),
		subscribeHandlerBitmex, wsHandler, errHandler)
}

func handleOrderBook(markets *model.Markets, action string, data []interface{}) {
	//markets.GetBidAsk(symbol, market)
	symbol := ``
	switch action {
	case `partial`:
		bidAsks := &model.BidAsk{Ts: int(util.GetNowUnixMillion())}
		bidAsks.Asks = model.Ticks{}
		bidAsks.Bids = model.Ticks{}
		for _, value := range data {
			item := value.(map[string]interface{})
			if item == nil {
				continue
			}
			tick := model.Tick{}
			if item[`symbol`] != nil {
				switch item[`symbol`].(string) {
				case `XBTUSD`:
					symbol = `btcusd_p`
					tick.Symbol = symbol
				}
			}
			if item[`id`] != nil {
				id, err := item[`id`].(json.Number).Int64()
				if err == nil {
					tick.Id = strconv.FormatInt(id, 10)
				}
			}
			if item[`size`] != nil {
				amount, err := item[`size`].(json.Number).Float64()
				if err == nil {
					tick.Amount = amount
				}
			}
			if item[`price`] != nil {
				price, err := item[`price`].(json.Number).Float64()
				if err == nil {
					tick.Price = price
				}
			}
			if item[`side`] != nil {
				if item[`side`].(string) == model.OrderSideBuy {
					bidAsks.Bids = append(bidAsks.Bids, tick)
				} else if item[`side`].(string) == model.OrderSideSell {
					bidAsks.Asks = append(bidAsks.Asks, tick)
				}
			}
		}
		sort.Sort(bidAsks.Asks)
		sort.Sort(sort.Reverse(bidAsks.Bids))
		markets.SetBidAsk(symbol, model.Bitmex, bidAsks)
	case `update`:
	case `insert`:
	case `delete`:
	}
}

func handleTrade(markets *model.Markets, action string, data []interface{}) {
	switch action {
	case `partial`:
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
		}
	case `delete`:
	}
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
