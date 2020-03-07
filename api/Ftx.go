package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/bitly/go-simplejson"
	"hello/model"
	"hello/util"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var lastDepthPingFtx = util.GetNowUnixMillion()
var socketLockFtx sync.Mutex

var subscribeHandlerFtx = func(subscribes []interface{}, subType string) error {
	var err error = nil
	ts := time.Now().UnixNano() / int64(time.Millisecond)
	toBeSign := fmt.Sprintf(`%dwebsocket_login`, ts)
	hash := hmac.New(sha256.New, []byte(model.AppConfig.FtxSecret))
	hash.Write([]byte(toBeSign))
	sign := hex.EncodeToString(hash.Sum(nil))
	authCmd := fmt.Sprintf(`{"op":"login","args":{"key":"%s","sign":"%s","time":%d}}`,
		model.AppConfig.FtxKey, sign, ts)
	if err = sendToWs(model.Ftx, []byte(authCmd)); err != nil {
		util.SocketInfo("ftx can not auth " + err.Error())
	}
	if err = sendToWs(model.Ftx, []byte(`{"op": "subscribe", "channel": "fills"}`)); err != nil {
		util.SocketInfo("ftx can not subscribe fill " + err.Error())
	}
	for i := 0; i < len(subscribes); i++ {
		cmdSubscribe := subscribes[i].([]string)
		subCmd := fmt.Sprintf(`{"op": "subscribe", "channel": "%s", "market": "%s"}`,
			cmdSubscribe[0], cmdSubscribe[1])
		if err = sendToWs(model.Ftx, []byte(subCmd)); err != nil {
			util.SocketInfo("ftx can not subscribe " + err.Error())
			return err
		}
	}
	return err
}

func WsDepthServeFtx(markets *model.Markets, errHandler ErrHandler) (chan struct{}, error) {
	wsHandler := func(event []byte) {
		defer socketLockFtx.Unlock()
		socketLockFtx.Lock()
		responseJson, err := util.NewJSON(event)
		if err != nil {
			errHandler(err)
			return
		}
		if responseJson == nil {
			return
		}
		if util.GetNowUnixMillion()-lastDepthPingFtx > 15000 {
			lastDepthPingFtx = util.GetNowUnixMillion()
			pingMsg := []byte(`{"op":"ping"}`)
			if err := sendToWs(model.Ftx, pingMsg); err != nil {
				util.SocketInfo("ftx server ping client error " + err.Error())
			}
		}
		msgType := responseJson.Get(`channel`).MustString()
		if msgType == `orderbook` {
			handleDepthFtx(markets, responseJson)
		} else {
			fmt.Println(string(event))
		}
	}
	requestUrl := model.AppConfig.WSUrls[model.Ftx]
	//subType := model.SubscribeDepth + `,` + model.SubscribeDeal
	return WebSocketServe(model.Ftx, requestUrl, ``, model.GetWSSubscribes(model.Ftx, ``),
		subscribeHandlerFtx, wsHandler, errHandler)
}

func handleDepthFtx(markets *model.Markets, response *simplejson.Json) {
	if response == nil {
		return
	}
	symbol := model.GetSymbol(model.Ftx, response.Get("market").MustString())
	dataType := response.Get(`type`).MustString()
	data := response.Get(`data`)
	if data != nil {
		bidAsk := &model.BidAsk{}
		ts := data.Get(`time`).MustFloat64()
		bidAsk.Ts = int(ts * 1000)
		bids := data.Get(`bids`).MustArray()
		asks := data.Get(`asks`).MustArray()
		bidAsk.Bids = make([]model.Tick, 0)
		bidAsk.Asks = make([]model.Tick, 0)
		if dataType == `partial` {
			for _, item := range bids {
				price, _ := item.([]interface{})[0].(json.Number).Float64()
				size, _ := item.([]interface{})[1].(json.Number).Float64()
				bidAsk.Bids = append(bidAsk.Bids, model.Tick{Price: price, Amount: price * size})
			}
			for _, item := range asks {
				price, _ := item.([]interface{})[0].(json.Number).Float64()
				size, _ := item.([]interface{})[1].(json.Number).Float64()
				bidAsk.Asks = append(bidAsk.Bids, model.Tick{Price: price, Amount: price * size})
			}
		} else if dataType == `update` {
			priceAmountBid := make(map[float64]*model.Tick)
			priceAmountAsk := make(map[float64]*model.Tick)
			for _, item := range bids {
				price, _ := item.([]interface{})[0].(json.Number).Float64()
				size, _ := item.([]interface{})[1].(json.Number).Float64()
				priceAmountBid[price] = &model.Tick{Price: price, Amount: price * size}
			}
			for _, item := range asks {
				price, _ := item.([]interface{})[0].(json.Number).Float64()
				size, _ := item.([]interface{})[1].(json.Number).Float64()
				priceAmountAsk[price] = &model.Tick{Price: price, Amount: price * size}
			}
			_, oldBidAsk := markets.GetBidAsk(symbol, model.Ftx)
			for _, bid := range oldBidAsk.Bids {
				if priceAmountBid[bid.Price] == nil {
					bidAsk.Bids = append(bidAsk.Bids, bid)
				} else if priceAmountBid[bid.Price].Amount > 0 {
					bidAsk.Bids = append(bidAsk.Bids, *priceAmountBid[bid.Price])
				}
				delete(priceAmountBid, bid.Price)
			}
			for _, bid := range priceAmountBid {
				if bid.Amount > 0 {
					bidAsk.Bids = append(bidAsk.Bids, *bid)
				}
			}
			for _, ask := range oldBidAsk.Asks {
				if priceAmountAsk[ask.Price] == nil {
					bidAsk.Asks = append(bidAsk.Asks, ask)
				} else if priceAmountAsk[ask.Price].Amount > 0 {
					bidAsk.Asks = append(bidAsk.Asks, *priceAmountAsk[ask.Price])
				}
				delete(priceAmountAsk, ask.Price)
			}
			for _, ask := range priceAmountAsk {
				if ask.Amount > 0 {
					bidAsk.Asks = append(bidAsk.Asks, *ask)
				}
			}
		}
		sort.Sort(bidAsk.Asks)
		sort.Sort(sort.Reverse(bidAsk.Bids))
		//util.SocketInfo(markets.ToStringBidAsk(bidAsk))
		if markets.SetBidAsk(symbol, model.Ftx, bidAsk) {
			for function, handler := range model.GetFunctions(model.Ftx, symbol) {
				if handler != nil {
					util.Notice(`handling by ftx ` + function)
					//handler(model.Ftx, symbol, function)
				}
			}
		}
	}
}

//
//func cancelOrderBybit(key, secret, symbol, orderId string) (result bool, errCode, msg string, order *model.Order) {
//	postData := make(map[string]interface{})
//	postData[`order_id`] = orderId
//	postData[`symbol`] = model.GetDialectSymbol(model.Bybit, symbol)
//	response := SignedRequestBybit(key, secret, `POST`, `/v2/private/order/cancel`, postData)
//	orderJson, err := util.NewJSON(response)
//	result = false
//	if err == nil {
//		retCode := orderJson.Get(`ret_code`).MustInt64()
//		if retCode == 0 {
//			result = true
//		}
//		errCode = strconv.FormatInt(retCode, 10)
//		msg = orderJson.Get(`ret_msg`).MustString()
//		if orderJson.Get(`result`) != nil {
//			item, _ := orderJson.Get(`result`).Map()
//			if item != nil {
//				order = &model.Order{}
//				parseOrderBybit(order, item)
//			}
//		}
//		return
//	}
//	return false, ``, ``, nil
//}
//
//func queryOrderBybit(key, secret, symbol, orderId string) (orders []*model.Order) {
//	orders = make([]*model.Order, 0)
//	postData := make(map[string]interface{})
//	symbol = model.GetDialectSymbol(model.Bybit, symbol)
//	postData[`symbol`] = model.GetDialectSymbol(model.Bybit, symbol)
//	postData[`order_id`] = orderId
//	response := SignedRequestBybit(key, secret, `GET`, `/open-api/order/list`, postData)
//	util.Notice(`query orders: ` + string(response))
//	orderJson, err := util.NewJSON(response)
//	if err == nil {
//		orderJson = orderJson.GetPath(`result`, `data`)
//		if orderJson == nil {
//			return
//		}
//		orderArray, _ := orderJson.Array()
//		for _, data := range orderArray {
//			order := &model.Order{Market: model.Bybit}
//			parseOrderBybit(order, data.(map[string]interface{}))
//			if order.OrderId != `` {
//				orders = append(orders, order)
//			}
//		}
//	}
//	return
//}

func getAccountFtx(key, secret string, accounts *model.Accounts) {
	postData := make(map[string]interface{})
	response := SignedRequestFtx(key, secret, `GET`, `/positions`, nil, postData)
	fmt.Println(string(response))
	util.Notice(fmt.Sprintf(string(response)))
	accounts.SetAccount(model.Ftx, ``, nil)
	//positionJson, err := util.NewJSON(response)
	//if err == nil {
	//	positionJson = positionJson.Get(`result`)
	//	if positionJson != nil {
	//		account := &model.Account{Market: model.Bybit, Ts: util.GetNowUnixMillion(), Currency: symbol}
	//		item, _ := positionJson.Map()
	//		parseAccountBybit(account, item)
	//		accounts.SetAccount(model.Bybit, account.Currency, account)
	//	}
	//}
}

func getFundingRateFtx(symbol string) (fundingRate float64, expire int64) {
	postData := make(map[string]interface{})
	symbol = model.GetDialectSymbol(model.Ftx, symbol)
	postData[`future`] = symbol
	response := SignedRequestFtx(``, ``, `GET`,
		`/funding_payments`, nil, postData)
	fmt.Println(string(response))
	instrumentJson, err := util.NewJSON(response)
	if err == nil {
		retCode := instrumentJson.Get(`ret_code`).MustFloat64()
		if retCode != 0 {
			return 0, 0
		}
		instrumentJson = instrumentJson.Get(`result`)
		if instrumentJson != nil {
			instrument, _ := instrumentJson.Map()
			if instrument == nil {
				return 0, 0
			}
			if instrument[`symbol`] != nil && instrument[`symbol`] == symbol &&
				instrument[`funding_rate`] != nil && instrument[`funding_rate_timestamp`] != nil {
				fundingRate, _ = strconv.ParseFloat(instrument[`funding_rate`].(string), 64)
				expire, _ = instrument[`funding_rate_timestamp`].(json.Number).Int64()
				expire += 28800
			}
		}
	}
	return
}

//remainingSize	number	31431.0
//reduceOnly	boolean	false
//ioc	boolean	false
//postOnly	boolean	false
//orderPrice	number	null	price of the order sent when this stop loss triggered
//retryUntilFilled	boolean	false	Whether or not to keep re-triggering until filled
//orderType	string	market	Values are market and limit
func parseOrderFtx(order *model.Order, item map[string]interface{}) {
	if order == nil || item == nil {
		return
	}
	if item[`createdAt`] != nil {
		order.OrderTime, _ = time.Parse(time.RFC3339, item[`createdAt`].(string))
	}
	if item[`filledSize`] != nil {
		order.DealAmount, _ = item[`filledSize`].(json.Number).Float64()
	}
	if item[`id`] != nil {
		order.OrderId = item[`id`].(json.Number).String()
	}
	if item[`market`] != nil {
		order.Symbol = model.GetStandardSymbol(model.Ftx, item[`market`].(string))
	}
	if item[`price`] != nil {
		order.Price, _ = item[`price`].(json.Number).Float64()
	}
	if item[`side`] != nil {
		order.OrderSide = strings.ToLower(item[`side`].(string))
	}
	if item[`size`] != nil {
		order.Amount, _ = item[`size`].(json.Number).Float64()
	}
	if item[`type`] != nil {
		order.OrderType = strings.ToLower(item[`type`].(string))
	}
	if item[`status`] != nil {
		order.Status = model.GetOrderStatus(model.Ftx, item[`status`].(string))
	}
	if item[`triggerPrice`] != nil {
		order.Price, _ = item[`triggerPrice`].(json.Number).Float64()
	}
	if item[`triggeredAt`] != nil {
		order.OrderUpdateTime, _ = time.Parse(time.RFC3339, item[`triggeredAt`].(string))
	}
	if order.Status != model.CarryStatusSuccess && order.Status != model.CarryStatusFail {
		order.Status = model.CarryStatusWorking
	}
	if order.DealAmount == 0 || order.DealPrice == 0 {
		order.DealPrice = order.Price
	}
	order.Amount = order.Amount * order.Price
	order.DealAmount = order.DealAmount * order.Price
	order.UnfilledQuantity = order.Amount - order.DealAmount
	return
}

//orderType: "limit", "market", "stop", "trailingStop", "takeProfit"
func placeOrderFtx(order *model.Order, key, secret, orderSide, orderType, orderParam, symbol, price,
	amount string) {
	uri := `/orders`
	param := make(map[string]interface{})
	symbol = model.GetDialectSymbol(model.Ftx, symbol)
	param[`market`] = symbol
	postData := make(map[string]interface{})
	postData[`market`] = symbol
	postData[`side`] = orderSide
	postData[`size`], _ = strconv.ParseFloat(amount, 64)
	postData[`type`] = orderType
	if orderType == `limit` || orderType == `market` {
		postData[`price`], _ = strconv.ParseFloat(price, 64)
		if orderParam == model.PostOnly {
			postData[`postOnly`] = true
		}
	} else if orderType == `stop` || orderType == `trailingStop` || orderType == `takeProfit` {
		uri = `/conditional_orders`
		postData[`triggerPrice`], _ = strconv.ParseFloat(price, 64)
	}
	response := SignedRequestFtx(key, secret, `POST`, uri, param, postData)
	util.Notice(`place ftx: ` + string(response))
	orderJson, err := util.NewJSON(response)
	if err == nil {
		success := orderJson.Get(`success`).MustBool()
		if success {
			data, _ := orderJson.Get(`result`).Map()
			parseOrderFtx(order, data)
		} else {
			order.Status = model.CarryStatusFail
			order.OrderId = ``
		}
	}
	return
}

func SignedRequestFtx(key, secret, method, path string, param, body map[string]interface{}) []byte {
	if key == `` {
		key = model.AppConfig.FtxKey
	}
	if secret == `` {
		secret = model.AppConfig.FtxSecret
	}
	if body == nil {
		body = make(map[string]interface{})
	}
	u, _ := url.ParseRequestURI(model.AppConfig.RestUrls[model.Ftx])
	u.Path += path
	ts := time.Now().UnixNano() / int64(time.Millisecond)
	hash := hmac.New(sha256.New, []byte(secret))
	bodyStr := string(util.JsonEncodeMapToByte(body))
	q := u.Query()
	for k, v := range param {
		q.Set(k, v.(string))
	}
	u.RawQuery = q.Encode()
	uri := u.Path
	if u.Query().Encode() != `` {
		uri = fmt.Sprintf(`%s?%s`, u.Path, u.Query().Encode())
	}
	if method == `POST` {
		hash.Write([]byte(fmt.Sprintf(`%d%s%s%s`, ts, method, uri, bodyStr)))
	} else {
		hash.Write([]byte(fmt.Sprintf(`%d%s%s`, ts, method, uri)))
	}
	sign := hex.EncodeToString(hash.Sum(nil))
	headers := map[string]string{`FTX-KEY`: key, `FTX-TS`: strconv.FormatInt(ts, 10), "FTX-SIGN": sign,
		"Content-Type": "application/json"}
	responseBody, _ := util.HttpRequest(method, u.String(), bodyStr, headers, 5)
	return responseBody
}
