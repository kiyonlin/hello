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
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

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
		GetWSSubscribes(model.OKFUTURE, `quarter,bi_quarter`), subscribeHandlerOKFuture, wsHandler, errHandler)
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

func getSymbol(contractSymbol string) (symbol string) {
	index := strings.Index(contractSymbol, `_`)
	if index == -1 {
		return ``
	}
	return contractSymbol[0:index] + `_usd`
}

func GetInstrumentOkFuture(symbol string, alias string) (instrumentId string) {
	responseBody := SignedRequestOKSwap(model.AppConfig.OkexKey, model.AppConfig.OkexSecret, `GET`,
		`/api/futures/v3/instruments`, nil)
	instrumentJson, err := util.NewJSON(responseBody)
	if err == nil {
		symbol = strings.ToUpper(symbol)
		for _, item := range instrumentJson.MustArray() {
			future := item.(map[string]interface{})
			if future[`underlying`] != nil && future[`underlying`].(string) == symbol &&
				future[`alias`] != nil && future[`alias`] == alias {
				model.SetOkFuturesSymbol(symbol, alias, future[`instrument_id`].(string))
				return future[`instrument_id`].(string)
			}
		}
	}
	return
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
	postData := map[string]interface{}{`price`: price, `symbol`: getSymbol(symbol), `amount`: amount,
		`type`: orderSide, `match_price`: orderType}
	SignedRequestOKSwap(model.AppConfig.OkexKey, model.AppConfig.OkexSecret, http.MethodPost,
		`/api/futures/v3/order`, postData)
	fmt.Println(order.OrderId)
	//responseBody := SignedRequestOKSwap(model.AppConfig.OkexKey, model.AppConfig.OkexSecret, `POST`,
	//	model.AppConfig.RestUrls[model.OKFUTURE]+"/future_trade.do", postData)
	//resultJson, err := util.NewJSON(responseBody)
	//if err == nil {
	//	//result, _ := resultJson.Get(`result`).Bool()
	//	oid, _ := resultJson.Get(`order_id`).Int64()
	//	order.OrderId = strconv.FormatInt(oid, 10)
	//	util.Notice(fmt.Sprintf(`[挂单Ok future] %s side: %s type: %s price: %s amount: %s order id: %s 返回%s`,
	//		symbol, orderSide, orderType, price, amount, order.OrderId, string(responseBody)))
	//}
}

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
