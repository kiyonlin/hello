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

var contractHoldingUpdate = int64(0)

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
	start := util.GetNowUnixMillion()
	_, tick := model.AppMarkets.GetBidAsk(symbol, market)
	dealBM := model.AppMarkets.GetTrade(start/1000, model.Bitmex, symbol)
	i := int64(1)
	second := start / 1000
	for ; i < model.AppConfig.TrendTime; i++ {
		if dealBM != nil {
			break
		}
		dealBM = model.AppMarkets.GetTrade(second-i, model.Bitmex, symbol)
	}
	if dealBM == nil || tick == nil || tick.Asks == nil || tick.Bids == nil || tick.Asks.Len() < 15 || i > 3 ||
		tick.Bids.Len() < 15 || int(start)-tick.Ts > 500 || model.AppConfig.Handle != `1` || model.AppPause {
		timeDis := 0
		if tick != nil {
			timeDis = int(start) - tick.Ts
		}
		if i <= 3 {
			util.Notice(fmt.Sprintf(`[for some reason cancel hang contract]%s %s %d deal bm:%d`,
				market, symbol, timeDis, i))
		}
		CancelHangContracts(key, secret, market, symbol)
		return
	}
	setting := model.GetSetting(model.FunctionHangContract, market, symbol)
	if hangContractOrders.hangingContract {
		return
	}
	hangContractOrders.setInHangingContract(true)
	defer hangContractOrders.setInHangingContract(false)
	if contractHoldingUpdate == 0 {
		CancelNonHang(market, symbol)
	}
	order := &model.Order{}
	trend := checkTrend(market, symbol, dealBM, tick)
	if trend == 0 {
		order = revertHolding(``, ``, market, symbol, setting, tick)
	} else {
		order = createHolding(``, ``, market, symbol, trend, setting, tick)
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

func checkTrend(market, symbol string, dealBM *model.Deal, tick *model.BidAsk) (trend float64) {
	trend = 0
	trendStart := model.AppMarkets.TrendStart
	if trendStart == nil || trendStart[symbol] == nil || trendStart[symbol][model.Bitmex] == nil {
		return trend
	}
	delta := (tick.Bids[0].Price+tick.Asks[0].Price)/2 - trendStart[symbol][market].Price
	deltaBM := dealBM.Price - trendStart[symbol][model.Bitmex].Price
	if model.AppMarkets.TrendAmount > 0 && deltaBM-delta > model.AppConfig.Trend && deltaBM > model.AppConfig.Trend {
		return model.AppMarkets.TrendAmount
	}
	if model.AppMarkets.TrendAmount < 0 && delta-deltaBM > model.AppConfig.Trend && -1*deltaBM > model.AppConfig.Trend {
		return model.AppMarkets.TrendAmount
	}
	return 0
}

func updateContractHolding(market, symbol string, setting *model.Setting) (orders []*model.Order) {
	api.RefreshAccount(key, secret, market)
	account := model.AppAccounts.GetAccount(market, symbol)
	if account.Direction == model.OrderSideBuy {
		hangContractOrders.holdingLong = account.Free
		hangContractOrders.holdingShort = 0
	}
	if account.Direction == model.OrderSideSell {
		hangContractOrders.holdingShort = account.Free
		hangContractOrders.holdingLong = 0
	}
	orders = api.QueryOrders(key, secret, market, symbol, ``, setting.AccountType, 0, 0)
	//filteredOrders := make([]*model.Order, 0)
	//for _, order := range orders {
	//	if order.OrderId != `` && order.Amount-order.DealAmount > 100 {
	//		filteredOrders = append(filteredOrders, order)
	//	}
	//}
	hangContractOrders.setHangContractOrders(symbol, orders)
	contractHoldingUpdate = util.GetNowUnixMillion()
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
			setting.AccountType, tick.Asks[0].Price, setting.AmountLimit-holdingLong+holdingShort)
	}
	if trend < 0 && setting.AmountLimit-holdingShort+holdingLong > 0 {
		order = api.PlaceOrder(key, secret, model.OrderSideSell, model.OrderTypeLimit, market, symbol, ``,
			setting.AccountType, tick.Bids[0].Price, setting.AmountLimit-holdingShort+holdingLong)
	}
	if order != nil && order.OrderId != `` {
		return order
	}
	return nil
}

func revertHolding(key, secret, market, symbol string, setting *model.Setting, tick *model.BidAsk) (
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
	if holdingShort != 0 || holdingLong != 0 {
		util.Notice(fmt.Sprintf(`revert holding long:%f short:%f tick:%f-%f`,
			holdingLong, holdingShort, tick.Bids[0].Price, tick.Asks[0].Price))
	}
	if holdingLong > holdingShort {
		amount := holdingLong - holdingShort
		for _, order := range orders {
			if order.OrderSide == model.OrderSideBuy ||
				(order.OrderSide == model.OrderSideSell && order.Price-priceDistance > tick.Asks[0].Price) {
				cancelHangContract(key, secret, order)
			} else if order.OrderSide == model.OrderSideSell &&
				math.Abs(order.Price-tick.Asks[0].Price) < priceDistance {
				amount = amount - (order.Amount - order.DealAmount)
				util.Notice(fmt.Sprintf(`revert holding already short %f`, order.Amount-order.DealAmount))
			}
		}
		if amount > 0 {
			util.Notice(fmt.Sprintf(`revert holding place short amount: %f at %f`, amount, tick.Asks[0].Price))
			revertOrder = api.PlaceOrder(key, secret, model.OrderSideSell, model.OrderTypeLimit, market, symbol,
				``, setting.AccountType, tick.Asks[0].Price, amount)
		}
	}
	if holdingLong < holdingShort {
		amount := holdingShort - holdingLong
		for _, order := range orders {
			if order.OrderSide == model.OrderSideSell ||
				(order.OrderSide == model.OrderSideBuy && order.Price+priceDistance < tick.Bids[0].Price) {
				cancelHangContract(key, secret, order)
			} else if order.OrderSide == model.OrderSideBuy &&
				math.Abs(order.Price-tick.Bids[0].Price) < priceDistance {
				amount = amount - (order.Amount - order.DealAmount)
				util.Notice(fmt.Sprintf(`revert holding already long %f`, order.Amount-order.DealAmount))
			}
		}
		if amount > 0 {
			util.Notice(fmt.Sprintf(`revert holding place long amount: %f at %f`, amount, tick.Bids[0].Price))
			revertOrder = api.PlaceOrder(key, secret, model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``,
				setting.AccountType, tick.Bids[0].Price, amount)
		}
	}
	if revertOrder != nil && revertOrder.OrderId != `` {
		return revertOrder
	}
	return nil
}

func CancelHangContracts(key, secret, market, symbol string) {
	util.Notice(`cancel all hang contract`)
	orders := hangContractOrders.getHangContractOrders(symbol)
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
