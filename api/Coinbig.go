package api

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"github.com/gorilla/websocket"
	"hello/model"
	"hello/util"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"fmt"
)

var subscribeHandlerCoinbig = func(subscribes []string, conn *websocket.Conn) error {
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

func WsDepthServeCoinbig(markets *model.Markets, carryHandler CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
	lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte, conn *websocket.Conn) {
		if util.GetNow().Unix()-lastPingTime > 30 { // ping okex server every 30 seconds
			lastPingTime = util.GetNow().Unix()
			pingMap := make(map[string]interface{})
			pingMap["event"] = "ping"
			pingParams := util.JsonEncodeMapToByte(pingMap)
			if err := conn.WriteMessage(websocket.TextMessage, pingParams); err != nil {
				util.SocketInfo("okex server ping client error " + err.Error())
			}
		}
		messages := make([]OKEXMessage, 1)
		if err := json.Unmarshal(event, &messages); err == nil {
			for _, message := range messages {
				symbol := model.GetSymbol(model.OKEX, message.Channel)
				if symbol != "" {
					bidAsk := model.BidAsk{}
					bidAsk.Asks = make([][]float64, len(message.Data.Asks))
					bidAsk.Bids = make([][]float64, len(message.Data.Bids))
					for i, v := range message.Data.Bids {
						price, _ := strconv.ParseFloat(v[0], 64)
						amount, _ := strconv.ParseFloat(v[1], 64)
						bidAsk.Bids[i] = []float64{price, amount}
					}
					for i, v := range message.Data.Asks {
						price, _ := strconv.ParseFloat(v[0], 64)
						amount, _ := strconv.ParseFloat(v[1], 64)
						bidAsk.Asks[i] = []float64{price, amount}
					}
					//bidAsk.Bids = message.Data.Bids
					sort.Sort(bidAsk.Asks)
					sort.Reverse(bidAsk.Bids)
					bidAsk.Ts = message.Data.Timestamp
					if markets.SetBidAsk(symbol, model.OKEX, &bidAsk) {
						if carry, err := markets.NewCarry(symbol); err == nil {
							carryHandler(carry)
						}
					}
				}
			}
		}
	}
	return WebSocketServe(model.ApplicationConfig.WSUrls[model.Coinbig],
		model.ApplicationConfig.GetSubscribes(model.Coinbig), subscribeHandlerCoinbig, wsHandler, errHandler)
}

func SignedRequestCoinbig(method, path string, postData *url.Values) []byte {
	hash := md5.New()
	toBeSign, _ := url.QueryUnescape(postData.Encode() + "&secret_key=" + model.ApplicationConfig.CoinbigSecret)
	hash.Write([]byte(toBeSign))
	sign := hex.EncodeToString(hash.Sum(nil))
	postData.Set("sign", strings.ToUpper(sign))
	uri := model.ApplicationConfig.RestUrls[model.Coinbig] + path
	var responseBody []byte
	responseBody, _ = util.HttpRequest(method, uri, postData.Encode(), nil)
	return responseBody
}

func GetAccountCoinbig(accounts *model.Accounts) {
	accounts.ClearAccounts(model.Coinbig)
	postData := &url.Values{}
	postData.Set(`apikey`, model.ApplicationConfig.CoinbigKey)
	responseBody := SignedRequestCoinbig(`POST`, `/userinfo`, postData)
	fmt.Print(string(responseBody))
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
	Maintain(accounts, model.Huobi)
}
