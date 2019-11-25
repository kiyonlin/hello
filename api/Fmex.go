package api

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hello/model"
	"hello/util"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 下单返回1016 资金不足// 下单返回1002 系统繁忙// 返回426 调用次数太频繁 // less than the min limit order amount
//{"status":3033,"msg":"market order is disabled for symbol bsvusdt"}
//{"status":1002,"msg":"system busy"}
var lastDepthPingFmex = util.GetNowUnixMillion()

var subscribeHandlerFmex = func(subscribes []interface{}, subType string) error {
	var err error = nil
	step := 8
	stepSubscribes := make([]interface{}, 0)
	for i := 0; i*step < len(subscribes); i++ {
		subscribeMap := make(map[string]interface{})
		subscribeMap[`cmd`] = `sub`
		if (i+1)*step < len(subscribes) {
			stepSubscribes = subscribes[i*step : (i+1)*step]
		} else {
			stepSubscribes = subscribes[i*step:]
		}
		subscribeMap[`args`] = stepSubscribes
		subscribeMessage := util.JsonEncodeMapToByte(subscribeMap)
		if err := sendToWs(model.Fmex, subscribeMessage); err != nil {
			util.SocketInfo("fmex can not subscribe " + err.Error())
			return err
		}
	}
	return err
}

func WsDepthServeFmex(markets *model.Markets, errHandler ErrHandler) (chan struct{}, error) {
	wsHandler := func(event []byte) {
		responseJson, err := util.NewJSON(event)
		if err != nil {
			errHandler(err)
			return
		}
		if responseJson == nil {
			return
		}
		if util.GetNowUnixMillion()-lastDepthPingFmex > 10000 {
			lastDepthPingFmex = util.GetNowUnixMillion()
			pingMsg := []byte(fmt.Sprintf(`{"cmd":"ping","args":[%d],"id":"id"}`, util.GetNowUnixMillion()))
			if err := sendToWs(model.Fmex, pingMsg); err != nil {
				util.SocketInfo("fmex server ping client error " + err.Error())
			}
		}
		msgType := responseJson.Get(`type`).MustString()
		symbol := model.GetSymbol(model.Fmex, responseJson.Get("type").MustString())
		if strings.Index(msgType, `trade.`) == 0 {
			deal := &model.Deal{Market: model.Fmex, Symbol: strings.Replace(msgType, `trade.`, ``, 1)}
			ts, err := responseJson.Get("ts").Int64()
			if err == nil {
				deal.Ts = ts
			}
			amount, err := responseJson.Get(`amount`).Float64()
			if err == nil {
				deal.Amount = amount
			}
			side, err := responseJson.Get(`side`).String()
			if err == nil {
				deal.Side = side
			}
			price, err := responseJson.Get(`price`).Float64()
			if err == nil {
				deal.Price = price
			}
			markets.SetTrade(deal)
		} else {
			if symbol != "" && symbol != "_" {
				bidAsk := model.BidAsk{}
				bidsLen := len(responseJson.Get("bids").MustArray()) / 2
				bidAsk.Bids = make([]model.Tick, bidsLen)
				for i := 0; i < bidsLen; i++ {
					price, _ := responseJson.Get("bids").GetIndex(i * 2).Float64()
					amount, _ := responseJson.Get("bids").GetIndex(i*2 + 1).Float64()
					bidAsk.Bids[i] = model.Tick{Price: price, Amount: amount}
				}
				asksLen := len(responseJson.Get("asks").MustArray()) / 2
				bidAsk.Asks = make([]model.Tick, asksLen)
				for i := 0; i < asksLen; i++ {
					price, _ := responseJson.Get("asks").GetIndex(i * 2).Float64()
					amount, _ := responseJson.Get("asks").GetIndex(i*2 + 1).Float64()
					bidAsk.Asks[i] = model.Tick{Price: price, Amount: amount}
				}
				sort.Sort(bidAsk.Asks)
				sort.Sort(sort.Reverse(bidAsk.Bids))
				bidAsk.Ts = responseJson.Get("ts").MustInt()
				//startTime := util.GetNowUnixMillion()
				//util.Info(fmt.Sprintf(`%d %s`, startTime-int64(bidAsk.Ts), string(event)))
				if markets.SetBidAsk(symbol, model.Fmex, &bidAsk) {
					for function, handler := range model.GetFunctions(model.Fmex, symbol) {
						if handler != nil && function != model.FunctionMaker {
							go handler(model.Fmex, symbol)
						}
					}
				}
			}
		}
	}
	requestUrl := model.AppConfig.WSUrls[model.Fmex]
	subType := model.SubscribeDepth + `,` + model.SubscribeDeal
	return WebSocketServe(model.Fmex, requestUrl, subType, model.GetWSSubscribes(model.Fmex, subType),
		subscribeHandlerFmex, wsHandler, errHandler)
}

func SignedRequestFmex(key, secret, method, path string, body map[string]interface{}) []byte {
	if key == `` {
		key = model.AppConfig.FcoinKey
	}
	if secret == `` {
		secret = model.AppConfig.FcoinSecret
	}
	uri := model.AppConfig.RestUrls[model.Fmex] + path
	current := util.GetNow()
	currentTime := strconv.FormatInt(current.UnixNano(), 10)[0:13]
	if method == `GET` && len(body) > 0 {
		uri += `?` + util.ComposeParams(body)
	}
	toBeBase := method + uri + currentTime
	if method == `POST` {
		toBeBase += util.ComposeParams(body)
	}
	based := base64.StdEncoding.EncodeToString([]byte(toBeBase))
	hash := hmac.New(sha1.New, []byte(secret))
	hash.Write([]byte(based))
	sign := base64.StdEncoding.EncodeToString(hash.Sum(nil))
	headers := map[string]string{`FC-ACCESS-KEY`: key,
		`FC-ACCESS-SIGNATURE`: sign, `FC-ACCESS-TIMESTAMP`: currentTime, "Content-Type": "application/json"}
	var responseBody []byte
	if body == nil {
		responseBody, _ = util.HttpRequest(method, uri, ``, headers)
	} else {
		responseBody, _ = util.HttpRequest(method, uri, string(util.JsonEncodeMapToByte(body)), headers)
	}
	return responseBody
}

//quantity	订单数量，至少为1
func placeOrderFmex(order *model.Order, key, secret, orderSide, orderType, symbol, price, amount string) {
	postData := make(map[string]interface{})
	if orderType == model.OrderTypeLimit {
		postData["price"] = price
	}
	orderSide = model.GetDictMap(model.Fmex, orderSide)
	if orderSide == `` {
		util.Notice(fmt.Sprintf(`[parameter error] order side: %s`, orderSide))
	}
	orderType = model.GetDictMap(model.Fmex, orderType)
	if orderType == `` {
		util.Notice(fmt.Sprintf(`[parameter error] order type: %s`, orderType))
	}
	postData["symbol"] = symbol
	//postData["symbol"] = strings.ToLower(strings.Replace(symbol, "_", "", 1))
	postData["type"] = orderType
	postData["direction"] = orderSide
	postData["quantity"] = amount
	responseBody := SignedRequestFmex(key, secret, "POST", "v3/contracts/orders", postData)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		data, _ := orderJson.Get(`data`).Map()
		errCode, _ := orderJson.Get("status").Int()
		if errCode == 0 {
			parseOrderFmex(order, data)
			util.Notice(fmt.Sprintf(
				`[挂单fmex] %s side: %s type: %s price: %s amount: %s order id %s errCode:%d 返回%s`,
				symbol, orderSide, orderType, price, amount, order.OrderId, errCode, string(responseBody)))
		} else {
			util.Notice(string(responseBody))
		}
	}
}

func queryOrderFmex(key, secret, orderId string) (order *model.Order) {
	responseBody := SignedRequestFmex(key, secret, `GET`, `v3/contracts/orders/`+orderId, nil)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		data, _ := orderJson.Get(`data`).Map()
		//status, _ := orderJson.Get("status").Int()
		order = &model.Order{}
		parseOrderFmex(order, data)
	}
	return
}

func queryOrdersFmex(key, secret, symbol string) (orders []*model.Order) {
	orders = make([]*model.Order, 0)
	responseBody := SignedRequestFmex(key, secret, `GET`, `v3/contracts/orders/open`, nil)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		jsonOrders := orderJson.GetPath(`data`, `results`)
		if jsonOrders != nil {
			orderArray, _ := jsonOrders.Array()
			for _, orderData := range orderArray {
				orderMap := orderData.(map[string]interface{})
				order := &model.Order{Symbol: symbol}
				parseOrderFmex(order, orderMap)
				orders = append(orders, order)
			}
		}
	}
	return orders
}

func CancelOrderFmex(key, secret, orderId string) (result bool, errCode, msg string, order *model.Order) {
	responseBody := SignedRequestFmex(key, secret, `POST`, `v3/contracts/orders/`+orderId+`/cancel`, nil)
	responseJson, err := util.NewJSON([]byte(responseBody))
	status := -1
	if err == nil {
		status, _ = responseJson.Get(`status`).Int()
		data, _ := responseJson.Get(`data`).Map()
		order = &model.Order{}
		parseOrderFmex(order, data)
	}
	util.Notice(orderId + "fmex cancel order" + string(responseBody))
	if status == 0 {
		return true, ``, ``, order
	}
	return false, strconv.FormatInt(int64(status), 10), ``, nil
}

func parseOrderFmex(order *model.Order, orderMap map[string]interface{}) {
	if orderMap == nil || orderMap[`id`] == nil || order == nil {
		return
	}
	order.Market = model.Fmex
	createTime := int64(0)
	if orderMap[`created_at`] != nil {
		createTime, _ = orderMap[`created_at`].(json.Number).Int64()
		order.OrderTime = time.Unix(0, createTime*1000000)
	}
	updateTime := int64(0)
	if orderMap[`updated_at`] != nil {
		updateTime, _ = orderMap[`updated_at`].(json.Number).Int64()
		order.OrderUpdateTime = time.Unix(0, updateTime*1000000)
	}
	if orderMap[`price`] != nil {
		order.Price, _ = orderMap[`price`].(json.Number).Float64()
	}
	if orderMap[`fee`] != nil {
		order.Fee, _ = orderMap[`fee`].(json.Number).Float64()
	}
	if orderMap[`direction`] != nil {
		order.OrderSide = model.GetDictMapRevert(model.Fmex, orderMap[`direction`].(string))
	}
	if orderMap[`trigger_direction`] != nil {
		order.TriggerDirection = model.GetDictMapRevert(model.Fmex, orderMap[`trigger_direction`].(string))
	}
	if orderMap[`features`] != nil {
		order.Features, _ = orderMap[`features`].(json.Number).Int64()
	}
	if orderMap[`quantity`] != nil {
		order.Amount, _ = orderMap[`quantity`].(json.Number).Float64()
	}
	if orderMap[`unfilled_quantity`] != nil {
		order.UnfilledQuantity, _ = orderMap[`unfilled_quantity`].(json.Number).Float64()
	}
	if orderMap[`maker_fee_rate`] != nil {
		order.MakerFeeRate, _ = orderMap[`maker_fee_rate`].(json.Number).Float64()
	}
	if orderMap[`taker_fee_rate`] != nil {
		order.TakerFeeRate, _ = orderMap[`taker_fee_rate`].(json.Number).Float64()
	}
	if orderMap[`trigger_on`] != nil {
		order.TriggerOn, _ = orderMap[`trigger_on`].(json.Number).Float64()
	}
	if orderMap[`trailing_base_price`] != nil {
		order.TrailingBasePrice, _ = orderMap[`trailing_base_price`].(json.Number).Float64()
	}
	if orderMap[`trailing_distance`] != nil {
		order.TrailingDistance, _ = orderMap[`trailing_distance`].(json.Number).Float64()
	}
	if orderMap[`frozen_margin`] != nil {
		order.FrozenMargin, _ = orderMap[`frozen_margin`].(json.Number).Float64()
	}
	if orderMap[`frozen_quantity`] != nil {
		order.FrozenQuantity, _ = orderMap[`frozen_quantity`].(json.Number).Float64()
	}
	if orderMap[`hidden`] != nil {
		order.Hidden = orderMap[`hidden`].(bool)
	}
	if orderMap[`type`] != nil {
		order.OrderType = model.GetDictMapRevert(model.Fmex, orderMap[`type`].(string))
	}
	if orderMap[`status`] != nil {
		order.Status = model.GetOrderStatus(model.Fmex, orderMap[`status`].(string))
	}
	order.OrderId = orderMap[`id`].(json.Number).String()
	order.DealAmount = order.Amount - order.UnfilledQuantity
}

func getAccountFmex(key, secret string) (account []*model.Account) {
	responseBody := SignedRequestFmex(key, secret, `GET`, `v3/contracts/positions`, nil)
	balanceJson, err := util.NewJSON(responseBody)
	if err == nil {
		status, _ := balanceJson.Get("status").Int()
		if status == 0 {
			data, _ := balanceJson.Get("data").Map()
			positions, _ := data[`results`].([]interface{})
			accounts := make([]*model.Account, 0)
			for _, value := range positions {
				account := value.(map[string]interface{})
				updateTime, _ := account[`updated_at`].(json.Number).Int64()
				free, _ := account["quantity"].(json.Number).Float64()
				profitReal, _ := account[`realized_pnl`].(json.Number).Float64()
				margin, _ := account[`margin`].(json.Number).Float64()
				bankruptcyPrice, _ := account[`bankruptcy_price`].(json.Number).Float64()
				liquidationPrice, _ := account[`liquidation_price`].(json.Number).Float64()
				entryPrice, _ := account[`entry_price`].(json.Number).Float64()
				minimumMaintenanceMarginRate, _ := account[`minimum_maintenance_margin_rate`].(json.Number).Float64()
				symbol := strings.ToLower(account["symbol"].(string))
				closed := account[`closed`].(bool)
				direction := model.GetDictMapRevert(model.Fmex, account[`direction`].(string))
				if direction == model.OrderSideSell {
					free = -1 * free
				}
				accounts = append(accounts,
					&model.Account{Market: model.Fmex,
						Currency:                     symbol,
						Direction:                    direction,
						Free:                         free,
						ProfitReal:                   profitReal,
						Margin:                       margin,
						AccountUpdateTime:            time.Unix(0, updateTime*1000000),
						BankruptcyPrice:              bankruptcyPrice,
						LiquidationPrice:             liquidationPrice,
						EntryPrice:                   entryPrice,
						MinimumMaintenanceMarginRate: minimumMaintenanceMarginRate,
						Closed:                       closed})
			}
			return accounts
		}
	}
	return nil
}
func getFundingRateFmex(symbol string) (fundingRate float64, updateTime int64) {
	if symbol == `btcusd_p` {
		symbol = `.btcusdfr8h`
	}
	responseBody := SignedRequestFmex(``, ``, `GET`, `v2/market/indexes`, nil)
	indexJson, err := util.NewJSON(responseBody)
	if err == nil && indexJson != nil {
		array, err := indexJson.GetPath(`data`, symbol).Array()
		if err == nil && len(array) > 1 {
			updateTime, err = array[0].(json.Number).Int64()
			if err == nil {
				updateTime = updateTime / 1000
			}
			fundingRate, _ = array[1].(json.Number).Float64()
		}
	}
	return
}
