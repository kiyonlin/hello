package api

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"github.com/gorilla/websocket"
	"hello/model"
	"hello/util"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

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
		json, err := util.NewJSON(event)
		if err != nil {
			errHandler(err)
			return
		}
		if json == nil {
			return
		}
		symbol := model.GetSymbol(model.Fcoin, json.Get("type").MustString())
		symbolSettings := model.GetMarketSettings(model.Fcoin)
		if symbolSettings == nil || symbolSettings[symbol] == nil {
			util.Notice(symbol + ` not supported`)
			return
		}
		if symbol != "" && symbol != "_" {
			bidAsk := model.BidAsk{}
			bidsLen := len(json.Get("bids").MustArray()) / 2
			bidAsk.Bids = make([]model.Tick, bidsLen)
			for i := 0; i < bidsLen; i++ {
				price, _ := json.Get("bids").GetIndex(i * 2).Float64()
				amount, _ := json.Get("bids").GetIndex(i*2 + 1).Float64()
				bidAsk.Bids[i] = model.Tick{Price: price, Amount: amount}
			}
			asksLen := len(json.Get("asks").MustArray()) / 2
			bidAsk.Asks = make([]model.Tick, asksLen)
			for i := 0; i < asksLen; i++ {
				price, _ := json.Get("asks").GetIndex(i * 2).Float64()
				amount, _ := json.Get("asks").GetIndex(i*2 + 1).Float64()
				bidAsk.Asks[i] = model.Tick{Price: price, Amount: amount}
			}
			sort.Sort(bidAsk.Asks)
			sort.Sort(sort.Reverse(bidAsk.Bids))
			bidAsk.Ts = json.Get("ts").MustInt()
			if markets.SetBidAsk(symbol, model.Fcoin, &bidAsk) {
				for _, handler := range carryHandlers {
					handler(symbol, model.Fcoin)
				}
			}
		}
	}
	requestUrl := model.ApplicationConfig.WSUrls[model.Fcoin]
	return WebSocketServe(requestUrl, model.GetDepthSubscribes(model.Fcoin), subscribeHandlerFcoin,
		wsHandler, errHandler)
}

func SignedRequestFcoin(method, path string, postMap map[string]interface{}) []byte {
	uri := model.ApplicationConfig.RestUrls[model.Fcoin] + path
	time := strconv.FormatInt(util.GetNow().UnixNano(), 10)[0:13]
	postData := &url.Values{}
	for key, value := range postMap {
		postData.Set(key, value.(string))
	}
	toBeBase := method + uri + time
	if method != `GET` {
		toBeBase += postData.Encode()
	}
	based := base64.StdEncoding.EncodeToString([]byte(toBeBase))
	hash := hmac.New(sha1.New, []byte(model.ApplicationConfig.FcoinSecret))
	hash.Write([]byte(based))
	sign := base64.StdEncoding.EncodeToString(hash.Sum(nil))
	headers := map[string]string{`FC-ACCESS-KEY`: model.ApplicationConfig.FcoinKey,
		`FC-ACCESS-SIGNATURE`: sign, `FC-ACCESS-TIMESTAMP`: time, "Content-Type": "application/json"}
	var responseBody []byte
	if postMap == nil {
		responseBody, _ = util.HttpRequest(method, uri, ``, headers)
	} else {
		responseBody, _ = util.HttpRequest(method, uri, string(util.JsonEncodeMapToByte(postMap)), headers)
	}
	return responseBody
}

// side: buy sell
// type: limit market
// fcoin中amount在市价买单中指的是右侧的钱，而参数中amount指的是左侧币种的数目，所以需要转换
func placeOrderFcoin(orderSide, orderType, symbol, price, amount string) (orderId, errCode string) {
	if orderSide == model.OrderSideBuy {
		orderSide = `buy`
	} else if orderSide == model.OrderSideSell {
		orderSide = `sell`
	} else {
		util.Notice(fmt.Sprintf(`[parameter error] order side: %s`, orderSide))
		return ``, ``
	}
	if orderType == model.OrderTypeLimit {
		orderType = `limit`
	} else if orderType == model.OrderTypeMarket {
		orderType = `market`
	} else {
		util.Notice(fmt.Sprintf(`[parameter error] order type: %s`, orderType))
		return ``, ``
	}
	if orderSide == model.OrderSideBuy && orderType == model.OrderTypeMarket {
		// fcoin中amount在市价买单中指的是右侧的钱，而参数中amount指的是左侧币种的数目，所以需要转换
		leftAmount, _ := strconv.ParseFloat(amount, 64)
		leftPrice, _ := strconv.ParseFloat(price, 64)
		money := leftAmount * leftPrice
		amount = strconv.FormatFloat(money, 'f', 2, 64)
	}
	postData := make(map[string]interface{})
	postData["symbol"] = strings.ToLower(strings.Replace(symbol, "_", "", 1))
	postData["type"] = orderType
	postData["side"] = orderSide
	postData["amount"] = amount
	if orderType == `limit` {
		postData["price"] = price
	}
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
	json, err := util.NewJSON([]byte(responseBody))
	status := -1
	if err == nil {
		status, _ = json.Get(`status`).Int()
		msg, _ = json.Get(`msg`).String()
	}
	util.Notice(orderId + "fcoin cancel order" + string(responseBody))
	if status == 0 {
		return true, ``, msg
	}
	return false, strconv.FormatInt(int64(status), 10), msg
}

func QueryOrderFcoin(symbol, orderId string) (dealAmount, dealPrice float64, status string) {
	postData := make(map[string]interface{})
	postData["symbol"] = strings.ToLower(strings.Replace(symbol, "_", "", 1))
	responseBody := SignedRequestFcoin(`GET`, `/orders/`+orderId, postData)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		orderJson = orderJson.Get(`data`)
		str, _ := orderJson.Get("filled_amount").String()
		if str != "" {
			dealAmount, _ = strconv.ParseFloat(str, 64)
		}
		strPrice, _ := orderJson.Get(`price`).String()
		if strPrice != `` {
			dealPrice, _ = strconv.ParseFloat(strPrice, 64)
		}
		status, _ = orderJson.Get("state").String()
		status = model.GetOrderStatus(model.Fcoin, status)
	}
	util.Notice(fmt.Sprintf("%s %s fcoin query order %f %s", symbol, status, dealAmount, responseBody))
	return dealAmount, dealPrice, status
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
	responseBody, err := util.HttpRequest(`GET`, model.ApplicationConfig.RestUrls[model.Fcoin]+`/market/ticker/`+requestSymbol,
		``, nil)
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
