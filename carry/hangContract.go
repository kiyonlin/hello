package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"sync"
	"time"
)

var tradeUpdate = int64(0)

type HangContractOrders struct {
	lock            sync.Mutex
	hangingContract bool
	holdingLong     float64
	holdingShort    float64
	orders          map[string][]*model.Order // symbol - order
}

var hangContractOrders = &HangContractOrders{}

func (hangContractOrders *HangContractOrders) setInHangingContract(in bool) {
	hangContractOrders.hangingContract = in
}

func (hangContractOrders *HangContractOrders) getHangContractOrders(symbol string) (orders []*model.Order) {
	defer hangContractOrders.lock.Unlock()
	hangContractOrders.lock.Lock()
	if hangContractOrders.orders == nil {
		hangContractOrders.orders = make(map[string][]*model.Order)
	}
	return hangContractOrders.orders[symbol]
}

func (hangContractOrders *HangContractOrders) setHangContractOrders(symbol string, orders []*model.Order) {
	defer hangContractOrders.lock.Unlock()
	hangContractOrders.lock.Lock()
	if hangContractOrders.orders == nil {
		hangContractOrders.orders = make(map[string][]*model.Order)
	}
	hangContractOrders.orders[symbol] = orders
}

var ProcessHangContract = func(market, symbol string) {
	startTime := util.GetNowUnixMillion()
	start, end := model.AppMarkets.GetTrends(symbol)
	if start == nil || end == nil || start[market] == nil || end[market] == nil {
		return
	}
	_, tick := model.AppMarkets.GetBidAsk(symbol, market)
	if tick == nil || tick.Asks == nil || tick.Bids == nil || tick.Asks.Len() < 5 || tradeUpdate == 0 ||
		tick.Bids.Len() < 5 || int(startTime)-tick.Ts > 500 || model.AppConfig.Handle != `1` || model.AppPause {
		timeDis := 0
		tradeUpdate = startTime
		if tick != nil {
			timeDis = int(startTime) - tick.Ts
		}
		util.Notice(fmt.Sprintf(`[for some reason cancel hang contract]%s %s %d deal`,
			market, symbol, timeDis))
		CancelHangContracts(key, secret, market, symbol)
		return
	}
	if hangContractOrders.hangingContract {
		return
	}
	hangContractOrders.setInHangingContract(true)
	defer hangContractOrders.setInHangingContract(false)
	setting := model.GetSetting(model.FunctionHangContract, market, symbol)
	deltaBM := end[model.Bitmex].Price - start[model.Bitmex].Price
	delta := end[market].Price - start[market].Price
	order := &model.Order{}
	if deltaBM >= setting.RefreshLimit && deltaBM-delta >= setting.RefreshLimit {
		order = createHolding(key, secret, market, symbol, setting.RefreshLimit, setting, tick)
	} else if deltaBM <= -1*setting.RefreshLimit && deltaBM-delta <= -1*setting.RefreshLimit {
		order = createHolding(key, secret, market, symbol, -1*setting.RefreshLimit, setting, tick)
	} else {
		order = revertHolding(key, secret, market, symbol, deltaBM, delta, setting, tick)
	}
	time.Sleep(time.Millisecond * 200)
	orders := updateContractHolding(market, symbol, setting)
	if order == nil || order.OrderId == `` {
		return
	}
	needAdd := true
	for _, value := range orders {
		if value.OrderId == order.OrderId {
			needAdd = false
			break
		}
	}
	if needAdd {
		util.Notice(fmt.Sprintf(`query order can not find %s %f amount %f`,
			order.OrderSide, order.Price, order.Amount))
		orders = append(orders, order)
		hangContractOrders.setHangContractOrders(symbol, orders)
	}
}

func updateContractHolding(market, symbol string, setting *model.Setting) (orders []*model.Order) {
	api.RefreshAccount(key, secret, market)
	account := model.AppAccounts.GetAccount(market, symbol)
	if account != nil && account.Direction == model.OrderSideBuy {
		hangContractOrders.holdingLong = account.Free
		hangContractOrders.holdingShort = 0
	}
	if account != nil && account.Direction == model.OrderSideSell {
		hangContractOrders.holdingShort = account.Free
		hangContractOrders.holdingLong = 0
	}
	orders = api.QueryOrders(key, secret, market, symbol, ``, setting.AccountType, 0, 0)
	hangContractOrders.setHangContractOrders(symbol, orders)
	//if hangContractOrders.holdingLong > 0 || hangContractOrders.holdingShort > 0 || len(filteredOrders) > 0 {
	//	util.Notice(fmt.Sprintf(`====long %f ====short %f pending >100 orders: %d`,
	//		hangContractOrders.holdingLong, hangContractOrders.holdingShort, len(filteredOrders)))
	//}
	return orders
}

func createHolding(key, secret, market, symbol string, trend float64,
	setting *model.Setting, tick *model.BidAsk) (order *model.Order) {
	orders := hangContractOrders.getHangContractOrders(symbol)
	priceDistance := 0.1 / math.Pow(10, api.GetPriceDecimal(market, symbol))
	holdingShort := hangContractOrders.holdingShort
	holdingLong := hangContractOrders.holdingLong
	for _, order := range orders {
		if order.OrderId == `` {
			continue
		}
		if order.OrderSide == model.OrderSideBuy &&
			order.Price+priceDistance >= tick.Asks[0].Price {
			holdingLong += order.Amount - order.DealAmount
		} else if order.OrderSide == model.OrderSideSell &&
			order.Price-priceDistance <= tick.Bids[0].Price {
			holdingShort += order.Amount - order.DealAmount
		} else {
			cancelHangContract(key, secret, order)
		}
	}
	util.Notice(fmt.Sprintf(`create holding trend:%f holding long:%f holding short:%f`,
		trend, holdingLong, holdingShort))
	if trend > 0 && setting.AmountLimit-holdingLong+holdingShort > 0 {
		order = api.PlaceOrder(key, secret, model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``,
			setting.AccountType, ``, tick.Asks[0].Price, setting.AmountLimit-holdingLong+holdingShort, true)
	}
	if trend < 0 && setting.AmountLimit-holdingShort+holdingLong > 0 {
		order = api.PlaceOrder(key, secret, model.OrderSideSell, model.OrderTypeLimit, market, symbol, ``,
			setting.AccountType, ``, tick.Bids[0].Price, setting.AmountLimit-holdingShort+holdingLong, true)
	}
	if order != nil && order.OrderId != `` {
		return order
	}
	return nil
}

func revertHolding(key, secret, market, symbol string, deltaBM, delta float64,
	setting *model.Setting, tick *model.BidAsk) (
	revertOrder *model.Order) {
	orders := hangContractOrders.getHangContractOrders(symbol)
	priceDistance := 0.1 / math.Pow(10, api.GetPriceDecimal(market, symbol))
	holdingShort := hangContractOrders.holdingShort
	holdingLong := hangContractOrders.holdingLong
	for _, order := range orders {
		if order.OrderId == `` {
			continue
		}
		if order.OrderSide == model.OrderSideBuy &&
			order.Price+priceDistance >= tick.Asks[0].Price {
			holdingLong += order.Amount - order.DealAmount
		} else if order.OrderSide == model.OrderSideSell &&
			order.Price-priceDistance <= tick.Bids[0].Price {
			holdingShort += order.Amount - order.DealAmount
		}
	}
	//if holdingShort != 0 || holdingLong != 0 {
	//	util.Notice(fmt.Sprintf(`revert holding long:%f short:%f tick:%f-%f`,
	//		holdingLong, holdingShort, tick.Bids[0].Price, tick.Asks[0].Price))
	//}
	if holdingLong > holdingShort && deltaBM-delta < setting.RefreshLimitLow {
		amount := holdingLong - holdingShort
		for _, order := range orders {
			if order.OrderSide == model.OrderSideBuy ||
				(order.OrderSide == model.OrderSideSell && order.Price-priceDistance > tick.Asks[0].Price) {
				cancelHangContract(key, secret, order)
			} else if order.OrderSide == model.OrderSideSell &&
				math.Abs(order.Price-tick.Asks[0].Price) < priceDistance {
				amount = amount - (order.Amount - order.DealAmount)
				util.Notice(fmt.Sprintf(`revert long with short %f long:%f short:%f deltaBM:%f delta:%f low:%f`,
					order.Amount-order.DealAmount, holdingLong, holdingShort, deltaBM, delta, setting.RefreshLimitLow))
			}
		}
		if amount > 0 {
			util.Notice(fmt.Sprintf(`revert long amount:%f at %f long:%f short:%f deltaBM:%f delta:%f low:%f`,
				amount, tick.Asks[0].Price, holdingLong, holdingShort, deltaBM, delta, setting.RefreshLimitLow))
			revertOrder = api.PlaceOrder(key, secret, model.OrderSideSell, model.OrderTypeLimit, market, symbol,
				``, setting.AccountType, ``, tick.Asks[0].Price, amount, true)
		}
	}
	if holdingLong < holdingShort && deltaBM-delta > setting.RefreshLimitLow {
		amount := holdingShort - holdingLong
		for _, order := range orders {
			if order.OrderSide == model.OrderSideSell ||
				(order.OrderSide == model.OrderSideBuy && order.Price+priceDistance < tick.Bids[0].Price) {
				cancelHangContract(key, secret, order)
			} else if order.OrderSide == model.OrderSideBuy &&
				math.Abs(order.Price-tick.Bids[0].Price) < priceDistance {
				amount = amount - (order.Amount - order.DealAmount)
				util.Notice(fmt.Sprintf(`revert short with long %f long:%f short:%f deltaBM:%f delta:%f low:%f`,
					order.Amount-order.DealAmount, holdingLong, holdingShort, deltaBM, delta, setting.RefreshLimitLow))
			}
		}
		if amount > 0 {
			util.Notice(fmt.Sprintf(`revert short amount:%f at %f long:%f short:%f deltaBM:%f delta:%f low:%f`,
				amount, tick.Bids[0].Price, holdingLong, holdingShort, deltaBM, delta, setting.RefreshLimitLow))
			revertOrder = api.PlaceOrder(key, secret, model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``,
				setting.AccountType, ``, tick.Bids[0].Price, amount, true)
		}
	}
	if revertOrder != nil && revertOrder.OrderId != `` {
		return revertOrder
	}
	return nil
}

func CancelHangContracts(key, secret, market, symbol string) {
	util.Notice(`cancel all hang contract`)
	orders := api.QueryOrders(key, secret, market, symbol, ``, ``, 0, 0)
	//orders := hangContractOrders.getHangContractOrders(symbol)
	for _, order := range orders {
		if order != nil && order.OrderId != `` {
			api.MustCancel(key, secret, market, symbol, order.OrderId, true)
			time.Sleep(time.Millisecond * 50)
		}
	}
	hangContractOrders.setHangContractOrders(symbol, nil)
}

func cancelHangContract(key, secret string, order *model.Order) {
	if order != nil && order.Amount-order.DealAmount > 100 {
		api.MustCancel(key, secret, order.Market, order.Symbol, order.OrderId, false)
	}
}
