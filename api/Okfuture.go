package api

import (
	"bytes"
	"compress/flate"
	"fmt"
	"hello/model"
	"hello/util"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var okfutureLock sync.Mutex
var okFutureInstrument = make(map[string]map[string]string) // symbol - (quarter;bi_quarter) - instrument

func GetOkFuturesInstrument(symbol, alias string) (instrument string) {
	if okFutureInstrument == nil || okFutureInstrument[symbol] == nil {
		SetInstrumentOkFuture()
	}
	if okFutureInstrument == nil || okFutureInstrument[symbol] == nil {
		util.Notice(`fatal error: can not get okfuture instrument`)
		return ``
	}
	return okFutureInstrument[symbol][alias]
}

func SetOkFuturesInstrument(symbol, alias, instrument string) {
	okfutureLock.Lock()
	defer okfutureLock.Unlock()
	if okFutureInstrument[symbol] == nil {
		okFutureInstrument[symbol] = make(map[string]string)
	}
	okFutureInstrument[symbol][alias] = instrument
}

var subscribeHandlerOKFuture = func(subscribes []interface{}, subType string) (err error) {
	subscribe := ``
	for _, item := range subscribes {
		if item == nil {
			continue
		}
		if subscribe == `` {
			subscribe = fmt.Sprintf(`"%s"`, item.(string))
		} else {
			subscribe = fmt.Sprintf(`%s,"%s"`, subscribe, item.(string))
		}
	}
	if err = sendToWs(
		model.OKFUTURE, []byte(fmt.Sprintf(`{"op": "subscribe", "args": [%s]}`, subscribe))); err != nil {
		util.SocketInfo("okfuture can not subscribe " + subscribe + err.Error())
	}
	return
}

func parseTickByOkFuture(data map[string]interface{}) (bidAsks *model.BidAsk, symbol string) {
	if data[`timestamp`] == nil || data == nil || data[`asks`] == nil || data[`bids`] == nil ||
		data[`instrument_id`] == nil {
		return
	}
	bidAsks = &model.BidAsk{}
	bidAsks.Asks = make([]model.Tick, len(data[`asks`].([]interface{})))
	bidAsks.Bids = make([]model.Tick, len(data[`bids`].([]interface{})))
	symbol = data[`instrument_id`].(string)
	if len(strings.Split(symbol, `-`)) > 1 {
		symbol = strings.ToLower(symbol[0:strings.LastIndex(symbol, `-`)])
	}
	for i, item := range data[`asks`].([]interface{}) {
		value := item.([]interface{})
		if len(value) < 2 {
			continue
		}
		price, _ := strconv.ParseFloat(value[0].(string), 64)
		amount, _ := strconv.ParseFloat(value[1].(string), 64)
		bidAsks.Asks[i] = model.Tick{Price: price, Amount: amount, Side: model.OrderSideSell, Symbol: symbol}
	}
	for i, item := range data[`bids`].([]interface{}) {
		value := item.([]interface{})
		if len(value) < 2 {
			continue
		}
		price, _ := strconv.ParseFloat(value[0].(string), 64)
		amount, _ := strconv.ParseFloat(value[1].(string), 64)
		bidAsks.Bids[i] = model.Tick{Price: price, Amount: amount, Side: model.OrderSideBuy, Symbol: symbol}
	}
	ts, _ := time.Parse(time.RFC3339, data[`timestamp`].(string))
	bidAsks.Ts = int(ts.UnixNano() / int64(time.Millisecond))
	return
}

func WsDepthServeOKFuture(markets *model.Markets, errHandler ErrHandler) (chan struct{}, error) {
	lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte) {
		if util.GetNow().Unix()-lastPingTime > 20 { // ping okfuture server every 30 seconds
			lastPingTime = util.GetNow().Unix()
			if err := sendToWs(model.OKFUTURE, []byte(`{"event":"ping"}`)); err != nil {
				util.SocketInfo("okfuture server ping client error " + err.Error())
			}
		}
		if len(event) == 0 {
			return
		}
		var out bytes.Buffer
		reader := flate.NewReader(bytes.NewReader(event))
		_, _ = io.Copy(&out, reader)
		event = out.Bytes()
		resultJson, err := util.NewJSON(event)
		if err != nil {
			return
		}
		table := resultJson.Get(`table`).MustString()
		switch table {
		case "futures/depth5":
			array := resultJson.Get(`data`).MustArray()
			if array != nil && len(array) > 0 {
				depthHandlerOkFuture(markets, array[0].(map[string]interface{}))
			}
		}
	}
	return WebSocketServe(model.OKFUTURE, model.AppConfig.WSUrls[model.OKFUTURE], model.SubscribeDepth,
		GetWSSubscribes(model.OKFUTURE, ``), subscribeHandlerOKFuture, wsHandler, errHandler)
}

func getCurrentInstrumentOkfuture(symbol string) (currentInstrument string) {
	instrument := GetOkFuturesInstrument(symbol, `quarter`)
	instrumentNext := GetOkFuturesInstrument(symbol, `bi_quarter`)
	index := strings.LastIndex(instrument, `-`)
	if index == -1 {
		return ``
	}
	year, _ := strconv.ParseInt(`20`+instrument[index+1:index+3], 10, 64)
	month, _ := strconv.ParseInt(instrument[index+3:index+5], 10, 64)
	day, _ := strconv.ParseInt(instrument[index+5:index+7], 10, 64)
	today := time.Now().In(time.UTC)
	duration, _ := time.ParseDuration(`312h`)
	days13 := today.Add(duration)
	date := time.Date(int(year), time.Month(month), int(day), 0, 0, 0, 0, today.Location())
	if today.After(date) {
		util.Notice(`future go cross ` + symbol + date.String())
		SetInstrumentOkFuture()
	}
	if days13.Before(date) {
		return instrument
	} else {
		return instrumentNext
	}
}

func depthHandlerOkFuture(markets *model.Markets, data map[string]interface{}) {
	bidAsks, symbol := parseTickByOkFuture(data)
	if bidAsks == nil {
		return
	}
	sort.Sort(bidAsks.Asks)
	sort.Sort(sort.Reverse(bidAsks.Bids))
	if markets.SetBidAsk(symbol, model.OKFUTURE, bidAsks) {
		for function, handler := range model.GetFunctions(model.OKFUTURE, symbol) {
			settings := model.GetSetting(function, model.OKFUTURE, symbol)
			for _, setting := range settings {
				go handler(setting)
			}
		}
	}
}

func SetInstrumentOkFuture() {
	responseBody := SignedRequestOKSwap(model.AppConfig.OkexKey, model.AppConfig.OkexSecret, `GET`,
		`/api/futures/v3/instruments`, nil)
	instrumentJson, err := util.NewJSON(responseBody)
	if err == nil {
		//symbol = strings.ToUpper(symbol)
		for _, item := range instrumentJson.MustArray() {
			future := item.(map[string]interface{})
			if future[`underlying`] != nil && future[`alias`] != nil {
				SetOkFuturesInstrument(strings.ToLower(future[`underlying`].(string)),
					future[`alias`].(string), future[`instrument_id`].(string))
			}
		}
	}
}

func parseAccountOkfuture(account *model.Account, data map[string]interface{}) {
	if data[`equity`] != nil {
		account.Free, _ = strconv.ParseFloat(data[`equity`].(string), 64)
	}
	if data[`currency`] != nil {
		account.Currency = data[`currency`].(string)
	}
	if data[`margin`] != nil {
		account.Margin, _ = strconv.ParseFloat(data[`margin`].(string), 64)
	}
	if data[`realized_pnl`] != nil {
		account.ProfitReal, _ = strconv.ParseFloat(data[`realized_pnl`].(string), 64)
	}
	if data[`margin_for_unfilled`] != nil {
		account.ProfitUnreal, _ = strconv.ParseFloat(data[`margin_for_unfilled`].(string), 64)
	}
	if data[`underlying`] != nil {
		account.Currency = strings.ToLower(data[`underlying`].(string))
	}
}

func GetAccountOkfuture(accounts *model.Accounts) (err error) {
	responseBody := SignedRequestOKSwap(``, ``, `GET`, "/api/futures/v3/accounts", nil)
	accountJson, err := util.NewJSON(responseBody)
	if err != nil {
		return
	}
	items := accountJson.Get(`info`).MustMap()
	for key, value := range items {
		account := accounts.GetAccount(model.OKFUTURE, key)
		if account == nil {
			account = &model.Account{Market: model.OKFUTURE, Ts: util.GetNowUnixMillion()}
		}
		data := value.(map[string]interface{})
		parseAccountOkfuture(account, data)
		accounts.SetAccount(model.OKFUTURE, account.Currency, account)
	}
	return nil
}

// orderSide:  1:开多 2:开空 3:平多 4:平空
// orderType: 是否为对手价 0:不是 1:是
func placeOrderOkfuture(order *model.Order, orderSide, orderType, instrument, price, size string) {
	switch orderSide {
	case model.OrderSideBuy:
		orderSide = `1`
	case model.OrderSideSell:
		orderSide = `2`
	case model.OrderSideLiquidateLong:
		orderSide = `3`
	case model.OrderSideLiquidateShort:
		orderSide = `4`
	default:
		util.Notice(`wrong order side for placeOrderOkfuture ` + orderSide)
		return
	}
	postData := map[string]interface{}{`instrument_id`: instrument, `type`: orderSide, `size`: size}
	algo := `order`
	switch orderType {
	case model.OrderTypeLimit:
		postData[`price`] = price
		postData[`order_type`] = `0`
	case model.PostOnly:
		postData[`order_type`] = `1`
		postData[`price`] = price
	case model.OrderTypeMarket:
		postData[`order_type`] = `4`
		postData[`price`] = price
	case model.OrderTypeStop:
		algo = `order_algo`
		postData[`order_type`] = `1`
		postData[`algo_type`] = `2`
		postData[`trigger_price`] = price
	default:
		util.Notice(`wrong order type for placeOrderOkfuture ` + orderType)
		return
	}
	responseBody := SignedRequestOKSwap(model.AppConfig.OkexKey, model.AppConfig.OkexSecret, http.MethodPost,
		`/api/futures/v3/`+algo, postData)
	resultJson, err := util.NewJSON(responseBody)
	if err == nil {
		result, _ := resultJson.Get(`result`).Bool()
		if !result {
			order.Status = model.CarryStatusFail
		}
		if orderType == model.OrderTypeStop {
			order.OrderId = resultJson.Get(`algo_id`).MustString()
		} else {
			order.OrderId = resultJson.Get(`order_id`).MustString()
		}
		util.Notice(fmt.Sprintf(`[挂单Ok future] %s side: %s type: %s price: %s size: %s order id: %s 返回%s`,
			instrument, orderSide, orderType, price, size, order.OrderId, string(responseBody)))
	}
}

func queryOrdersOkfuture(key, secret, instrument string) (orders []*model.Order) {
	param := url.Values{}
	param.Set(`order_type`, `1`)
	param.Set(`status`, `1`)
	responseBody := SignedRequestOKSwap(key, secret, `GET`,
		fmt.Sprintf(`/api/futures/v3/order_algo/%s?%s`, instrument, param.Encode()), nil)
	orderJson, _ := util.NewJSON(responseBody)
	ids := orderJson.MustArray()
	for _, value := range ids {
		item := value.(map[string]interface{})
		id := item[`algo_ids`].(string)
		if id == `` {
			continue
		}
		result, code, msg := cancelOrderOkfuture(instrument, id, model.OrderTypeStop)
		fmt.Println(fmt.Sprintf(`%v %s %s`, result, code, msg))
		time.Sleep(time.Second)
	}
	return
}

//status: 订单状态(0等待成交 1部分成交 2全部成交 -1撤单 4撤单处理中 5撤单中)
func queryOrderOkfuture(instrument, orderType, orderId string) (dealAmount, dealPrice float64, status string) {
	if orderType == model.OrderTypeStop {
		param := url.Values{}
		param.Set(`order_type`, `1`)
		param.Set(`algo_id`, orderId)
		responseBody := SignedRequestOKSwap(``, ``, `GET`,
			fmt.Sprintf(`/api/futures/v3/order_algo/%s?%s`, instrument, param.Encode()), nil)
		orderJson, err := util.NewJSON(responseBody)
		if err != nil {
			return 0, -1, err.Error()
		}
		fmt.Println(string(responseBody))
		value := orderJson.MustArray()
		for _, item := range value {
			data := item.(map[string]interface{})
			if data[`algo_ids`] != nil && data[`algo_ids`].(string) == orderId {
				if data[`trigger_price`] != nil {
					dealPrice, _ = strconv.ParseFloat(data[`trigger_price`].(string), 64)
				}
				if data[`real_amount`] != nil {
					dealAmount, _ = strconv.ParseFloat(data[`real_amount`].(string), 64)
				}
				if data[`status`] != nil {
					if data[`status`] == `1` || data[`status`] == `4` {
						status = model.CarryStatusWorking
					} else if data[`status`] == `2` {
						status = model.CarryStatusSuccess
					} else if data[`status`] == `3` || data[`status`] == `5` || data[`status`] == `6` {
						status = model.CarryStatusFail
					}
				}
			}
		}
	} else {
		responseBody := SignedRequestOKSwap(``, ``, `GET`,
			fmt.Sprintf(`/api/futures/v3/orders/%s/%s`, instrument, orderId), nil)
		orderJson, err := util.NewJSON(responseBody)
		if err != nil {
			return 0, -1, err.Error()
		}
		data := orderJson.MustMap()
		if data[`filled_qty`] != nil {
			dealAmount, _ = strconv.ParseFloat(data[`filled_qty`].(string), 64)
		}
		if data[`price_avg`] != nil {
			dealPrice, _ = strconv.ParseFloat(data[`price_avg`].(string), 64)
		}
		if data[`instrument_id`] != nil && data[`price`] != nil && dealPrice == 0 {
			dealPrice, _ = strconv.ParseFloat(data[`price`].(string), 64)
		}
		if data[`state`] != nil {
			status = model.GetOrderStatus(model.OKFUTURE, data[`state`].(string))
		}
	}
	return
}

func cancelOrderOkfuture(instrument string, orderId string, orderType string) (result bool, errCode, msg string) {
	var responseBody []byte
	if orderType == model.OrderTypeStop {
		postData := make(map[string]interface{})
		postData[`instrument_id`] = instrument
		postData[`order_type`] = `1`
		orderIds := []string{orderId}
		postData[`algo_ids`] = orderIds
		responseBody = SignedRequestOKSwap(``, ``, `POST`,
			`/api/futures/v3/cancel_algos`, postData)
	} else {
		responseBody = SignedRequestOKSwap(``, ``, `POST`,
			fmt.Sprintf(`/api/futures/v3/cancel_order/%s/%s`, instrument, orderId), nil)
	}
	orderJson, err := util.NewJSON(responseBody)
	if err == nil {
		result, _ = orderJson.Get(`result`).Bool()
		msg, _ = orderJson.Get(`error_message`).String()
		return result, ``, msg
	}
	return false, ``, err.Error()
}

func getCandlesOkfuture(key, secret, symbol, instrument, binSize string, start, end time.Time) (
	candles map[string]*model.Candle) {
	candles = make(map[string]*model.Candle)
	param := make(map[string]interface{})
	if binSize == `1d` {
		param[`granularity`] = `86400`
	}
	duration, _ := time.ParseDuration(`-24h`)
	start = start.Add(duration)
	duration, _ = time.ParseDuration(`-48h`)
	end = end.Add(duration)
	param[`start`] = fmt.Sprintf(`%d-%d-%dT%d:%d:%dZ`, start.Year(), start.Month(), start.Day(), 16, 0, 0)
	param[`end`] = fmt.Sprintf(`%d-%d-%dT%d:%d:%dZ`, end.Year(), end.Month(), end.Day(), 16, 0, 0)
	path := fmt.Sprintf(`/api/futures/v3/instruments/%s/candles?%s`, instrument, util.ComposeParams(param))
	response := SignedRequestOKSwap(key, secret, `GET`, path, nil)
	duration, _ = time.ParseDuration(`8h`)
	candleJson, err := util.NewJSON(response)
	if err == nil {
		candleJsons := candleJson.MustArray()
		for _, value := range candleJsons {
			item := value.([]interface{})
			if len(item) < 5 {
				continue
			}
			candle := &model.Candle{Market: model.OKFUTURE, Symbol: symbol, Period: binSize}
			candle.PriceOpen, _ = strconv.ParseFloat(item[1].(string), 64)
			candle.PriceHigh, _ = strconv.ParseFloat(item[2].(string), 64)
			candle.PriceLow, _ = strconv.ParseFloat(item[3].(string), 64)
			candle.PriceClose, _ = strconv.ParseFloat(item[4].(string), 64)
			start, _ := time.Parse(time.RFC3339, item[0].(string))
			start = start.Add(duration)
			candle.UTCDate = start.Format(time.RFC3339)[0:10]
			candles[candle.UTCDate] = candle
		}
	}
	return
}
