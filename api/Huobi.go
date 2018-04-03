package api

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"sort"
	"strconv"
	"net/url"
	"fmt"
	"time"
	"strings"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"hello/util"
	"hello/model"
)

type HuobiMessage struct {
	Ping   int    `json:"ping"`
	Ch     string `json:"ch"`
	Ts     int    `json:"ts"`
	Req    string `json:"req"`
	Rep    string `json:"rep"`
	Status string `json:"status"`
	Id     string `json:"id"`
	Tick struct {
		Id     string      `json:"id"`     // K线id
		Amount float64     `json:"amount"` // 成交量
		Count  int         `json:"count"`  // 成交笔数
		Open   float64     `json:"open"`   // 开盘价
		Close  float64     `json:"close"`  // 收盘价,当K线为最晚的一根时，是最新成交价
		Low    float64     `json:"low"`    // 最低价
		High   float64     `json:"high"`   // 最高价
		Vol    float64     `json:"vol"`    // 成交额, 即 sum(每一笔成交价 * 该笔的成交量)
		Bids   [][]float64 `json:"bids"`
		Asks   [][]float64 `json:"asks"`
	} `json:"tick"`
}

var subscribeHandlerHuobi = func(subscribes []string, conn *websocket.Conn) error {
	var err error = nil
	for _, v := range subscribes {
		subscribeMap := make(map[string]interface{})
		subscribeMap["id"] = strconv.Itoa(util.GetNow().Nanosecond())
		subscribeMap["sub"] = v
		subscribeMessage := util.JsonEncodeMapToByte(subscribeMap)
		if err = conn.WriteMessage(websocket.TextMessage, subscribeMessage); err != nil {
			util.SocketInfo("huobi can not subscribe " + err.Error())
			return err
		}
	}
	return err
}

func WsDepthServeHuobi(markets *model.Markets, carryHandler CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
	wsHandler := func(event []byte, conn *websocket.Conn) {
		res := util.UnGzip(event)
		resMap := util.JsonDecodeByte(res)
		message := &HuobiMessage{}
		json.Unmarshal(res, message)
		if v, ok := resMap["ping"]; ok {
			pingMap := make(map[string]interface{})
			pingMap["pong"] = v
			pingParams := util.JsonEncodeMapToByte(pingMap)
			if err := conn.WriteMessage(websocket.TextMessage, pingParams); err != nil {
				util.SocketInfo("huobi server ping client error " + err.Error())
			}
		} else {
			if symbol := model.SubscribeSymbol[message.Ch]; symbol != "" {
				bidAsk := model.BidAsk{}
				bidAsk.Asks = message.Tick.Asks
				bidAsk.Bids = message.Tick.Bids
				sort.Sort(bidAsk.Asks)
				sort.Reverse(bidAsk.Bids)
				bidAsk.Ts = message.Ts
				markets.SetBidAsk(symbol, model.Huobi, &bidAsk)
				if carry, err := markets.NewCarry(symbol); err == nil {
					carryHandler(carry)
				}
			}
		}
	}
	return WebSocketServe(model.ApplicationConfig.WSUrls[model.Huobi], model.ApplicationConfig.GetSubscribes(model.Huobi), subscribeHandlerHuobi, wsHandler, errHandler)
}

func buildFormHuobi(postData *url.Values, path string, method string) {
	postData.Set("AccessKeyId", model.ApplicationConfig.ApiKeys[model.Huobi])
	postData.Set("SignatureMethod", "HmacSHA256")
	postData.Set("SignatureVersion", "2")
	postData.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05"))
	domain := strings.Replace(model.ApplicationConfig.RestUrls[model.Huobi], "https://", "", len(model.ApplicationConfig.RestUrls[model.Huobi]))
	payload := fmt.Sprintf("%s\n%s\n%s\n%s", method, domain, path, postData.Encode())
	hash := hmac.New(sha256.New, []byte(model.ApplicationConfig.ApiSecrets[model.Huobi]))
	hash.Write([]byte(payload))
	sign := base64.StdEncoding.EncodeToString(hash.Sum(nil))
	postData.Set("Signature", sign)
}

func GetSpotAccountId(config *model.Config) (accountId string, err error) {
	path := "/v1/account/accounts"
	postData := &url.Values{}
	buildFormHuobi(postData, path, "GET")
	requestUrl := config.RestUrls[model.Huobi] + path + "?" + postData.Encode()
	headers := map[string]string{"Content-Type": "application/json", "Accept-Language": "zh-cn"}
	responseBody, _ := util.HttpRequest("GET", requestUrl, "", headers)
	accountJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		accounts, _ := accountJson.Get("data").Array()
		for _, value := range accounts {
			account := value.(map[string]interface{})
			typeName := account["type"].(string)
			if typeName == "spot" {
				accountId = account["id"].(json.Number).String()
			}
		}
	}
	return accountId, err
}

// orderType: buy-market：市价买, sell-market：市价卖, buy-limit：限价买, sell-limit：限价卖
func PlaceOrderHuobi(symbol string, orderType string, price string, amount string) (orderId, errCode string) {
	path := "/v1/order/orders/place"
	postData := &url.Values{}
	postData.Set("account-id", model.HuobiAccountId)
	postData.Set("amount", amount)
	postData.Set("symbol", strings.ToLower(strings.Replace(symbol, "_", "", 1)))
	postData.Set("type", orderType)
	postData.Set("price", price)
	buildFormHuobi(postData, path, "POST")

	headers := map[string]string{"Content-Type": "application/json", "Accept-Language": "zh-cn"}
	responseBody, _ := util.HttpRequest("POST", model.ApplicationConfig.RestUrls[model.Huobi]+path+"?"+postData.Encode(), util.ToJson(postData), headers)

	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		status, _ := orderJson.Get("status").String()
		if status == "ok" {
			orderId, _ = orderJson.Get("data").String()
		} else if status == "error" {
			errCode, _ = orderJson.Get("err-code").String()
		}
	}
	util.Notice(symbol + "挂单huobi:" + price + orderType + amount + "返回" + string(responseBody) + "errCode:" + errCode + "orderId" + orderId)
	return orderId, errCode
}

func CancelOrderHuobi(orderId string) {
	path := fmt.Sprintf("/v1/order/orders/%s/submitcancel", orderId)
	postData := &url.Values{}
	buildFormHuobi(postData, path, "POST")
	requestUrl := model.ApplicationConfig.RestUrls[model.Huobi] + path + "?" + postData.Encode()
	headers := map[string]string{"Content-Type": "application/json", "Accept-Language": "zh-cn"}
	responseBody, _ := util.HttpRequest("POST", requestUrl, util.ToJson(postData), headers)
	util.SocketInfo("huobi cancel order" + orderId + string(responseBody))
}

func QueryOrderHuobi(orderId string) (dealAmount float64, status string) {
	path := fmt.Sprintf("/v1/order/orders/%s", orderId)
	postData := &url.Values{}
	buildFormHuobi(postData, path, "GET")
	requestUrl := model.ApplicationConfig.RestUrls[model.Huobi] + path + "?" + postData.Encode()
	headers := map[string]string{"Content-Type": "application/json", "Accept-Language": "zh-cn"}
	responseBody, _ := util.HttpRequest("GET", requestUrl, "", headers)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		status, _ = orderJson.GetPath("data", "state").String()
		status = model.OrderStatusMap[status]
		dealAmount, _ = orderJson.GetPath("data", "field-amount").Float64()
	}
	util.SocketInfo(status + "huobi query order" + string(responseBody))
	return dealAmount, status
}

func GetAccountHuobi(accounts *model.Accounts) {
	path := fmt.Sprintf("/v1/account/accounts/%s/balance", model.HuobiAccountId)
	postData := &url.Values{}
	postData.Set("accountId-id", model.HuobiAccountId)
	buildFormHuobi(postData, path, "GET")
	requestUrl := model.ApplicationConfig.RestUrls[model.Huobi] + path + "?" + postData.Encode()
	util.SocketInfo("huobi get account url" + requestUrl)
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded",
		"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	responseBody, _ := util.HttpRequest("GET", requestUrl, "", headers)
	balanceJson, err := util.NewJSON(responseBody)
	if err == nil {
		accountType, _ := balanceJson.GetPath("data", "type").String()
		state, _ := balanceJson.GetPath("data", "state").String()
		if accountType == "spot" && state == "working" {
			currencies, _ := balanceJson.GetPath("data", "list").Array()
			for _, value := range currencies {
				currency := value.(map[string]interface{})
				balance, _ := strconv.ParseFloat(currency["balance"].(string), 64)
				if balance > 0 {
					account := accounts.GetAccount(model.Huobi, currency["currency"].(string))
					if account == nil {
						currencyName := strings.ToLower(currency["currency"].(string))
						account = &model.Account{Market: model.Huobi, Currency: currencyName}
						accounts.SetAccount(model.Huobi, currencyName, account)
					}
					if currency["type"].(string) == "trade" {
						account.Free = balance
					}
					if currency["type"].(string) == "frozen" {
						account.Frozen = balance
					}
					model.AccountChannel <- *account
				}
			}
		}
	}
}
