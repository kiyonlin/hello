package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"time"
)

var carrySameTiming = false

func setCarrySameTiming(value bool) {
	carrySameTiming = value
}

var ProcessCarrySameTime = func(ignore, symbol string) {
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
		util.Notice(`info env not good, return`)
		return
	}
	if carrySameTiming {
		return
	}
	setCarrySameTiming(true)
	defer setCarrySameTiming(false)
	setting := model.GetSetting(model.FunctionCarry, model.Fmex, symbol)
	p1 := 0.0
	p2 := 0.0
	a1 := setting.AmountLimit
	a2 := setting.AmountLimit
	pb := accountBM.EntryPrice
	pf := accountFM.EntryPrice
	if accountFM.Free == 0 {
		pf = 0
	}
	if accountBM.Free == 0 {
		pb = 0
	}
	if accountFM.Free > setting.AmountLimit/10 && accountBM.Free < setting.AmountLimit/-10 {
		p1 = pb - pf - setting.PriceX - setting.GridPriceDistance
		p2 = setting.GridPriceDistance * accountBM.Free * 3 / setting.AmountLimit
		a1 = accountFM.Free
		a2 = setting.AmountLimit - accountFM.Free
	} else if accountFM.Free < setting.AmountLimit/-10 && accountBM.Free > setting.AmountLimit/10 {
		p1 = setting.GridPriceDistance * accountFM.Free * 3 / setting.AmountLimit
		p2 = pf - pb + setting.PriceX - setting.GridPriceDistance
		a1 = setting.AmountLimit - accountBM.Free
		a2 = accountBM.Free
	}
	priceDistance := 0.1 / math.Pow(10, api.GetPriceDecimal(model.Fmex, symbol))
	calcAmtPriceBuy := tickBM.Bids[0].Price + setting.GridPriceDistance - p1 - setting.PriceX
	calcAmtPriceSell := tickBM.Asks[0].Price - setting.GridPriceDistance + p2 - setting.PriceX
	fmba := getDepthAmountBuy(calcAmtPriceBuy, priceDistance, tickFM)
	fmsa := getDepthAmountSell(calcAmtPriceSell, priceDistance, tickFM)
	fmb1 := tickFM.Bids[0].Price + setting.PriceX
	fms1 := tickFM.Asks[0].Price + setting.PriceX
	util.Notice(fmt.Sprintf(`amt fm:%f amt bm:%f p1:%f p2:%f a1:%f a2:%f
			fmba:%f=%f->b0:%f fmsa:%f=a0:%f->%f
			BM价1:b0:%f a0:%f BM量1:b0:%f a0:%f
			FM价1:b0:%f a0:%f FM量1:b0:%f a0:%f`,
		accountFM.Free, accountBM.Free, p1, p2, a1, a2,
		fmba, calcAmtPriceBuy, tickFM.Bids[0].Price, fmsa, tickFM.Asks[0].Price, calcAmtPriceSell,
		tickBM.Bids[0].Price, tickBM.Asks[0].Price,
		tickBM.Bids[0].Amount, tickBM.Asks[0].Amount,
		tickFM.Bids[0].Price, tickFM.Asks[0].Price,
		tickFM.Bids[0].Amount, tickFM.Asks[0].Amount))
	if fmb1-tickBM.Bids[0].Price >= setting.GridPriceDistance-p1 && fmba >= setting.RefreshLimitLow &&
		tickBM.Bids[0].Amount*10 < tickBM.Asks[0].Amount {
		amount := math.Min(math.Min(fmba/2, a1), setting.GridAmount)
		placeBothOrders(model.OrderSideBuy, model.OrderSideSell, symbol,
			tickBM.Bids[0].Price, calcAmtPriceBuy, amount)
	} else if tickBM.Asks[0].Price-fms1 >= setting.GridPriceDistance-p2 && fmsa >= setting.RefreshLimitLow &&
		tickBM.Asks[0].Amount*10 < tickBM.Bids[0].Amount {
		amount := math.Min(math.Min(fmsa/2, a2), setting.GridAmount)
		placeBothOrders(model.OrderSideSell, model.OrderSideBuy, symbol,
			tickBM.Asks[0].Price, calcAmtPriceSell, amount)
	}
}

func placeBothOrders(orderSideBM, orderSideFM, symbol string, priceBM, priceFM, amount float64) {
	if amount > 1 {
		for i := 0; i < 10; i++ {
			orderBM := api.PlaceOrder(``, ``, orderSideBM, model.OrderTypeLimit, model.Bitmex, symbol,
				``, ``, priceBM, amount)
			if orderBM != nil && orderBM.OrderId != `` && orderBM.Status != model.CarryStatusFail {
				go model.AppDB.Save(&orderBM)
				util.Notice(fmt.Sprintf(`== bm order %s at %f amount %f return %s`,
					orderBM.OrderSide, orderBM.Price, orderBM.Amount, orderBM.OrderId))
				for j := 0; i < 10; j++ {
					orderFM := api.PlaceOrder(``, ``, orderSideFM, model.OrderTypeLimit, model.Fmex,
						symbol, ``, ``, priceFM, amount)
					if orderFM != nil && orderFM.OrderId != `` {
						api.RefreshAccount(``, ``, model.Fmex)
						go model.AppDB.Save(&orderFM)
						util.Notice(fmt.Sprintf(`== fm order %s at %f amount %f return %s`,
							orderFM.OrderSide, orderFM.Price, orderFM.Amount, orderFM.OrderId))
						break
					} else {
						util.Notice(fmt.Sprintf(`-- fm place order fail time: %d %s %f %f`,
							j, orderSideFM, priceFM, amount))
						if j == 9 {
							time.Sleep(time.Second * 10)
						}
					}
				}
				break
			} else {
				util.Notice(fmt.Sprintf(`-- bm post order fail time: %d %s %f %f`,
					i, orderSideBM, priceBM, amount))
				if i == 9 {
					time.Sleep(time.Second * 10)
				}
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
