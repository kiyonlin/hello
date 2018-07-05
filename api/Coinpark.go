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

func WsDepthServeCoinpark(markets *model.Markets, carryHandler CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
	wsHandler := func(event []byte, conn *websocket.Conn) {
		fmt.Println(string(event))
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
				bidAsk.Asks = make([][]float64, len(askArray))
				bidAsk.Bids = make([][]float64, len(bidArray))
				for i, value := range bidArray {
					bidAsk.Bids[i] = make([]float64, 2)
					str := value.(map[string]interface{})["price"].(string)
					bidAsk.Bids[i][0], _ = strconv.ParseFloat(str, 10)
					str = value.(map[string]interface{})["volume"].(string)
					bidAsk.Bids[i][1], _ = strconv.ParseFloat(str, 10)
				}
				for i, value := range askArray {
					bidAsk.Asks[i] = make([]float64, 2)
					str := value.(map[string]interface{})["price"].(string)
					bidAsk.Asks[i][0], _ = strconv.ParseFloat(str, 10)
					str = value.(map[string]interface{})["volume"].(string)
					bidAsk.Asks[i][1], _ = strconv.ParseFloat(str, 10)
				}
				sort.Sort(bidAsk.Asks)
				sort.Reverse(bidAsk.Bids)
				if markets.SetBidAsk(symbol, model.Coinpark, &bidAsk) {
					if carry, err := markets.NewCarry(symbol); err == nil {
						carryHandler(carry)
					}
				}
			}
		}
	}
	return WebSocketServe(model.ApplicationConfig.WSUrls[model.Coinpark],
		model.ApplicationConfig.GetSubscribes(model.Coinpark), subscribeHandlerCoinpark, wsHandler, errHandler)
}

func SignedRequestCoinpark(method, path, cmds string) []byte {
	hash := hmac.New(md5.New, []byte(model.ApplicationConfig.CoinparkSecret))
	hash.Write([]byte(cmds))
	sign := hex.EncodeToString(hash.Sum(nil))
	postData := &url.Values{}
	postData.Set("cmds", cmds)
	postData.Set("apikey", model.ApplicationConfig.CoinparkKey)
	postData.Set("sign", sign)
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded", "User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	responseBody, _ := util.HttpRequest(method, model.ApplicationConfig.RestUrls[model.Coinpark]+path,
		postData.Encode(), headers)
	return responseBody
}

func getBuyPriceCoinpark(symbol string) (float64, error) {
	cmd := fmt.Sprintf(`[{"cmd":"api/ticker","body":{"pair":"%s"}}]`, strings.ToUpper(symbol))
	responseBody := SignedRequestCoinpark(`POST`, "/mdata", cmd)
	accountJson, err := util.NewJSON(responseBody)
	if err == nil {
		results, err := accountJson.Get("result").Array()
		if err == nil && len(results) > 0 {
			strPrice := results[0].(map[string]interface{})["result"].(map[string]interface{})[`buy`].(string)
			return strconv.ParseFloat(strPrice, 10)
		}
	}
	return 0, err
}

func GetAccountCoinpark(accounts *model.Accounts) {
	accounts.ClearAccounts(model.Coinpark)
	cmds := `[{"cmd":"transfer/assets","body":{"select":1}}]`
	responseBody := SignedRequestCoinpark(`POST`, `/transfer`, cmds)
	accountJson, err := util.NewJSON(responseBody)
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
	Maintain(accounts, model.Coinpark)
}

// order_type 交易类型，2-限价单
// order_side 交易方向，1-买，2-卖
func PlaceOrderCoinpark(symbol string, side, orderType int, price, amount string) (orderId, errCode, errMsg string) {
	symbol = strings.ToUpper(symbol)
	cmds := fmt.Sprintf(`[{"cmd":"orderpending/trade",
		"body":{"pair":"%s","account_type":0,"order_type":%d,"order_side":"%d","price":%s,"amount":"%s"}}]`,
		symbol, orderType, side, price, amount)
	responseBody := SignedRequestCoinpark(`POST`, `/orderpending`, cmds)
	util.Notice(`[place order]` + string(responseBody))
	orderJson, err := util.NewJSON([]byte(responseBody))
	if orderJson.Get(`result`) != nil {
		results, err := orderJson.Get("result").Array()
		if err == nil && len(results) > 0 {
			errorData := results[0].(map[string]interface{})[`error`]
			resultData := results[0].(map[string]interface{})["result"]
			if resultData != nil {
				str, _ := resultData.(json.Number).Int64()
				return strconv.FormatInt(str, 10), ``, ``
			}
			if errorData != nil {
				errCode = errorData.(map[string]interface{})[`code`].(string)
				errMsg = errorData.(map[string]interface{})[`msg`].(string)
				return ``, errCode, errMsg
			}
		}
	}
	return ``, err.Error(), `response format err`
}

func QueryOrderCoinpark(orderId string) (dealAmount float64, status string) {
	cmds := fmt.Sprintf(`[{"cmd":"orderpending/order","body":{"id":"%s"}}]`, orderId)
	responseBody := SignedRequestCoinpark(`POST`, `/orderpending`, cmds)
	orderJson, err := util.NewJSON([]byte(responseBody))
	results, err := orderJson.Get("result").Array()
	if err == nil && len(results) > 0 {
		resultData := results[0].(map[string]interface{})[`result`]
		if resultData != nil {
			strDealAmount := resultData.(map[string]interface{})[`deal_amount`].(string)
			intStatus, _ := resultData.(map[string]interface{})[`status`].(json.Number).Int64()
			status = model.OrderStatusMap[fmt.Sprintf(`%s%d`, model.Coinpark, intStatus)]
			if strDealAmount != "" {
				dealAmount, _ = strconv.ParseFloat(strDealAmount, 64)
			}
		}
	}
	return dealAmount, status
}

func CancelOrderCoinpark(orderId string) (code, msg string) {
	cmds := fmt.Sprintf(`[{"cmd":"orderpending/cancelTrade","body":{"orders_id":"%s"}}]`, orderId)
	responseBody := SignedRequestCoinpark(`POST`, `/orderpending`, cmds)
	orderJson, err := util.NewJSON([]byte(responseBody))
	results, err := orderJson.Get("result").Array()
	if err == nil && len(results) > 0 {
		errorData := results[0].(map[string]interface{})[`error`]
		resultData := results[0].(map[string]interface{})["result"]
		if resultData != nil {
			return `0`, resultData.(string)
		}
		if errorData != nil {
			code = errorData.(map[string]interface{})[`code`].(string)
			msg = errorData.(map[string]interface{})[`msg`].(string)
			return code, msg
		}
	}
	return ``, err.Error()
}
