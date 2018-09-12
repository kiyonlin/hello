package api

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"hello/model"
	"hello/util"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

var subscribeHandlerOKFuture = func(subscribes []string, conn *websocket.Conn) error {
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
		err = conn.WriteMessage(websocket.TextMessage, []byte(subBook))
		if err != nil {
			util.SocketInfo("okfuture can not subscribe " + err.Error())
			return err
		}
	}
	return err
}

func WsAccountServeOKFuture(errHandler ErrHandler) (chan struct{}, error) {
	lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte, conn *websocket.Conn) {
		if util.GetNow().Unix()-lastPingTime > 30 { // ping okfuture server every 30 seconds
			lastPingTime = util.GetNow().Unix()
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"event":"ping"}`)); err != nil {
				util.SocketInfo("okfuture server ping client error " + err.Error())
			}
		}
		if len(event) == 0 {
			return
		}
	}
	return WebSocketServe(model.AppConfig.WSUrls[model.OKFUTURE],
		model.GetAccountInfoSubscribe(model.OKFUTURE), subscribeHandlerOKFuture, wsHandler, errHandler)
}

func WsDepthServeOKFuture(markets *model.Markets, carryHandlers []CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
	lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte, conn *websocket.Conn) {
		if util.GetNow().Unix()-lastPingTime > 10 { // ping okfuture server every 30 seconds
			lastPingTime = util.GetNow().Unix()
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"event":"ping"}`)); err != nil {
				util.SocketInfo("okfuture server ping client error " + err.Error())
			}
		}
		if len(event) == 0 {
			return
		}
		subJson, err := util.NewJSON([]byte(event))
		if err != nil {
			return
		}
		data, _ := subJson.Array()
		for _, value := range data {
			bidAsks := &model.BidAsk{}
			bidAsks.Asks = model.Ticks{}
			bidAsks.Bids = model.Ticks{}
			bidMap := make(map[float64]*model.Tick)
			askMap := make(map[float64]*model.Tick)
			subscribe := value.(map[string]interface{})[`channel`].(string)
			if !strings.Contains(subscribe, `_`) {
				return
			}
			symbol := model.GetSymbol(model.OKFUTURE, subscribe)
			if markets.BidAsks[symbol][model.OKFUTURE] != nil {
				bidMap = markets.BidAsks[symbol][model.OKFUTURE].Bids.GetMap()
				askMap = markets.BidAsks[symbol][model.OKFUTURE].Asks.GetMap()
			}
			subscribeData := value.(map[string]interface{})[`data`].(map[string]interface{})
			if subscribeData[`timestamp`] == nil || subscribeData[`asks`] == nil || subscribeData[`bids`] == nil {
				continue
			}
			ts, _ := subscribeData[`timestamp`].(json.Number).Int64()
			bidAsks.Ts = int(ts)
			asks := subscribeData[`asks`].([]interface{})
			bids := subscribeData[`bids`].([]interface{})
			for _, ask := range asks {
				if len(ask.([]interface{})) < 2 {
					continue
				}
				price, _ := ask.([]interface{})[0].(json.Number).Float64()
				amount, _ := ask.([]interface{})[1].(json.Number).Float64()
				if amount == 0 {
					delete(askMap, price)
				} else {
					askMap[price] = &model.Tick{Price: price, Amount: amount}
				}
			}
			for _, bid := range bids {
				if len(bid.([]interface{})) < 2 {
					continue
				}
				price, _ := bid.([]interface{})[0].(json.Number).Float64()
				amount, _ := bid.([]interface{})[1].(json.Number).Float64()
				if amount == 0 {
					delete(bidMap, price)
				} else {
					bidMap[price] = &model.Tick{Price: price, Amount: amount}
				}
			}
			bidAsks.Bids = model.GetTicks(bidMap)
			bidAsks.Asks = model.GetTicks(askMap)
			sort.Sort(bidAsks.Asks)
			sort.Sort(sort.Reverse(bidAsks.Bids))
			if markets.SetBidAsk(symbol, model.OKFUTURE, bidAsks) {
				for _, handler := range carryHandlers {
					handler(symbol, model.OKFUTURE)
				}
			}
		}
	}
	return WebSocketServe(model.AppConfig.WSUrls[model.OKFUTURE],
		model.GetDepthSubscribes(model.OKFUTURE), subscribeHandlerOKFuture, wsHandler, errHandler)
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
func placeOrderOkfuture(orderSide, orderType, symbol, price, amount string) (orderId, errCode string) {
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
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+"/future_trade.do", &postData)
	resultJson, err := util.NewJSON(responseBody)
	if err == nil {
		//result, _ := resultJson.Get(`result`).Bool()
		oid, _ := resultJson.Get(`order_id`).Int64()
		orderId = strconv.FormatInt(oid, 10)
		util.Notice(fmt.Sprintf(`[挂单Ok future] %s side: %s type: %s price: %s amount: %s order id %s errCode: %s 返回%s`,
			symbol, orderSide, orderType, price, amount, orderId, errCode, string(responseBody)))
		return orderId, ``
	}
	return orderId, errCode
}

func QueryPendingOrderAmount(symbol string) (orderAmount int, err error){
	postData := url.Values{}
	postData.Set(`symbol`, getSymbol(symbol))
	postData.Set(`contract_type`, getContractType(symbol))
	postData.Set(`order_id`, `-1`)
	postData.Set(`status`, `1`)
	postData.Set(`current_page`, `1`)
	postData.Set(`page_length`, `50`)
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+"/future_order_info.do", &postData)
	fmt.Println(string(responseBody))
	orderJson, err := util.NewJSON(responseBody)
	if err != nil {
		return 0, err
	}
	orderJson = orderJson.Get(`orders`)
	if orderJson != nil {
		orders, _ := orderJson.Array()
		return len(orders), nil
	}
	return 0, nil
}

//status: 订单状态(0等待成交 1部分成交 2全部成交 -1撤单 4撤单处理中 5撤单中)
func QueryOrderOkfuture(symbol string, orderId string) (dealAmount, dealPrice float64, status string) {
	postData := url.Values{}
	postData.Set(`symbol`, getSymbol(symbol))
	postData.Set(`contract_type`, getContractType(symbol))
	//postData.Set(`status`, `1`)
	//postData.Set(`page_length`, `50`)
	//postData.Set(`current_page`, `1`)
	postData.Set(`order_id`, orderId)
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+"/future_order_info.do", &postData)
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
	return dealAmount, dealPrice, status
}

func CancelOrderOkfuture(symbol string, orderId string) (result bool, errCode, msg string) {
	//future_cancel.do
	postData := url.Values{}
	postData.Set(`symbol`, getSymbol(symbol))
	postData.Set(`order_id`, orderId)
	postData.Set(`contract_type`, getContractType(symbol))
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+"/future_cancel.do", &postData)
	orderJson, err := util.NewJSON(responseBody)
	if err == nil {
		result, _ = orderJson.Get(`result`).Bool()
		msg, _ = orderJson.Get(`order_id`).String()
		return result, ``, msg
	}
	return false, ``, err.Error()
}

func GetCurrencyOkfuture(currency string) (accountRights, keepDeposit float64) {
	postData := url.Values{}
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKEX]+"/future_userinfo.do", &postData)
	balanceJson, err := util.NewJSON(responseBody)
	if err == nil {
		accountRights, _ = balanceJson.GetPath(`info`, currency, `account_rights`).Float64()
		keepDeposit, _ = balanceJson.GetPath(`info`, currency, `keep_deposit`).Float64()
	}
	return accountRights, keepDeposit
}

func GetAccountOkfuture(symbol string) (accountRight, profit float64, err error) {
	postData := url.Values{}
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+
		"/future_userinfo.do", &postData)
	util.Notice(string(responseBody))
	accountJson, err := util.NewJSON(responseBody)
	if err != nil {
		return 0, 0, errors.New(`fail to get unreal profit`)
	}
	index := strings.Index(symbol, `_`)
	if index == -1 {
		return 0, 0, errors.New(`wrong symbol format`)
	}
	accountRightsJson := accountJson.GetPath(`info`, symbol[0:index], `account_rights`)
	realProfitJson := accountJson.GetPath(`info`, symbol[0:index], `profit_real`)
	if accountRightsJson == nil || realProfitJson == nil {
		return 0, 0, errors.New(`no account data for ` + symbol[0:index])
	}
	accountRights, _ := accountRightsJson.Float64()
	realProfit, _ := realProfitJson.Float64()
	return accountRights, realProfit, nil
}

func GetPositionOkfuture(market, symbol string) (futureAccount *model.FutureAccount, err error) {
	postData := url.Values{}
	postData.Set(`symbol`, getSymbol(symbol))
	postData.Set(`contract_type`, getContractType(symbol))
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+
		"/future_position.do", &postData)
	orderJson, err := util.NewJSON(responseBody)
	if err != nil {
		return nil, err
	}
	result, _ := orderJson.Get(`result`).Bool()
	if !result {
		return nil, errors.New(`result false`)
	}
	holdings, _ := orderJson.Get(`holding`).Array()
	if len(holdings) > 0 {
		value := holdings[0].(map[string]interface{})
		openLong, _ := value[`buy_available`].(json.Number).Float64()
		openShort, _ := value[`sell_available`].(json.Number).Float64()
		futureAccount = &model.FutureAccount{Market: market, Symbol: symbol, OpenedLong: openLong, OpenedShort: openShort}
	}
	return futureAccount, nil
}

func GetKLineOkexFuture(symbol, timeSlot string, size int64) []*model.KLinePoint {
	postData := url.Values{}
	symbol = getSymbol(symbol)
	contractType := getContractType(symbol)
	postData.Set(`symbol`, symbol)
	postData.Set(`type`, timeSlot)
	postData.Set(`contract_type`, contractType)
	postData.Set(`size`, strconv.FormatInt(size, 10))
	responseBody := sendSignRequest(`GET`, model.AppConfig.RestUrls[model.OKEX]+"/future_kline.do", &postData)
	dataJson, _ := util.NewJSON(responseBody)
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
