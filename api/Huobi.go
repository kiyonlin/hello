package api

import (
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"hello/model"
	"hello/util"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type HuobiMessage struct {
	Ping   int    `json:"ping"`
	Ch     string `json:"ch"`
	Ts     int    `json:"ts"`
	Req    string `json:"req"`
	Rep    string `json:"rep"`
	Status string `json:"status"`
	Id     string `json:"id"`
	Tick   struct {
		SeqNum float64     `json:"seqNum"`
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

var subscribeHandlerHuobi = func(subscribes []interface{}, subType string) error {
	var err error = nil
	for _, v := range subscribes {
		subscribeMap := make(map[string]interface{})
		subscribeMap["id"] = strconv.Itoa(util.GetNow().Nanosecond())
		subscribeMap["sub"] = v
		subscribeMessage := util.JsonEncodeMapToByte(subscribeMap)
		if err = sendToWs(model.Huobi, subscribeMessage); err != nil {
			util.SocketInfo("huobi can not subscribe " + err.Error())
			return err
		}
		//util.SocketInfo(`huobi subscribed ` + v)
	}
	return err
}

func WsDepthServeHuobi(markets *model.Markets, errHandler ErrHandler) (chan struct{}, error) {
	wsHandler := func(event []byte) {
		res := util.UnGzip(event)
		resMap := util.JsonDecodeByte(res)
		message := &HuobiMessage{}
		_ = json.Unmarshal(res, message)
		if v, ok := resMap["ping"]; ok {
			pingMap := make(map[string]interface{})
			pingMap["pong"] = v
			pingParams := util.JsonEncodeMapToByte(pingMap)
			if err := sendToWs(model.Huobi, pingParams); err != nil {
				util.SocketInfo("huobi server ping client error " + err.Error())
			}
		} else {
			symbol := model.GetSymbol(model.Huobi, message.Ch)
			if symbol != "" {
				bidAsk := model.BidAsk{}
				bidAsk.Asks = make([]model.Tick, len(message.Tick.Asks))
				bidAsk.Bids = make([]model.Tick, len(message.Tick.Bids))
				for key, value := range message.Tick.Asks {
					bidAsk.Asks[key] = model.Tick{Price: value[0], Amount: value[1]}
				}
				for key, value := range message.Tick.Bids {
					bidAsk.Bids[key] = model.Tick{Price: value[0], Amount: value[1]}
				}
				sort.Sort(bidAsk.Asks)
				sort.Sort(sort.Reverse(bidAsk.Bids))
				bidAsk.Ts = message.Ts
				if markets.SetBidAsk(symbol, model.Huobi, &bidAsk) {
					for function, handler := range model.GetFunctions(model.Huobi, symbol) {
						if handler != nil {
							settings := model.GetSetting(function, model.Huobi, symbol)
							for _, setting := range settings {
								go handler(setting)
							}
						}
					}
				}
			}
		}
	}
	return WebSocketServe(model.Huobi, model.AppConfig.WSUrls[model.Huobi], model.SubscribeDepth,
		GetWSSubscribes(model.Huobi, model.SubscribeDepth), subscribeHandlerHuobi, wsHandler, errHandler)
}

func SignedRequestHuobi(method, path string, data map[string]string) []byte {
	urlValues := &url.Values{}
	urlValues.Set("AccessKeyId", model.AppConfig.HuobiKey)
	urlValues.Set("SignatureMethod", "HmacSHA256")
	urlValues.Set("SignatureVersion", "2")
	urlValues.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05"))
	if method == `GET` && data != nil {
		for key, value := range data {
			urlValues.Set(key, value)
		}
	}
	domain := strings.Replace(model.AppConfig.RestUrls[model.Huobi], "https://", "",
		len(model.AppConfig.RestUrls[model.Huobi]))
	payload := fmt.Sprintf("%s\n%s\n%s\n%s", method, domain, path, urlValues.Encode())
	hash := hmac.New(sha256.New, []byte(model.AppConfig.HuobiSecret))
	hash.Write([]byte(payload))
	sign := base64.StdEncoding.EncodeToString(hash.Sum(nil))
	urlValues.Set("Signature", sign)
	var pemBytes = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIJUh+m2GyS9GKsEZ0/5WqM3owjYGtttQXPl9pR8nks+moAoGCCqGSM49
AwEHoUQDQgAEF+5o7rybYv7/40CSReXKr2jxiW9iVE1+l/6vjnDSkyK8mCw220QM
J2k98epEs68Y+OjaRp0uP8821WkP5tLM1Q==
-----END EC PRIVATE KEY-----`
	block, _ := pem.Decode([]byte(pemBytes))
	ecdsaPk, _ := x509.ParseECPrivateKey(block.Bytes)
	digest := sha256.Sum256([]byte(sign))
	r, s, _ := ecdsa.Sign(rand.Reader, ecdsaPk, digest[:])
	//	encode the signature {R, S}
	params := ecdsaPk.Curve.Params()
	curveByteSize := params.P.BitLen() / 8
	rBytes, sBytes := r.Bytes(), s.Bytes()
	privateSign := make([]byte, curveByteSize*2)
	copy(privateSign[curveByteSize-len(rBytes):], rBytes)
	copy(privateSign[curveByteSize*2-len(sBytes):], sBytes)
	urlValues.Set(`PrivateSignature`, base64.StdEncoding.EncodeToString(privateSign))
	requestUrl := model.AppConfig.RestUrls[model.Huobi] + path + "?" + urlValues.Encode()
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded",
		"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	var postBody = ``
	if method == `POST` && data != nil {
		for key, value := range data {
			urlValues.Set(key, value)
		}
		headers = map[string]string{"Content-Type": "application/json", "Accept-Language": "zh-cn"}
		postBody = util.ToJson(urlValues)
	}
	responseBody, _ := util.HttpRequest(method, requestUrl, postBody, headers, 60)
	return responseBody
}

func GetSpotAccountId() (accountId string, err error) {
	responseBody := SignedRequestHuobi(`GET`, "/v1/account/accounts", nil)
	accountJson, err := util.NewJSON(responseBody)
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
// huobi中amount在市价买单中指的是右侧的钱
func placeOrderHuobi(order *model.Order, orderSide, orderType, symbol, price, amount string) {
	orderParam := ``
	if orderSide == model.OrderSideBuy && orderType == model.OrderTypeLimit {
		orderParam = `buy-limit`
	} else if orderSide == model.OrderSideBuy && orderType == model.OrderTypeMarket {
		orderParam = `buy-market`
	} else if orderSide == model.OrderSideSell && orderType == model.OrderTypeLimit {
		orderParam = `sell-limit`
	} else if orderSide == model.OrderSideSell && orderType == model.OrderTypeMarket {
		orderParam = `sell-market`
	} else {
		util.Notice(fmt.Sprintf(`[parameter error] order side: %s order type: %s`, orderSide, orderType))
	}
	if model.HuobiAccountId == `` {
		model.HuobiAccountId, _ = GetSpotAccountId()
	}
	path := "/v1/order/orders/place"
	postData := make(map[string]string)
	postData["account-id"] = model.HuobiAccountId
	postData["amount"] = amount
	postData["symbol"] = strings.ToLower(strings.Replace(symbol, "_", "", 1))
	postData["type"] = orderParam
	if orderType == model.OrderTypeLimit {
		postData["price"] = price
	}
	responseBody := SignedRequestHuobi(`POST`, path, postData)
	orderJson, err := util.NewJSON(responseBody)
	if err == nil {
		status, _ := orderJson.Get("status").String()
		if status == "ok" {
			order.OrderId, _ = orderJson.Get("data").String()
		} else if status == "error" {
			order.OrderId, _ = orderJson.Get("err-code").String()
		}
	}
	util.Notice(fmt.Sprintf(`[挂单huobi] %s side: %s type: %s price: %s amount: %s order id %s 返回%s`,
		symbol, orderSide, orderType, price, amount, order.OrderId, string(responseBody)))
}

func cancelOrderHuobi(orderId string) (result bool, errCode, msg string) {
	path := fmt.Sprintf("/v1/order/orders/%s/submitcancel", orderId)
	responseBody := SignedRequestHuobi(`POST`, path, nil)
	orderJson, err := util.NewJSON(responseBody)
	util.Notice("huobi cancel order" + orderId + string(responseBody))
	if err == nil {
		status, _ := orderJson.Get("status").String()
		if status == "ok" {
			return true, ``, ``
		} else if status == "error" {
			errCode, _ = orderJson.Get("err-code").String()
			msg, _ = orderJson.Get(`err-msg`).String()
			return false, errCode, msg
		}
	} else {
		return false, err.Error(), err.Error()
	}
	return false, ``, ``
}

func queryOrderHuobi(orderId string) (dealAmount, dealPrice float64, status string) {
	path := fmt.Sprintf("/v1/order/orders/%s", orderId)
	responseBody := SignedRequestHuobi(`GET`, path, nil)
	orderJson, err := util.NewJSON(responseBody)
	if err == nil {
		status, _ = orderJson.GetPath("data", "state").String()
		status = model.GetOrderStatus(model.Huobi, status)
		str, _ := orderJson.GetPath("data", "field-amount").String()
		if str != "" {
			dealAmount, _ = strconv.ParseFloat(str, 64)
		}
		strDealPrice, _ := orderJson.GetPath(`data`, `price`).String()
		if strDealPrice != `` {
			dealPrice, _ = strconv.ParseFloat(strDealPrice, 64)
		}
	}
	util.Notice(fmt.Sprintf("%s huobi query order %f %s", status, dealAmount, responseBody))
	return dealAmount, dealPrice, status
}

func getAccountHuobi(accounts *model.Accounts) {
	if model.HuobiAccountId == `` {
		model.HuobiAccountId, _ = GetSpotAccountId()
	}
	path := fmt.Sprintf("/v1/account/accounts/%s/balance", model.HuobiAccountId)
	postData := make(map[string]string)
	postData["accountId-id"] = model.HuobiAccountId
	responseBody := SignedRequestHuobi(`GET`, path, postData)
	balanceJson, err := util.NewJSON(responseBody)
	if err == nil {
		accountType, _ := balanceJson.GetPath("data", "type").String()
		state, _ := balanceJson.GetPath("data", "state").String()
		if accountType == "spot" && state == "working" {
			currencies, _ := balanceJson.GetPath("data", "list").Array()
			for _, value := range currencies {
				currency := value.(map[string]interface{})
				balance, _ := strconv.ParseFloat(currency["balance"].(string), 64)
				if balance == 0 {
					continue
				}
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
			}
		}
	}
}
