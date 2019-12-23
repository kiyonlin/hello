package api

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hello/model"
	"hello/util"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 下单返回1016 资金不足// 下单返回1002 系统繁忙// 返回426 调用次数太频繁 // less than the min limit order amount
//{"status":3033,"msg":"market order is disabled for symbol bsvusdt"}
//{"status":1002,"msg":"system busy"}
var lastDepthPingFcoin = util.GetNowUnixMillion()

var subscribeHandlerFcoin = func(subscribes []interface{}, subType string) error {
	var err error = nil
	step := 8
	stepSubscribes := make([]interface{}, 0)
	for i := 0; i*step < len(subscribes); i++ {
		subscribeMap := make(map[string]interface{})
		subscribeMap[`cmd`] = `sub`
		if (i+1)*step < len(subscribes) {
			stepSubscribes = subscribes[i*step : (i+1)*step]
		} else {
			stepSubscribes = subscribes[i*step:]
		}
		subscribeMap[`args`] = stepSubscribes
		subscribeMessage := util.JsonEncodeMapToByte(subscribeMap)
		if err := sendToWs(model.Fcoin, subscribeMessage); err != nil {
			util.SocketInfo("fcoin can not subscribe " + err.Error())
			return err
		}
	}
	return err
}

//func requestDeal(symbol string) {
//	subscribeMap := make(map[string]interface{})
//	subscribeMap[`cmd`] = `req`
//	subscribeMap[`id`] = `deal#` + symbol
//	subscribeMap[`args`] = model.GetWSSubscribe(model.Fcoin, symbol, model.SubscribeDeal)
//	subscribeMessage := util.JsonEncodeMapToByte(subscribeMap)
//
//	if err := sendToWs(model.Fcoin, subscribeMessage); err != nil {
//		util.SocketInfo("fcoin can not request " + err.Error())
//	}
//	time.Sleep(time.Millisecond * 490)
//	if err := sendToWs(model.Fcoin, subscribeMessage); err != nil {
//		util.SocketInfo("fcoin can not request twice" + err.Error())
//	}
//}

func WsDepthServeFcoin(markets *model.Markets, errHandler ErrHandler) (chan struct{}, error) {
	wsHandler := func(event []byte) {
		//util.Info(string(event))
		responseJson, err := util.NewJSON(event)
		if err != nil {
			errHandler(err)
			return
		}
		if responseJson == nil {
			return
		}
		if util.GetNowUnixMillion()-lastDepthPingFcoin > 30000 {
			lastDepthPingFcoin = util.GetNowUnixMillion()
			pingMsg := []byte(fmt.Sprintf(`{"cmd":"ping","args":[%d],"id":"id"}`, util.GetNowUnixMillion()))
			if err := sendToWs(model.Fcoin, pingMsg); err != nil {
				util.SocketInfo("fcoin server ping client error " + err.Error())
			}
		}
		msgType := responseJson.Get(`type`).MustString()
		symbol := model.GetSymbol(model.Fcoin, responseJson.Get("type").MustString())
		symbols := model.GetMarketSymbols(model.Fcoin)
		if symbols == nil || symbols[symbol] == false {
			//util.Notice(symbol + ` not supported`)
			return
		}
		if strings.Index(msgType, `trade.`) == 0 {
			ts, _ := responseJson.Get("ts").Int64()
			amount := responseJson.Get(`amount`).MustFloat64()
			side := responseJson.Get(`side`).MustString()
			price := responseJson.Get(`price`).MustFloat64()
			//deal := markets.GetBigDeal(symbol, model.Fcoin)
			//if deal == nil || deal.Ts < ts {
			if markets.SetBigDeal(symbol, model.Fcoin, &model.Deal{
				Symbol: symbol, Market: model.Fcoin, Amount: amount, Ts: ts, Side: side, Price: price}) {
				for function, handler := range model.GetFunctions(model.Fcoin, symbol) {
					if handler != nil && function == model.FunctionMaker {
						util.Notice(fmt.Sprintf(`[try makerl]%s`, symbol))
						handler(model.Fcoin, symbol)
					}
				}
			}
		} else {
			if symbol != "" && symbol != "_" {
				bidAsk := model.BidAsk{}
				bidsLen := len(responseJson.Get("bids").MustArray()) / 2
				bidAsk.Bids = make([]model.Tick, bidsLen)
				for i := 0; i < bidsLen; i++ {
					price, _ := responseJson.Get("bids").GetIndex(i * 2).Float64()
					amount, _ := responseJson.Get("bids").GetIndex(i*2 + 1).Float64()
					bidAsk.Bids[i] = model.Tick{Price: price, Amount: amount}
				}
				asksLen := len(responseJson.Get("asks").MustArray()) / 2
				bidAsk.Asks = make([]model.Tick, asksLen)
				for i := 0; i < asksLen; i++ {
					price, _ := responseJson.Get("asks").GetIndex(i * 2).Float64()
					amount, _ := responseJson.Get("asks").GetIndex(i*2 + 1).Float64()
					bidAsk.Asks[i] = model.Tick{Price: price, Amount: amount}
				}
				sort.Sort(bidAsk.Asks)
				sort.Sort(sort.Reverse(bidAsk.Bids))
				bidAsk.Ts = responseJson.Get("ts").MustInt()
				if markets.SetBidAsk(symbol, model.Fcoin, &bidAsk) {
					for function, handler := range model.GetFunctions(model.Fcoin, symbol) {
						if handler != nil && function != model.FunctionMaker {
							go handler(model.Fcoin, symbol)
						}
					}
				}
			}
		}
	}
	requestUrl := model.AppConfig.WSUrls[model.Fcoin]
	return WebSocketServe(model.Fcoin, requestUrl, model.SubscribeDepth,
		model.GetWSSubscribes(model.Fcoin, model.SubscribeDepth), subscribeHandlerFcoin, wsHandler, errHandler)
}

func SignedRequestFcoin(key, secret, method, path string, body map[string]interface{}) []byte {
	if key == `` {
		key = model.AppConfig.FcoinKey
	}
	if secret == `` {
		secret = model.AppConfig.FcoinSecret
	}
	uri := model.AppConfig.RestUrls[model.Fcoin] + path
	current := util.GetNow()
	currentTime := strconv.FormatInt(current.UnixNano(), 10)[0:13]
	if method == `GET` && len(body) > 0 {
		uri += `?` + util.ComposeParams(body)
	}
	toBeBase := method + uri + currentTime
	if method == `POST` {
		toBeBase += util.ComposeParams(body)
	}
	based := base64.StdEncoding.EncodeToString([]byte(toBeBase))
	hash := hmac.New(sha1.New, []byte(secret))
	hash.Write([]byte(based))
	sign := base64.StdEncoding.EncodeToString(hash.Sum(nil))
	headers := map[string]string{`FC-ACCESS-KEY`: key,
		`FC-ACCESS-SIGNATURE`: sign, `FC-ACCESS-TIMESTAMP`: currentTime, "Content-Type": "application/json"}
	var responseBody []byte
	if body == nil {
		responseBody, _ = util.HttpRequest(method, uri, ``, headers)
	} else {
		responseBody, _ = util.HttpRequest(method, uri, string(util.JsonEncodeMapToByte(body)), headers)
	}
	return responseBody
}

// side: buy sell
// type: limit market
// fcoin中amount在市价买单中指的是右侧的钱
func placeOrderFcoin(order *model.Order, key, secret, orderSide, orderType, symbol, accountType, price, amount string) {
	postData := make(map[string]interface{})
	if orderType == model.OrderTypeLimit {
		postData["price"] = price
	}
	orderSide = model.GetDictMap(model.Fcoin, orderSide)
	if orderSide == `` {
		util.Notice(fmt.Sprintf(`[parameter error] order side: %s`, orderSide))
	}
	orderType = model.GetDictMap(model.Fcoin, orderType)
	if orderType == `` {
		util.Notice(fmt.Sprintf(`[parameter error] order type: %s`, orderType))
	}
	postData["symbol"] = strings.ToLower(strings.Replace(symbol, "_", "", 1))
	postData["type"] = orderType
	postData["side"] = orderSide
	postData["amount"] = amount
	if accountType == model.AccountTypeLever {
		postData[`account_type`] = `margin`
	}
	responseBody := SignedRequestFcoin(key, secret, "POST", "/orders", postData)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		order.OrderId, _ = orderJson.Get("data").String()
		status, _ := orderJson.Get("status").Int()
		order.ErrCode = strconv.Itoa(status)
		util.Notice(fmt.Sprintf(`[挂单fcoin] %s side: %s type: %s price: %s amount: %s order id %s errCode: %s 返回%s`,
			symbol, orderSide, orderType, price, amount, order.OrderId, order.ErrCode, string(responseBody)))
	}
}

func cancelOrderFcoin(key, secret, orderId string) (result bool, errCode, msg string) {
	responseBody := SignedRequestFcoin(key, secret, `POST`, `/orders/`+orderId+`/submit-cancel`, nil)
	responseJson, err := util.NewJSON([]byte(responseBody))
	status := -1
	if err == nil {
		status, _ = responseJson.Get(`status`).Int()
		msg, _ = responseJson.Get(`msg`).String()
	}
	util.Notice(orderId + "fcoin cancel order" + string(responseBody))
	if status == 0 {
		return true, ``, msg
	}
	return false, strconv.FormatInt(int64(status), 10), msg
}

func parseOrder(symbol string, orderMap map[string]interface{}) (order *model.Order) {
	if orderMap == nil || orderMap[`created_at`] == nil || orderMap[`amount`] == nil ||
		orderMap[`price`] == nil || orderMap[`filled_amount`] == nil ||
		orderMap[`fill_fees`] == nil || orderMap[`fees_income`] == nil ||
		orderMap[`id`] == nil || orderMap[`type`] == nil || orderMap[`side`] == nil ||
		orderMap[`state`] == nil {
		return nil
	}
	createTime, _ := orderMap[`created_at`].(json.Number).Int64()
	amount, _ := strconv.ParseFloat(orderMap[`amount`].(string), 64)
	price, _ := strconv.ParseFloat(orderMap[`price`].(string), 64)
	filledAmount, _ := strconv.ParseFloat(orderMap[`filled_amount`].(string), 64)
	fee, _ := strconv.ParseFloat(orderMap[`fill_fees`].(string), 64)
	feeIncome, _ := strconv.ParseFloat(orderMap[`fees_income`].(string), 64)
	orderSide := model.GetDictMapRevert(model.Fcoin, orderMap[`side`].(string))
	return &model.Order{
		OrderId:    orderMap[`id`].(string),
		Symbol:     symbol,
		Market:     model.Fcoin,
		Amount:     amount,
		DealAmount: filledAmount,
		OrderTime:  time.Unix(0, createTime*1000000),
		OrderType:  model.GetDictMapRevert(model.Fcoin, orderMap[`type`].(string)),
		OrderSide:  orderSide,
		DealPrice:  price,
		Price:      price,
		Fee:        fee,
		FeeIncome:  feeIncome,
		Status:     model.GetOrderStatus(model.Fcoin, orderMap[`state`].(string)),
	}
}

//测试发现，只有after参数管用， before无效，可以作为内部逻辑控制条件
func queryOrdersFcoin(key, secret, symbol, states, accountType string, before, after int64) (orders []*model.Order) {
	util.Info(fmt.Sprintf(`cancel parameters %d %d`, before, after))
	states, _ = model.GetOrderStatusRevert(model.Fcoin, states)
	states = strings.Replace(states, `pending_cancel,`, ``, 1)
	states = strings.Replace(states, `pending_cancel`, ``, 1)
	body := make(map[string]interface{})
	body[`symbol`] = strings.ToLower(strings.Replace(symbol, "_", "", 1))
	body[`states`] = states
	body[`limit`] = `100`
	//if after > 0 {
	//	body[`after`] = strconv.FormatInt(after, 10)
	//}
	if accountType == model.AccountTypeLever {
		body[`account_type`] = `margin`
	}
	orders = make([]*model.Order, 0)
	//runNext := true
	//for runNext {
	//	line := int64(0)
	responseBody := SignedRequestFcoin(key, secret, `GET`, `/orders`, body)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		jsonOrders, _ := orderJson.Get(`data`).Array()
		for _, order := range jsonOrders {
			orderMap := order.(map[string]interface{})
			order := parseOrder(symbol, orderMap)
			//if line < order.OrderTime.Unix() {
			//	line = order.OrderTime.Unix()
			//}
			//if line > before {
			//	runNext = false
			//}
			//fmt.Println(order.OrderTime.Unix())
			orders = append(orders, order)
		}
		//if len(jsonOrders) == 0 {
		//	break
		//}
	}
	//body[`after`] = strconv.FormatInt(line, 10)
	//time.Sleep(time.Second)
	return orders
}

func queryOrderFcoin(key, secret, symbol, orderId string) (order *model.Order) {
	postData := make(map[string]interface{})
	postData["symbol"] = strings.ToLower(strings.Replace(symbol, "_", "", 1))
	responseBody := SignedRequestFcoin(key, secret, `GET`, `/orders/`+orderId, postData)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		orderMap, _ := orderJson.Get(`data`).Map()
		return parseOrder(symbol, orderMap)
	}
	return nil
}

func getLeverAccountFcoin(key, secret string) (accounts map[string]map[string]*model.Account) {
	responseBody := SignedRequestFcoin(key, secret, `GET`, `/broker/leveraged_accounts`, nil)
	balanceJson, err := util.NewJSON(responseBody)
	accounts = make(map[string]map[string]*model.Account)
	if err == nil {
		status, _ := balanceJson.Get(`status`).String()
		if status == `ok` {
			data, _ := balanceJson.Get(`data`).Array()
			for _, value := range data {
				accountJson := value.(map[string]interface{})
				if accountJson[`open`].(bool) {
					symbol := accountJson[`leveraged_account_type`]
					base := accountJson[`base`].(string)
					quote := accountJson[`quote`].(string)
					freeBase, _ := strconv.ParseFloat(accountJson[`available_base_currency_amount`].(string), 64)
					freeQuote, _ := strconv.ParseFloat(accountJson[`available_quote_currency_amount`].(string), 64)
					frozenBase, _ := strconv.ParseFloat(accountJson[`frozen_base_currency_amount`].(string), 64)
					frozenQuote, _ := strconv.ParseFloat(accountJson[`frozen_quote_currency_amount`].(string), 64)
					market := fmt.Sprintf(`%s_%s_%s`, model.Fcoin, model.AccountTypeLever, symbol)
					accountBase := &model.Account{Market: market, Currency: base, Free: freeBase, Frozen: frozenBase}
					accountQuote := &model.Account{Market: market, Currency: quote, Free: freeQuote, Frozen: frozenQuote}
					coinAccount := make(map[string]*model.Account)
					coinAccount[base] = accountBase
					coinAccount[quote] = accountQuote
					accounts[market] = coinAccount
				}
			}
		}
	}
	return accounts
}

func getAccountFcoin(key, secret string) (currency []string, account []*model.Account) {
	responseBody := SignedRequestFcoin(key, secret, `GET`, `/accounts/balance`, nil)
	balanceJson, err := util.NewJSON(responseBody)
	if err == nil {
		status, _ := balanceJson.Get("status").Int()
		if status == 0 {
			currencies, _ := balanceJson.Get("data").Array()
			coins := make([]string, 0)
			accounts := make([]*model.Account, 0)
			for _, value := range currencies {
				asset := value.(map[string]interface{})
				free, _ := strconv.ParseFloat(asset["available"].(string), 64)
				frozen, _ := strconv.ParseFloat(asset["frozen"].(string), 64)
				//if free == 0 && frozen == 0 {
				//	continue
				//}
				currency := strings.ToLower(asset["currency"].(string))
				coins = append(coins, currency)
				accounts = append(accounts,
					&model.Account{Market: model.Fcoin, Currency: currency, Free: free, Frozen: frozen})
			}
			return coins, accounts
		}
	}
	return nil, nil
}

func getBuyPriceFcoin(key, secret, symbol string) (buy float64, err error) {
	model.AppConfig.SetSymbolPrice(symbol, 0)
	requestSymbol := strings.ToLower(strings.Replace(symbol, "_", "", 1))
	responseBody := SignedRequestFcoin(key, secret, `GET`, `/market/ticker/`+requestSymbol, nil)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		orderJson = orderJson.Get(`data`)
		tickerType, _ := orderJson.Get(`type`).String()
		if strings.Contains(tickerType, requestSymbol) {
			price, _ := orderJson.Get("ticker").GetIndex(0).Float64()
			model.AppConfig.SetSymbolPrice(symbol, price)
		}
	}
	return model.AppConfig.SymbolPrice[symbol], nil
}
