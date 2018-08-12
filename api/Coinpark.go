package api

import (
	"crypto/hmac"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"hello/model"
	"hello/util"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

var subscribeHandlerCoinpark = func(subscribes []string, conn *websocket.Conn) error {
	var err error = nil
	for _, v := range subscribes {
		subscribeMap := make(map[string]interface{})
		subscribeMap["event"] = "addChannel"
		subscribeMap["channel"] = v
		subscribeMap[`binary`] = 0
		subscribeMessage := util.JsonEncodeMapToByte(subscribeMap)
		if err = conn.WriteMessage(websocket.TextMessage, subscribeMessage); err != nil {
			util.SocketInfo("coinpark can not subscribe " + err.Error())
			return err
		}
	}
	return err
}

func WsDepthServeCoinpark(markets *model.Markets, carryHandlers []CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
	wsHandler := func(event []byte, conn *websocket.Conn) {
		depthJson, err := util.NewJSON(event)
		if err != nil {
			errHandler(err)
			return
		}
		if depthJson == nil {
			return
		}
		depthArray, err := depthJson.Array()
		if err == nil && len(depthArray) > 0 {
			data := depthArray[0].(map[string]interface{})[`data`].(map[string]interface{})
			if data != nil {
				if data[`pair`] == nil {
					return
				}
				symbol := strings.ToLower(data[`pair`].(string))
				time, _ := data[`update_time`].(json.Number).Int64()
				bidAsk := model.BidAsk{Ts: int(time)}
				askArray := data[`asks`].([]interface{})
				bidArray := data[`bids`].([]interface{})
				bidAsk.Asks = make([]model.Tick, len(askArray))
				bidAsk.Bids = make([]model.Tick, len(bidArray))
				for i, value := range bidArray {
					str := value.(map[string]interface{})["price"].(string)
					price, _ := strconv.ParseFloat(str, 10)
					str = value.(map[string]interface{})["volume"].(string)
					amount, _ := strconv.ParseFloat(str, 10)
					bidAsk.Bids[i] = model.Tick{Price: price, Amount: amount}
				}
				for i, value := range askArray {
					str := value.(map[string]interface{})["price"].(string)
					price, _ := strconv.ParseFloat(str, 10)
					str = value.(map[string]interface{})["volume"].(string)
					amount, _ := strconv.ParseFloat(str, 10)
					bidAsk.Asks[i] = model.Tick{Price: price, Amount: amount}
				}
				sort.Sort(bidAsk.Asks)
				sort.Sort(sort.Reverse(bidAsk.Bids))
				if markets.SetBidAsk(symbol, model.Coinpark, &bidAsk) {
					for _, handler := range carryHandlers {
						handler(symbol, model.Coinpark)
					}
				}
			}
		}
	}
	return WebSocketServe(model.ApplicationConfig.WSUrls[model.Coinpark],
		model.GetDepthSubscribes(model.Coinpark), subscribeHandlerCoinpark, wsHandler, errHandler)
}

func SignedRequestCoinpark(method, path, cmds string) []byte {
	hash := hmac.New(md5.New, []byte(model.ApplicationConfig.CoinparkSecret))
	hash.Write([]byte(cmds))
	sign := hex.EncodeToString(hash.Sum(nil))
	postData := &url.Values{}
	postData.Set("cmds", cmds)
	postData.Set("apikey", model.ApplicationConfig.CoinparkKey)
	postData.Set("sign", sign)
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded",
		"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	responseBody, _ := util.HttpRequest(method, model.ApplicationConfig.RestUrls[model.Coinpark]+path,
		postData.Encode(), headers)
	return responseBody
}

func getBuyPriceCoinpark(symbol string) (float64, error) {
	symbol = strings.ToUpper(symbol)
	//cmd := fmt.Sprintf(`[{"cmd":"api/ticker","body":{"pair":"%s"}}]`, strings.ToUpper(symbol))
	//responseBody := SignedRequestCoinpark(`POST`, "/mdata", cmd)
	responseBody, _ := util.HttpRequest(`GET`, fmt.Sprintf(`%s/mdata?cmd=ticker&pair=%s`,
		model.ApplicationConfig.RestUrls[model.Coinpark], symbol), ``, nil)
	util.Notice(symbol + `[account]` + string(responseBody))
	accountJson, err := util.NewJSON(responseBody)
	if err == nil {
		strPrice, _ := accountJson.GetPath("result", `last`).String()
		return strconv.ParseFloat(strPrice, 10)
	}
	return 0, err
}

func getAccountCoinpark(accounts *model.Accounts) {
	cmds := `[{"cmd":"transfer/assets","body":{"select":1}}]`
	responseBody := SignedRequestCoinpark(`POST`, `/transfer`, cmds)
	accountJson, err := util.NewJSON(responseBody)
	if accountJson == nil {
		return
	}
	if err == nil {
		results, err := accountJson.Get("result").Array()
		if err == nil && len(results) > 0 {
			assets := results[0].(map[string]interface{})["result"].(map[string]interface{})["assets_list"].([]interface{})
			for _, value := range assets {
				value := value.(map[string]interface{})
				currencyName := strings.ToLower(value["coin_symbol"].(string))
				account := accounts.GetAccount(model.Coinpark, currencyName)
				if account == nil {
					account = &model.Account{Market: model.Coinpark, Currency: currencyName}
					accounts.SetAccount(model.Coinpark, currencyName, account)
				}
				account.Free, _ = strconv.ParseFloat(value["balance"].(string), 64)
				account.Frozen, _ = strconv.ParseFloat(value["freeze"].(string), 64)
			}
		}
	}
}

// order_side 交易方向，1-买，2-卖
// order_type 交易类型，2-限价单
func placeOrderCoinpark(orderSide, orderType, symbol, price, amount string) (orderId, errCode, errMsg string) {
	if orderSide == model.OrderSideBuy {
		orderSide = `1`
	} else if orderSide == model.OrderSideSell {
		orderSide = `2`
		temp, _ := strconv.ParseFloat(amount, 64)
		if temp > 50000 {
			util.Notice(orderType + `==sell==do not execute ` + amount)
			return ``, ``, orderType + `==sell==do not execute ` + amount
		}
	} else {
		util.Notice(fmt.Sprintf(`[parameter error] order side: %s`, orderSide))
		return ``, ``, fmt.Sprintf(`[parameter error] order side: %s`, orderSide)
	}
	if orderType == model.OrderTypeLimit {
		orderType = `2`
	} else {
		orderType = `2`
		util.Info(fmt.Sprintf(`[parameter error] order type: %s`, orderType))
	}
	symbol = strings.ToUpper(symbol)
	cmds := fmt.Sprintf(`[{"cmd":"orderpending/trade",
		"body":{"pair":"%s","account_type":0,"order_type":%s,"order_side":"%s","price":%s,"amount":"%s"}}]`,
		symbol, orderType, orderSide, price, amount)
	responseBody := SignedRequestCoinpark(`POST`, `/orderpending`, cmds)
	util.Notice(cmds + `[place order]` + string(responseBody))
	orderJson, _ := util.NewJSON([]byte(responseBody))
	if orderJson == nil {
		return
	}
	if orderJson.Get(`result`) != nil {
		results, err := orderJson.Get("result").Array()
		if err == nil && len(results) > 0 {
			resultData := results[0].(map[string]interface{})["result"]
			if resultData != nil {
				str, _ := resultData.(json.Number).Int64()
				return strconv.FormatInt(str, 10), ``, ``
			}
		}
	}
	errorJson := orderJson.Get(`error`)
	if errorJson.Get(`error`) != nil {
		errorCodeJson := errorJson.Get(`code`)
		if errorCodeJson != nil {
			errCode, _ = errorCodeJson.String()
		}
		errMsgJson := errorJson.Get(`msg`)
		if errMsgJson != nil {
			errMsg, _ = errMsgJson.String()
		}
		return ``, errCode, errMsg
	}
	return ``, ``, `response format err`
}

//dealPrice 返回委托价格，市价单是0
func QueryOrderCoinpark(orderId string) (dealAmount, dealPrice float64, status string) {
	cmds := fmt.Sprintf(`[{"cmd":"orderpending/order","body":{"id":"%s"}}]`, orderId)
	responseBody := SignedRequestCoinpark(`POST`, `/orderpending`, cmds)
	orderJson, err := util.NewJSON([]byte(responseBody))
	util.Notice(string(responseBody))
	results, err := orderJson.Get("result").Array()
	if orderJson == nil {
		return
	}
	if err == nil && len(results) > 0 {
		resultData := results[0].(map[string]interface{})[`result`]
		if resultData != nil {
			strDealAmount := resultData.(map[string]interface{})[`deal_amount`].(string)
			if strDealAmount != "" {
				dealAmount, _ = strconv.ParseFloat(strDealAmount, 64)
			}
			strDealPrice := resultData.(map[string]interface{})[`price`].(string)
			if strDealPrice != `` {
				dealPrice, _ = strconv.ParseFloat(strDealPrice, 64)
			}
			intStatus, _ := resultData.(map[string]interface{})[`status`].(json.Number).Int64()
			status = model.GetOrderStatus(model.Coinpark, fmt.Sprintf(`%s%d`, model.Coinpark, intStatus))
		}
	}
	return dealAmount, dealPrice, status
}

func CancelOrderCoinpark(orderId string) (result bool, code, msg string) {
	cmds := fmt.Sprintf(`[{"cmd":"orderpending/cancelTrade","body":{"orders_id":"%s"}}]`, orderId)
	responseBody := SignedRequestCoinpark(`POST`, `/orderpending`, cmds)
	util.Notice(orderId + `[cancel order]` + string(responseBody))
	if strings.TrimSpace(string(responseBody)) == `` {
		return
	}
	orderJson, _ := util.NewJSON([]byte(responseBody))
	if orderJson == nil {
		util.Notice(`no result in response coinpark ` + orderId)
	}
	orderJson = orderJson.Get("result")
	if orderJson == nil {
		util.Notice(`no result in response coinpark ` + orderId)
	}
	results, err := orderJson.Array()
	if err == nil && len(results) > 0 {
		errorData := results[0].(map[string]interface{})[`error`]
		resultData := results[0].(map[string]interface{})["result"]
		if resultData != nil {
			return true, ``, resultData.(string)
		}
		if errorData != nil {
			code = errorData.(map[string]interface{})[`code`].(string)
			msg = errorData.(map[string]interface{})[`msg`].(string)
			return false, code, msg
		}
	}
	return false, err.Error(), err.Error()
}
