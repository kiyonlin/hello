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
	"time"
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

func WsDepthServeOKFuture(markets *model.Markets, carryHandlers []CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
	lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte, conn *websocket.Conn) {
		if util.GetNow().Unix()-lastPingTime > 20 { // ping okfuture server every 30 seconds
			lastPingTime = util.GetNow().Unix()
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"event":"ping"}`)); err != nil {
				util.SocketInfo("okfuture server ping client error " + err.Error())
			}
		}
		if len(event) == 0 {
			return
		}
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
			for _, handler := range carryHandlers {
				handler(symbol, model.OKFUTURE)
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
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+"/future_trade.do",
		&postData, 100)
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

func QueryPendingOrderAmount(symbol string) (orderAmount int, err error) {
	postData := url.Values{}
	postData.Set(`symbol`, getSymbol(symbol))
	postData.Set(`contract_type`, getContractType(symbol))
	postData.Set(`order_id`, `-1`)
	postData.Set(`status`, `1`)
	postData.Set(`current_page`, `1`)
	postData.Set(`page_length`, `50`)
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+"/future_order_info.do",
		&postData, 100)
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

func CancelOrderOkfuture(symbol string, orderId string) (result bool, errCode, msg string) {
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

//func GetCurrencyOkfuture(currency string) (accountrights, keepdeposit float64) {
//	postdata := url.values{}
//	responsebody := sendsignrequest(`post`, model.appconfig.resturls[model.okex]+"/future_userinfo.do", &postdata)
//	balancejson, err := util.newjson(responsebody)
//	if err == nil {
//		accountrights, _ = balancejson.getpath(`info`, currency, `account_rights`).float64()
//		keepdeposit, _ = balancejson.getpath(`info`, currency, `keep_deposit`).float64()
//	}
//	return accountrights, keepdeposit
//}

func GetAccountOkfuture(accounts *model.Accounts, currency string) (accountRight, profitReal, profitUnreal float64, err error) {
	postData := url.Values{}
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+
		"/future_userinfo.do", &postData, 400)
	accountJson, err := util.NewJSON(responseBody)
	if err != nil {
		return 0, 0, 0, errors.New(`fail to get unreal profit`)
	}
	index := strings.Index(currency, `_`)
	if index != -1 {
		currency = currency[0:index]
	}
	accountRightsJson := accountJson.GetPath(`info`, currency, `account_rights`)
	realProfitJson := accountJson.GetPath(`info`, currency, `profit_real`)
	unrealProfitJson := accountJson.GetPath(`info`, currency, `profit_unreal`)
	if accountRightsJson == nil || realProfitJson == nil {
		return 0, 0, 0, errors.New(`no account data for ` + currency)
	}
	accountRight, _ = accountRightsJson.Float64()
	profitReal, _ = realProfitJson.Float64()
	profitUnreal, _ = unrealProfitJson.Float64()
	account := accounts.GetAccount(model.OKFUTURE, currency)
	if account == nil {
		account = &model.Account{Market: model.OKFUTURE, Currency: currency}
	}
	account.Free = accountRight
	accounts.SetAccount(model.OKFUTURE, currency, account)
	return accountRight, profitReal, profitUnreal, nil
}

func GetAllHoldings(currency string) (allHoldings float64, err error) {
	index := strings.Index(currency, `_`)
	if index > 0 {
		currency = currency[0:index]
	}
	futureSymbols := []string{currency + `_this_week`, currency + `_next_week`, currency + `_quarter`}
	for _, value := range futureSymbols {
		futureAccount, positionErr := GetPositionOkfuture(model.OKFUTURE, value)
		if futureAccount == nil || positionErr != nil {
			return 0, errors.New(`account or position nil`)
		}
		allHoldings += futureAccount.OpenedShort
		time.Sleep(time.Millisecond * 500)
	}
	return allHoldings, nil
}

func GetPositionOkfuture(market, symbol string) (futureAccount *model.FutureAccount, err error) {
	postData := url.Values{}
	postData.Set(`symbol`, getSymbol(symbol))
	postData.Set(`contract_type`, getContractType(symbol))
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKFUTURE]+
		"/future_position.do", &postData, 200)
	orderJson, err := util.NewJSON(responseBody)
	if err != nil {
		return nil, err
	}
	result, _ := orderJson.Get(`result`).Bool()
	if !result {
		return nil, errors.New(`result false`)
	}
	holdings, _ := orderJson.Get(`holding`).Array()
	futureAccount = &model.FutureAccount{Market: market, Symbol: symbol, OpenedShort: 0, OpenedLong: 0}
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
