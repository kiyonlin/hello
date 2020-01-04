package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/bitly/go-simplejson"
	"hello/model"
	"hello/util"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

var subscribeHandlerBybit = func(subscribes []interface{}, subType string) error {
	var err error = nil
	step := 1
	expire := util.GetNowUnixMillion() + 1000
	toBeSign := fmt.Sprintf(`GET/realtime%d`, expire)
	hash := hmac.New(sha256.New, []byte(model.AppConfig.BybitSecret))
	hash.Write([]byte(toBeSign))
	sign := hex.EncodeToString(hash.Sum(nil))
	authCmd := fmt.Sprintf(`{"op": "auth", "args": ["%s", %d, "%s"]}`,
		model.AppConfig.BybitKey, expire, sign)
	if err = sendToWs(model.Bybit, []byte(authCmd)); err != nil {
		util.SocketInfo("bybit can not auth " + err.Error())
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
		if err = sendToWs(model.Bybit, subscribeMessage); err != nil {
			util.SocketInfo("bybit can not subscribe " + err.Error())
			return err
		}
	}
	return err
}

func WsDepthServeBybit(markets *model.Markets, errHandler ErrHandler) (chan struct{}, error) {
	lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte) {
		if util.GetNow().Unix()-lastPingTime > 30 { // ping ws server every 5 seconds
			lastPingTime = util.GetNow().Unix()
			if err := sendToWs(model.Bybit, []byte(`{"op":"ping"}`)); err != nil {
				util.SocketInfo("bybit server ping client error " + err.Error())
			}
		}
		if len(event) == 0 {
			return
		}
		depthJson, depthErr := util.NewJSON(event)
		if depthJson == nil {
			return
		}
		topic := depthJson.Get(`topic`).MustString()
		ts := depthJson.Get(`timestamp_e6`).MustInt64()
		if depthErr != nil {
			util.SocketInfo(`bybit parse err` + string(event))
			return
		}
		if strings.Contains(topic, `orderBookL2_25.`) {
			symbol := model.StandardSymbol[model.Bybit][topic[strings.LastIndex(topic, `.`)+1:]]
			go handleOrderBookBybit(markets, symbol, ts, depthJson)
		} else if topic == `position` {
			go handleAccountBybit(depthJson.Get(`data`))
		}
	}
	return WebSocketServe(model.Bybit, model.AppConfig.WSUrls[model.Bybit], model.SubscribeDepth,
		model.GetWSSubscribes(model.Bybit, model.SubscribeDepth),
		subscribeHandlerBybit, wsHandler, errHandler)
}

func parseTickBybit(item map[string]interface{}) (tick *model.Tick) {
	if item == nil {
		return nil
	}
	tick = &model.Tick{}
	if item[`symbol`] != nil {
		tick.Symbol = model.StandardSymbol[model.Bybit][item[`symbol`].(string)]
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
		price, err := strconv.ParseFloat(item[`price`].(string), 64)
		if err == nil {
			tick.Price = price
		}
	}
	if item[`side`] != nil {
		tick.Side = strings.ToLower(item[`side`].(string))
	}
	return tick
}

func handleOrderBookBybit(markets *model.Markets, symbol string, ts int64, response *simplejson.Json) {
	if response == nil {
		return
	}
	action := response.Get(`type`).MustString()
	var bidAsk *model.BidAsk
	if action == `snapshot` {
		bidAsk = &model.BidAsk{}
		bidAsk.Bids = make([]model.Tick, 0)
		bidAsk.Asks = make([]model.Tick, 0)
		data, err := response.Get(`data`).Array()
		if err != nil {
			return
		}
		for _, value := range data {
			tick := parseTickBybit(value.(map[string]interface{}))
			if tick.Side == model.OrderSideSell {
				bidAsk.Asks = append(bidAsk.Asks, *tick)
			} else if tick.Side == model.OrderSideBuy {
				bidAsk.Bids = append(bidAsk.Bids, *tick)
			}
		}
	} else if action == `delta` {
		_, bidAsk = markets.GetBidAsk(symbol, model.Bybit)
		data := response.Get(`data`)
		if bidAsk == nil || data == nil {
			return
		}
		arrayDelete, errDelete := data.Get(`delete`).Array()
		arrayUpdate, errUpdate := data.Get(`update`).Array()
		arrayInsert, errInsert := data.Get(`insert`).Array()
		if errDelete != nil || errInsert != nil || errUpdate != nil {
			return
		}
		for _, value := range arrayInsert {
			tick := parseTickBybit(value.(map[string]interface{}))
			if tick.Side == model.OrderSideBuy {
				bidAsk.Bids = append(bidAsk.Bids, *tick)
			}
			if tick.Side == model.OrderSideSell {
				bidAsk.Asks = append(bidAsk.Asks, *tick)
			}
		}
		for _, value := range arrayUpdate {
			tick := parseTickBybit(value.(map[string]interface{}))
			if tick.Side == model.OrderSideBuy {
				for key, bid := range bidAsk.Bids {
					if tick.Id == bid.Id {
						bidAsk.Bids[key] = *tick
					}
				}
			}
			if tick.Side == model.OrderSideSell {
				for key, ask := range bidAsk.Asks {
					if tick.Id == ask.Id {
						bidAsk.Asks[key] = *tick
					}
				}
			}
		}
		deleteMap := make(map[string]bool)
		for _, value := range arrayDelete {
			tick := parseTickBybit(value.(map[string]interface{}))
			deleteMap[tick.Id] = true
		}
		bidNew := make([]model.Tick, 0)
		askNew := make([]model.Tick, 0)
		for _, value := range bidAsk.Bids {
			if deleteMap[value.Id] == false {
				bidNew = append(bidNew, value)
			}
		}
		for _, value := range bidAsk.Asks {
			if deleteMap[value.Id] == false {
				askNew = append(askNew, value)
			}
		}
		bidAsk.Bids = bidNew
		bidAsk.Asks = askNew
	}
	if bidAsk != nil {
		bidAsk.Ts = int(ts / 1000)
		//util.Info(fmt.Sprintf(`---------%d`, util.GetNowUnixMillion()-int64(bidAsk.Ts)))
		sort.Sort(bidAsk.Asks)
		sort.Sort(sort.Reverse(bidAsk.Bids))
		markets.SetBidAsk(symbol, model.Bybit, bidAsk)
		for function, handler := range model.GetFunctions(model.Bybit, symbol) {
			if handler != nil && function != model.FunctionMaker {
				//go handler(model.Bybit, symbol)
			}
		}
	}
}

func parseAccountBybit(account *model.Account, item map[string]interface{}) {
	if item[`symbol`] != nil {
		account.Currency = model.StandardSymbol[model.Bybit][item[`symbol`].(string)]
	}
	if item[`size`] != nil && item[`side`] != nil {
		account.Direction = strings.ToLower(item[`side`].(string))
		free, err := item[`size`].(json.Number).Float64()
		if err == nil {
			if model.OrderSideSell == account.Direction {
				account.Free = math.Abs(free) * -1
			} else {
				account.Free = free
			}
		}
	}
	if item[`entry_price`] != nil {
		account.EntryPrice, _ = strconv.ParseFloat(item[`entry_price`].(string), 64)
	}
	if item[`liq_price`] != nil {
		account.LiquidationPrice, _ = strconv.ParseFloat(item[`liq_price`].(string), 64)
	}
	if item[`bust_price`] != nil {
		account.BankruptcyPrice, _ = strconv.ParseFloat(item[`bust_price`].(string), 64)
	}
	if item[`position_margin`] != nil {
		account.Margin, _ = strconv.ParseFloat(item[`position_margin`].(string), 64)
	}
	if item[`realised_pnl`] != nil {
		account.ProfitReal, _ = strconv.ParseFloat(item[`realised_pnl`].(string), 64)
	}
	if item[`unrealised_pnl`] != nil {
		pnlUnreal, err := item[`unrealised_pnl`].(json.Number).Float64()
		if err == nil {
			account.ProfitUnreal = pnlUnreal
		}
	}
}

func handleAccountBybit(dataJson *simplejson.Json) {
	if dataJson == nil {
		return
	}
	data, _ := dataJson.Array()
	for _, value := range data {
		account := &model.Account{Market: model.Bybit, Ts: util.GetNowUnixMillion()}
		if value != nil {
			item := value.(map[string]interface{})
			parseAccountBybit(account, item)
		}
		model.AppAccounts.SetAccount(model.Bybit, account.Currency, account)
	}
}

func SignedRequestBybit(key, secret, method, path string, body map[string]interface{}) []byte {
	if key == `` {
		key = model.AppConfig.BybitKey
	}
	if secret == `` {
		secret = model.AppConfig.BybitSecret
	}
	if body == nil {
		body = make(map[string]interface{})
	}
	body[`api_key`] = key
	body[`timestamp`] = strconv.FormatInt(util.GetNowUnixMillion(), 10)
	uri := model.AppConfig.RestUrls[model.Bybit] + path
	paramStr := util.ComposeParams(body)
	hash := hmac.New(sha256.New, []byte(secret))
	hash.Write([]byte(paramStr))
	sign := hex.EncodeToString(hash.Sum(nil))
	body[`sign`] = sign
	paramStr = util.ComposeParams(body)
	//headers := map[string]string{"Content-Type": "application/json"}
	headers := map[string]string{`api_key`: key, `sign`: sign, "Content-Type": "application/json"}
	if method == `GET` {
		uri = uri + `?` + paramStr
	}
	responseBody, _ := util.HttpRequest(method, uri, string(util.JsonEncodeMapToByte(body)), headers)
	return responseBody
}

func cancelOrderBybit(key, secret, symbol, orderId string) (result bool, errCode, msg string, order *model.Order) {
	postData := make(map[string]interface{})
	postData[`order_id`] = orderId
	postData[`symbol`] = model.DialectSymbol[model.Bybit][symbol]
	response := SignedRequestBybit(key, secret, `POST`, `/v2/private/order/cancel`, postData)
	orderJson, err := util.NewJSON(response)
	result = false
	if err == nil {
		retCode := orderJson.Get(`ret_code`).MustInt64()
		if retCode == 0 {
			result = true
		}
		errCode = strconv.FormatInt(retCode, 10)
		msg = orderJson.Get(`ret_msg`).MustString()
		if orderJson.Get(`result`) != nil {
			item, _ := orderJson.Get(`result`).Map()
			if item != nil {
				order = &model.Order{}
				parseOrderBybit(order, item)
			}
		}
		return
	}
	return false, ``, ``, nil
}

func queryOrderBybit(key, secret, symbol, orderId string) (orders []*model.Order) {
	orders = make([]*model.Order, 0)
	postData := make(map[string]interface{})
	symbol = model.DialectSymbol[model.Bybit][symbol]
	postData[`symbol`] = model.DialectSymbol[model.Bybit][symbol]
	postData[`order_id`] = orderId
	response := SignedRequestBybit(key, secret, `GET`, `/open-api/order/list`, postData)
	util.Notice(`query orders: ` + string(response))
	orderJson, err := util.NewJSON(response)
	if err == nil {
		orderJson = orderJson.GetPath(`result`, `data`)
		if orderJson == nil {
			return
		}
		orderArray, _ := orderJson.Array()
		for _, data := range orderArray {
			order := &model.Order{Market: model.Bybit}
			parseOrderBybit(order, data.(map[string]interface{}))
			if order.OrderId != `` {
				orders = append(orders, order)
			}
		}
	}
	return
}

func getAccountBybit(key, secret, symbol string, accounts *model.Accounts) {
	postData := make(map[string]interface{})
	postData[`symbol`] = model.DialectSymbol[model.Bybit][symbol]
	response := SignedRequestBybit(key, secret, `GET`, `/v2/private/position/list`, postData)
	util.Info(fmt.Sprintf(string(response)))
	positionJson, err := util.NewJSON(response)
	if err == nil {
		positionJson = positionJson.Get(`result`)
		if positionJson != nil {
			account := &model.Account{Market: model.Bybit, Ts: util.GetNowUnixMillion(), Currency: symbol}
			item, _ := positionJson.Map()
			parseAccountBybit(account, item)
			accounts.SetAccount(model.Bybit, account.Currency, account)
		}
	}
}

// timeInForce 有效选项:GoodTillCancel, ImmediateOrCancel, FillOrKill,PostOnly
func placeOrderBybit(order *model.Order, key, secret, orderSide, orderType, timeInForce, symbol, price,
	amount string) {
	postData := make(map[string]interface{})
	symbol = model.DialectSymbol[model.Bybit][symbol]
	postData["side"] = strings.ToUpper(orderSide[0:1]) + orderSide[1:]
	postData["order_type"] = strings.ToUpper(orderType[0:1]) + orderType[1:]
	if orderType != model.OrderTypeMarket && orderType != model.OrderTypeStop {
		postData[`price`] = price
	}
	if timeInForce == `` {
		timeInForce = `GoodTillCancel`
	}
	postData["symbol"] = symbol
	postData["qty"] = amount
	postData[`time_in_force`] = timeInForce
	response := SignedRequestBybit(key, secret, `POST`, `/v2/private/order/create`, postData)
	util.Notice(`place bybit` + string(response))
	orderJson, err := util.NewJSON(response)
	if err == nil {
		orderJson = orderJson.Get(`result`)
		if orderJson != nil {
			item, err := orderJson.Map()
			if err == nil {
				parseOrderBybit(order, item)
			}
		}
	}
	return
}

func GetRiskLimit() {
	postData := make(map[string]interface{})
	response := SignedRequestBybit(``, ``, `GET`,
		`/open-api/wallet/risk-limit/list`, postData)
	fmt.Println(string(response))
}

func getFundingRateBybit(symbol string) (fundingRate float64, expire int64) {
	postData := make(map[string]interface{})
	symbol = model.DialectSymbol[model.Bybit][symbol]
	postData[`symbol`] = symbol
	response := SignedRequestBybit(``, ``, `GET`,
		`/open-api/funding/prev-funding-rate`, postData)
	instrumentJson, err := util.NewJSON(response)
	if err == nil {
		retCode := instrumentJson.Get(`ret_code`).MustFloat64()
		if retCode != 0 {
			return 0, 0
		}
		instrumentJson = instrumentJson.Get(`result`)
		if instrumentJson != nil {
			instrument, _ := instrumentJson.Map()
			if instrument == nil {
				return 0, 0
			}
			if instrument[`symbol`] != nil && instrument[`symbol`] == symbol &&
				instrument[`funding_rate`] != nil && instrument[`funding_rate_timestamp`] != nil {
				fundingRate, _ = strconv.ParseFloat(instrument[`funding_rate`].(string), 64)
				expire, _ = instrument[`funding_rate_timestamp`].(json.Number).Int64()
				expire += 28800
			}
		}
	}
	return
}

func parseOrderBybit(order *model.Order, item map[string]interface{}) {
	if order == nil {
		return
	}
	if item[`order_id`] != nil {
		order.OrderId = item[`order_id`].(string)
	}
	if item[`symbol`] != nil {
		order.Symbol = model.StandardSymbol[model.Bybit][item[`symbol`].(string)]
	}
	if item[`side`] != nil {
		order.OrderSide = strings.ToLower(item[`side`].(string))
	}
	if item[`order_type`] != nil {
		order.OrderType = strings.ToLower(item[`order_type`].(string))
	}
	if item[`qty`] != nil {
		order.Amount, _ = item[`qty`].(json.Number).Float64()
	}
	if item[`price`] != nil {
		order.Price, _ = item[`price`].(json.Number).Float64()
	}
	if item[`last_exec_price`] != nil {
		order.DealPrice, _ = item[`last_exec_price`].(json.Number).Float64()
	}
	if item[`cum_exec_qty`] != nil {
		order.DealAmount, _ = item[`cum_exec_qty`].(json.Number).Float64()
	}
	if item[`cum_exec_fee`] != nil {
		order.Fee, _ = item[`cum_exec_fee`].(json.Number).Float64()
	}
	if item[`created_at`] != nil {
		order.OrderTime, _ = time.Parse(time.RFC3339, item[`created_at`].(string))
	}
	if item[`updated_at`] != nil {
		order.OrderUpdateTime, _ = time.Parse(time.RFC3339, item[`updated_at`].(string))
	}
	if item[`order_status`] != nil {
		order.Status = model.GetOrderStatus(model.Bybit, item[`order_status`].(string))
	}
	return
}
