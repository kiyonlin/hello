package api

import (
	"bytes"
	"compress/flate"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"hello/model"
	"hello/util"
	"io"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type OKEXMessage struct {
	Binary  int    `json:"binary"`
	Channel string `json:"channel"`
	Data    struct {
		Asks      [][]string `json:"asks"`
		Bids      [][]string `json:"bids"`
		Timestamp int        `json:"timestamp"`
	} `json:"data"`
}

var apiLastAccessTime map[string]*time.Time // url-time

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
		//util.SocketInfo(`okex subscribed ` + v)
	}
	return err
}

func WsDepthServeOkex(markets *model.Markets, carryHandlers []CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
	lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte, conn *websocket.Conn) {
		if util.GetNow().Unix()-lastPingTime > 20 { // ping okex server every 30 seconds
			lastPingTime = util.GetNow().Unix()
			pingMap := make(map[string]interface{})
			pingMap["event"] = "ping"
			pingParams := util.JsonEncodeMapToByte(pingMap)
			if err := conn.WriteMessage(websocket.TextMessage, pingParams); err != nil {
				util.SocketInfo("okex server ping client error " + err.Error())
			}
		}
		messages := make([]OKEXMessage, 1)
		var out bytes.Buffer
		reader := flate.NewReader(bytes.NewReader(event))
		_, _ = io.Copy(&out, reader)
		event = out.Bytes()
		if err := json.Unmarshal(event, &messages); err == nil {
			for _, message := range messages {
				symbol := model.GetSymbol(model.OKEX, message.Channel)
				if symbol != "" {
					bidAsk := model.BidAsk{}
					bidAsk.Asks = make([]model.Tick, len(message.Data.Asks))
					bidAsk.Bids = make([]model.Tick, len(message.Data.Bids))
					for i, v := range message.Data.Bids {
						price, _ := strconv.ParseFloat(v[0], 64)
						amount, _ := strconv.ParseFloat(v[1], 64)
						bidAsk.Bids[i] = model.Tick{Price: price, Amount: amount}
					}
					for i, v := range message.Data.Asks {
						price, _ := strconv.ParseFloat(v[0], 64)
						amount, _ := strconv.ParseFloat(v[1], 64)
						bidAsk.Asks[i] = model.Tick{Price: price, Amount: amount}
					}
					sort.Sort(bidAsk.Asks)
					sort.Sort(sort.Reverse(bidAsk.Bids))
					bidAsk.Ts = message.Data.Timestamp
					if markets.SetBidAsk(symbol, model.OKEX, &bidAsk) {
						for _, handler := range carryHandlers {
							handler(model.OKEX, symbol)
						}
					}
				}
			}
		}
	}
	return WebSocketServe(model.AppConfig.WSUrls[model.OKEX],
		model.GetDepthSubscribes(model.OKEX), subscribeHandlerOkex, wsHandler, errHandler)
}

func getSign(postData *url.Values) string {
	hash := md5.New()
	toBeSign, _ := url.QueryUnescape(postData.Encode() + "&secret_key=" + model.AppConfig.OkexSecret)
	hash.Write([]byte(toBeSign))
	return strings.ToUpper(hex.EncodeToString(hash.Sum(nil)))
}

func sendSignRequest(method, path string, postData *url.Values, waitMillionSeconds int64) (response []byte) {
	if apiLastAccessTime == nil {
		apiLastAccessTime = make(map[string]*time.Time)
	}
	current := util.GetNow()
	if apiLastAccessTime[path] != nil && current.UnixNano()-apiLastAccessTime[path].UnixNano() < waitMillionSeconds*1000000 {
		time.Sleep(time.Duration(waitMillionSeconds) * time.Millisecond)
		util.SocketInfo(fmt.Sprintf(`[api break]sleep %d m-seconds after last access %s`, waitMillionSeconds, path))
	}
	apiLastAccessTime[path] = &current
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded",
		"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	if method == `GET` {
		path += `?` + postData.Encode()
	} else {
		postData.Set("api_key", model.AppConfig.OkexKey)
		postData.Set("sign", getSign(postData))
	}
	responseBody, _ := util.HttpRequest(method, path, postData.Encode(), headers)
	util.SocketInfo(fmt.Sprintf(`[%s] %s returns: %s`, path, postData.Encode(), string(responseBody)))
	return responseBody
}

// orderType:  限价单（buy/sell） 市价单（buy_market/sell_market）
// okex中amount在市价买单中指的是右侧的钱
func placeOrderOkex(orderSide, orderType, symbol, price, amount string) (orderId, errCode string) {
	orderParam := ``
	postData := url.Values{}
	if orderSide == model.OrderSideBuy && orderType == model.OrderTypeLimit {
		orderParam = `buy`
		postData.Set("price", price)
		postData.Set("amount", amount)
	} else if orderSide == model.OrderSideBuy && orderType == model.OrderTypeMarket {
		orderParam = `buy_market`
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
	postData.Set("symbol", symbol)
	postData.Set("type", orderParam)
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKEX]+"/trade.do",
		&postData, 100)
	orderJson, err := util.NewJSON(responseBody)
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

func CancelOrderOkex(symbol string, orderId string) (result bool, errCode, msg string) {
	postData := url.Values{}
	postData.Set("order_id", orderId)
	postData.Set("symbol", symbol)
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKEX]+"/cancel_order.do",
		&postData, 100)
	util.Notice("okex cancel order" + orderId + string(responseBody))
	orderJson, err := util.NewJSON(responseBody)
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

func queryOrderOkex(symbol string, orderId string) (dealAmount, dealPrice float64, status string) {
	postData := url.Values{}
	postData.Set("order_id", orderId)
	postData.Set("symbol", symbol)
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKEX]+"/order_info.do",
		&postData, 100)
	orderJson, err := util.NewJSON(responseBody)
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

func getAccountOkex(accounts *model.Accounts) {
	postData := url.Values{}
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKEX]+"/userinfo.do",
		&postData, 340)
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

func MustFundTransferOkex(symbol string, amount float64, from, to string) (result bool, errCode string) {
	for i := 0.0; i < 100; i++ {
		transfer, errCode := FundTransferOkex(symbol, amount, from, to)
		if transfer {
			return transfer, errCode
		}
		time.Sleep(time.Second * 3)
		util.Notice(fmt.Sprintf(fmt.Sprintf(`[fail when must transfer]%s %s->%s %f %v`,
			symbol, from, to, amount, transfer)))
		amount = amount * (1.0 - i*i*0.01)
	}
	return false, `>100tries`
}

// from 转出账户(1：币币账户 3：合约账户 6：我的钱包)
// to 转入账户(1：币币账户 3：合约账户 6：我的钱包)
func FundTransferOkex(symbol string, amount float64, from, to string) (result bool, errCode string) {
	if amount <= 0 {
		return false, `0 transfer amount`
	}
	decimal := GetAmountDecimal(model.OKEX, symbol)
	amount = math.Floor(amount*math.Pow(10, float64(decimal))) / math.Pow(10, float64(decimal))
	strAmount := strconv.FormatFloat(amount, 'f', -1, 64)
	postData := url.Values{}
	index := strings.Index(symbol, `_`)
	if index > 0 {
		postData.Set(`symbol`, symbol[0:index]+`_usd`)
	}
	postData.Set(`amount`, strAmount)
	postData.Set(`from`, from)
	postData.Set(`to`, to)
	responseBody := sendSignRequest(`POST`, model.AppConfig.RestUrls[model.OKEX]+"/funds_transfer.do",
		&postData, 200)
	resultJson, err := util.NewJSON(responseBody)
	if err == nil {
		result, _ = resultJson.Get(`result`).Bool()
		code, _ := resultJson.Get(`error_code`).Float64()
		errCode = strconv.FormatFloat(code, 'f', -1, 64)
		return result, errCode
	}
	return false, err.Error()
}

func getBuyPriceOkex(symbol string) (buy float64, err error) {
	model.CurrencyPrice[symbol] = 0
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded",
		"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	responseBody, _ := util.HttpRequest("GET", model.AppConfig.RestUrls[model.OKEX]+
		"/ticker.do?symbol="+symbol, "", headers)
	tickerJson, err := util.NewJSON(responseBody)
	if err == nil {
		strBuy, _ := tickerJson.GetPath("ticker", "buy").String()
		model.CurrencyPrice[symbol], err = strconv.ParseFloat(strBuy, 64)
	}
	return model.CurrencyPrice[symbol], err
}

func GetKLineOkex(symbol, timeSlot string, size int64) []*model.KLinePoint {
	postData := url.Values{}
	postData.Set(`symbol`, symbol)
	postData.Set(`type`, timeSlot)
	postData.Set(`size`, strconv.FormatInt(size, 10))
	responseBody := sendSignRequest(`GET`, model.AppConfig.RestUrls[model.OKEX]+"/kline.do",
		&postData, 100)
	//fmt.Println(string(responseBody))
	dataJson, _ := util.NewJSON(responseBody)
	if dataJson == nil {
		return nil
	}
	data, _ := dataJson.Array()
	priceKLine := make([]*model.KLinePoint, len(data))
	for key, value := range data {
		ts, _ := value.([]interface{})[0].(json.Number).Int64()
		str := value.([]interface{})[4].(string)
		strHigh := value.([]interface{})[2].(string)
		strLow := value.([]interface{})[3].(string)
		price, _ := strconv.ParseFloat(str, 64)
		high, _ := strconv.ParseFloat(strHigh, 64)
		low, _ := strconv.ParseFloat(strLow, 64)
		priceKLine[key] = &model.KLinePoint{TS: ts, EndPrice: price, HighPrice: high, LowPrice: low}
	}
	return priceKLine
}
