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
		subBook := fmt.Sprintf(`{'event':'addChannel','channel':'%s'}`, v)
		err = conn.WriteMessage(websocket.TextMessage, []byte(subBook))
		if err != nil {
			util.SocketInfo("okfuture can not subscribe " + err.Error())
			return err
		}
	}
	return err
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
		fmt.Println(string(event))
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
			if markets.BidAsks[symbol][model.OKFUTURE] != nil &&
				util.GetNowUnixMillion()-int64(markets.BidAsks[symbol][model.OKFUTURE].Ts) < 5000 {
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
				price,_ := bid.([]interface{})[0].(json.Number).Float64()
				amount,_ := bid.([]interface{})[1].(json.Number).Float64()
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
		model.GetSubscribes(model.OKFUTURE), subscribeHandlerOKFuture, wsHandler, errHandler)
}

func CancelOrderOkfuture(symbol string, orderId string) (result bool, errCode, msg string) {
	postData := url.Values{}
	postData.Set("order_id", orderId)
	postData.Set("symbol", symbol)
	postData.Set("api_key", model.ApplicationConfig.OkexKey)
	signOkex(&postData, model.ApplicationConfig.OkexSecret)
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded", "User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	responseBody, _ := util.HttpRequest("POST",
		model.ApplicationConfig.RestUrls[model.OKEX]+"/cancel_order.do", postData.Encode(), headers)
	util.Notice("okex cancel order" + orderId + string(responseBody))
	orderJson, err := util.NewJSON([]byte(responseBody))
	cancelResult := false
	if err == nil {
		successOrders, _ := orderJson.Get(`success`).Array()
		for _, value := range successOrders {
			if value.(string) == orderId {
				cancelResult = true
				break
			}
		}
		return cancelResult, ``, ``
	}
	return false, err.Error(), err.Error()
}

// orderType:  限价单（buy/sell） 市价单（buy_market/sell_market）
// okex中amount在市价买单中指的是右侧的钱，而参数中amount指的是左侧币种的数目，所以需要转换
func placeOrderOkfuture(orderSide, orderType, symbol, price, amount string) (orderId, errCode string) {
	orderParam := ``
	postData := url.Values{}
	if orderSide == model.OrderSideBuy && orderType == model.OrderTypeLimit {
		orderParam = `buy`
		postData.Set("price", price)
		postData.Set("amount", amount)
	} else if orderSide == model.OrderSideBuy && orderType == model.OrderTypeMarket {
		orderParam = `buy_market`
		// okex中amount在市价买单中指的是右侧的钱，而参数中amount指的是左侧币种的数目，所以需要转换
		leftAmount, _ := strconv.ParseFloat(amount, 64)
		leftPrice, _ := strconv.ParseFloat(price, 64)
		money := leftAmount * leftPrice
		amount = strconv.FormatFloat(money, 'f', 2, 64)
		// 市价买单需传price作为买入总金额
		postData.Set("price", amount)
	} else if orderSide == model.OrderSideSell && orderType == model.OrderTypeLimit {
		orderParam = `sell`
		postData.Set("price", price)
		postData.Set("amount", amount)
	} else if orderSide == model.OrderSideSell && orderType == model.OrderTypeMarket {
		orderParam = `sell_market`
		postData.Set("amount", amount)
	} else {
		util.Notice(fmt.Sprintf(`[parameter error] order side: %s order type: %s`, orderSide, orderType))
		return ``, ``
	}
	postData.Set("api_key", model.ApplicationConfig.OkexKey)
	postData.Set("symbol", symbol)
	postData.Set("type", orderParam)
	signOkex(&postData, model.ApplicationConfig.OkexSecret)
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded", "User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	responseBody, _ := util.HttpRequest("POST",
		model.ApplicationConfig.RestUrls[model.OKEX]+"/trade.do", postData.Encode(), headers)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		orderIdInt, _ := orderJson.Get("order_id").Int()
		if orderIdInt != 0 {
			orderId = strconv.Itoa(orderIdInt)
		}
		errCodeInt, _ := orderJson.Get("error_code").Int()
		if errCodeInt != 0 {
			errCode = strconv.Itoa(errCodeInt)
		}
	}
	util.Notice(fmt.Sprintf(`[挂单Okex] %s side: %s type: %s price: %s amount: %s order id %s errCode: %s 返回%s`,
		symbol, orderSide, orderType, price, amount, orderId, errCode, string(responseBody)))
	return orderId, errCode
}

func getAccountOkfuture(accounts *model.Accounts) {
	postData := url.Values{}
	postData.Set("api_key", model.ApplicationConfig.OkexKey)
	signOkex(&postData, model.ApplicationConfig.OkexSecret)
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded", "User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	responseBody, _ := util.HttpRequest("POST", model.ApplicationConfig.RestUrls[model.OKEX]+"/userinfo.do",
		postData.Encode(), headers)
	balanceJson, err := util.NewJSON(responseBody)
	if err == nil {
		free, _ := balanceJson.GetPath("info", "funds", "free").Map()
		lock, _ := balanceJson.GetPath("info", "funds", "freezed").Map()
		for k, v := range free {
			balance, _ := strconv.ParseFloat(v.(string), 64)
			if balance == 0 {
				continue
			}
			currency := strings.ToLower(k)
			account := accounts.GetAccount(model.OKEX, currency)
			if account == nil {
				account = &model.Account{Market: model.OKEX, Currency: k}
			}
			accounts.SetAccount(model.OKEX, currency, account)
			account.Free = balance
		}
		for k, v := range lock {
			balance, _ := strconv.ParseFloat(v.(string), 64)
			if balance == 0 {
				continue
			}
			currency := strings.ToLower(k)
			account := accounts.GetAccount(model.OKEX, currency)
			if account == nil {
				account = &model.Account{Market: model.OKEX, Currency: currency}
			}
			accounts.SetAccount(model.OKEX, currency, account)
			account.Frozen = balance
		}
	}
}

func QueryOrderOkfuture(symbol string, orderId string) (dealAmount, dealPrice float64, status string) {
	postData := url.Values{}
	postData.Set("order_id", orderId)
	postData.Set("symbol", symbol)
	postData.Set("api_key", model.ApplicationConfig.OkexKey)
	signOkex(&postData, model.ApplicationConfig.OkexSecret)
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded",
		"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	responseBody, _ := util.HttpRequest("POST", model.ApplicationConfig.RestUrls[model.OKEX]+"/order_info.do",
		postData.Encode(), headers)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		orders, _ := orderJson.Get("orders").Array()
		if len(orders) > 0 {
			order := orders[0].(map[string]interface{})
			if order["order_id"].(json.Number).String() == orderId {
				dealAmount, _ = order["deal_amount"].(json.Number).Float64()
				dealPrice, _ = order[`avg_price`].(json.Number).Float64()
				status = model.GetOrderStatus(model.OKEX, order["status"].(json.Number).String())
			}
		}
	}
	util.Notice(fmt.Sprintf("%s okex query order %f %s", status, dealAmount, responseBody))
	return dealAmount, dealPrice, status
}
