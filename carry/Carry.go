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

// 5 setting.GridPriceDistance
// A总 setting.AmountLimit
// 4000 setting.refreshLimitLow
// 20000 setting.GridAmount

var carrying = false
var carryLock sync.Mutex
var bmOrder *model.Order
var fmTakeAmount = 0.0
var bmStartUpTime = int64(0)

func setCarrying(value bool) {
	carrying = value
}

var ProcessCarryOrder = func(market, symbol string) {
	carryLock.Lock()
	defer carryLock.Unlock()
	orders := model.AppMarkets.GetBmPendingOrders()
	if bmStartUpTime == 0 {
		util.Notice(`first entry order handler, cancel all orders`)
		for _, value := range orders {
			api.MustCancel(``, ``, value.Market, value.Symbol, value.OrderId, false)
		}
		bmStartUpTime = util.GetNowUnixMillion()
	}
	if bmOrder == nil {
		return
	}
	market = model.Fmex
	symbol = bmOrder.Symbol
	orderSide := model.OrderSideBuy
	if bmOrder.OrderSide == model.OrderSideBuy {
		orderSide = model.OrderSideSell
	}
	if orders != nil && orders[bmOrder.OrderId] != nil {
		bmOrder = orders[bmOrder.OrderId]
		if bmOrder.DealAmount-fmTakeAmount >= 1 {
			util.Notice(fmt.Sprintf(`follow place market order %s-%s amount:%f->%f`,
				bmOrder.OrderSide, orderSide, fmTakeAmount, bmOrder.DealAmount))
			fmOrder := api.PlaceOrder(``, ``, orderSide, model.OrderTypeMarket, market,
				symbol, ``, ``, bmOrder.DealPrice, bmOrder.DealAmount-fmTakeAmount)
			if fmOrder != nil && fmOrder.OrderId != `` {
				fmTakeAmount = bmOrder.DealAmount
				go model.AppDB.Save(&fmOrder)
			}
		}
	} else {
		bmOrder = api.QueryOrderById(``, ``, model.Bitmex, symbol, bmOrder.OrderId)
		util.Notice(fmt.Sprintf(`lost bm renew order %s amount:%f carry:%f left:%f`,
			bmOrder.OrderId, bmOrder.Amount, fmTakeAmount, bmOrder.DealAmount-fmTakeAmount))
	}
	if math.Abs(fmTakeAmount-bmOrder.Amount) < 1 ||
		(math.Abs(fmTakeAmount-bmOrder.DealAmount) < 1 && bmOrder.Status != model.CarryStatusWorking) {
		time.Sleep(time.Second)
		api.RefreshAccount(``, ``, market)
		util.Notice(fmt.Sprintf(`carry done set nil and 0, %s %s-%s amount:%f-%f`,
			bmOrder.Status, bmOrder.OrderSide, orderSide, bmOrder.DealAmount, fmTakeAmount))
		bmOrder = nil
		fmTakeAmount = 0
	}
}

var ProcessCarry = func(market, symbol string) {
	startTime := util.GetNowUnixMillion()
	_, tickBM := model.AppMarkets.GetBidAsk(symbol, model.Bitmex)
	_, tick := model.AppMarkets.GetBidAsk(symbol, market)
	accountBM := model.AppAccounts.GetAccount(model.Bitmex, symbol)
	account := model.AppAccounts.GetAccount(market, symbol)
	if account == nil {
		api.RefreshAccount(``, ``, market)
		util.Notice(`account is nil, refresh and return`)
		return
	}
	if tick == nil || tickBM == nil || tick.Asks == nil || tick.Bids == nil || tickBM.Asks == nil ||
		tickBM.Bids == nil || tick.Asks.Len() < 18 || tick.Bids.Len() < 18 || tickBM.Asks.Len() < 18 ||
		tickBM.Bids.Len() < 18 || int(startTime)-tickBM.Ts > 500 || int(startTime)-tick.Ts > 500 ||
		model.AppConfig.Handle != `1` || model.AppPause || accountBM == nil ||
		startTime-accountBM.Ts > 10000 {
		//if bmOrder != nil {
		//	util.Notice(fmt.Sprintf(`[for some reason cancel bm order]%s %s %s`, market, symbol, bmOrder.OrderId))
		//	api.MustCancel(``, ``, model.Bitmex, symbol, bmOrder.OrderId, true)
		//}
		return
	}
	if carrying {
		return
	}
	setCarrying(true)
	defer setCarrying(false)
	setting := model.GetSetting(model.FunctionCarry, market, symbol)
	p1 := 0.0
	p2 := 0.0
	a1 := setting.AmountLimit
	a2 := setting.AmountLimit
	if account.Free > setting.AmountLimit/3 && accountBM.Free < setting.AmountLimit/-3 {
		p1 = accountBM.EntryPrice - account.EntryPrice - setting.GridPriceDistance
		a1 = account.Free
		a2 = setting.AmountLimit - account.Free
	} else if account.Free < setting.AmountLimit/-3 && accountBM.Free > setting.AmountLimit/3 {
		p2 = account.EntryPrice - accountBM.EntryPrice - setting.GridPriceDistance
		a1 = setting.AmountLimit - accountBM.Free
		a2 = accountBM.Free
	}
	priceDistance := 1 / math.Pow(10, api.GetPriceDecimal(market, symbol))
	fmba := getDepthAmountBuy(tickBM.Bids[0].Price+setting.GridPriceDistance-p1, priceDistance, tick)
	fmsa := getDepthAmountSell(tickBM.Asks[0].Price-setting.GridPriceDistance+p2, priceDistance, tick)
	var order *model.Order
	if bmOrder == nil {
		if tick.Bids[0].Price-tickBM.Bids[0].Price >= setting.GridPriceDistance-p1 && fmba >= setting.RefreshLimitLow {
			amount := math.Min(math.Min(fmba/2, a1), setting.GridAmount)
			price := tickBM.Bids[0].Price
			if tickBM.Bids[0].Amount < tickBM.Asks[0].Amount/10 {
				price = tickBM.Bids[1].Price
			}
			util.Notice(fmt.Sprintf(`amt fm:%f amt bm:%f p1:%f p2:%f a1:%f a2:%f fmba:%f=%f-%f fmsa:%f=%f-%f`,
				account.Free, accountBM.Free, p1, p2, a1, a2, fmba, tickBM.Bids[0].Price+setting.GridPriceDistance-p1,
				tickBM.Bids[0].Price, fmsa, tickBM.Asks[0].Price, tickBM.Asks[0].Price-setting.GridPriceDistance+p2))
			order = api.PlaceOrder(``, ``, model.OrderSideBuy, model.OrderTypeLimit, model.Bitmex, symbol,
				``, ``, price, amount)
		} else if tickBM.Asks[0].Price-tick.Asks[0].Price >= setting.GridPriceDistance-p2 &&
			fmsa >= setting.RefreshLimitLow {
			amount := math.Min(math.Min(fmsa/2, a2), setting.GridAmount)
			price := tickBM.Asks[0].Price
			if tickBM.Asks[0].Amount < tickBM.Bids[0].Amount/10 {
				price = tickBM.Asks[1].Price
			}
			util.Notice(fmt.Sprintf(`amt fm:%f amt bm:%f p1:%f p2:%f a1:%f a2:%f fmba:%f=%f-%f fmsa:%f=%f-%f`,
				account.Free, accountBM.Free, p1, p2, a1, a2, fmba, tickBM.Bids[0].Price+setting.GridPriceDistance-p1,
				tickBM.Bids[0].Price, fmsa, tickBM.Asks[0].Price, tickBM.Asks[0].Price-setting.GridPriceDistance+p2))
			order = api.PlaceOrder(``, ``, model.OrderSideSell, model.OrderTypeLimit, model.Bitmex, symbol,
				``, ``, price, amount)
		}
		if order != nil && order.OrderId != `` {
			go model.AppDB.Save(&order)
			bmOrder = order
			fmTakeAmount = 0
		}
	} else {
		switch bmOrder.OrderSide {
		case model.OrderSideBuy:
			if fmba < 1.2*(bmOrder.Amount-bmOrder.DealAmount) || tickBM.Bids[1].Price-priceDistance > bmOrder.Price ||
				(math.Abs(tickBM.Bids[1].Price-bmOrder.Price) < priceDistance &&
					tickBM.Asks[0].Amount < 10*tickBM.Bids[0].Amount) {
				api.MustCancel(``, ``, model.Bitmex, symbol, bmOrder.OrderId, true)
			}
		case model.OrderSideSell:
			if fmsa < 1.2*(bmOrder.Amount-bmOrder.DealAmount) || tickBM.Asks[1].Price+priceDistance < bmOrder.Price ||
				(math.Abs(tickBM.Asks[1].Price-bmOrder.Price) < priceDistance &&
					tickBM.Bids[0].Amount < 10*tickBM.Asks[0].Amount) {
				api.MustCancel(``, ``, model.Bitmex, symbol, bmOrder.OrderId, true)
			}
		}
	}
}

func getDepthAmountSell(price, priceDistance float64, tick *model.BidAsk) (amount float64) {
	amount = 0
	for i := 0; i < tick.Asks.Len(); i++ {
		if price > tick.Asks[i].Price-priceDistance {
			amount += tick.Asks[i].Amount
		} else {
			break
		}
	}
	return
}

func getDepthAmountBuy(price, priceDistance float64, tick *model.BidAsk) (amount float64) {
	amount = 0
	for i := 0; i < tick.Bids.Len(); i++ {
		if price < tick.Bids[i].Price+priceDistance {
			amount += tick.Bids[i].Amount
		} else {
			break
		}
	}
	return
}
