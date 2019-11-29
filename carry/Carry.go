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

func setCarrying(value bool) {
	carrying = value
}

var ProcessCarryOrder = func(market, symbol string) {
	carryLock.Lock()
	defer carryLock.Unlock()
	orders := model.AppMarkets.GetBmPendingOrders()
	if model.ConnectionResetTime == 0 {
		util.Notice(`first entry order handler, cancel all orders`)
		for _, value := range orders {
			api.MustCancel(``, ``, value.Market, value.Symbol, value.OrderId, false)
		}
		model.ConnectionResetTime = util.GetNowUnixMillion()
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
			fmOrder := api.PlaceOrder(``, ``, orderSide, model.OrderTypeMarket, market, symbol,
				``, ``, ``, bmOrder.DealPrice, bmOrder.DealAmount-fmTakeAmount, true)
			if fmOrder != nil && fmOrder.OrderId != `` {
				fmTakeAmount = bmOrder.DealAmount
			}
		}
	} else {
		util.Notice(fmt.Sprintf(`= = = = lost bm need renew order`))
		accountBM := model.AppAccounts.GetAccount(model.Bitmex, symbol)
		accountFM := model.AppAccounts.GetAccount(model.Fmex, symbol)
		now := util.GetNowUnixMillion()
		bmOrder = api.QueryOrderById(``, ``, model.Bitmex, symbol, bmOrder.OrderId)
		if bmOrder != nil {
			if orders == nil {
				orders = make(map[string]*model.Order)
			}
			orders[bmOrder.OrderId] = bmOrder
			model.AppMarkets.SetBMPendingOrders(orders)
			util.Notice(fmt.Sprintf(`lost bm renew order %s amount:%f carry:%f left:%f`,
				bmOrder.OrderId, bmOrder.Amount, fmTakeAmount, bmOrder.DealAmount-fmTakeAmount))
		} else {
			if accountBM == nil || accountFM == nil || now-accountBM.Ts > 30000 || now-accountFM.Ts > 30000 {
				api.RefreshAccount(``, ``, model.Bitmex)
				api.RefreshAccount(``, ``, model.Fmex)
				util.Notice(`= = = fm bm 持仓信息未能及时更新`)
			} else {
				if accountFM.Free+accountBM.Free > 1 {
					fmOrder := api.PlaceOrder(``, ``, model.OrderSideSell, model.OrderTypeMarket, market,
						symbol, ``, ``, ``, bmOrder.DealPrice, accountFM.Free+accountBM.Free, true)
					if fmOrder != nil && fmOrder.OrderId != `` {
						util.Notice(fmt.Sprintf(`-- -- 扯平仓位 fm:%f bm:%f place order %s %s amount %f`,
							accountFM.Free, accountBM.Free, fmOrder.OrderSide, fmOrder.OrderId, fmOrder.Amount))
					}
				} else if accountFM.Free+accountBM.Free < -1 {
					fmOrder := api.PlaceOrder(``, ``, model.OrderSideBuy, model.OrderTypeMarket, market,
						symbol, ``, ``, ``, bmOrder.DealPrice,
						math.Abs(accountFM.Free+accountBM.Free), true)
					if fmOrder != nil && fmOrder.OrderId != `` {
						util.Notice(fmt.Sprintf(`-- -- 扯平仓位 fm:%f bm:%f place order %s %s amount %f`,
							accountFM.Free, accountBM.Free, fmOrder.OrderSide, fmOrder.OrderId, fmOrder.Amount))
					}
				}
			}
			fmTakeAmount = 0
		}
	}
	if bmOrder != nil && (math.Abs(fmTakeAmount-bmOrder.Amount) < 1 ||
		(math.Abs(fmTakeAmount-bmOrder.DealAmount) < 1 && bmOrder.Status != model.CarryStatusWorking)) {
		time.Sleep(time.Second)
		api.RefreshAccount(``, ``, market)
		util.Notice(fmt.Sprintf(`--->>>> carry done set nil and 0, %s %s-%s amount:%f-%f`,
			bmOrder.Status, bmOrder.OrderSide, orderSide, bmOrder.DealAmount, fmTakeAmount))
		bmOrder = nil
		fmTakeAmount = 0
	}
}

var _ = func(ignore, symbol string) {
	carryLock.Lock()
	defer carryLock.Unlock()
	startTime := util.GetNowUnixMillion()
	_, tickBM := model.AppMarkets.GetBidAsk(symbol, model.Bitmex)
	_, tickFM := model.AppMarkets.GetBidAsk(symbol, model.Fmex)
	accountBM := model.AppAccounts.GetAccount(model.Bitmex, symbol)
	accountFM := model.AppAccounts.GetAccount(model.Fmex, symbol)
	if accountFM == nil {
		api.RefreshAccount(``, ``, model.Fmex)
		util.Notice(`account is nil, refresh and return`)
		return
	}
	if tickFM == nil || tickBM == nil || tickFM.Asks == nil || tickFM.Bids == nil || tickBM.Asks == nil ||
		tickBM.Bids == nil || tickFM.Asks.Len() < 18 || tickFM.Bids.Len() < 18 || tickBM.Asks.Len() < 18 ||
		tickBM.Bids.Len() < 18 || int(startTime)-tickBM.Ts > 500 || int(startTime)-tickFM.Ts > 500 ||
		model.AppConfig.Handle != `1` || model.AppPause || accountBM == nil ||
		startTime-accountBM.Ts > 10000 {
		return
	}
	if carrying {
		return
	}
	setCarrying(true)
	defer setCarrying(false)
	setting := model.GetSetting(model.FunctionCarry, model.Fmex, symbol)
	p1 := 0.0
	p2 := 0.0
	a1 := setting.AmountLimit
	a2 := setting.AmountLimit
	if accountFM.Free > setting.AmountLimit/3 && accountBM.Free < setting.AmountLimit/-3 {
		p1 = accountBM.EntryPrice - accountFM.EntryPrice - setting.PriceX - setting.GridPriceDistance
		p2 = setting.GridPriceDistance * accountBM.Free * 2 / setting.AmountLimit
		a1 = accountFM.Free
		a2 = setting.AmountLimit - accountFM.Free
	} else if accountFM.Free < setting.AmountLimit/-3 && accountBM.Free > setting.AmountLimit/3 {
		p1 = setting.GridPriceDistance * accountFM.Free * 2 / setting.AmountLimit
		p2 = accountFM.EntryPrice - accountBM.EntryPrice + setting.PriceX - setting.GridPriceDistance
		a1 = setting.AmountLimit - accountBM.Free
		a2 = accountBM.Free
	}
	priceDistance := 0.1 / math.Pow(10, api.GetPriceDecimal(model.Fmex, symbol))
	fmba := getDepthAmountBuy(tickBM.Bids[0].Price+setting.GridPriceDistance-p1-setting.PriceX,
		priceDistance, tickFM)
	fmsa := getDepthAmountSell(tickBM.Asks[0].Price-setting.GridPriceDistance+p2-setting.PriceX,
		priceDistance, tickFM)
	fmb1 := tickFM.Bids[0].Price + setting.PriceX
	fms1 := tickFM.Asks[0].Price + setting.PriceX
	var order *model.Order
	if bmOrder == nil {
		if fmb1-tickBM.Bids[0].Price >= setting.GridPriceDistance-p1 && fmba >= setting.RefreshLimitLow {
			amount := math.Min(math.Min(fmba/2, a1), setting.GridAmount)
			price := tickBM.Bids[0].Price
			if amount > 1 {
				util.Notice(fmt.Sprintf(`amt fm:%f amt bm:%f p1:%f p2:%f a1:%f a2:%f fmba:%f=%f-%f 
			fmsa:%f=%f-%f 价1:%f %f 量1:%f %f`, accountFM.Free, accountBM.Free, p1, p2, a1, a2, fmba,
					tickBM.Bids[0].Price+setting.GridPriceDistance-p1, tickBM.Bids[0].Price, fmsa, tickBM.Asks[0].Price,
					tickBM.Asks[0].Price-setting.GridPriceDistance+p2, tickBM.Bids[0].Price, tickBM.Asks[0].Price,
					tickBM.Bids[0].Amount, tickBM.Asks[0].Amount))
				order = api.PlaceOrder(``, ``, model.OrderSideBuy, model.OrderTypeLimit, model.Bitmex, symbol,
					``, ``, ``, price, amount, true)
			}
		} else if tickBM.Asks[0].Price-fms1 >= setting.GridPriceDistance-p2 &&
			fmsa >= setting.RefreshLimitLow {
			amount := math.Min(math.Min(fmsa/2, a2), setting.GridAmount)
			price := tickBM.Asks[0].Price
			if amount > 1 {
				util.Notice(fmt.Sprintf(`amt fm:%f amt bm:%f p1:%f p2:%f a1:%f a2:%f fmba:%f=%f-%f fmsa:%f=%f-%f 
			价1:%f %f 量1:%f %f`, accountFM.Free, accountBM.Free, p1, p2, a1, a2, fmba,
					tickBM.Bids[0].Price+setting.GridPriceDistance-p1, tickBM.Bids[0].Price, fmsa, tickBM.Asks[0].Price,
					tickBM.Asks[0].Price-setting.GridPriceDistance+p2, tickBM.Bids[0].Price, tickBM.Asks[0].Price,
					tickBM.Bids[0].Amount, tickBM.Asks[0].Amount))
				order = api.PlaceOrder(``, ``, model.OrderSideSell, model.OrderTypeLimit, model.Bitmex, symbol,
					``, ``, ``, price, amount, true)
			}
		}
		if order != nil && order.OrderId != `` {
			bmOrder = order
			fmTakeAmount = 0
		}
	} else {
		cancelResult := true
		switch bmOrder.OrderSide {
		case model.OrderSideBuy:
			if fmba < 1.2*(bmOrder.Amount-bmOrder.DealAmount) || tickBM.Bids[1].Price-priceDistance > bmOrder.Price ||
				(math.Abs(tickBM.Bids[1].Price-bmOrder.Price) < priceDistance &&
					tickBM.Asks[0].Amount < 10*tickBM.Bids[0].Amount) {
				util.Notice(fmt.Sprintf(`=== cancel order bid %f<%f || %f<%f`,
					fmba, 1.2*(bmOrder.Amount-bmOrder.DealAmount), tickBM.Bids[1].Price-priceDistance, bmOrder.Price))
				cancelResult, _ = api.MustCancel(``, ``, model.Bitmex, symbol, bmOrder.OrderId, false)
			}
		case model.OrderSideSell:
			if fmsa < 1.2*(bmOrder.Amount-bmOrder.DealAmount) || tickBM.Asks[0].Price+priceDistance < bmOrder.Price {
				util.Notice(fmt.Sprintf(`=== cancel order ask %f<%f || %f<%f`,
					fmsa, 1.2*(bmOrder.Amount-bmOrder.DealAmount), tickBM.Asks[1].Price+priceDistance, bmOrder.Price))
				cancelResult, _ = api.MustCancel(``, ``, model.Bitmex, symbol, bmOrder.OrderId, false)
			}
		}
		if !cancelResult {
			order = api.QueryOrderById(``, ``, model.Bitmex, symbol, bmOrder.OrderId)
		}
	}
	orders := model.AppMarkets.GetBmPendingOrders()
	if order != nil && orders[order.OrderId] == nil {
		orders[order.OrderId] = order
		model.AppMarkets.SetBMPendingOrders(orders)
	}
}
