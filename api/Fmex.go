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
		//util.Info(string(event))
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
			ts := responseJson.Get("ts").MustInt()
			amount := responseJson.Get(`amount`).MustFloat64()
			side := responseJson.Get(`side`).MustString()
			price := responseJson.Get(`price`).MustFloat64()
			markets.SetTrade(&model.Deal{Amount: amount, Ts: ts, Side: side, Price: price}, nil)
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
		key = model.AppConfig.FmexKey
	}
	if secret == `` {
		secret = model.AppConfig.FmexSecret
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
func placeOrderFmex(key, secret, orderSide, orderType, symbol, price, amount string) (orderId, errCode string) {
	postData := make(map[string]interface{})
	if orderType == model.OrderTypeLimit {
		postData["price"] = price
	}
	orderSide = model.GetDictMap(model.Fmex, orderSide)
	if orderSide == `` {
		util.Notice(fmt.Sprintf(`[parameter error] order side: %s`, orderSide))
		return ``, ``
	}
	orderType = model.GetDictMap(model.Fmex, orderType)
	if orderType == `` {
		util.Notice(fmt.Sprintf(`[parameter error] order type: %s`, orderType))
		return ``, ``
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
		status, _ := orderJson.Get("status").Int()
		order := parseOrderFmex(symbol, data)
		if order != nil && status == 0 {
			orderId = order.OrderId
			util.Notice(fmt.Sprintf(
				`[挂单fmex] %s side: %s type: %s price: %s amount: %s order id %s errCode:%s 返回%s`,
				symbol, orderSide, orderType, price, amount, orderId, errCode, string(responseBody)))
			return orderId, strconv.Itoa(status)
		} else {
			util.Notice(string(responseBody))
			return ``, ``
		}
	}
	return ``, err.Error()
}

func queryOrdersFmex(key, secret, symbol string) (orders []*model.Order) {
	orders = make([]*model.Order, 0)
	responseBody := SignedRequestFmex(key, secret, `GET`, `v3/contracts/orders/open`, nil)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		jsonOrders := orderJson.GetPath(`data`, `results`)
		if jsonOrders != nil {
			orderArray, _ := jsonOrders.Array()
			for _, order := range orderArray {
				orderMap := order.(map[string]interface{})
				order := parseOrderFmex(symbol, orderMap)
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
		order = parseOrderFmex(``, data)
	}
	util.Notice(orderId + "fmex cancel order" + string(responseBody))
	if status == 0 {
		return true, ``, ``, order
	}
	return false, strconv.FormatInt(int64(status), 10), ``, nil
}

func parseOrderFmex(symbol string, orderMap map[string]interface{}) (order *model.Order) {
	if orderMap == nil || orderMap[`id`] == nil {
		return nil
	}
	createTime := int64(0)
	if orderMap[`created_at`] != nil {
		createTime, _ = orderMap[`created_at`].(json.Number).Int64()
	}
	updateTime := int64(0)
	if orderMap[`updated_at`] != nil {
		updateTime, _ = orderMap[`updated_at`].(json.Number).Int64()
	}
	price := 0.0
	if orderMap[`price`] != nil {
		price, _ = orderMap[`price`].(json.Number).Float64()
	}
	fee := 0.0
	if orderMap[`fee`] != nil {
		fee, _ = orderMap[`fee`].(json.Number).Float64()
	}
	orderSide := ``
	if orderMap[`direction`] != nil {
		orderSide = model.GetDictMapRevert(model.Fmex, orderMap[`direction`].(string))
	}
	triggerDirection := ``
	if orderMap[`trigger_direction`] != nil {
		triggerDirection = model.GetDictMapRevert(model.Fmex, orderMap[`trigger_direction`].(string))
	}
	features := int64(0)
	if orderMap[`features`] != nil {
		features, _ = orderMap[`features`].(json.Number).Int64()
	}
	amount := 0.0
	if orderMap[`quantity`] != nil {
		amount, _ = orderMap[`quantity`].(json.Number).Float64()
	}
	unfilledQuantity := 0.0
	if orderMap[`unfilled_quantity`] != nil {
		unfilledQuantity, _ = orderMap[`unfilled_quantity`].(json.Number).Float64()
	}
	makerFeeRate := 0.0
	if orderMap[`maker_fee_rate`] != nil {
		makerFeeRate, _ = orderMap[`maker_fee_rate`].(json.Number).Float64()
	}
	takerFeeRate := 0.0
	if orderMap[`taker_fee_rate`] != nil {
		takerFeeRate, _ = orderMap[`taker_fee_rate`].(json.Number).Float64()
	}
	triggerOn := 0.0
	if orderMap[`trigger_on`] != nil {
		triggerOn, _ = orderMap[`trigger_on`].(json.Number).Float64()
	}
	trailingBasePrice := 0.0
	if orderMap[`trailing_base_price`] != nil {
		trailingBasePrice, _ = orderMap[`trailing_base_price`].(json.Number).Float64()
	}
	trailingDistance := 0.0
	if orderMap[`trailing_distance`] != nil {
		trailingDistance, _ = orderMap[`trailing_distance`].(json.Number).Float64()
	}
	frozenMargin := 0.0
	if orderMap[`frozen_margin`] != nil {
		frozenMargin, _ = orderMap[`frozen_margin`].(json.Number).Float64()
	}
	frozenQuantity := 0.0
	if orderMap[`frozen_quantity`] != nil {
		frozenQuantity, _ = orderMap[`frozen_quantity`].(json.Number).Float64()
	}
	hidden := false
	if orderMap[`hidden`] != nil {
		hidden = orderMap[`hidden`].(bool)
	}
	orderType := ``
	if orderMap[`type`] != nil {
		orderType = orderMap[`type`].(string)
	}
	status := ``
	if orderMap[`status`] != nil {
		status = orderMap[`status`].(string)
	}
	return &model.Order{
		OrderId:           orderMap[`id`].(json.Number).String(),
		Symbol:            symbol,
		Market:            model.Fmex,
		Amount:            amount,
		DealAmount:        amount - unfilledQuantity,
		OrderTime:         time.Unix(0, createTime*1000000),
		OrderUpdateTime:   time.Unix(0, updateTime*1000000),
		OrderType:         model.GetDictMapRevert(model.Fmex, orderType),
		OrderSide:         orderSide,
		Price:             price,
		Fee:               fee,
		Status:            model.GetOrderStatus(model.Fmex, status),
		TriggerDirection:  triggerDirection,
		Features:          features,
		Hidden:            hidden,
		UnfilledQuantity:  unfilledQuantity,
		MakerFeeRate:      makerFeeRate,
		TakerFeeRate:      takerFeeRate,
		TriggerOn:         triggerOn,
		TrailingBasePrice: trailingBasePrice,
		TrailingDistance:  trailingDistance,
		FrozenMargin:      frozenMargin,
		FrozenQuantity:    frozenQuantity,
	}
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
				accounts = append(accounts,
					&model.Account{Market: model.Fmex,
						Currency:                     symbol,
						Direction:                    model.GetDictMapRevert(model.Fmex, account[`direction`].(string)),
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
