package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hello/model"
	"hello/util"
	"net/http"
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

func SignedRequestHuobi(market, method, path string, data map[string]interface{}) []byte {
	param := map[string]interface{}{"AccessKeyId": model.AppConfig.HuobiKey, "SignatureMethod": "HmacSHA256",
		"SignatureVersion": "2", `Timestamp`: url.QueryEscape(time.Now().UTC().Format("2006-01-02T15:04:05"))}
	strData := ``
	if method == `GET` {
		for key, value := range data {
			param[key] = value
		}
	} else if method == `POST` && data != nil {
		strData = string(util.JsonEncodeMapToByte(data))
	}
	strParam := util.ComposeParams(param)
	toBeSign := fmt.Sprintf("%s\n%s\n%s\n%s",
		method, model.AppConfig.RestUrls[market], path, strParam)
	hash := hmac.New(sha256.New, []byte(model.AppConfig.HuobiSecret))
	hash.Write([]byte(toBeSign))
	sign := url.QueryEscape(base64.StdEncoding.EncodeToString(hash.Sum(nil)))
	param["Signature"] = sign
	requestUrl := fmt.Sprintf(`https://%s%s?%s`, model.AppConfig.RestUrls[market], path, util.ComposeParams(param))
	headers := map[string]string{"Content-Type": "application/json", "Accept-Language": "zh-cn",
		"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	responseBody, _ := util.HttpRequest(method, requestUrl, strData, headers, 60)
	util.SocketInfo(fmt.Sprintf(`%s %s %s`, requestUrl, strData, string(responseBody)))
	return responseBody
}

func GetAccountIdsHuobi() (accountIds map[string]string, err error) {
	responseBody := SignedRequestHuobi(model.Huobi, `GET`, "/v1/account/accounts", nil)
	util.SocketInfo(`get huobi accounts: ` + string(responseBody))
	accountJson, err := util.NewJSON(responseBody)
	if err == nil {
		accounts, _ := accountJson.Get("data").Array()
		accountIds = make(map[string]string)
		for _, value := range accounts {
			account := value.(map[string]interface{})
			typeName := account["type"].(string)
			accountIds[typeName] = account["id"].(json.Number).String()
		}
	}
	return accountIds, err
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
	if model.HuobiAccountIds == nil || model.HuobiAccountIds[`spot`] == `` {
		model.HuobiAccountIds, _ = GetAccountIdsHuobi()
	}
	path := "/v1/order/orders/place"
	postData := make(map[string]interface{})
	postData["account-id"] = model.HuobiAccountIds[`spot`]
	postData["amount"] = amount
	postData["symbol"] = strings.ToLower(strings.Replace(symbol, "_", "", 1))
	postData["type"] = orderParam
	if orderType == model.OrderTypeLimit {
		postData["price"] = price
	}
	responseBody := SignedRequestHuobi(model.Huobi, `POST`, path, postData)
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
	responseBody := SignedRequestHuobi(model.Huobi, `POST`, path, nil)
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
	responseBody := SignedRequestHuobi(model.Huobi, `GET`, path, nil)
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

func parseBalanceHuobi(data map[string]interface{}) (balance *model.Balance) {
	if data == nil || data[`id`] == nil {
		return nil
	}
	balance = &model.Balance{
		AccountId: model.AppConfig.HuobiKey,
		Market:    model.Huobi,
	}
	balance.ID = model.Huobi + `_` + data[`id`].(json.Number).String()
	if data[`type`] != nil {
		if data[`type`].(string) == `deposit` {
			balance.Action = 1
		} else if data[`type`].(string) == `withdraw` {
			balance.Action = -1
		}
	}
	if data[`currency`] != nil {
		balance.Coin = strings.ToLower(data[`currency`].(string))
	}
	if data[`amount`] != nil {
		balance.Amount, _ = data[`amount`].(json.Number).Float64()
	}
	if data[`address`] != nil {
		balance.Address, _ = data[`address`].(string)
	}
	if data[`fee`] != nil {
		balance.Fee = data[`fee`].(json.Number).String()
	}
	if data[`state`] != nil {
		balance.Status, _ = data[`state`].(string)
	}
	if data[`updated-at`] != nil {
		seconds, _ := data[`updated-at`].(json.Number).Int64()
		balance.BalanceTime = time.Unix(seconds/1000, 0)
		fmt.Println(balance.BalanceTime.String())
	}
	return balance
}

func getTransactionHuobi() (balances []*model.Balance) {
	data := map[string]interface{}{`type`: `deposit`}
	response := SignedRequestHuobi(model.Huobi, http.MethodGet, `/v1/query/deposit-withdraw`, data)
	responseJson, err := util.NewJSON(response)
	if err == nil && responseJson != nil && responseJson.Get(`data`) != nil {
		items := responseJson.Get(`data`).MustArray()
		for _, item := range items {
			balance := parseBalanceHuobi(item.(map[string]interface{}))
			if balance != nil {
				balances = append(balances, balance)
			}
		}
	}
	data = map[string]interface{}{`type`: `withdraw`}
	response = SignedRequestHuobi(model.Huobi, http.MethodGet, `/v1/query/deposit-withdraw`, data)
	responseJson, err = util.NewJSON(response)
	if err == nil && responseJson != nil && responseJson.Get(`data`) != nil {
		items := responseJson.Get(`data`).MustArray()
		for _, item := range items {
			balance := parseBalanceHuobi(item.(map[string]interface{}))
			if balances != nil {
				balances = append(balances, balance)
			}
		}
	}
	return balances
}

// 资产账户 getBalanceHuobi
func _() (balances []*model.Balance) {
	if model.HuobiAccountIds == nil || len(model.HuobiAccountIds) == 0 {
		model.HuobiAccountIds, _ = GetAccountIdsHuobi()
	}
	balances = make([]*model.Balance, 0)
	for _, accountId := range model.HuobiAccountIds {
		path := fmt.Sprintf("/v1/account/accounts/%s/balance", accountId)
		response := SignedRequestHuobi(model.Huobi, http.MethodGet, path, nil)
		responseJson, err := util.NewJSON(response)
		if err == nil {
			balanceArray := responseJson.GetPath(`data`, `list`).MustArray()
			for _, item := range balanceArray {
				value := item.(map[string]interface{})
				balance := &model.Balance{
					AccountId:   accountId,
					BalanceTime: util.GetNow(),
					Market:      model.Huobi,
				}
				if value[`currency`] != nil {
					balance.Coin = value[`currency`].(string)
				}
				if value[`type`] != nil {
					balance.Status = value[`type`].(string)
				}
				if value[`balance`] != nil {
					balance.Amount, _ = strconv.ParseFloat(value[`balance`].(string), 64)
				}
				if balance.Amount > 0 {
					balance.ID = fmt.Sprintf(`%s_%s_%s_%s`,
						balance.Market, balance.Coin, balance.Status, balance.BalanceTime.String()[0:10])
					balances = append(balances, balance)
				}
			}
		}
	}
	return
}

func getAccountHuobiSpot(accounts *model.Accounts) {
	if model.HuobiAccountIds == nil || model.HuobiAccountIds[`spot`] == `` {
		model.HuobiAccountIds, _ = GetAccountIdsHuobi()
	}
	path := fmt.Sprintf("/v1/account/accounts/%s/balance", model.HuobiAccountIds[`spot`])
	postData := make(map[string]interface{})
	postData["accountId-id"] = model.HuobiAccountIds[`spot`]
	responseBody := SignedRequestHuobi(model.Huobi, `GET`, path, postData)
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
