package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/gorilla/websocket"
	"hello/model"
	"hello/util"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

var lastTickId = int64(0)

var subscribeHandlerBinance = func(subscribes []string, conn *websocket.Conn) error {
	return nil
}

func WsDepthServeBinance(markets *model.Markets, carryHandlers []CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
	wsHandler := func(event []byte, conn *websocket.Conn) {
		json, err := util.NewJSON(event)
		if err != nil {
			errHandler(err)
			return
		}
		subscribe, _ := json.Get("stream").String()
		symbol := model.GetSymbol(model.Binance, subscribe)
		if symbol != "" {
			json = json.Get("data")
			if json == nil {
				return
			}
			bidAsk := model.BidAsk{}
			tickId, _ := json.Get(`lastUpdateId`).Int64()
			if tickId > lastTickId {
				lastTickId = tickId
				bidAsk.Ts = int(util.GetNowUnixMillion())
			}
			bids, _ := json.Get(`bids`).Array()
			asks, _ := json.Get(`asks`).Array()
			bidAsk.Bids = make([]model.Tick, len(bids))
			for i, value := range bids {
				if len(value.([]interface{})) < 2 {
					return
				}
				price, _ := strconv.ParseFloat(value.([]interface{})[0].(string), 64)
				amount, _ := strconv.ParseFloat(value.([]interface{})[1].(string), 64)
				bidAsk.Bids[i] = model.Tick{Price: price, Amount: amount}
			}
			bidAsk.Asks = make([]model.Tick, len(asks))
			for i, value := range asks {
				if len(value.([]interface{})) < 2 {
					return
				}
				price, _ := strconv.ParseFloat(value.([]interface{})[0].(string), 64)
				amount, _ := strconv.ParseFloat(value.([]interface{})[1].(string), 64)
				bidAsk.Asks[i] = model.Tick{Price: price, Amount: amount}
			}
			sort.Sort(bidAsk.Asks)
			sort.Sort(sort.Reverse(bidAsk.Bids))
			if markets.SetBidAsk(symbol, model.Binance, &bidAsk) {
				for _, handler := range carryHandlers {
					handler(symbol, model.Binance)
				}
			}
		}
	}
	requestUrl := model.ApplicationConfig.WSUrls[model.Binance]

	for _, subscribe := range model.GetSubscribes(model.Binance) {
		requestUrl += subscribe + "/"
	}
	return WebSocketServe(requestUrl, model.GetSubscribes(model.Binance), subscribeHandlerBinance,
		wsHandler, errHandler)
}
func signBinance(postData *url.Values, secretKey string) {
	postData.Set("recvWindow", "6000000")
	time := strconv.FormatInt(util.GetNow().UnixNano(), 10)[0:13]
	postData.Set("timestamp", time)
	hash := hmac.New(sha256.New, []byte(secretKey))
	hash.Write([]byte(postData.Encode()))
	postData.Set("signature", hex.EncodeToString(hash.Sum(nil)))
}

// orderType: BUY SELL
// 注意，binance中amount无论是市价还是限价，都指的是要买入或者卖出的左侧币种，而非右侧的钱
func placeOrderBinance(orderSide, orderType, symbol, price, amount string) (orderId, errCode string) {
	postData := url.Values{}
	if orderSide == model.OrderSideBuy {
		orderSide = `BUY`
	} else if orderSide == model.OrderSideSell {
		orderSide = `SELL`
	} else {
		util.Notice(fmt.Sprintf(`[parameter error] order side: %s`, orderSide))
		return ``, ``
	}
	if orderType == model.OrderTypeMarket {
		orderType = `MARKET`
	} else if orderType == model.OrderTypeLimit {
		orderType = `LIMIT`
		postData.Set("price", price)
		postData.Set("timeInForce", "GTC")
	} else {
		util.Notice(fmt.Sprintf(`[parameter error] order type: %s`, orderType))
		return ``, ``
	}
	postData.Set("symbol", strings.ToUpper(strings.Replace(symbol, "_", "", 1)))
	postData.Set("type", orderType)
	postData.Set("side", orderSide)
	postData.Set("quantity", amount)
	signBinance(&postData, model.ApplicationConfig.BinanceSecret)
	headers := map[string]string{"X-MBX-APIKEY": model.ApplicationConfig.BinanceKey}
	responseBody, _ := util.HttpRequest("POST",
		model.ApplicationConfig.RestUrls[model.Binance]+"/api/v3/order?", postData.Encode(), headers)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		orderIdInt, _ := orderJson.Get("orderId").Int()
		if orderIdInt != 0 {
			orderId = strconv.Itoa(orderIdInt)
		}
		errCodeInt, _ := orderJson.Get("code").Int()
		if errCodeInt != 0 {
			errCode = strconv.Itoa(errCodeInt)
		}
	}
	util.Notice(fmt.Sprintf(`[挂单binance] %s side: %s type: %s price: %s amount: %s order id %s errCode: %s 返回%s`,
		symbol, orderSide, orderType, price, amount, orderId, errCode, string(responseBody)))
	return orderId, errCode
}

func CancelOrderBinance(symbol string, orderId string) (result bool, errCode, msg string) {
	postData := url.Values{}
	postData.Set("symbol", strings.ToUpper(strings.Replace(symbol, "_", "", 1)))
	postData.Set("orderId", orderId)
	signBinance(&postData, model.ApplicationConfig.BinanceSecret)
	headers := map[string]string{"X-MBX-APIKEY": model.ApplicationConfig.BinanceKey}
	requestUrl := model.ApplicationConfig.RestUrls[model.Binance] + "/api/v3/order?" + postData.Encode()
	responseBody, _ := util.HttpRequest("DELETE", requestUrl, "", headers)
	util.Notice("binance cancel order" + string(responseBody))

	return true, ``, ``
}

func QueryOrderBinance(symbol string, orderId string) (dealAmount, dealPrice float64, status string) {
	postData := url.Values{}
	postData.Set("symbol", strings.ToUpper(strings.Replace(symbol, "_", "", 1)))
	postData.Set("orderId", orderId)
	signBinance(&postData, model.ApplicationConfig.BinanceSecret)
	headers := map[string]string{"X-MBX-APIKEY": model.ApplicationConfig.BinanceKey}
	requestUrl := model.ApplicationConfig.RestUrls[model.Binance] + "/api/v3/order?" + postData.Encode()
	responseBody, _ := util.HttpRequest("GET", requestUrl, "", headers)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		str, _ := orderJson.Get("executedQty").String()
		if str != "" {
			dealAmount, _ = strconv.ParseFloat(str, 64)
		}
		strDealPrice, _ := orderJson.Get(`price`).String()
		if strDealPrice != `` {
			dealPrice, _ = strconv.ParseFloat(strDealPrice, 64)
		}
		status, _ = orderJson.Get("status").String()
		status = model.GetOrderStatus(model.Binance, status)
	}
	util.Notice(fmt.Sprintf("%s binance query order %f %s", status, dealAmount, responseBody))
	return dealAmount, dealPrice, status
}

func getAccountBinance(accounts *model.Accounts) {
	postData := url.Values{}
	signBinance(&postData, model.ApplicationConfig.BinanceSecret)
	headers := map[string]string{"X-MBX-APIKEY": model.ApplicationConfig.BinanceKey}
	requestUrl := model.ApplicationConfig.RestUrls[model.Binance] + "/api/v3/account?" + postData.Encode()
	responseBody, _ := util.HttpRequest("GET", requestUrl, "", headers)
	balanceJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		if balanceJson.Get("canTrade").MustBool() {
			currencies, _ := balanceJson.Get("balances").Array()
			for _, value := range currencies {
				asset := value.(map[string]interface{})
				free, _ := strconv.ParseFloat(asset["free"].(string), 64)
				frozen, _ := strconv.ParseFloat(asset["locked"].(string), 64)
				if free == 0 && frozen == 0 {
					continue
				}
				currency := strings.ToLower(asset["asset"].(string))
				account := &model.Account{Market: model.Binance, Currency: currency, Free: free, Frozen: frozen}
				accounts.SetAccount(model.Binance, currency, account)
			}
		}
	}
}
