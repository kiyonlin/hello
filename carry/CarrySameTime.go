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
var bmLastOrder = &model.Order{}

func setCarrySameTiming(value bool) {
	carrySameTiming = value
}

var ProcessCarrySameTime = func(ignore, symbol string) {
	startTime := util.GetNowUnixMillion()
	_, tickBM := model.AppMarkets.GetBidAsk(symbol, model.Bitmex)
	_, tickFM := model.AppMarkets.GetBidAsk(symbol, model.Fmex)
	accountBM := model.AppAccounts.GetAccount(model.Bitmex, symbol)
	accountFM := model.AppAccounts.GetAccount(model.Fmex, symbol)
	if accountFM == nil || accountBM == nil {
		api.RefreshAccount(``, ``, model.Fmex)
		util.Notice(`error1 account is nil, refresh and return`)
		return
	}
	if tickFM == nil || tickBM == nil || tickFM.Asks == nil || tickFM.Bids == nil || tickBM.Asks == nil ||
		tickBM.Bids == nil {
		util.Notice(`error2 fm/bm tick or account is nil`)
		return
	}
	if tickFM.Asks.Len() < 15 || tickFM.Bids.Len() < 15 || tickBM.Asks.Len() < 1 ||
		tickBM.Bids.Len() < 1 {
		util.Notice(`error3 fm/bm depth tick length not good`)
		return
	}
	if carrySameTiming {
		return
	}
	setCarrySameTiming(true)
	defer setCarrySameTiming(false)
	setting := model.GetSetting(model.FunctionCarry, model.Bitmex, symbol)
	if bmLastOrder != nil && bmLastOrder.Status == model.CarryStatusFail {
		reOrder(tickBM, setting)
		return
	}
	if int(startTime)-tickBM.Ts > 1000 || int(startTime)-tickFM.Ts > 1000 || model.AppConfig.Handle != `1` ||
		model.AppPause || startTime-accountBM.Ts > 60000 {
		util.Notice(fmt.Sprintf(`error4 now:%d tickBM delta:%d tickFM delta:%d accountBM delta:%d`,
			startTime, int(startTime)-tickBM.Ts, int(startTime)-tickFM.Ts, startTime-accountBM.Ts))
		return
	}
	placeBothOrders(symbol, tickBM, tickFM, accountBM, accountFM, setting)
}

func reOrder(tickBM *model.BidAsk, setting *model.Setting) {
	if bmLastOrder.Amount-bmLastOrder.DealAmount < 1 {
		bmLastOrder = nil
	}
	price := tickBM.Bids[0].Price
	if bmLastOrder.OrderSide == model.OrderSideSell {
		price = tickBM.Asks[0].Price
	}
	util.Notice(fmt.Sprintf(`complement last bm order %s %s %s %s %f %f`,
		bmLastOrder.OrderSide, bmLastOrder.OrderType, bmLastOrder.Market,
		bmLastOrder.Symbol, price, bmLastOrder.Amount-bmLastOrder.DealAmount))
	bmLastOrder = api.PlaceOrder(``, ``, bmLastOrder.OrderSide, bmLastOrder.OrderType, bmLastOrder.Market,
		bmLastOrder.Symbol, ``, setting.AccountType, price, bmLastOrder.Amount-bmLastOrder.DealAmount, true)
}

func placeBothOrders(symbol string, tickBM, tickFM *model.BidAsk, accountBM, accountFM *model.Account,
	setting *model.Setting) {
	p1 := 0.0
	p2 := 0.0
	a1 := setting.AmountLimit
	a2 := setting.AmountLimit
	zb := api.GetFundingRate(model.Bitmex, symbol)
	zf := api.GetFundingRate(model.Fmex, symbol)
	priceX := setting.PriceX + (zf-zb)*(tickFM.Bids[0].Price+tickFM.Asks[0].Price)/2
	py := priceX
	if accountFM.Free > setting.AmountLimit/10 && accountBM.Free < setting.AmountLimit/-10 {
		p1 = 0
		p2 = setting.GridPriceDistance * accountBM.Free / setting.AmountLimit
		a1 = accountFM.Free
		a2 = setting.AmountLimit - accountFM.Free
		priceX -= 3 * p2
	} else if accountFM.Free < setting.AmountLimit/-10 && accountBM.Free > setting.AmountLimit/10 {
		p1 = setting.GridPriceDistance * accountFM.Free / setting.AmountLimit
		p2 = 0
		a1 = setting.AmountLimit - accountBM.Free
		a2 = accountBM.Free
		priceX += 3 * p1
	}
	model.CarryInfo = fmt.Sprintf("[搬砖参数] zb:%f zf:%f p1:%f p2:%f px:%f py:%f abm:%f afm:%f\n",
		zb, zf, p1, p2, py, priceX, accountBM.Free, accountFM.Free)
	priceDistance := 0.1 / math.Pow(10, api.GetPriceDecimal(model.Fmex, symbol))
	calcAmtPriceBuy := tickBM.Bids[0].Price + setting.GridPriceDistance - p1 - priceX
	calcAmtPriceSell := tickBM.Asks[0].Price - setting.GridPriceDistance + p2 - priceX
	fmba := getDepthAmountBuy(calcAmtPriceBuy, priceDistance, tickFM)
	fmsa := getDepthAmountSell(calcAmtPriceSell, priceDistance, tickFM)
	fmb1 := tickFM.Bids[0].Price + priceX
	fms1 := tickFM.Asks[0].Price + priceX
	//util.Info(fmt.Sprintf(`amt fm:%f amt bm:%f p1:%f p2:%f a1:%f a2:%f
	//		fmba:%f=%f->b0:%f fmsa:%f=a0:%f->%f
	//		BM价1:b0:%f a0:%f BM量1:b0:%f a0:%f
	//		FM价1:b0:%f a0:%f FM量1:b0:%f a0:%f`,
	//	accountFM.Free, accountBM.Free, p1, p2, a1, a2,
	//	fmba, calcAmtPriceBuy, tickFM.Bids[0].Price, fmsa, tickFM.Asks[0].Price, calcAmtPriceSell,
	//	tickBM.Bids[0].Price, tickBM.Asks[0].Price,
	//	tickBM.Bids[0].Amount, tickBM.Asks[0].Amount,
	//	tickFM.Bids[0].Price, tickFM.Asks[0].Price,
	//	tickFM.Bids[0].Amount, tickFM.Asks[0].Amount))
	if fmb1-tickBM.Bids[0].Price >= setting.GridPriceDistance-p1 && fmba >= setting.RefreshLimitLow &&
		tickBM.Bids[0].Amount*10 < tickBM.Asks[0].Amount && tickBM.Asks[0].Amount > 700000 {
		amount := math.Min(math.Min(fmba*0.5, a1), setting.GridAmount)
		if amount > 1 {
			go api.PlaceOrder(``, ``, model.OrderSideSell, model.OrderTypeLimit, model.Fmex,
				symbol, ``, setting.AccountType, calcAmtPriceBuy, amount, true)
			bmLastOrder = api.PlaceOrder(``, ``, model.OrderSideBuy, model.OrderTypeLimit, model.Bitmex,
				symbol, ``, setting.AccountType, tickBM.Bids[0].Price, amount, true)
			if bmLastOrder != nil && bmLastOrder.OrderId != `` && bmLastOrder.Status != model.CarryStatusFail {
				time.Sleep(time.Millisecond * 500)
				api.RefreshAccount(``, ``, model.Fmex)
			}
			util.Notice(fmt.Sprintf(`bm order %s at %f amount %f return %s zb %f zf %f px:%f`,
				bmLastOrder.OrderSide, bmLastOrder.Price, bmLastOrder.Amount, bmLastOrder.OrderId, zb, zf, priceX))
		}
	} else if tickBM.Asks[0].Price-fms1 >= setting.GridPriceDistance-p2 && fmsa >= setting.RefreshLimitLow &&
		tickBM.Asks[0].Amount*10 < tickBM.Bids[0].Amount && tickBM.Bids[0].Amount > 700000 {
		amount := math.Min(math.Min(fmsa*0.5, a2), setting.GridAmount)
		if amount > 1 {
			go api.PlaceOrder(``, ``, model.OrderSideBuy, model.OrderTypeLimit, model.Fmex,
				symbol, ``, setting.AccountType, calcAmtPriceSell, amount, true)
			bmLastOrder = api.PlaceOrder(``, ``, model.OrderSideSell, model.OrderTypeLimit, model.Bitmex,
				symbol, ``, setting.AccountType, tickBM.Asks[0].Price, amount, true)
			if bmLastOrder != nil && bmLastOrder.OrderId != `` && bmLastOrder.Status != model.CarryStatusFail {
				time.Sleep(time.Millisecond * 500)
				api.RefreshAccount(``, ``, model.Fmex)
			}
			util.Notice(fmt.Sprintf(`bm order %s at %f amount %f return %s zb %f zf %f px:%f`,
				bmLastOrder.OrderSide, bmLastOrder.Price, bmLastOrder.Amount, bmLastOrder.OrderId, zb, zf, priceX))
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
