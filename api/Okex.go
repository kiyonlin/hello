package api

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"sort"
	"strconv"
	"crypto/md5"
	"net/url"
	"strings"
	"encoding/hex"
	"hello/model"
	"hello/util"
)

type OKEXMessage struct {
	Binary  int    `json:"binary"`
	Channel string `json:"channel"`
	Data struct {
		Asks      [][]string `json:"asks"`
		Bids      [][]string `json:"bids"`
		Timestamp int        `json:"timestamp"`
	} `json:"data"`
}

var subscribeHandlerOkex = func(subscribes []string, conn *websocket.Conn) error {
	var err error = nil
	for _, v := range subscribes {
		subscribeMap := make(map[string]interface{})
		subscribeMap["event"] = "addChannel"
		subscribeMap["channel"] = v
		subscribeMessage := util.JsonEncodeMapToByte(subscribeMap)
		if err = conn.WriteMessage(websocket.TextMessage, subscribeMessage); err != nil {
			util.SocketInfo("okex can not subscribe " + err.Error())
			return err
		}
	}
	return err
}

func WsDepthServeOkex(markets *model.Markets, carryHandler CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
	lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte, conn *websocket.Conn) {
		if util.GetNow().Unix()-lastPingTime > 30 { // ping okex server every 30 seconds
			lastPingTime = util.GetNow().Unix()
			pingMap := make(map[string]interface{})
			pingMap["event"] = "ping"
			pingParams := util.JsonEncodeMapToByte(pingMap)
			if err := conn.WriteMessage(websocket.TextMessage, pingParams); err != nil {
				util.SocketInfo("okex server ping client error " + err.Error())
			}
		}
		//util.Info(string(event))
		messages := make([]OKEXMessage, 1)
		if err := json.Unmarshal(event, &messages); err == nil {
			for _, message := range messages {
				if symbol := model.SubscribeSymbol[message.Channel]; symbol != "" {
					bidAsk := model.BidAsk{}
					bidAsk.Asks = make([][]float64, len(message.Data.Asks))
					bidAsk.Bids = make([][]float64, len(message.Data.Bids))
					for i, v := range message.Data.Bids {
						price, _ := strconv.ParseFloat(v[0], 64)
						amount, _ := strconv.ParseFloat(v[1], 64)
						bidAsk.Bids[i] = []float64{price, amount}
					}
					for i, v := range message.Data.Asks {
						price, _ := strconv.ParseFloat(v[0], 64)
						amount, _ := strconv.ParseFloat(v[1], 64)
						bidAsk.Asks[i] = []float64{price, amount}
					}
					//bidAsk.Bids = message.Data.Bids
					sort.Sort(bidAsk.Asks)
					sort.Reverse(bidAsk.Bids)
					bidAsk.Ts = message.Data.Timestamp
					markets.SetBidAsk(symbol, model.OKEX, &bidAsk)
					if carry, err := markets.NewCarry(symbol); err == nil {
						carryHandler(carry)
					}
				}
			}
		}
	}
	return WebSocketServe(model.ApplicationConfig.WSUrls[model.OKEX], model.ApplicationConfig.GetSubscribes(model.OKEX), subscribeHandlerOkex, wsHandler, errHandler)
}

func signOkex(postData *url.Values, secretKey string) {
	hash := md5.New()
	toBeSign, _ := url.QueryUnescape(postData.Encode() + "&secret_key=" + secretKey)
	hash.Write([]byte(toBeSign))
	sign := hex.EncodeToString(hash.Sum(nil))
	postData.Set("sign", strings.ToUpper(sign))
}

// orderType:  限价单（buy/sell） 市价单（buy_market/sell_market）
func PlaceOrderOkex(symbol string, orderType string, price string, amount string) (orderId, errCode string) {
	postData := url.Values{}
	postData.Set("api_key", model.ApplicationConfig.ApiKeys[model.OKEX])
	postData.Set("symbol", symbol)
	postData.Set("type", orderType)
	postData.Set("price", price)
	postData.Set("amount", amount)
	signOkex(&postData, model.ApplicationConfig.ApiSecrets[model.OKEX])
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded",
		"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	responseBody, _ := util.HttpRequest("POST", model.ApplicationConfig.RestUrls[model.OKEX]+"/trade.do", postData.Encode(), headers)
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
	util.Notice(symbol + "挂单okex:" + price + orderType + amount + "返回" + string(responseBody) + "errCode:" + errCode + "orderId" + orderId)
	return orderId, errCode
}

func CancelOrderOkex(symbol string, orderId string) {
	postData := url.Values{}
	postData.Set("order_id", orderId)
	postData.Set("symbol", symbol)
	postData.Set("api_key", model.ApplicationConfig.ApiKeys[model.OKEX])
	signOkex(&postData, model.ApplicationConfig.ApiSecrets[model.OKEX])
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded",
		"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	responseBody, _ := util.HttpRequest("POST", model.ApplicationConfig.RestUrls[model.OKEX]+"/cancel_order.do", postData.Encode(), headers)
	util.SocketInfo("okex cancel order" + orderId + string(responseBody))
}

func QueryOrderOkex(symbol string, orderId string) (dealAmount float64, status string) {
	postData := url.Values{}
	postData.Set("order_id", orderId)
	postData.Set("symbol", symbol)
	postData.Set("api_key", model.ApplicationConfig.ApiKeys[model.OKEX])
	signOkex(&postData, model.ApplicationConfig.ApiSecrets[model.OKEX])
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded",
		"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	responseBody, _ := util.HttpRequest("POST", model.ApplicationConfig.RestUrls[model.OKEX]+"/order_info.do", postData.Encode(), headers)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		orders, _ := orderJson.Get("orders").Array()
		if len(orders) > 0 {
			order := orders[0].(map[string]interface{})
			if order["order_id"].(json.Number).String() == orderId {
				dealAmount, _ = order["deal_amount"].(json.Number).Float64()
				status = model.OrderStatusMap[order["status"].(json.Number).String()]
			}
		}
	}
	util.SocketInfo(status + "okex query order" + string(responseBody))
	return dealAmount, status
}

func GetAccountOkex(accounts *model.Accounts) {
	postData := url.Values{}
	postData.Set("api_key", model.ApplicationConfig.ApiKeys[model.OKEX])
	signOkex(&postData, model.ApplicationConfig.ApiSecrets[model.OKEX])
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded",
		"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	responseBody, _ := util.HttpRequest("POST", model.ApplicationConfig.RestUrls[model.OKEX]+"/userinfo.do", postData.Encode(), headers)
	util.SocketInfo("okex account:" + string(responseBody))
	balanceJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		free, _ := balanceJson.GetPath("info", "funds", "free").Map()
		lock, _ := balanceJson.GetPath("info", "funds", "freezed").Map()
		for k, v := range free {
			balance, _ := strconv.ParseFloat(v.(string), 64)
			account := accounts.GetAccount(model.OKEX, k)
			if account == nil {
				account = &model.Account{Market: model.OKEX, Currency: k}
			}
			if balance > 0 {
				accounts.SetAccount(model.OKEX, k, account)
				account.Free = balance
				model.AccountChannel <- *account
			}
		}
		for k, v := range lock {
			balance, _ := strconv.ParseFloat(v.(string), 64)
			currency := strings.ToLower(k)
			account := accounts.GetAccount(model.OKEX, currency)
			if account == nil {
				account = &model.Account{Market: model.OKEX, Currency: currency}
			}
			if balance > 0 {
				accounts.SetAccount(model.OKEX, currency, account)
				account.Frozen = balance
				model.AccountChannel <- *account
			}
		}
	}
}
