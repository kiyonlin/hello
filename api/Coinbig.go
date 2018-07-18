package api

import (
	"bytes"
	"compress/zlib"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"hello/model"
	"hello/util"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

var subscribeHandlerCoinbig = func(subscribes []string, conn *websocket.Conn) error {
	var err error = nil
	for _, v := range subscribes {
		subscribeMessage := fmt.Sprintf(`{"datatype":"ALL","data":"%s"}`, v)
		if err = conn.WriteMessage(websocket.TextMessage, []byte(subscribeMessage)); err != nil {
			util.SocketInfo("coinbig can not subscribe " + subscribeMessage + err.Error())
			return err
		}
		//util.SocketInfo(`coinbig subscribed ` + v)
	}
	return err
}

func WsDepthServeCoinbig(markets *model.Markets, carryHandlers []CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
	//lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte, conn *websocket.Conn) {
		var out bytes.Buffer
		r, _ := zlib.NewReader(bytes.NewReader(event))
		io.Copy(&out, r)
		dataJson, err := util.NewJSON(out.Bytes())
		if err == nil {
			subscribe, _ := dataJson.GetPath(`data`, `tradeMappingId`).String()
			symbol := model.GetSymbol(model.Coinbig, subscribe)
			if symbol != `` {
				bidAsk := model.BidAsk{}
				bids, _ := dataJson.Get(`data`).Get(`bids`).Array()
				asks, _ := dataJson.Get(`data`).Get(`asks`).Array()
				bidAsk.Bids = make([][]float64, len(bids))
				bidAsk.Asks = make([][]float64, len(asks))
				for key, value := range bids {
					bidAsk.Bids[key] = make([]float64, 2)
					bidAsk.Bids[key][0], _ = strconv.ParseFloat(value.(map[string]interface{})[`price`].(string), 64)
					bidAsk.Bids[key][1], _ = strconv.ParseFloat(value.(map[string]interface{})[`quantity`].(string), 64)
				}
				for key, value := range asks {
					bidAsk.Asks[key] = make([]float64, 2)
					bidAsk.Asks[key][0], _ = strconv.ParseFloat(value.(map[string]interface{})[`price`].(string), 64)
					bidAsk.Asks[key][1], _ = strconv.ParseFloat(value.(map[string]interface{})[`quantity`].(string), 64)
				}
				sort.Sort(bidAsk.Asks)
				sort.Reverse(bidAsk.Bids)
				realTimeQueue, _ := dataJson.Get(`data`).Get(`RealTimeQueue`).Array()
				ts, _ := realTimeQueue[0].(map[string]interface{})[`createTime`].(json.Number).Int64()
				bidAsk.Ts = int(ts)
				if markets.SetBidAsk(symbol, model.Coinbig, &bidAsk) {
					for _, handler := range carryHandlers {
						handler(symbol, model.Coinbig)
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
	if postData != nil {
		time := strconv.FormatInt(util.GetNow().UnixNano(), 10)[0:13]
		postData.Set(`time`, time)
	}
	toBeSign, _ := url.QueryUnescape(postData.Encode() + "&secret_key=" + model.ApplicationConfig.CoinbigSecret)
	hash.Write([]byte(toBeSign))
	sign := hex.EncodeToString(hash.Sum(nil))
	postData.Set("sign", strings.ToUpper(sign))
	uri := model.ApplicationConfig.RestUrls[model.Coinbig] + path
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded",
		"User-Agent": "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	var responseBody []byte
	responseBody, _ = util.HttpRequest(method, uri, postData.Encode(), headers)
	return responseBody
}

func GetAccountCoinbig(accounts *model.Accounts) {
	accounts.ClearAccounts(model.Coinbig)
	postData := &url.Values{}
	postData.Set(`apikey`, model.ApplicationConfig.CoinbigKey)
	responseBody := SignedRequestCoinbig(`POST`, `/userinfo`, postData)
	balanceJson, err := util.NewJSON(responseBody)
	if err == nil {
		code, _ := balanceJson.Get(`code`).Int()
		if code == 0 {
			freeMap, _ := balanceJson.GetPath(`data`, `info`, `free`).Map()
			frozenMap, _ := balanceJson.GetPath(`data`, `info`, `freezed`).Map()
			for key, value := range freeMap {
				balance, _ := value.(json.Number).Float64()
				if balance == 0 {
					continue
				}
				key = strings.ToLower(key)
				account := accounts.GetAccount(model.Coinbig, key)
				if account == nil {
					account = &model.Account{Market: model.Coinbig, Currency: key}
					accounts.SetAccount(model.Coinbig, key, account)
				}
				account.Free = balance
			}
			for key, value := range frozenMap {
				balance, _ := value.(json.Number).Float64()
				if balance == 0 {
					continue
				}
				key = strings.ToLower(key)
				account := accounts.GetAccount(model.Coinbig, key)
				if account == nil {
					account = &model.Account{Market: model.Coinbig, Currency: key}
					accounts.SetAccount(model.Coinbig, key, account)
				}
				account.Frozen = balance
			}
		}
	}
	Maintain(accounts, model.Coinbig)
}

// orderType 买卖类型: 限价单(buy/sell) 市价单(buy_market/sell_market)
func placeOrderCoinbig(orderSide, orderType, symbol, price, amount string) (orderId, errCode string) {
	orderParam := ``
	if orderSide == model.OrderSideBuy && orderType == model.OrderTypeLimit {
		orderParam = `buy`
	} else if orderSide == model.OrderSideSell && orderType == model.OrderTypeLimit {
		orderParam = `sell`
	} else {
		util.Notice(fmt.Sprintf(`[parameter error] order side: %s order type: %s`, orderSide, orderType))
		return ``, ``
	}
	postData := &url.Values{}
	postData.Set(`apikey`, model.ApplicationConfig.CoinbigKey)
	postData.Set(`type`, orderParam)
	postData.Set(`price`, price)
	postData.Set(`amount`, amount)
	postData.Set(`symbol`, symbol)
	responseBody := SignedRequestCoinbig(`POST`, `/trade`, postData)
	util.Notice("coinbig place order" + string(responseBody))
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		orderId, _ := orderJson.GetPath("data", `order_id`).Int64()
		status, _ := orderJson.Get("code").Int()
		return strconv.FormatInt(orderId, 10), strconv.Itoa(status)
	}
	return ``, err.Error()
}

//// 批量下单接口，只支持限价单, side sell/buy
//func PlaceOrdersCoinbig(symbol string, side []string, price, amount []float64) {
//	postData := &url.Values{}
//	postData.Set(`apikey`, model.ApplicationConfig.CoinbigKey)
//	postData.Set(`symbol`, symbol)
//	data := `[`
//	for i := 0; i < len(side); i++ {
//		data += fmt.Sprintf(`{price:%f,amount:%f,type:"%s"}`, price[i], amount[i], side[i])
//		if i < len(side)-1 {
//			data += `,`
//		}
//	}
//	data += `]`
//	postData.Set(`ordersdata`, data)
//	responseBody := SignedRequestCoinbig(`POST`, `/batch_trade`, postData)
//	util.Notice(`[place order]` + string(responseBody))
//}

//状态:1未成交,2部分成交,3完全成交,4用户撤销,5部分撤回,6成交失败
func QueryOrderCoinbig(orderId string) (dealAmount float64, status string) {
	postData := &url.Values{}
	postData.Set(`apikey`, model.ApplicationConfig.CoinbigKey)
	postData.Set(`orderId`, orderId)
	responseBody := SignedRequestCoinbig(`POST`, `/getOrderInfoById`, postData)
	orderJson, err := util.NewJSON([]byte(responseBody))
	if err == nil {
		dealAmount, _ = orderJson.GetPath(`data`, `successAmount`).Float64()
		intStatus, _ := orderJson.GetPath(`data`, "status").Int()
		status = model.OrderStatusMap[fmt.Sprintf(`%s%d`, model.Coinbig, intStatus)]
	}
	util.Notice(fmt.Sprintf("%s coinbig query order %f %s", status, dealAmount, responseBody))
	return dealAmount, status
}

func CancelOrderCoinbig(orderId string)  (result bool, errCode, msg string){
	postData := &url.Values{}
	postData.Set(`apikey`, model.ApplicationConfig.CoinbigKey)
	postData.Set(`order_id`, orderId)
	responseBody := SignedRequestCoinbig(`POST`, `/cancel_order`, postData)
	orderJson, err := util.NewJSON([]byte(responseBody))
	status := -1
	if err == nil {
		status, _ = orderJson.Get(`code`).Int()
	}
	if status == 0 {
		result = true
	}
	util.Notice("coinbig cancel order" + string(responseBody))
	return result, ``, ``
}

//func CancelOrdersCoinbig(symbol string) {
//	postData := &url.Values{}
//	postData.Set(`apikey`, model.ApplicationConfig.CoinbigKey)
//	postData.Set(`symbol`, symbol)
//	responseBody := SignedRequestCoinbig(`POST`, `/cance_all_orders`, postData)
//	fmt.Println(string(responseBody))
//}
