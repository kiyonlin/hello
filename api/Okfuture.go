package api

import (
	"bytes"
	"compress/flate"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"hello/model"
	"hello/util"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

var subscribeHandlerOKFuture = func(subscribes []interface{}, subType string) error {
	var err error = nil
	for _, v := range subscribes {
		postData := url.Values{}
		postData.Set("api_key", model.AppConfig.OkexKey)
		var subBook string
		if v == `login` {
			subBook = fmt.Sprintf(`{"event":"login","parameters":{"api_key":"%s","sign":"%s"}}`,
				model.AppConfig.OkexKey, getSign(&postData))
		} else {
			subBook = fmt.Sprintf(`{'event':'addChannel','channel':'%s','parameters':{'api_key':'%s','sign':'%s'}}`,
				v, model.AppConfig.OkexKey, getSign(&postData))
		}
		err = sendToWs(model.OKFUTURE, []byte(subBook))
		if err != nil {
			util.SocketInfo("okfuture can not subscribe " + err.Error())
			return err
		}
	}
	return err
}

//WsAccountServeOKFuture
func _(errHandler ErrHandler) (chan struct{}, error) {
	lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte) {
		if util.GetNow().Unix()-lastPingTime > 30 { // ping okfuture server every 30 seconds
			lastPingTime = util.GetNow().Unix()
			if err := sendToWs(model.OKFUTURE, []byte(`{"event":"ping"}`)); err != nil {
				util.SocketInfo("okfuture server ping client error " + err.Error())
			}
		}
		if len(event) == 0 {
			return
		}
	}
	return WebSocketServe(model.OKFUTURE, model.AppConfig.WSUrls[model.OKFUTURE], model.SubscribeDepth,
		model.GetAccountInfoSubscribe(model.OKFUTURE), subscribeHandlerOKFuture, wsHandler, errHandler)
}

func parseByDepth(bidAsks *model.BidAsk, data interface{}) {
	subscribeData := data.(map[string]interface{})[`data`].(map[string]interface{})
	if subscribeData[`timestamp`] == nil || subscribeData[`asks`] == nil || subscribeData[`bids`] == nil {
		return
	}
	ts, _ := subscribeData[`timestamp`].(json.Number).Int64()
	bidAsks.Ts = int(ts)
	askValues := subscribeData[`asks`].([]interface{})
	bidValues := subscribeData[`bids`].([]interface{})
	bidAsks.Asks = make([]model.Tick, len(askValues))
	bidAsks.Bids = make([]model.Tick, len(bidValues))
	for index, value := range askValues {
		if len(value.([]interface{})) < 2 {
			continue
		}
		price, _ := value.([]interface{})[0].(json.Number).Float64()
		amount, _ := value.([]interface{})[2].(json.Number).Float64()
		bidAsks.Asks[index] = model.Tick{Price: price, Amount: amount}
	}
	for index, value := range bidValues {
		if len(value.([]interface{})) < 2 {
			continue
		}
		price, _ := value.([]interface{})[0].(json.Number).Float64()
		amount, _ := value.([]interface{})[2].(json.Number).Float64()
		bidAsks.Bids[index] = model.Tick{Price: price, Amount: amount}
	}
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
		resultJson, err := util.NewJSON([]byte(event))
		if err != nil {
			return
		}
		data, _ := resultJson.Array()
		if len(data) <= 0 {
			return
		}
		bidAsks := &model.BidAsk{}
		bidAsks.Asks = model.Ticks{}
		bidAsks.Bids = model.Ticks{}
		subscribe := data[0].(map[string]interface{})[`channel`].(string)
		if !strings.Contains(subscribe, `_`) {
			return
		}
		symbol := model.GetSymbol(model.OKFUTURE, subscribe)
		parseByDepth(bidAsks, data[0])
		sort.Sort(bidAsks.Asks)
		sort.Sort(sort.Reverse(bidAsks.Bids))
		if markets.SetBidAsk(symbol, model.OKFUTURE, bidAsks) {
			for function, handler := range model.GetFunctions(model.OKFUTURE, symbol) {
				go handler(model.OKFUTURE, symbol, function)
			}
		}
	}
	return WebSocketServe(model.OKFUTURE, model.AppConfig.WSUrls[model.OKFUTURE], model.SubscribeDepth,
		model.GetWSSubscribes(model.OKFUTURE, model.SubscribeDepth), subscribeHandlerOKFuture, wsHandler, errHandler)
}

func getSymbol(contractSymbol string) (symbol string) {
	index := strings.Index(contractSymbol, `_`)
	if index == -1 {
		return ``
	}
	return contractSymbol[0:index] + `_usd`
}

func getContractType(contractSymbol string) (contractType string) {
	index := strings.Index(contractSymbol, `_`)
	if index > 0 && index < len(contractSymbol) {
		return contractSymbol[index+1:]
	}
	return ``
}

// orderSide:  1:开多 2:开空 3:平多 4:平空
// orderType: 是否为对手价 0:不是 1:是
func placeOrderOkfuture(order *model.Order, orderSide, orderType, symbol, price, amount string) {
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
	switch orderType {
	case model.OrderTypeLimit:
		orderType = `0`
	case model.OrderTypeMarket:
		orderType = `1`
	default:
		util.Notice(`wrong order type for placeOrderOkfuture ` + orderType)
		return
	}
	postData := url.Values{}
	postData.Set(`price`, price)
	postData.Set(`symbol`, getSymbol(symbol))
	postData.Set(`contract_type`, getContractType(symbol))
	postData.Set(`amount`, amount)
	postData.Set(`type`, orderSide)
	postData.Set(`match_price`, orderType)
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+"/future_trade.do",
		&postData, 100)
	resultJson, err := util.NewJSON(responseBody)
	if err == nil {
		//result, _ := resultJson.Get(`result`).Bool()
		oid, _ := resultJson.Get(`order_id`).Int64()
		order.OrderId = strconv.FormatInt(oid, 10)
		util.Notice(fmt.Sprintf(`[挂单Ok future] %s side: %s type: %s price: %s amount: %s order id: %s 返回%s`,
			symbol, orderSide, orderType, price, amount, order.OrderId, string(responseBody)))
	}
}

//func QueryPendingOrderAmount(symbol string) (orderAmount int, err error) {
//	postData := url.Values{}
//	postData.Set(`symbol`, getSymbol(symbol))
//	postData.Set(`contract_type`, getContractType(symbol))
//	postData.Set(`order_id`, `-1`)
//	postData.Set(`status`, `1`)
//	postData.Set(`current_page`, `1`)
//	postData.Set(`page_length`, `50`)
//	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+"/future_order_info.do",
//		&postData, 100)
//	orderJson, err := util.NewJSON(responseBody)
//	if err != nil {
//		return 0, err
//	}
//	orderJson = orderJson.Get(`orders`)
//	if orderJson != nil {
//		orders, _ := orderJson.Array()
//		return len(orders), nil
//	}
//	return 0, nil
//}

//status: 订单状态(0等待成交 1部分成交 2全部成交 -1撤单 4撤单处理中 5撤单中)
func queryOrderOkfuture(symbol string, orderId string) (dealAmount, dealPrice float64, status string) {
	postData := url.Values{}
	postData.Set(`symbol`, getSymbol(symbol))
	postData.Set(`contract_type`, getContractType(symbol))
	//postData.Set(`status`, `1`)
	//postData.Set(`page_length`, `50`)
	//postData.Set(`current_page`, `1`)
	postData.Set(`order_id`, orderId)
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+"/future_order_info.do",
		&postData, 100)
	orderJson, err := util.NewJSON(responseBody)
	if err != nil {
		return 0, -1, err.Error()
	}
	orders, _ := orderJson.Get(`orders`).Array()
	for _, value := range orders {
		order, _ := value.(map[string]interface{})[`order_id`].(json.Number).Int64()
		if strconv.FormatInt(order, 10) == orderId {
			dealAmount, _ = value.(map[string]interface{})[`deal_amount`].(json.Number).Float64()
			contractPrice, _ := value.(map[string]interface{})[`price_avg`].(json.Number).Float64()
			statusCode, _ := value.(map[string]interface{})[`status`].(json.Number).Float64()
			status = model.GetOrderStatus(model.OKFUTURE, strconv.FormatFloat(statusCode, 'f', 0, 64))
			return dealAmount, contractPrice, status
		}
	}
	return dealAmount, dealPrice, model.CarryStatusFail
}

func cancelOrderOkfuture(symbol string, orderId string) (result bool, errCode, msg string) {
	//future_cancel.do
	postData := url.Values{}
	postData.Set(`symbol`, getSymbol(symbol))
	postData.Set(`order_id`, orderId)
	postData.Set(`contract_type`, getContractType(symbol))
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+"/future_cancel.do",
		&postData, 200)
	orderJson, err := util.NewJSON(responseBody)
	if err == nil {
		result, _ = orderJson.Get(`result`).Bool()
		msg, _ = orderJson.Get(`order_id`).String()
		return result, ``, msg
	}
	return false, ``, err.Error()
}

func GetAccountOkfuture(accounts *model.Accounts) (err error) {
	postData := url.Values{}
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+
		"/future_userinfo.do", &postData, 400)
	accountJson, err := util.NewJSON(responseBody)
	if err != nil || accountJson.Get(`info`) == nil {
		return errors.New(`fail to get unreal profit`)
	}
	accountMap, err := accountJson.Get(`info`).Map()
	if err == nil {
		for currency, value := range accountMap {
			if value == nil {
				return errors.New(`no account data for ` + currency)
			}
			accountRight, _ := value.(map[string]interface{})[`account_rights`]
			profitReal, _ := value.(map[string]interface{})[`profit_real`]
			profitUnreal, _ := value.(map[string]interface{})[`profit_unreal`]
			account := accounts.GetAccount(model.OKFUTURE, currency)
			if account == nil {
				account = &model.Account{Market: model.OKFUTURE, Currency: currency}
			}
			account.Free, _ = accountRight.(json.Number).Float64()
			account.ProfitReal, _ = profitReal.(json.Number).Float64()
			account.ProfitUnreal, _ = profitUnreal.(json.Number).Float64()
			accounts.SetAccount(model.OKFUTURE, currency, account)
		}
	}
	return nil
}

//func GetAllHoldings(currency string) (allHoldings float64, err error) {
//	index := strings.Index(currency, `_`)
//	if index > 0 {
//		currency = currency[0:index]
//	}
//	futureSymbols := []string{currency + `_this_week`, currency + `_next_week`, currency + `_quarter`}
//	for _, value := range futureSymbols {
//		futureAccount, positionErr := GetPositionOkfuture(model.OKFUTURE, value)
//		if futureAccount == nil || positionErr != nil {
//			return 0, errors.New(`account or position nil`)
//		}
//		allHoldings += futureAccount.OpenedShort
//		time.Sleep(time.Millisecond * 500)
//	}
//	return allHoldings, nil
//}

//func GetPositionOkfuture(market, symbol string) (futureAccount *model.FutureAccount, err error) {
//	postData := url.Values{}
//	postData.Set(`symbol`, getSymbol(symbol))
//	postData.Set(`contract_type`, getContractType(symbol))
//	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+
//		"/future_position.do", &postData, 200)
//	orderJson, err := util.NewJSON(responseBody)
//	if err != nil {
//		return nil, err
//	}
//	result, _ := orderJson.Get(`result`).Bool()
//	if !result {
//		return nil, errors.New(`result false`)
//	}
//	holdings, _ := orderJson.Get(`holding`).Array()
//	futureAccount = &model.FutureAccount{Market: market, Symbol: symbol, OpenedShort: 0, OpenedLong: 0}
//	if len(holdings) > 0 {
//		value := holdings[0].(map[string]interface{})
//		openLong, _ := value[`buy_available`].(json.Number).Float64()
//		openShort, _ := value[`sell_available`].(json.Number).Float64()
//		futureAccount = &model.FutureAccount{Market: market, Symbol: symbol, OpenedLong: openLong, OpenedShort: openShort}
//	}
//	return futureAccount, nil
//}

// GetKLineOkexFuture
func _(symbol, timeSlot string, size int64) []*model.KLinePoint {
	postData := url.Values{}
	symbol = getSymbol(symbol)
	contractType := getContractType(symbol)
	postData.Set(`symbol`, symbol)
	postData.Set(`type`, timeSlot)
	postData.Set(`contract_type`, contractType)
	postData.Set(`size`, strconv.FormatInt(size, 10))
	responseBody := sendSignRequest(`GET`, model.AppConfig.RestUrls[model.OKEX]+"/future_kline.do",
		&postData, 100)
	dataJson, err := util.NewJSON(responseBody)
	if err != nil || dataJson == nil {
		return nil
	}
	data, _ := dataJson.Array()
	priceKLine := make([]*model.KLinePoint, len(data))
	for key, value := range data {
		ts, _ := value.([]interface{})[0].(json.Number).Int64()
		price, _ := value.([]interface{})[4].(json.Number).Float64()
		high, _ := value.([]interface{})[2].(json.Number).Float64()
		low, _ := value.([]interface{})[3].(json.Number).Float64()
		priceKLine[key] = &model.KLinePoint{TS: ts, EndPrice: price, HighPrice: high, LowPrice: low}
	}
	return priceKLine
}
