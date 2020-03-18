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
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var lastDepthPingFtx = util.GetNowUnixMillion()
var socketLockFtx sync.Mutex

var subscribeHandlerFtx = func(subscribes []interface{}, subType string) error {
	var err error = nil
	ts := time.Now().UnixNano() / int64(time.Millisecond)
	toBeSign := fmt.Sprintf(`%dwebsocket_login`, ts)
	hash := hmac.New(sha256.New, []byte(model.AppConfig.FtxSecret))
	hash.Write([]byte(toBeSign))
	sign := hex.EncodeToString(hash.Sum(nil))
	authCmd := fmt.Sprintf(`{"op":"login","args":{"key":"%s","sign":"%s","time":%d}}`,
		model.AppConfig.FtxKey, sign, ts)
	if err = sendToWs(model.Ftx, []byte(authCmd)); err != nil {
		util.SocketInfo("ftx can not auth " + err.Error())
	}
	if err = sendToWs(model.Ftx, []byte(`{"op": "subscribe", "channel": "fills"}`)); err != nil {
		util.SocketInfo("ftx can not subscribe fill " + err.Error())
	}
	for i := 0; i < len(subscribes); i++ {
		cmdSubscribe := subscribes[i].([]string)
		subCmd := fmt.Sprintf(`{"op": "subscribe", "channel": "%s", "market": "%s"}`,
			cmdSubscribe[0], cmdSubscribe[1])
		if err = sendToWs(model.Ftx, []byte(subCmd)); err != nil {
			util.SocketInfo("ftx can not subscribe " + err.Error())
			return err
		}
	}
	return err
}

func WsDepthServeFtx(markets *model.Markets, errHandler ErrHandler) (chan struct{}, error) {
	wsHandler := func(event []byte) {
		defer socketLockFtx.Unlock()
		socketLockFtx.Lock()
		responseJson, err := util.NewJSON(event)
		if err != nil {
			errHandler(err)
			return
		}
		if responseJson == nil {
			return
		}
		if util.GetNowUnixMillion()-lastDepthPingFtx > 15000 {
			lastDepthPingFtx = util.GetNowUnixMillion()
			pingMsg := []byte(`{"op":"ping"}`)
			if err := sendToWs(model.Ftx, pingMsg); err != nil {
				util.SocketInfo("ftx server ping client error " + err.Error())
			}
		}
		msgType := responseJson.Get(`channel`).MustString()
		if msgType == `orderbook` {
			handleDepthFtx(markets, responseJson)
		}
	}
	requestUrl := model.AppConfig.WSUrls[model.Ftx]
	//subType := model.SubscribeDepth + `,` + model.SubscribeDeal
	return WebSocketServe(model.Ftx, requestUrl, ``, model.GetWSSubscribes(model.Ftx, ``),
		subscribeHandlerFtx, wsHandler, errHandler)
}

func handleDepthFtx(markets *model.Markets, response *simplejson.Json) {
	if response == nil {
		return
	}
	symbol := model.GetStandardSymbol(model.Ftx, response.Get("market").MustString())
	dataType := response.Get(`type`).MustString()
	data := response.Get(`data`)
	if data != nil {
		bidAsk := &model.BidAsk{}
		ts := data.Get(`time`).MustFloat64()
		bidAsk.Ts = int(ts * 1000)
		bids := data.Get(`bids`).MustArray()
		asks := data.Get(`asks`).MustArray()
		bidAsk.Bids = make([]model.Tick, 0)
		bidAsk.Asks = make([]model.Tick, 0)
		if dataType == `partial` {
			for _, item := range bids {
				price, _ := item.([]interface{})[0].(json.Number).Float64()
				size, _ := item.([]interface{})[1].(json.Number).Float64()
				bidAsk.Bids = append(bidAsk.Bids, model.Tick{Price: price, Amount: size})
			}
			for _, item := range asks {
				price, _ := item.([]interface{})[0].(json.Number).Float64()
				size, _ := item.([]interface{})[1].(json.Number).Float64()
				bidAsk.Asks = append(bidAsk.Asks, model.Tick{Price: price, Amount: size})
			}
		} else if dataType == `update` {
			priceAmountBid := make(map[float64]*model.Tick)
			priceAmountAsk := make(map[float64]*model.Tick)
			for _, item := range bids {
				price, _ := item.([]interface{})[0].(json.Number).Float64()
				size, _ := item.([]interface{})[1].(json.Number).Float64()
				priceAmountBid[price] = &model.Tick{Price: price, Amount: size}
			}
			for _, item := range asks {
				price, _ := item.([]interface{})[0].(json.Number).Float64()
				size, _ := item.([]interface{})[1].(json.Number).Float64()
				priceAmountAsk[price] = &model.Tick{Price: price, Amount: size}
			}
			_, oldBidAsk := markets.GetBidAsk(symbol, model.Ftx)
			for _, bid := range oldBidAsk.Bids {
				if priceAmountBid[bid.Price] == nil {
					bidAsk.Bids = append(bidAsk.Bids, bid)
				} else if priceAmountBid[bid.Price].Amount > 0 {
					bidAsk.Bids = append(bidAsk.Bids, *priceAmountBid[bid.Price])
				}
				delete(priceAmountBid, bid.Price)
			}
			for _, bid := range priceAmountBid {
				if bid.Amount > 0 {
					bidAsk.Bids = append(bidAsk.Bids, *bid)
				}
			}
			for _, ask := range oldBidAsk.Asks {
				if priceAmountAsk[ask.Price] == nil {
					bidAsk.Asks = append(bidAsk.Asks, ask)
				} else if priceAmountAsk[ask.Price].Amount > 0 {
					bidAsk.Asks = append(bidAsk.Asks, *priceAmountAsk[ask.Price])
				}
				delete(priceAmountAsk, ask.Price)
			}
			for _, ask := range priceAmountAsk {
				if ask.Amount > 0 {
					bidAsk.Asks = append(bidAsk.Asks, *ask)
				}
			}
		}
		sort.Sort(bidAsk.Asks)
		sort.Sort(sort.Reverse(bidAsk.Bids))
		if markets.SetBidAsk(symbol, model.Ftx, bidAsk) {
			//util.SocketInfo(markets.ToStringBidAsk(bidAsk))
			for function, handler := range model.GetFunctions(model.Ftx, symbol) {
				if handler != nil {
					settings := model.GetSetting(function, model.Ftx, symbol)
					for _, setting := range settings {
						handler(setting)
					}
				}
			}
		}
	}
}

func getCandlesFtx(key, secret, symbol, binSize string, start, end time.Time, count int) (
	candles map[string]*model.Candle) {
	candles = make(map[string]*model.Candle)
	symbolNew := model.GetDialectSymbol(model.Ftx, symbol)
	param := make(map[string]interface{})
	if binSize == `1d` {
		param[`resolution`] = `86400`
	}
	param[`limit`] = fmt.Sprintf(`%d`, count)
	param[`start_time`] = fmt.Sprintf(`%d`, start.Unix())
	param[`end_time`] = fmt.Sprintf(`%d`, end.Unix())
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	response := SignedRequestFtx(key, secret, `GET`,
		fmt.Sprintf(`/markets/%s/candles`, symbolNew), param, nil)
	candleJson, err := util.NewJSON(response)
	if err == nil {
		candleJsons := candleJson.Get(`result`).MustArray()
		for _, value := range candleJsons {
			item := value.(map[string]interface{})
			candle := &model.Candle{Market: model.Ftx, Symbol: symbol, Period: binSize}
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
			if item[`startTime`] != nil {
				start, _ := time.Parse(time.RFC3339, item[`startTime`].(string))
				candle.UTCDate = start.Format(time.RFC3339)[0:10]
				candles[candle.UTCDate] = candle
			}
		}
	}
	return
}

func GetWalletHistoryFtx(key, secret string) (history string) {
	history = string(SignedRequestFtx(key, secret, `GET`, `/wallet/deposits`, nil, nil))
	history += string(SignedRequestFtx(key, secret, `GET`, `/wallet/withdrawals`, nil, nil))
	return
}

func getUSDBalanceFtx(key, secret string) (balance float64) {
	response := SignedRequestFtx(key, secret, `GET`, `/wallet/balances`, nil, nil)
	util.Notice(`get usd balance ftx: ` + string(response))
	balanceJson, err := util.NewJSON(response)
	if err == nil {
		items := balanceJson.Get(`result`).MustArray()
		for _, item := range items {
			data := item.(map[string]interface{})
			if data[`coin`] != nil && data[`coin`] == `USD` {
				balance, _ = data[`free`].(json.Number).Float64()
			}
		}
	}
	return
}

func cancelOrderFtx(key, secret, orderId string) (result bool) {
	response := SignedRequestFtx(key, secret, `DELETE`, fmt.Sprintf(`/orders/%s`, orderId), nil, nil)
	util.Notice(`cancel ftx ` + string(response))
	orderJson, err := util.NewJSON(response)
	if err == nil {
		return orderJson.Get(`success`).MustBool()
	}
	return false
}

func queryOrderFtx(key, secret, orderId string) (order *model.Order) {
	response := SignedRequestFtx(key, secret, `GET`, fmt.Sprintf(`/orders/%s`, orderId), nil, nil)
	util.Notice(`query orders ftx: ` + string(response))
	orderJson, err := util.NewJSON(response)
	if err == nil {
		data, _ := orderJson.Get(`result`).Map()
		order = &model.Order{Market: model.Ftx}
		parseOrderFtx(order, data)
	}
	return
}

func getAccountFtx(key, secret string, accounts *model.Accounts) {
	postData := make(map[string]interface{})
	response := SignedRequestFtx(key, secret, `GET`, `/positions`, nil, postData)
	util.Notice(`get account ftx ` + fmt.Sprintf(string(response)))
	positionJson, err := util.NewJSON(response)
	if err == nil {
		positionJson = positionJson.Get(`result`)
		if positionJson != nil {
			data := positionJson.MustArray()
			for _, item := range data {
				account := &model.Account{Market: model.Ftx, Ts: util.GetNowUnixMillion()}
				parseAccountFtx(account, item.(map[string]interface{}))
				accounts.SetAccount(model.Ftx, account.Currency, account)
			}
		}
	}
}

func getFundingRateFtx(symbol string) (fundingRate float64, expire int64) {
	postData := make(map[string]interface{})
	symbol = model.GetDialectSymbol(model.Ftx, symbol)
	postData[`future`] = symbol
	response := SignedRequestFtx(``, ``, `GET`,
		`/funding_payments`, nil, postData)
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

func parseAccountFtx(account *model.Account, item map[string]interface{}) {
	if item[`entryPrice`] != nil {
		account.EntryPrice, _ = item[`entryPrice`].(json.Number).Float64()
	}
	if item[`estimatedLiquidationPrice`] != nil {
		account.LiquidationPrice, _ = item[`estimatedLiquidationPrice`].(json.Number).Float64()
	}
	if item[`future`] != nil {
		account.Currency = model.GetStandardSymbol(model.Ftx, item[`future`].(string))
	}
	if item[`netSize`] != nil {
		account.Free, _ = item[`netSize`].(json.Number).Float64()
		//account.Free = account.Free * account.EntryPrice
	}
	if item[`realizedPnl`] != nil {
		account.ProfitReal, _ = item[`realizedPnl`].(json.Number).Float64()
	}
	if item[`side`] != nil {
		account.Direction = item[`side`].(string)
	}
	if item[`bust_price`] != nil {
		account.BankruptcyPrice, _ = strconv.ParseFloat(item[`bust_price`].(string), 64)
	}
	if item[`position_margin`] != nil {
		account.Margin, _ = strconv.ParseFloat(item[`position_margin`].(string), 64)
	}
	if item[`unrealizedPnl`] != nil {
		account.ProfitUnreal, _ = item[`unrealizedPnl`].(json.Number).Float64()
	}
}

//remainingSize	number	31431.0
//reduceOnly	boolean	false
//ioc	boolean	false
//postOnly	boolean	false
//orderPrice	number	null	price of the order sent when this stop loss triggered
//retryUntilFilled	boolean	false	Whether or not to keep re-triggering until filled
//orderType	string	market	Values are market and limit
func parseOrderFtx(order *model.Order, item map[string]interface{}) {
	if order == nil || item == nil {
		return
	}
	if item[`createdAt`] != nil {
		order.OrderTime, _ = time.Parse(time.RFC3339, item[`createdAt`].(string))
	}
	if item[`filledSize`] != nil {
		order.DealAmount, _ = item[`filledSize`].(json.Number).Float64()
	}
	if item[`id`] != nil {
		order.OrderId = item[`id`].(json.Number).String()
	}
	if item[`market`] != nil {
		order.Symbol = model.GetStandardSymbol(model.Ftx, item[`market`].(string))
	}
	if item[`price`] != nil {
		order.Price, _ = item[`price`].(json.Number).Float64()
	}
	if item[`avgFillPrice`] != nil {
		order.DealPrice, _ = item[`avgFillPrice`].(json.Number).Float64()
	}
	if item[`side`] != nil {
		order.OrderSide = strings.ToLower(item[`side`].(string))
	}
	if item[`size`] != nil {
		order.Amount, _ = item[`size`].(json.Number).Float64()
	}
	if item[`type`] != nil {
		order.OrderType = strings.ToLower(item[`type`].(string))
	}
	if item[`status`] != nil {
		order.Status = model.GetOrderStatus(model.Ftx, item[`status`].(string))
	}
	if item[`triggerPrice`] != nil {
		order.Price, _ = item[`triggerPrice`].(json.Number).Float64()
	}
	if item[`triggeredAt`] != nil {
		order.OrderUpdateTime, _ = time.Parse(time.RFC3339, item[`triggeredAt`].(string))
	}

	if order.Status != model.CarryStatusSuccess && order.Status != model.CarryStatusFail {
		order.Status = model.CarryStatusWorking
	}
	if order.DealAmount == 0 || order.DealPrice == 0 {
		order.DealPrice = order.Price
	}
	//order.Amount = order.Amount * order.Price
	//order.DealAmount = order.DealAmount * order.Price
	order.UnfilledQuantity = order.Amount - order.DealAmount
	return
}

//orderType: "limit", "market", "stop", "trailingStop", "takeProfit"
func placeOrderFtx(order *model.Order, key, secret, orderSide, orderType, orderParam, symbol, price,
	amount string) {
	uri := `/orders`
	param := make(map[string]interface{})
	symbol = model.GetDialectSymbol(model.Ftx, symbol)
	param[`market`] = symbol
	postData := make(map[string]interface{})
	postData[`market`] = symbol
	postData[`side`] = orderSide
	postData[`size`], _ = strconv.ParseFloat(amount, 64)
	postData[`type`] = orderType
	if orderType == `limit` || orderType == `market` {
		postData[`price`], _ = strconv.ParseFloat(price, 64)
		if orderParam == model.PostOnly {
			postData[`postOnly`] = true
		}
	} else if orderType == `stop` || orderType == `trailingStop` || orderType == `takeProfit` {
		uri = `/conditional_orders`
		postData[`triggerPrice`], _ = strconv.ParseFloat(price, 64)
	}
	response := SignedRequestFtx(key, secret, `POST`, uri, param, postData)
	util.Notice(`place ftx: ` + string(response))
	orderJson, err := util.NewJSON(response)
	if err == nil {
		success := orderJson.Get(`success`).MustBool()
		if success {
			data, _ := orderJson.Get(`result`).Map()
			parseOrderFtx(order, data)
		} else {
			order.Status = model.CarryStatusFail
			order.OrderId = ``
		}
	}
	return
}

func SignedRequestFtx(key, secret, method, path string, param, body map[string]interface{}) []byte {
	if key == `` {
		key = model.AppConfig.FtxKey
	}
	if secret == `` {
		secret = model.AppConfig.FtxSecret
	}
	if body == nil {
		body = make(map[string]interface{})
	}
	u, _ := url.ParseRequestURI(model.AppConfig.RestUrls[model.Ftx])
	u.Path += path
	ts := time.Now().UnixNano() / int64(time.Millisecond)
	hash := hmac.New(sha256.New, []byte(secret))
	bodyStr := string(util.JsonEncodeMapToByte(body))
	q := u.Query()
	for k, v := range param {
		q.Set(k, v.(string))
	}
	u.RawQuery = q.Encode()
	uri := u.Path
	if u.Query().Encode() != `` {
		uri = fmt.Sprintf(`%s?%s`, u.Path, u.Query().Encode())
	}
	if method == `POST` {
		hash.Write([]byte(fmt.Sprintf(`%d%s%s%s`, ts, method, uri, bodyStr)))
	} else {
		hash.Write([]byte(fmt.Sprintf(`%d%s%s`, ts, method, uri)))
		bodyStr = ``
	}
	sign := hex.EncodeToString(hash.Sum(nil))
	headers := map[string]string{`FTX-KEY`: key, `FTX-TS`: strconv.FormatInt(ts, 10), "FTX-SIGN": sign,
		"Content-Type": "application/json"}
	responseBody, _ := util.HttpRequest(method, u.String(), bodyStr, headers, 5)
	return responseBody
}
