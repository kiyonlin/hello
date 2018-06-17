package api

import (
	"github.com/gorilla/websocket"
	"sort"
	"strconv"
	"strings"
	"net/url"
	"encoding/hex"
	"crypto/hmac"
	"crypto/sha256"
	"hello/model"
	"hello/util"
	"fmt"
)

var subscribeHandlerBinance = func(subscribes []string, conn *websocket.Conn) error {
	return nil
}

func WsDepthServeBinance(markets *model.Markets, carryHandler CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
	wsHandler := func(event []byte, conn *websocket.Conn) {
		json, err := util.NewJSON(event)
		if err != nil {
			errHandler(err)
			return
		}
		json = json.Get("data")
		if json == nil {
			return
		}
		symbol := model.GetSymbol(model.Binance, json.Get("s").MustString())
		if symbol != "" {
			bidAsk := model.BidAsk{}
			bidsLen := len(json.Get("b").MustArray())
			bidAsk.Bids = make([][]float64, bidsLen)
			for i := 0; i < bidsLen; i++ {
				item := json.Get("b").GetIndex(i)
				bidAsk.Bids[i] = make([]float64, 2)
				strPrice, _ := item.GetIndex(0).String()
				strAmount, _ := item.GetIndex(1).String()
				bidAsk.Bids[i][0], _ = strconv.ParseFloat(strPrice, 64)
				bidAsk.Bids[i][1], _ = strconv.ParseFloat(strAmount, 64)
			}
			asksLen := len(json.Get("a").MustArray())
			bidAsk.Asks = make([][]float64, asksLen)
			for i := 0; i < asksLen; i++ {
				item := json.Get("a").GetIndex(i)
				bidAsk.Asks[i] = make([]float64, 2)
				strPrice, _ := item.GetIndex(0).String()
				strAmount, _ := item.GetIndex(1).String()
				bidAsk.Asks[i][0], _ = strconv.ParseFloat(strPrice, 64)
				bidAsk.Asks[i][1], _ = strconv.ParseFloat(strAmount, 64)
			}
			sort.Sort(bidAsk.Asks)
			sort.Reverse(bidAsk.Bids)
			bidAsk.Ts = json.Get("E").MustInt()
			if markets.SetBidAsk(symbol, model.Binance, &bidAsk) {
				if carry, err := markets.NewCarry(symbol); err == nil {
					carryHandler(carry)
				}
			}
		}
	}
	requestUrl := model.ApplicationConfig.WSUrls[model.Binance]
	for _, v := range model.ApplicationConfig.GetSubscribes(model.Binance) {
		requestUrl += strings.ToLower(v) + "@depth/"
	}
	return WebSocketServe(requestUrl, model.ApplicationConfig.GetSubscribes(model.Binance), subscribeHandlerBinance,
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
func PlaceOrderBinance(symbol string, orderType string, price string, amount string) (orderId, errCode string) {
	postData := url.Values{}
	postData.Set("symbol", strings.ToUpper(strings.Replace(symbol, "_", "", 1)))
	postData.Set("type", "LIMIT")
	postData.Set("side", orderType)
	postData.Set("quantity", amount)
	postData.Set("price", price)
	postData.Set("timeInForce", "GTC")
	signBinance(&postData, model.ApplicationConfig.ApiSecrets[model.Binance])
	headers := map[string]string{"X-MBX-APIKEY": model.ApplicationConfig.ApiKeys[model.Binance]}
	responseBody, _ := util.HttpRequest("POST",
		model.ApplicationConfig.RestUrls[model.Binance]+"/api/v3/order?", postData.Encode(), headers)
	util.Notice(symbol + "挂单binance:" + price + orderType + amount + "返回" + string(responseBody))
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
	util.Notice(symbol + "挂单binance:" + price + orderType + amount + "返回" + string(responseBody) + "errCode:" +
		errCode + "orderId" + orderId)
	return orderId, errCode
}

func CancelOrderBinance(symbol string, orderId string) {
	postData := url.Values{}
	postData.Set("symbol", strings.ToUpper(strings.Replace(symbol, "_", "", 1)))
	postData.Set("orderId", orderId)
	signBinance(&postData, model.ApplicationConfig.ApiSecrets[model.Binance])
	headers := map[string]string{"X-MBX-APIKEY": model.ApplicationConfig.ApiKeys[model.Binance]}
	requestUrl := model.ApplicationConfig.RestUrls[model.Binance] + "/api/v3/order?" + postData.Encode()
	responseBody, _ := util.HttpRequest("DELETE", requestUrl, "", headers)
	util.Notice("binance cancel order" + string(responseBody))
}

func QueryOrderBinance(symbol string, orderId string) (dealAmount float64, status string) {
	postData := url.Values{}
	postData.Set("symbol", strings.ToUpper(strings.Replace(symbol, "_", "", 1)))
	postData.Set("orderId", orderId)
	signBinance(&postData, model.ApplicationConfig.ApiSecrets[model.Binance])
	headers := map[string]string{"X-MBX-APIKEY": model.ApplicationConfig.ApiKeys[model.Binance]}
	requestUrl := model.ApplicationConfig.RestUrls[model.Binance] + "/api/v3/order?" + postData.Encode()
	responseBody, _ := util.HttpRequest("GET", requestUrl, "", headers)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		str, _ := orderJson.Get("executedQty").String()
		if str != "" {
			dealAmount, _ = strconv.ParseFloat(str, 64)
		}
		status, _ = orderJson.Get("status").String()
		status = model.OrderStatusMap[status]
	}
	util.SocketInfo(fmt.Sprintf("%s binance query order %f %s", status, dealAmount, responseBody))
	return dealAmount, status
}

func GetAccountBinance(accounts *model.Accounts) {
	accounts.ClearAccounts(model.Binance)
	postData := url.Values{}
	signBinance(&postData, model.ApplicationConfig.ApiSecrets[model.Binance])
	headers := map[string]string{"X-MBX-APIKEY": model.ApplicationConfig.ApiKeys[model.Binance]}
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
	Maintain(accounts, model.Binance)
}
