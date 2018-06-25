package api

//var subscribeHandlerBigOne = func(subscribes []string, conn *websocket.Conn) error {
//	var err error = nil
//	subscription := make([]interface{}, 1)
//	subscription[0] = make(map[string])
//	subscribeMap[`cmd`] = `sub`
//	subscribeMap[`args`] = subscribes
//	json.Marshal()
//	subscribeMessage := util.JsonEncodeMapToByte(subscribeMap)
//	if err = conn.WriteMessage(websocket.TextMessage, []byte(subscribeMessage)); err != nil {
//		util.SocketInfo("huobi can not subscribe " + err.Error())
//		return err
//	}
//	return err
//}
//
//func WsDepthServeBigOne(markets *model.Markets, carryHandler CarryHandler, errHandler ErrHandler) (chan struct{}, error) {
//	wsHandler := func(event []byte, conn *websocket.Conn) {
//		json, err := util.NewJSON(event)
//		if err != nil {
//			errHandler(err)
//			return
//		}
//		json = json.Get("data")
//		if json == nil {
//			return
//		}
//		symbol := model.GetSymbol(model.Binance, json.Get("s").MustString())
//		if symbol != "" {
//			bidAsk := model.BidAsk{}
//			bidsLen := len(json.Get("b").MustArray())
//			bidAsk.Bids = make([][]float64, bidsLen)
//			for i := 0; i < bidsLen; i++ {
//				item := json.Get("b").GetIndex(i)
//				bidAsk.Bids[i] = make([]float64, 2)
//				strPrice, _ := item.GetIndex(0).String()
//				strAmount, _ := item.GetIndex(1).String()
//				bidAsk.Bids[i][0], _ = strconv.ParseFloat(strPrice, 64)
//				bidAsk.Bids[i][1], _ = strconv.ParseFloat(strAmount, 64)
//			}
//			asksLen := len(json.Get("a").MustArray())
//			bidAsk.Asks = make([][]float64, asksLen)
//			for i := 0; i < asksLen; i++ {
//				item := json.Get("a").GetIndex(i)
//				bidAsk.Asks[i] = make([]float64, 2)
//				strPrice, _ := item.GetIndex(0).String()
//				strAmount, _ := item.GetIndex(1).String()
//				bidAsk.Asks[i][0], _ = strconv.ParseFloat(strPrice, 64)
//				bidAsk.Asks[i][1], _ = strconv.ParseFloat(strAmount, 64)
//			}
//			sort.Sort(bidAsk.Asks)
//			sort.Reverse(bidAsk.Bids)
//			bidAsk.Ts = json.Get("E").MustInt()
//			if markets.SetBidAsk(symbol, model.Binance, &bidAsk) {
//				if carry, err := markets.NewCarry(symbol); err == nil {
//					carryHandler(carry)
//				}
//			}
//		}
//	}
//	requestUrl := model.ApplicationConfig.WSUrls[model.BigOne]
//	return WebSocketServe(requestUrl, model.ApplicationConfig.GetSubscribes(model.Fcoin), subscribeHandlerFcoin,
//		wsHandler, errHandler)
//}
