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
		symbol := json.Get("type").MustString()
		if symbol == `hello` {
			return
		}
		symbol = model.GetSymbol(model.Fcoin, symbol)
		if symbol != "" && symbol != "_" {
			bidAsk := model.BidAsk{}
			bidsLen := len(json.Get("bids").MustArray()) / 2
			bidAsk.Bids = make([][]float64, bidsLen)
			for i := 0; i < bidsLen; i++ {
				bidAsk.Bids[i] = make([]float64, 2)
				bidAsk.Bids[i][0], _ = json.Get("bids").GetIndex(i * 2).Float64()
				bidAsk.Bids[i][1], _ = json.Get("bids").GetIndex(i*2 + 1).Float64()
			}
			asksLen := len(json.Get("asks").MustArray()) / 2
			bidAsk.Asks = make([][]float64, asksLen)
			for i := 0; i < asksLen; i++ {
				bidAsk.Asks[i] = make([]float64, 2)
				bidAsk.Asks[i][0], _ = json.Get("asks").GetIndex(i * 2).Float64()
				bidAsk.Asks[i][1], _ = json.Get("asks").GetIndex(i*2 + 1).Float64()
			}
			sort.Sort(bidAsk.Asks)
			sort.Reverse(bidAsk.Bids)
			bidAsk.Ts = json.Get("ts").MustInt()
			if markets.SetBidAsk(symbol, model.Fcoin, &bidAsk) {
				for _, handler := range carryHandlers {
					handler(symbol, model.Fcoin)
				}
			}
		}
	}
	requestUrl := model.ApplicationConfig.WSUrls[model.Fcoin]
	return WebSocketServe(requestUrl, model.ApplicationConfig.GetSubscribes(model.Fcoin), subscribeHandlerFcoin,
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
// type limit market
func PlaceOrderFcoin(symbol, side, orderType, price, amount string) (orderId, errCode string) {
	postData := make(map[string]interface{})
	postData["symbol"] = strings.ToLower(strings.Replace(symbol, "_", "", 1))
	postData["type"] = orderType
	postData["side"] = side
	postData["amount"] = amount
	if orderType == `limit` {
		postData["price"] = price
	}
	responseBody := SignedRequestFcoin("POST", "/orders", postData)
	util.Notice("fcoin place order" + string(responseBody))
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		orderId, _ := orderJson.Get("data").String()
		status, _ := orderJson.Get("status").Int()
		return orderId, strconv.Itoa(status)
	}
	return ``, err.Error()
}

func CancelOrderFcoin(orderId string) int {
	responseBody := SignedRequestFcoin(`POST`, `/orders/`+orderId+`/submit-cancel`, nil)
	json, err := util.NewJSON([]byte(responseBody))
	status := -1
	if err == nil {
		status, _ = json.Get(`status`).Int()
	}
	util.Notice("fcoin cancel order" + string(responseBody))
	return status
}

func QueryOrderFcoin(symbol, orderId string) (dealAmount float64, status string) {
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
		status, _ = orderJson.Get("state").String()
		status = model.OrderStatusMap[status]
	}
	util.Notice(fmt.Sprintf("%s fcoin query order %f %s", status, dealAmount, responseBody))
	return dealAmount, status
}

func GetAccountFcoin(accounts *model.Accounts) {
	//accounts.ClearAccounts(model.Fcoin)
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
	Maintain(accounts, model.Fcoin)
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
