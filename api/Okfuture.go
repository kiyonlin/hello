package api

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
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
		postData.Set("api_key", model.ApplicationConfig.OkexKey)
		var subBook string
		if v == `login` {
			subBook = fmt.Sprintf(`{"event":"login","parameters":{"api_key":"%s","sign":"%s"}}`,
				model.ApplicationConfig.OkexKey, getSign(&postData))
		} else {
			subBook = fmt.Sprintf(`{'event':'addChannel','channel':'%s','parameters':{'api_key':'%s','sign':'%s'}}`,
				v, model.ApplicationConfig.OkexKey, getSign(&postData))
		}
		fmt.Println(subBook)
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
		fmt.Println(string(event))
	}
	return WebSocketServe(model.ApplicationConfig.WSUrls[model.OKFUTURE],
		model.GetAccountInfoSubscribe(model.OKFUTURE), subscribeHandlerOKFuture, wsHandler, errHandler)
}

func WsDepthServeOKFuture(markets *model.Markets, carryHandlers []CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
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
		util.Notice(string(event))
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
			symbol := model.GetSymbol(model.OKFUTURE, subscribe)
			if markets.BidAsks[symbol][model.OKFUTURE] != nil {
				bidMap = markets.BidAsks[symbol][model.OKFUTURE].Asks.GetMap()
				askMap = markets.BidAsks[symbol][model.OKFUTURE].Bids.GetMap()
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
	return WebSocketServe(model.ApplicationConfig.WSUrls[model.OKFUTURE],
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
	postData.Set(`symbol`, getSymbol(symbol))
	postData.Set(`contract_type`, getContractType(symbol))
	postData.Set(`amount`, amount)
	postData.Set(`price`, price)
	postData.Set(`type`, orderSide)
	postData.Set(`match_price`, orderType)
	responseBody := sendSignRequest(`POST`, model.ApplicationConfig.RestUrls[model.OKFUTURE]+"/future_trade.do", &postData)
	fmt.Println(string(responseBody))
	resultJson, err := util.NewJSON(responseBody)
	if err == nil {
		//result, _ := resultJson.Get(`result`).Bool()
		orderId, _ := resultJson.Get(`order_id`).Int64()
		return strconv.FormatInt(orderId, 10), ``
	}
	return orderId, errCode
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
	responseBody := sendSignRequest(`POST`, model.ApplicationConfig.RestUrls[model.OKFUTURE]+"/future_order_info.do", &postData)
	fmt.Println(string(responseBody))
	orderJson, err := util.NewJSON(responseBody)
	if err != nil {
		return 0, -1, err.Error()
	}
	orders, _ := orderJson.Get(`orders`).Array()
	for _, value := range orders {
		order, _ := value.(map[string]interface{})[`order_id`].(json.Number).Int64()
		if strconv.FormatInt(order, 10) == orderId {
			contractAmount, _ := value.(map[string]interface{})[`deal_amount`].(json.Number).Float64()
			contractPrice, _ := value.(map[string]interface{})[`price_avg`].(json.Number).Float64()
			unitAmount, _ := value.(map[string]interface{})[`unit_amount`].(json.Number).Float64()
			if contractAmount > 0 && contractPrice > 0 {
				dealAmount = contractAmount * unitAmount / contractPrice
			}
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
	responseBody := sendSignRequest(`POST`, model.ApplicationConfig.RestUrls[model.OKFUTURE]+"/future_cancel.do", &postData)
	fmt.Println(string(responseBody))
	orderJson, err := util.NewJSON(responseBody)
	if err == nil {
		result, _ = orderJson.Get(`result`).Bool()
		msg, _ = orderJson.Get(`order_id`).String()
		return result, ``, msg
	}
	return false, ``, err.Error()
}

func getAccountOkfuture(accounts *model.Accounts) {
}