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
	"time"
)

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
		case `quote`:
			go handleQuote(markets, action, data)
		case `trade`:
			go handleTrade(markets, action, data)
		case `orderBookL2_25`:
			go handleOrderBook(markets, action, data)
		case `orderBookL2`:
			go handleOrderBook(markets, action, data)
		case `orderBook10`:
			go handOrderBook10(markets, data)
		case `order`:
			go handleOrder(markets, action, data)
			//model.HandlerMap[model.FunctionBMCarryHang](``, ``)
		case `position`:
			go handleAccount(action, data)
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
		account.Currency = model.StandardSymbol[model.Bitmex][item[`symbol`].(string)]
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
	account.Ts = util.GetNowUnixMillion()
	return
}

func parseQuote(item map[string]interface{}) (bid, ask *model.Tick, quoteTime time.Time, symbol string) {
	if item == nil {
		return
	}
	bid = &model.Tick{Side: model.OrderSideBuy}
	ask = &model.Tick{Side: model.OrderSideSell}
	if item[`symbol`] != nil {
		symbol = model.StandardSymbol[model.Bitmex][item[`symbol`].(string)]
		bid.Symbol = symbol
		ask.Symbol = symbol
	}
	if item[`bidPrice`] != nil {
		price, err := item[`bidPrice`].(json.Number).Float64()
		if err == nil {
			bid.Price = price
		}
	}
	if item[`askPrice`] != nil {
		price, err := item[`askPrice`].(json.Number).Float64()
		if err == nil {
			ask.Price = price
		}
	}
	if item[`bidSize`] != nil {
		size, err := item[`bidSize`].(json.Number).Float64()
		if err == nil {
			bid.Amount = size
		}
	}
	if item[`askSize`] != nil {
		size, err := item[`askSize`].(json.Number).Float64()
		if err == nil {
			ask.Amount = size
		}
	}
	if item[`timestamp`] != nil {
		quoteTime, _ = time.Parse(time.RFC3339, item[`timestamp`].(string))
	}
	return
}

func parseTick(item map[string]interface{}) (tick *model.Tick) {
	if item == nil {
		return nil
	}
	tick = &model.Tick{}
	if item[`symbol`] != nil {
		tick.Symbol = model.StandardSymbol[model.Bitmex][item[`symbol`].(string)]
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
		order.Symbol = model.StandardSymbol[model.Bitmex][item[`symbol`].(string)]
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

func handOrderBook10(markets *model.Markets, data []interface{}) {
	if data == nil || len(data) < 1 {
		return
	}
	bidAsk := &model.BidAsk{}
	symbol := ``
	item := data[0].(map[string]interface{})
	if item[`symbol`] != nil {
		symbol = model.StandardSymbol[model.Bitmex][item[`symbol`].(string)]
	}
	if item[`timestamp`] != nil {
		tickTime, _ := time.Parse(time.RFC3339, item[`timestamp`].(string))
		bidAsk.Ts = int(tickTime.UnixNano() / 1000000)
	}
	if item[`bids`] != nil {
		bidItems := item[`bids`].([]interface{})
		bidAsk.Bids = make([]model.Tick, len(bidItems))
		for key, value := range bidItems {
			if value == nil {
				continue
			}
			pair := value.([]interface{})
			if len(pair) == 2 {
				price, _ := pair[0].(json.Number).Float64()
				amount, _ := pair[1].(json.Number).Float64()
				bidAsk.Bids[key] = model.Tick{Side: model.OrderSideBuy, Symbol: symbol, Price: price, Amount: amount}
			}
		}
	}
	if item[`asks`] != nil {
		askItems := item[`asks`].([]interface{})
		bidAsk.Asks = make([]model.Tick, len(askItems))
		for key, value := range askItems {
			if value == nil {
				continue
			}
			pair := value.([]interface{})
			if len(pair) == 2 {
				price, _ := pair[0].(json.Number).Float64()
				amount, _ := pair[1].(json.Number).Float64()
				bidAsk.Asks[key] = model.Tick{Side: model.OrderSideSell, Symbol: symbol, Price: price, Amount: amount}
			}
		}
	}
	sort.Sort(bidAsk.Asks)
	sort.Sort(sort.Reverse(bidAsk.Bids))
	if markets.SetBidAsk(symbol, model.Bitmex, bidAsk) {
		for function, handler := range model.GetFunctions(model.Bitmex, symbol) {
			if handler != nil && function != model.FunctionMaker {
				go handler(model.Bitmex, symbol)
			}
		}
	}
}

func handleQuote(markets *model.Markets, action string, data []interface{}) {
	symbolTicks := make(map[string]*model.BidAsk)
	var compareTime *time.Time
	for _, value := range data {
		item := value.(map[string]interface{})
		bid, ask, quoteTime, symbol := parseQuote(item)
		switch action {
		case `partial`:
			symbolTicks[symbol] = &model.BidAsk{Ts: int(quoteTime.UnixNano() / 1000000),
				Bids: []model.Tick{*bid}, Asks: []model.Tick{*ask}}
		case `insert`:
			if compareTime == nil || compareTime.Before(quoteTime) {
				compareTime = &quoteTime
				symbolTicks[symbol] = &model.BidAsk{Ts: int(quoteTime.UnixNano() / 1000000),
					Bids: []model.Tick{*bid}, Asks: []model.Tick{*ask}}
			}
		}
	}
	for symbol, bidAsks := range symbolTicks {
		markets.SetBidAsk(symbol, model.Bitmex, bidAsks)
		for function, handler := range model.GetFunctions(model.Bitmex, symbol) {
			if handler != nil && function != model.FunctionMaker {
				go handler(model.Bitmex, symbol)
			}
		}
	}
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
			if symbolTicks[tick.Symbol] == nil {
				continue
			}
			newBids := model.Ticks{}
			newAsks := model.Ticks{}
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
				newAsks = append(newAsks, ask)
			}
			symbolTicks[tick.Symbol].Asks = newAsks
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
				newBids = append(newBids, bid)
			}
			symbolTicks[tick.Symbol].Bids = newBids
		case `insert`:
			if symbolTicks[tick.Symbol] == nil {
				_, symbolTicks[tick.Symbol] = markets.GetBidAsk(tick.Symbol, model.Bitmex)
			}
			if symbolTicks[tick.Symbol] == nil {
				continue
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
			if symbolTicks[tick.Symbol] == nil {
				continue
			}
			if tick.Side == model.OrderSideBuy {
				newBids := model.Ticks{}
				for _, bid := range symbolTicks[tick.Symbol].Bids {
					if bid.Id != tick.Id {
						newBids = append(newBids, bid)
					}
				}
				symbolTicks[tick.Symbol].Bids = newBids
			}
			if tick.Side == model.OrderSideSell {
				newAsks := model.Ticks{}
				for _, ask := range symbolTicks[tick.Symbol].Asks {
					if ask.Id != tick.Id {
						newAsks = append(newAsks, ask)
					}
				}
				symbolTicks[tick.Symbol].Asks = newAsks
			}
		}
	}
	for symbol, bidAsks := range symbolTicks {
		if bidAsks == nil {
			continue
		}
		sort.Sort(bidAsks.Asks)
		sort.Sort(sort.Reverse(bidAsks.Bids))
		util.SocketInfo(fmt.Sprintf(`----- %f %f %f %f %f`, bidAsks.Bids[0].Price,
			bidAsks.Bids[1].Price, bidAsks.Bids[2].Price, bidAsks.Bids[3].Price, bidAsks.Bids[4].Price))
		util.SocketInfo(fmt.Sprintf(`_____ %f %f %f %f %f`, bidAsks.Asks[0].Price,
			bidAsks.Asks[1].Price, bidAsks.Asks[2].Price, bidAsks.Asks[3].Price, bidAsks.Asks[4].Price))
		if bidAsks.Bids != nil && bidAsks.Asks != nil && len(bidAsks.Bids) > 0 && len(bidAsks.Asks) > 0 &&
			bidAsks.Asks[0].Price > bidAsks.Bids[0].Price {
			util.SocketInfo(fmt.Sprintf(`1111 %f %f %f %f %f`, bidAsks.Bids[0].Price,
				bidAsks.Bids[1].Price, bidAsks.Bids[2].Price, bidAsks.Bids[3].Price, bidAsks.Bids[4].Price))
			util.SocketInfo(fmt.Sprintf(`2222 %f %f %f %f %f`, bidAsks.Asks[0].Price,
				bidAsks.Asks[1].Price, bidAsks.Asks[2].Price, bidAsks.Asks[3].Price, bidAsks.Asks[4].Price))
			bidAsks.Ts = int(util.GetNowUnixMillion())
			if markets.SetBidAsk(symbol, model.Bitmex, bidAsks) {
				util.Notice(`bm set bid ask true`)
				for function, handler := range model.GetFunctions(model.Bitmex, symbol) {
					util.Notice(`bm ` + symbol + ` ` + function)
					if handler != nil && function != model.FunctionMaker {
						go handler(model.Bitmex, symbol)
					}
				}
			}
		}
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
					deal.Ts = dealTime.UnixNano() / 1000000
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
				deal.Symbol = model.StandardSymbol[model.Bitmex][item[`symbol`].(string)]
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
		key = model.AppConfig.BitmexKey
	}
	if secret == `` {
		secret = model.AppConfig.BitmexSecret
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

func cancelOrderBitmex(key, secret, orderId string) (result bool, errCode, msg string) {
	postData := make(map[string]interface{})
	postData[`orderID`] = orderId
	response := SignedRequestBitmex(key, secret, `DELETE`, `/order`, postData)
	util.Notice(fmt.Sprintf(`cancel bm order %s return %s`, orderId, string(response)))
	fmt.Println(string(response))
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

func queryOrderBitmex(key, secret, symbol, orderId string) (orders []*model.Order) {
	orders = make([]*model.Order, 0)
	postData := make(map[string]interface{})
	symbol = model.DialectSymbol[model.Bitmex][symbol]
	postData[`symbol`] = symbol
	postData[`reverse`] = `true`
	postData[`filter`] = fmt.Sprintf(`{"orderID":"%s"}`, orderId)
	response := SignedRequestBitmex(key, secret, `GET`, `/order`, postData)
	//util.Info(string(response))
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

func getAccountBitmex(key, secret string, accounts *model.Accounts) {
	postData := make(map[string]interface{})
	postData[`count`] = `100`
	response := SignedRequestBitmex(key, secret, `GET`, `/position`, postData)
	positionJson, err := util.NewJSON(response)
	if err == nil {
		positions, err := positionJson.Array()
		if err == nil {
			for _, data := range positions {
				account := &model.Account{Market: model.Bitmex, Ts: util.GetNowUnixMillion()}
				parseAccount(account, data.(map[string]interface{}))
				accounts.SetAccount(model.Bitmex, account.Currency, account)
			}
		}
	}
}

func placeOrderBitmex(order *model.Order, key, secret, orderSide, orderType, execInst, symbol, price, amount string) {
	postData := make(map[string]interface{})
	symbol = model.DialectSymbol[model.Bitmex][symbol]
	postData["symbol"] = symbol
	postData["side"] = strings.ToUpper(orderSide[0:1]) + orderSide[1:]
	postData["orderQty"] = amount
	if orderType != model.OrderTypeMarket && orderType != model.OrderTypeStop {
		postData[`price`] = price
	}
	postData["ordType"] = strings.ToUpper(orderType[0:1]) + orderType[1:]
	if execInst != `` {
		postData[`execInst`] = execInst
	}
	if orderType == model.OrderTypeStop {
		postData[`stopPx`] = price
		postData[`execInst`] = `LastPrice`
	}
	response := SignedRequestBitmex(key, secret, `POST`, `/order`, postData)
	util.Notice(`---- place bitmex` + string(response))
	orderJson, err := util.NewJSON(response)
	if err == nil {
		item, err := orderJson.Map()
		if err == nil {
			parseOrderBM(order, item)
		}
	}
	return
}

// binSize [1m,5m,1h,1d]
func getCandlesBitmex(key, secret, symbol, binSize string, start, end time.Time, count int) (
	candles map[string]*model.Candle) {
	candles = make(map[string]*model.Candle)
	postData := make(map[string]interface{})
	symbolNew := model.DialectSymbol[model.Bitmex][symbol]
	postData[`symbol`] = symbolNew
	postData[`reverse`] = `false`
	postData[`binSize`] = binSize
	postData[`count`] = strconv.Itoa(count)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	postData[`startTime`] = start.Format(time.RFC3339)
	//fmt.Println(start.Format(time.RFC3339))
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	//fmt.Println(end.Format(time.RFC3339))
	postData[`endTime`] = end.Format(time.RFC3339)
	response := SignedRequestBitmex(key, secret, `GET`, `/trade/bucketed`, postData)
	candleJson, err := util.NewJSON(response)
	duration, _ := time.ParseDuration(`-24h`)
	if err == nil {
		candleJsons, err := candleJson.Array()
		if err == nil {
			for _, value := range candleJsons {
				item := value.(map[string]interface{})
				candle := &model.Candle{Market: model.Bitmex, Symbol: symbol, Period: binSize}
				if item[`open`] != nil {
					candle.PriceOpen, _ = item[`open`].(json.Number).Float64()
				}
				if item[`close`] != nil {
					candle.PriceClose, _ = item[`close`].(json.Number).Float64()
				}
				if item[`high`] != nil {
					candle.PriceHigh, _ = item[`high`].(json.Number).Float64()
				}
				if item[`low`] != nil {
					candle.PriceLow, _ = item[`low`].(json.Number).Float64()
				}
				if item[`timestamp`] != nil {
					start, _ := time.Parse(time.RFC3339, item[`timestamp`].(string))
					start = start.Add(duration)
					candle.UTCDate = start.Format(time.RFC3339)[0:10]
					candles[candle.UTCDate] = candle
				}
			}
		}
	}
	return
}

func getBtcBalanceBitmex(key, secret string) (balance float64) {
	postData := make(map[string]interface{})
	postData[`currency`] = `XBt`
	response := SignedRequestBitmex(key, secret, `GET`, `/user/margin`, postData)
	balanceJson, err := util.NewJSON(response)
	if err == nil {
		balanceJson = balanceJson.Get(`marginBalance`)
		if balanceJson != nil {
			balance, err = balanceJson.Float64()
			balance = balance / 100000000
		}
	}
	return
}

func getFundingRateBitmex(symbol string) (fundingRate float64, update int64) {
	postData := make(map[string]interface{})
	symbol = model.DialectSymbol[model.Bitmex][symbol]
	postData[`symbol`] = symbol
	response := SignedRequestBitmex(``, ``, `GET`, `/instrument`, postData)
	instrumentJson, err := util.NewJSON(response)
	if err == nil {
		array, err := instrumentJson.Array()
		if err == nil {
			for _, value := range array {
				item := value.(map[string]interface{})
				if item[`symbol`] != nil && item[`symbol`] == symbol && item["fundingRate"] != nil &&
					item[`timestamp`] != nil {
					fundingRate, err = item["fundingRate"].(json.Number).Float64()
					updateTime, err := time.Parse(time.RFC3339, item[`fundingTimestamp`].(string))
					if err == nil {
						update = updateTime.Unix()
					}
				}
			}
		}
	}
	return
}
