package api

import (
	"net/url"
	"crypto/md5"
	"hello/model"
	"encoding/hex"
	"strings"
	"hello/util"
	"fmt"
	"strconv"
	"encoding/json"
	"github.com/gorilla/websocket"
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

func WsDepthServeCoinbig(markets *model.Markets, carryHandler CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
	//lastPingTime := util.GetNow().Unix()
	wsHandler := func(event []byte, conn *websocket.Conn) {

		//buf := bytes.NewBuffer(event)
		//bufOut := new(bytes.Buffer)
		//for i := 0; i < buf.Len(); {
		//	ch, size := utf8.DecodeRune(event[i:])
		//	fmt.Printf("下标: %d\t字符: %c\n", i, ch)
		//	binary.Write(bufOut, binary.LittleEndian, ch)
		//	i += size
		//}
		//rdata := bytes.NewReader(bufOut.Bytes())
		//r, err := gzip.NewReader(rdata)
		//if err != nil {
		//	fmt.Println(err)
		//}
		//s, _ := ioutil.ReadAll(r)
		//fmt.Println(s)
		////data := make([]uint16, len(event)/2)
		////binary.Read(buf, binary.LittleEndian, data)
		////buf = new(bytes.Buffer)
		////buf.Write()
		//to := ``
		//for i, bl, br, r := 0, len(s), bytes.NewReader(s), uint16(0); i < bl; i += 2 {
		//	binary.Read(br, binary.LittleEndian, &r)
		//	to += string(r)
		//}
		//fmt.Println(string(to))
		fmt.Println(event)
		//if util.GetNow().Unix()-lastPingTime > 30 { // ping okex server every 30 seconds
		//	lastPingTime = util.GetNow().Unix()
		//	pingMap := make(map[string]interface{})
		//	pingMap["event"] = "ping"
		//	pingParams := util.JsonEncodeMapToByte(pingMap)
		//	if err := conn.WriteMessage(websocket.TextMessage, pingParams); err != nil {
		//		util.SocketInfo("okex server ping client error " + err.Error())
		//	}
		//}
		//messages := make([]OKEXMessage, 1)
		//if err := json.Unmarshal(event, &messages); err == nil {
		//	for _, message := range messages {
		//		symbol := model.GetSymbol(model.OKEX, message.Channel)
		//		if symbol != "" {
		//			bidAsk := model.BidAsk{}
		//			bidAsk.Asks = make([][]float64, len(message.Data.Asks))
		//			bidAsk.Bids = make([][]float64, len(message.Data.Bids))
		//			for i, v := range message.Data.Bids {
		//				price, _ := strconv.ParseFloat(v[0], 64)
		//				amount, _ := strconv.ParseFloat(v[1], 64)
		//				bidAsk.Bids[i] = []float64{price, amount}
		//			}
		//			for i, v := range message.Data.Asks {
		//				price, _ := strconv.ParseFloat(v[0], 64)
		//				amount, _ := strconv.ParseFloat(v[1], 64)
		//				bidAsk.Asks[i] = []float64{price, amount}
		//			}
		//			//bidAsk.Bids = message.Data.Bids
		//			sort.Sort(bidAsk.Asks)
		//			sort.Reverse(bidAsk.Bids)
		//			bidAsk.Ts = message.Data.Timestamp
		//			if markets.SetBidAsk(symbol, model.OKEX, &bidAsk) {
		//				if carry, err := markets.NewCarry(symbol); err == nil {
		//					carryHandler(carry)
		//				}
		//			}
		//		}
		//	}
		//}
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
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded", "User-Agent":
	"Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.71 Safari/537.36"}
	var responseBody []byte
	responseBody, _ = util.HttpRequest(method, uri, postData.Encode(), headers)
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
	Maintain(accounts, model.Huobi)
}

// order_side 限价单(buy/sell) 市价单(buy_market/sell_market)
func PlaceOrderCoinbigs(symbol string) (orderId, errCode, errMsg string) {
	postData := &url.Values{}
	postData.Set(`apikey`, model.ApplicationConfig.CoinbigKey)
	postData.Set(`symbol`, symbol)
	//postData.Set(`type`, side)
	postData.Set(`ordersdata`, `[{price:3,amount:5,type:"sell"},{price:3,amount:3,type:"buy"}]`)
	responseBody := SignedRequestCoinbig(`POST`, `/batch_trade`, postData)
	fmt.Println(string(responseBody))
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

func QueryOrderCoinbig(orderId string) (dealAmount float64, status string) {
	postData := &url.Values{}
	postData.Set(`apikey`, model.ApplicationConfig.CoinbigKey)
	postData.Set(`orderId`, orderId)
	responseBody := SignedRequestCoinbig(`POST`, `/getOrderInfoById`, postData)
	fmt.Println()
	fmt.Println(string(responseBody))
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