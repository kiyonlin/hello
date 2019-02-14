package api

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"hello/model"
	"hello/util"
	"sort"
	"strconv"
	"strings"
	"time"
)

var fcoinLastApiAccessTime *time.Time

var subscribeHandlerFcoin = func(subscribes []string, conn *websocket.Conn) error {
	var err error = nil
	subscribeMap := make(map[string]interface{})
	subscribeMap[`cmd`] = `sub`
	subscribeMap[`args`] = subscribes
	subscribeMessage := util.JsonEncodeMapToByte(subscribeMap)
	if err = conn.WriteMessage(websocket.TextMessage, []byte(subscribeMessage)); err != nil {
		util.SocketInfo("fcoin can not subscribe " + err.Error())
		return err
	}
	return err
}

func WsDepthServeFcoin(markets *model.Markets, carryHandlers []CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
	wsHandler := func(event []byte, conn *websocket.Conn) {
		responseJson, err := util.NewJSON(event)
		if err != nil {
			errHandler(err)
			return
		}
		if responseJson == nil {
			return
		}
		symbol := model.GetSymbol(model.Fcoin, responseJson.Get("type").MustString())
		symbolSettings := model.GetMarketSettings(model.Fcoin)
		if symbolSettings == nil || symbolSettings[symbol] == nil {
			util.Notice(symbol + ` not supported`)
			return
		}
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
				for _, handler := range carryHandlers {
					handler(model.Fcoin, symbol)
				}
			}
		}
	}
	requestUrl := model.AppConfig.WSUrls[model.Fcoin]
	return WebSocketServe(requestUrl, model.GetDepthSubscribes(model.Fcoin), subscribeHandlerFcoin,
		wsHandler, errHandler)
}

func SignedRequestFcoin(method, path string, body map[string]interface{}) []byte {
	uri := model.AppConfig.RestUrls[model.Fcoin] + path
	current := util.GetNow()
	if fcoinLastApiAccessTime == nil {
		fcoinLastApiAccessTime = &current
	}
	if current.UnixNano()-fcoinLastApiAccessTime.UnixNano() < 150000000 {
		time.Sleep(time.Duration(200) * time.Millisecond)
		util.SocketInfo(fmt.Sprintf(`[api break]sleep %d m-seconds after last access %s`, 100, path))
	}
	currentTime := strconv.FormatInt(current.UnixNano(), 10)[0:13]
	if method == `GET` && len(body) > 0 {
		uri += `?` + util.ComposeParams(body)
	}
	toBeBase := method + uri + currentTime
	if method == `POST` {
		toBeBase += util.ComposeParams(body)
	}
	based := base64.StdEncoding.EncodeToString([]byte(toBeBase))
	hash := hmac.New(sha1.New, []byte(model.AppConfig.FcoinSecret))
	hash.Write([]byte(based))
	sign := base64.StdEncoding.EncodeToString(hash.Sum(nil))
	headers := map[string]string{`FC-ACCESS-KEY`: model.AppConfig.FcoinKey,
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
func placeOrderFcoin(orderSide, orderType, symbol, price, amount string) (orderId, errCode string) {
	postData := make(map[string]interface{})
	if orderType == model.OrderTypeLimit {
		postData["price"] = price
	}
	orderSide = model.GetDictMap(model.Fcoin, orderSide)
	if orderSide == `` {
		util.Notice(fmt.Sprintf(`[parameter error] order side: %s`, orderSide))
		return ``, ``
	}
	orderType = model.GetDictMap(model.Fcoin, orderType)
	if orderType == `` {
		util.Notice(fmt.Sprintf(`[parameter error] order type: %s`, orderType))
		return ``, ``
	}
	postData["symbol"] = strings.ToLower(strings.Replace(symbol, "_", "", 1))
	postData["type"] = orderType
	postData["side"] = orderSide
	postData["amount"] = amount
	responseBody := SignedRequestFcoin("POST", "/orders", postData)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		orderId, _ := orderJson.Get("data").String()
		status, _ := orderJson.Get("status").Int()
		util.Notice(fmt.Sprintf(`[挂单fcoin] %s side: %s type: %s price: %s amount: %s order id %s errCode: %s 返回%s`,
			symbol, orderSide, orderType, price, amount, orderId, errCode, string(responseBody)))
		return orderId, strconv.Itoa(status)
	}
	return ``, err.Error()
}

func CancelOrderFcoin(orderId string) (result bool, errCode, msg string) {
	responseBody := SignedRequestFcoin(`POST`, `/orders/`+orderId+`/submit-cancel`, nil)
	responseJson, err := util.NewJSON([]byte(responseBody))
	status := -1
	if err == nil {
		status, _ = responseJson.Get(`status`).Int()
		msg, _ = responseJson.Get(`msg`).String()
	}
	util.Notice(orderId + "fcoin cancel order" + string(responseBody))
	if status == 0 || status == 3008 { // 3008代表订单状态已经处于完成
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
	return &model.Order{OrderId: orderMap[`id`].(string),
		Symbol:     symbol,
		Market:     model.Fcoin,
		Amount:     amount,
		DealAmount: filledAmount,
		OrderTime:  time.Unix(0, createTime*1000000),
		OrderType:  model.GetDictMapRevert(model.Fcoin, orderMap[`type`].(string)),
		OrderSide:  model.GetDictMapRevert(model.Fcoin, orderMap[`side`].(string)),
		DealPrice:  price,
		Fee:        fee,
		FeeIncome:  feeIncome,
		Status:     model.GetOrderStatus(model.Fcoin, orderMap[`state`].(string)),
	}
}

func queryOrdersFcoin(symbol, states string) (orders map[string]*model.Order) {
	states, _ = model.GetOrderStatusRevert(model.Fcoin, states)
	body := make(map[string]interface{})
	body[`symbol`] = strings.ToLower(strings.Replace(symbol, "_", "", 1))
	body[`states`] = states
	body[`limit`] = `100`
	//body[`before`] = `2019-01-01 00:00:00`
	//body[`after`] = `2018-01-01 00:00:00`
	responseBody := SignedRequestFcoin(`GET`, `/orders`, body)
	//fmt.Println(string(responseBody))
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		jsonOrders, _ := orderJson.Get(`data`).Array()
		orders := make(map[string]*model.Order)
		for _, order := range jsonOrders {
			orderMap := order.(map[string]interface{})
			orders[orderMap[`id`].(string)] = parseOrder(symbol, orderMap)

		}
		return orders
	}
	return nil
}

func queryOrderFcoin(symbol, orderId string) (order *model.Order) {
	postData := make(map[string]interface{})
	postData["symbol"] = strings.ToLower(strings.Replace(symbol, "_", "", 1))
	responseBody := SignedRequestFcoin(`GET`, `/orders/`+orderId, postData)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		orderMap, _ := orderJson.Get(`data`).Map()
		return parseOrder(symbol, orderMap)
	}
	return nil
}

func getAccountFcoin(accounts *model.Accounts) {
	responseBody := SignedRequestFcoin(`GET`, `/accounts/balance`, nil)
	balanceJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		status, _ := balanceJson.Get("status").Int()
		if status == 0 {
			currencies, _ := balanceJson.Get("data").Array()
			for _, value := range currencies {
				asset := value.(map[string]interface{})
				free, _ := strconv.ParseFloat(asset["available"].(string), 64)
				frozen, _ := strconv.ParseFloat(asset["frozen"].(string), 64)
				if free == 0 && frozen == 0 {
					continue
				}
				currency := strings.ToLower(asset["currency"].(string))
				account := &model.Account{Market: model.Fcoin, Currency: currency, Free: free, Frozen: frozen}
				accounts.SetAccount(model.Fcoin, currency, account)
			}
		}
	}
}

func getBuyPriceFcoin(symbol string) (buy float64, err error) {
	model.CurrencyPrice[symbol] = 0
	requestSymbol := strings.ToLower(strings.Replace(symbol, "_", "", 1))
	responseBody := SignedRequestFcoin(`GET`, `/market/ticker/`+requestSymbol, nil)
	if err == nil {
		orderJson, err := util.NewJSON([]byte(responseBody))
		if err == nil {
			orderJson = orderJson.Get(`data`)
			tickerType, _ := orderJson.Get(`type`).String()
			if strings.Contains(tickerType, requestSymbol) {
				model.CurrencyPrice[symbol], _ = orderJson.Get("ticker").GetIndex(0).Float64()
			}
		}
	}
	return model.CurrencyPrice[symbol], nil
}
