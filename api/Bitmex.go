package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hello/model"
	"hello/util"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var bmMsgLock sync.Mutex

var subscribeHandlerBitmex = func(subscribes []interface{}, subType string) error {
	var err error = nil
	step := 8
	expire := util.GetNow().Unix() + 5
	toBeSign := fmt.Sprintf(`GET/realtime%d`, expire)
	hash := hmac.New(sha256.New, []byte(model.AppConfig.BitmexSecret))
	hash.Write([]byte(toBeSign))
	sign := hex.EncodeToString(hash.Sum(nil))
	authCmd := fmt.Sprintf(`{"op": "authKeyExpires", "args": ["%s", %d, "%s"]}`,
		model.AppConfig.BitmexKey, expire, sign)
	if err = sendToWs(model.Bitmex, []byte(authCmd)); err != nil {
		util.SocketInfo("bitmex can not auth " + err.Error())
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
		if err = sendToWs(model.Bitmex, subscribeMessage); err != nil {
			util.SocketInfo("bitmex can not subscribe " + err.Error())
			return err
		}
	}
	return err
}

func WsDepthServeBitmex(markets *model.Markets, errHandler ErrHandler) (chan struct{}, error) {
	lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte) {
		bmMsgLock.Lock()
		defer bmMsgLock.Unlock()
		if util.GetNow().Unix()-lastPingTime > 30 { // ping bitmex server every 5 seconds
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
		case `order`:
			fmt.Println(string(event))
			handleOrder(markets, action, data)
			model.HandlerMap[model.FunctionBMCarryHang](``, ``)
		case `position`:
			handleAccount(action, data)
		}
	}
	return WebSocketServe(model.Bitmex, model.AppConfig.WSUrls[model.Bitmex], model.SubscribeDepth,
		model.GetWSSubscribes(model.Bitmex, model.SubscribeDepth),
		subscribeHandlerBitmex, wsHandler, errHandler)
}

func parseAccount(account *model.Account, item map[string]interface{}) {
	if item == nil {
		return
	}
	if item[`symbol`] != nil {
		switch item[`symbol`].(string) {
		case `XBTUSD`:
			account.Currency = `btcusd_p`
		}
	}
	if item[`currentQty`] != nil {
		free, err := item[`currentQty`].(json.Number).Float64()
		if err == nil {
			account.Free = free
		}
	}
	if item[`avgEntryPrice`] != nil {
		price, err := item[`avgEntryPrice`].(json.Number).Float64()
		if err == nil {
			account.EntryPrice = price
		}
	}
	return
}

func parseTick(item map[string]interface{}) (tick *model.Tick) {
	if item == nil {
		return nil
	}
	tick = &model.Tick{}
	if item[`symbol`] != nil {
		switch item[`symbol`].(string) {
		case `XBTUSD`:
			tick.Symbol = `btcusd_p`
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
		tick.Side = strings.ToLower(item[`side`].(string))
	}
	return tick
}

func parseOrderBM(order *model.Order, item map[string]interface{}) {
	if item[`orderID`] != nil {
		order.OrderId = item[`orderID`].(string)
	}
	if item[`symbol`] != nil {
		symbol := item[`symbol`].(string)
		switch symbol {
		case `XBTUSD`:
			symbol = `btcusd_p`
		}
		order.Symbol = symbol
	}
	if item[`side`] != nil {
		order.OrderSide = strings.ToLower(item[`side`].(string))
	}
	if item[`orderQty`] != nil {
		order.Amount, _ = item[`orderQty`].(json.Number).Float64()
	}
	if item[`price`] != nil {
		order.Price, _ = item[`price`].(json.Number).Float64()
	}
	if item[`workingIndicator`] != nil && item[`workingIndicator`].(bool) {
		order.Status = model.CarryStatusWorking
	} else if item[`ordStatus`] != nil {
		order.Status = model.GetOrderStatus(model.Bitmex, item[`ordStatus`].(string))
	}
	if item[`cumQty`] != nil {
		order.DealAmount, _ = item[`cumQty`].(json.Number).Float64()
	}
	if item[`avgPx`] != nil {
		order.DealPrice, _ = item[`avgPx`].(json.Number).Float64()
	}
	return
}

func handleOrder(markets *model.Markets, action string, data []interface{}) {
	var orders map[string]*model.Order
	for _, value := range data {
		switch action {
		case `partial`:
			if orders == nil {
				orders = make(map[string]*model.Order)
			}
			order := &model.Order{Market: model.Bitmex}
			parseOrderBM(order, value.(map[string]interface{}))
			if order.OrderId != `` {
				orders[order.OrderId] = order
			}
		case `insert`:
			if orders == nil {
				orders = markets.GetBmPendingOrders()
			}
			order := &model.Order{Market: model.Bitmex}
			parseOrderBM(order, value.(map[string]interface{}))
			if order.OrderId != `` {
				orders[order.OrderId] = order
			}
		case `update`:
			if orders == nil {
				orders = markets.GetBmPendingOrders()
			}
			if orders != nil {
				order := &model.Order{Market: model.Bitmex}
				parseOrderBM(order, value.(map[string]interface{}))
				if order.OrderId != `` {
					orderOld := orders[order.OrderId]
					if orderOld != nil {
						parseOrderBM(orderOld, value.(map[string]interface{}))
					}
				}
			}
		}
	}
	markets.SetBMPendingOrders(orders)
}

func handleOrderBook(markets *model.Markets, action string, data []interface{}) {
	//markets.GetBidAsk(symbol, market)
	symbolTicks := make(map[string]*model.BidAsk)
	for _, value := range data {
		tick := parseTick(value.(map[string]interface{}))
		if tick == nil || tick.Symbol == `` {
			continue
		}
		switch action {
		case `partial`:
			if symbolTicks[tick.Symbol] == nil {
				symbolTicks[tick.Symbol] = &model.BidAsk{Ts: int(util.GetNowUnixMillion())}
				symbolTicks[tick.Symbol].Asks = model.Ticks{}
				symbolTicks[tick.Symbol].Bids = model.Ticks{}
			}
			if tick.Side == model.OrderSideBuy {
				symbolTicks[tick.Symbol].Bids = append(symbolTicks[tick.Symbol].Bids, *tick)
			}
			if tick.Side == model.OrderSideSell {
				symbolTicks[tick.Symbol].Asks = append(symbolTicks[tick.Symbol].Asks, *tick)
			}
		case `update`:
			if symbolTicks[tick.Symbol] == nil {
				_, symbolTicks[tick.Symbol] = markets.GetBidAsk(tick.Symbol, model.Bitmex)
			}
			for _, ask := range symbolTicks[tick.Symbol].Asks {
				if ask.Id == tick.Id {
					if tick.Amount > 0 {
						ask.Amount = tick.Amount
					}
					if tick.Side != `` {
						ask.Side = tick.Side
					}
					if tick.Price > 0 {
						ask.Price = tick.Price
					}
				}
			}
			for _, bid := range symbolTicks[tick.Symbol].Bids {
				if bid.Id == tick.Id {
					if tick.Amount > 0 {
						bid.Amount = tick.Amount
					}
					if tick.Side != `` {
						bid.Side = tick.Side
					}
					if tick.Price > 0 {
						bid.Price = tick.Price
					}
				}
			}
		case `insert`:
			if symbolTicks[tick.Symbol] == nil {
				_, symbolTicks[tick.Symbol] = markets.GetBidAsk(tick.Symbol, model.Bitmex)
			}
			if tick.Side == model.OrderSideBuy {
				symbolTicks[tick.Symbol].Bids = append(symbolTicks[tick.Symbol].Bids, *tick)
			}
			if tick.Side == model.OrderSideSell {
				symbolTicks[tick.Symbol].Asks = append(symbolTicks[tick.Symbol].Asks, *tick)
			}
		case `delete`:
			if symbolTicks[tick.Symbol] == nil {
				_, symbolTicks[tick.Symbol] = markets.GetBidAsk(tick.Symbol, model.Bitmex)
			}
			if tick.Side == model.OrderSideBuy {
				bids := model.Ticks{}
				for _, bid := range symbolTicks[tick.Symbol].Bids {
					if bid.Id != tick.Id {
						bids = append(bids, bid)
					}
				}
				symbolTicks[tick.Symbol].Bids = bids
			}
			if tick.Side == model.OrderSideSell {
				asks := model.Ticks{}
				for _, ask := range symbolTicks[tick.Symbol].Asks {
					if ask.Id != tick.Id {
						asks = append(asks, ask)
					}
				}
				symbolTicks[tick.Symbol].Asks = asks
			}
		}
	}
	for symbol, bidAsks := range symbolTicks {
		bids := model.Ticks{}
		asks := model.Ticks{}
		for _, value := range bidAsks.Bids {
			if value.Side == model.OrderSideSell {
				asks = append(asks, value)
			}
			if value.Side == model.OrderSideBuy {
				bids = append(bids, value)
			}
		}
		for _, value := range bidAsks.Asks {
			if value.Side == model.OrderSideSell {
				asks = append(asks, value)
			}
			if value.Side == model.OrderSideBuy {
				bids = append(bids, value)
			}
		}
		sort.Sort(asks)
		sort.Sort(sort.Reverse(bids))
		bidAsks.Bids = bids
		bidAsks.Asks = asks
		bidAsks.Ts = int(util.GetNowUnixMillion())
		markets.SetBidAsk(symbol, model.Bitmex, bidAsks)
	}
}

func handleAccount(action string, data []interface{}) {
	for _, value := range data {
		account := &model.Account{Market: model.Bitmex, Ts: util.GetNowUnixMillion()}
		parseAccount(account, value.(map[string]interface{}))
		switch action {
		case `partial`:
			model.AppAccounts.SetAccount(model.Bitmex, account.Currency, account)
		case `update`:
			preAccount := model.AppAccounts.GetAccount(model.Bitmex, account.Currency)
			if preAccount != nil {
				parseAccount(preAccount, value.(map[string]interface{}))
			}
			model.AppAccounts.SetAccount(model.Bitmex, account.Currency, preAccount)
		}
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

func SignedRequestBitmex(key, secret, method, path string, body map[string]interface{}) []byte {
	if key == `` {
		key = model.AppConfig.FmexKey
	}
	if secret == `` {
		secret = model.AppConfig.FmexSecret
	}
	uri := model.AppConfig.RestUrls[model.Bitmex] + path
	expire := util.GetNow().Unix() + 5
	if method == `GET` && len(body) > 0 {
		uri += `?` + util.ComposeParams(body)
	}
	signPath := uri
	if strings.Contains(signPath, `//`) {
		signPath = signPath[strings.Index(signPath, `//`)+2:]
		signPath = signPath[strings.Index(signPath, `/`):]
	}
	toBeSign := fmt.Sprintf(`%s%s%d`, method, signPath, expire)
	//if method != `GET` {
	toBeSign += string(util.JsonEncodeMapToByte(body))
	//}
	hash := hmac.New(sha256.New, []byte(secret))
	hash.Write([]byte(toBeSign))
	sign := hex.EncodeToString(hash.Sum(nil))
	headers := map[string]string{`api-key`: key, `api-expires`: strconv.FormatInt(expire, 10),
		`api-signature`: sign, "Content-Type": "application/json"}
	var responseBody []byte
	if body == nil {
		responseBody, _ = util.HttpRequest(method, uri, ``, headers)
	} else {
		responseBody, _ = util.HttpRequest(method, uri, string(util.JsonEncodeMapToByte(body)), headers)
	}
	return responseBody
}

func CancelOrderBitmex(key, secret, orderId string) (result bool, errCode, msg string) {
	postData := make(map[string]interface{})
	postData[`orderID`] = orderId
	response := SignedRequestBitmex(key, secret, `DELETE`, `/order`, postData)
	orderJson, err := util.NewJSON(response)
	if err == nil {
		orderArray, arrayErr := orderJson.Array()
		if arrayErr == nil {
			for _, value := range orderArray {
				item := value.(map[string]interface{})
				if item == nil {
					continue
				}
				if item[`orderID`] != nil {
					itemId := item[`orderID`].(string)
					if itemId == orderId {
						if item[`ordStatus`] != nil {
							status := item[`ordStatus`].(string)
							status = model.GetOrderStatus(model.Bitmex, status)
							if status != `` && status != model.CarryStatusWorking {
								return true, ``, ``
							}
						}
					}
				}
			}
		}
	} else {
		return false, ``, err.Error()
	}
	return false, ``, ``
}

func queryOrderBitmex(key, secret, symbol string) (orders []*model.Order) {
	orders = make([]*model.Order, 0)
	postData := make(map[string]interface{})
	postData[`symbol`] = symbol
	response := SignedRequestBitmex(key, secret, `GET`, `/order`, postData)
	orderJson, err := util.NewJSON(response)
	if err == nil {
		orderArray, _ := orderJson.Array()
		for _, data := range orderArray {
			order := &model.Order{Market: model.Bitmex}
			parseOrderBM(order, data.(map[string]interface{}))
			if order.OrderId != `` {
				orders = append(orders, order)
			}
		}
	}
	return
}

func getAccountBitmex(accounts *model.Accounts) {
	fmt.Println(len(accounts.Data))
}

func placeOrderBitmex(key, secret, orderSide, orderType, execInst, symbol, price, amount string) (orderId, errCode string) {
	postData := make(map[string]interface{})
	if symbol == `btcusd_p` {
		symbol = `XBTUSD`
	}
	postData["symbol"] = symbol
	postData["side"] = strings.ToUpper(orderSide[0:1]) + orderSide[1:]
	postData["orderQty"] = amount
	postData[`price`] = price
	postData["ordType"] = strings.ToUpper(orderType[0:1]) + orderType[1:]
	postData[`execInst`] = execInst
	response := SignedRequestBitmex(key, secret, `POST`, `/order`, postData)
	orderJson, err := util.NewJSON(response)
	if err == nil {
		orderId = orderJson.Get(`orderID`).MustString()
		errCode = orderJson.Get(`error`).MustString()
	} else {
		return ``, err.Error()
	}
	return
}
