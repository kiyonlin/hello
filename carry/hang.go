package carry

//var hangStatus = &HangStatus{}
//
//type HangStatus struct {
//	lock       sync.Mutex
//	hanging    map[string]bool           // symbol - hanging
//	hangOrders map[string][]*model.Order // symbol - orders
//}
//
//func (hangStatus *HangStatus) setHanging(symbol string, value bool) {
//	hangStatus.lock.Lock()
//	defer hangStatus.lock.Unlock()
//
//	hanging = value
//}
//
//func (hangStatus *HangStatus) getHanging(symbol string) (hanging bool) {
//
//}
//
//func (hangStatus *HangStatus) getHangOrders(symbol string) (hanging []*model.Order) {
//	hangStatus.lock.Lock()
//	defer hangStatus.lock.Unlock()
//	if hangStatus.hangOrders == nil {
//		hangStatus.hangOrders = make(map[string][]*model.Order)
//	}
//	if hangStatus.hangOrders[symbol] == nil {
//		hangStatus.hangOrders[symbol] = make([]*model.Order, 0)
//	}
//	return hangStatus.hangOrders[symbol]
//}
//
//func (hangStatus *HangStatus) setHangOrders(symbol string, orders []*model.Order) {
//	if hangStatus.hangOrders == nil {
//		hangStatus.hangOrders = make(map[string][]*model.Order)
//	}
//	if hangStatus.hangOrders[symbol] == nil {
//		hangStatus.hangOrders[symbol] = make([]*model.Order, 0)
//	}
//	hangStatus.hangOrders[symbol] = orders
//}
//
//var ProcessHang = func(market, symbol string) {
//	if hanging || model.AppConfig.Handle != `1` {
//		return
//	}
//	hangStatus.setHanging(true)
//	defer hangStatus.setHanging(false)
//	if model.AppMarkets.BidAsks[symbol] == nil {
//		return
//	}
//	tick := model.AppMarkets.BidAsks[symbol][market]
//	if tick == nil || tick.Asks == nil || tick.Bids == nil || tick.Asks.Len() < 15 || tick.Bids.Len() < 15 {
//		util.Notice(fmt.Sprintf(`[tick not good]%s %s`, market, symbol))
//		return
//	}
//	delay := util.GetNowUnixMillion() - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
//	if delay > 200 {
//		util.Notice(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
//		return
//	}
//	setting := model.GetSetting(model.FunctionHang, market, symbol)
//	if validHang(market, symbol, tick) {
//		time.Sleep(time.Second)
//		api.RefreshAccount(market)
//		return
//	}
//	hang(market, symbol, setting.AccountType, tick)
//	api.RefreshAccount(market)
//}
//
//func hang(market, symbol, accountType string, tick *model.BidAsk) {
//	leftFree, rightFree, leftFroze, rightFroze, err := getBalance(market, symbol, accountType)
//	if err != nil {
//		return
//	}
//	rightFree = rightFree / tick.Asks[0].Price
//	rightFroze = rightFroze / tick.Asks[0].Price
//	if rightFroze+leftFroze < (leftFree+leftFroze+rightFree+rightFroze)*model.AppConfig.AmountRate {
//		placeHangOrder(model.OrderSideSell, market, symbol, accountType,
//			tick.Asks[3].Price, leftFree*model.AppConfig.AmountRate/2)
//		placeHangOrder(model.OrderSideSell, market, symbol, accountType,
//			tick.Asks[9].Price, leftFree*model.AppConfig.AmountRate/2)
//		placeHangOrder(model.OrderSideBuy, market, symbol, accountType,
//			tick.Bids[3].Price, rightFree*model.AppConfig.AmountRate/2)
//		placeHangOrder(model.OrderSideBuy, market, symbol, accountType,
//			tick.Bids[9].Price, rightFree*model.AppConfig.AmountRate/2)
//	}
//}
//
//func validHang(market, symbol string, tick *model.BidAsk) (needCancel bool) {
//	needCancel = false
//	if hangingOrders == nil || hangingOrders[symbol] == nil {
//		return needCancel
//	}
//	newHangOrders := make([]*model.Order, 0)
//	for _, value := range hangingOrders[symbol] {
//		if value != nil && value.OrderSide == model.OrderSideBuy {
//			if value.Price > tick.Bids[14].Price && value.Price < tick.Bids[1].Price {
//				newHangOrders = append(newHangOrders, value)
//			} else {
//				needCancel = true
//				api.MustCancel(market, symbol, value.OrderId, true)
//			}
//		}
//		if value != nil && value.OrderSide == model.OrderSideSell {
//			if value.Price < tick.Asks[14].Price && value.Price > tick.Asks[1].Price {
//				newHangOrders = append(newHangOrders, value)
//			} else {
//				needCancel = true
//				api.MustCancel(market, symbol, value.OrderId, true)
//			}
//		}
//	}
//	setHangingOrders(symbol, newHangOrders)
//	return needCancel
//}
//
//func placeHangOrder(orderSide, market, symbol, accountType string, price, amount float64) {
//	if amount*price < 5 {
//		return
//	}
//	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``,
//		accountType, price, amount)
//	time.Sleep(time.Millisecond * 20)
//	if order != nil && order.Status != model.CarryStatusFail && order.OrderId != `` {
//		order.OrderType = model.FunctionHang
//		model.AppDB.Save(order)
//		orders := getHangingOrders(symbol)
//		hangingOrders[symbol] = append(orders, order)
//	}
//}
