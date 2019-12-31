package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"time"
)

const PostOnly = `ParticipateDoNotInitiate`

var carrySameTiming = false
var lastOrder = &model.Order{}

func setCarrySameTiming(value bool) {
	carrySameTiming = value
}

var ProcessCarrySameTime = func(market, symbol string) {
	startTime := util.GetNowUnixMillion()
	setting := model.GetSetting(model.FunctionCarry, market, symbol)
	if setting == nil || setting.MarketRelated == `` {
		return
	}
	_, tick := model.AppMarkets.GetBidAsk(symbol, market)
	_, tickRelated := model.AppMarkets.GetBidAsk(symbol, setting.MarketRelated)
	//account := model.AppAccounts.GetAccount(market, symbol)
	accountRelated := model.AppAccounts.GetAccount(setting.MarketRelated, symbol)
	util.Info(fmt.Sprintf(`account %s %s %f`, setting.MarketRelated, accountRelated.Currency, accountRelated.Free))
	if accountRelated == nil {
		api.RefreshAccount(``, ``, setting.MarketRelated)
		util.Info(`error1 account is nil, refresh and return`)
		return
	}
	if tickRelated == nil || tick == nil || tickRelated.Asks == nil || tickRelated.Bids == nil || tick.Asks == nil ||
		tick.Bids == nil {
		util.Info(fmt.Sprintf(`error2 %s or %s tick or account is nil`, market, setting.MarketRelated))
		return
	}
	if tickRelated.Asks.Len() < 10 || tickRelated.Bids.Len() < 10 || tick.Asks.Len() < 10 ||
		tick.Bids.Len() < 10 {
		util.Info(fmt.Sprintf(`error3 %s or %s depth tick length not good`, market, setting.MarketRelated))
		return
	}
	if carrySameTiming {
		return
	}
	setCarrySameTiming(true)
	defer setCarrySameTiming(false)
	if lastOrder != nil && lastOrder.Status == model.CarryStatusFail {
		reOrder(tick, setting)
		return
	}
	if (int(startTime)-tick.Ts > 500 || int(startTime)-tickRelated.Ts > 500) ||
		model.AppConfig.Handle != `1` || model.AppPause {
		//util.Info(fmt.Sprintf(`error4 now:%d related:%s tick_%s delta:%d tick_%s delta:%d`,
		//	startTime, setting.MarketRelated, market, int(startTime)-tick.Ts, setting.MarketRelated,
		//	int(startTime)-tickRelated.Ts))
		return
	}
	placeBothOrders(market, symbol, tick, tickRelated, accountRelated, setting)
}

func reOrder(tick *model.BidAsk, setting *model.Setting) {
	if lastOrder.Amount-lastOrder.DealAmount < 1 {
		lastOrder = nil
	}
	price := lastOrder.Price
	priceType := `保持价格`
	if lastOrder.RefreshType == PostOnly && lastOrder.OrderId != `` {
		price = tick.Bids[0].Price
		if lastOrder.OrderSide == model.OrderSideSell {
			price = tick.Asks[0].Price
		}
		priceType = `买卖1价格`
	}
	refreshType := lastOrder.RefreshType
	util.Notice(fmt.Sprintf(`complement last %s order %s %s %s %s %f %f orderParam:<%s> %s`,
		lastOrder.Market, lastOrder.OrderSide, lastOrder.OrderType, lastOrder.Market, lastOrder.Symbol,
		price, lastOrder.Amount-lastOrder.DealAmount, lastOrder.RefreshType, priceType))
	lastOrder = api.PlaceOrder(``, ``, lastOrder.OrderSide, lastOrder.OrderType, lastOrder.Market,
		lastOrder.Symbol, ``, setting.AccountType, refreshType,
		price, lastOrder.Amount-lastOrder.DealAmount, true)
	lastOrder.RefreshType = refreshType
}

// account.free被设置成-1 * accountRelated.free
func placeBothOrders(market, symbol string, tick, tickRelated *model.BidAsk, accountRelated *model.Account,
	setting *model.Setting) {
	p1 := 0.0
	p2 := 0.0
	a1 := setting.AmountLimit
	a2 := setting.AmountLimit
	zFee := api.GetFundingRate(market, symbol)
	zFeeRelated := api.GetFundingRate(setting.MarketRelated, symbol)
	priceX := setting.PriceX + 1.2*(zFeeRelated-zFee)*(tickRelated.Bids[0].Price+tickRelated.Asks[0].Price)/2
	py := priceX
	if accountRelated.Free > setting.AmountLimit/10 && -1*accountRelated.Free < setting.AmountLimit/-10 {
		p1 = 0
		p2 = -1 * accountRelated.Free / setting.AmountLimit
		a1 = accountRelated.Free
		a2 = setting.AmountLimit - accountRelated.Free
		priceX -= 4 * p2
	} else if accountRelated.Free < setting.AmountLimit/-10 &&
		-1*accountRelated.Free > setting.AmountLimit/10 {
		p1 = accountRelated.Free / setting.AmountLimit
		p2 = 0
		a1 = setting.AmountLimit + accountRelated.Free
		a2 = -1 * accountRelated.Free
		priceX += 4 * p1
	}
	model.SetCarryInfo(fmt.Sprintf(`%s_%s_%s`, model.FunctionCarry, market, setting.MarketRelated),
		fmt.Sprintf("[搬砖参数] %s money price:%f %s:%f p1:%f p2:%f py:%f px:%f ask:%f askRelated:%f\n",
			market, zFee, setting.MarketRelated, zFeeRelated,
			p1, p2, py, priceX, -1*accountRelated.Free, accountRelated.Free))
	priceDistance := 0.1 / math.Pow(10, api.GetPriceDecimal(setting.MarketRelated, symbol))
	calcAmtPriceBuy := tick.Bids[0].Price + setting.GridPriceDistance - p1 - priceX
	calcAmtPriceSell := tick.Asks[0].Price - setting.GridPriceDistance + p2 - priceX
	fmba := getDepthAmountBuy(calcAmtPriceBuy, 0, tickRelated)
	fmsa := getDepthAmountSell(calcAmtPriceSell, 0, tickRelated)
	calcAmtPriceBuyNew := tick.Asks[0].Price*1002/1000 + setting.GridPriceDistance - p1 - priceX
	calcAmtPriceSellNew := tick.Bids[0].Price*998/1000 - setting.GridPriceDistance + p2 - priceX
	fmbaNew := getDepthAmountBuy(calcAmtPriceBuyNew, 0, tickRelated)
	fmsaNew := getDepthAmountSell(calcAmtPriceSellNew, 0, tickRelated)
	fmb1 := tickRelated.Bids[0].Price + priceX
	fms1 := tickRelated.Asks[0].Price + priceX
	if fmb1+priceDistance >= calcAmtPriceBuyNew+priceX && fmbaNew >= setting.RefreshLimitLow {
		amount := math.Min(math.Min(0.8*fmbaNew, a1), setting.GridAmount)
		if amount > 1 {
			go api.PlaceOrder(``, ``, model.OrderSideSell, model.OrderTypeLimit, setting.MarketRelated,
				symbol, ``, setting.AccountType, ``, calcAmtPriceBuyNew, amount, true)
			lastOrder = api.PlaceOrder(``, ``, model.OrderSideBuy, model.OrderTypeLimit, market,
				symbol, ``, setting.AccountType, ``, tick.Asks[0].Price*1.001, amount, true)
			lastOrder.RefreshType = ``
			if lastOrder != nil && lastOrder.OrderId != `` && lastOrder.Status != model.CarryStatusFail {
				time.Sleep(time.Millisecond * 500)
				api.RefreshAccount(``, ``, setting.MarketRelated)
			}
			util.Notice(fmt.Sprintf(`情况1 %f amount %f return %s %s money price: %f
				%s:%f px:%f orderParam:%s relatedB1:%f Ask0:%f p1:%f relatedBa:%f`,
				lastOrder.Price, lastOrder.Amount, lastOrder.OrderId, market, zFee,
				setting.MarketRelated, zFeeRelated, priceX, lastOrder.RefreshType, fmb1,
				tick.Asks[0].Price, p1, fmbaNew))
		}
	} else if fmb1+priceDistance >= calcAmtPriceBuy+priceX && fmba >= setting.RefreshLimitLow &&
		tick.Bids[0].Amount*7 < tick.Asks[0].Amount && tick.Asks[0].Amount > 700000 {
		amount := math.Min(math.Min(fmba*0.8, a1), setting.GridAmount)
		if amount > 1 {
			go api.PlaceOrder(``, ``, model.OrderSideSell, model.OrderTypeLimit, setting.MarketRelated,
				symbol, ``, setting.AccountType, ``, calcAmtPriceBuy, amount, true)
			lastOrder = api.PlaceOrder(``, ``, model.OrderSideBuy, model.OrderTypeLimit, market,
				symbol, ``, setting.AccountType, PostOnly,
				tick.Bids[0].Price, amount, true)
			lastOrder.RefreshType = PostOnly
			if lastOrder != nil && lastOrder.OrderId != `` && lastOrder.Status != model.CarryStatusFail {
				time.Sleep(time.Millisecond * 500)
				api.RefreshAccount(``, ``, setting.MarketRelated)
			}
			util.Notice(fmt.Sprintf(`情况 2%f amount %f return %s %s money fee %f %s:%f px:%f orderParam:%s
				relatedB1:%f Bid0:%f p1:%f relatedBa:%f B0Amt:%f A0Amt:%f`,
				lastOrder.Price, lastOrder.Amount, lastOrder.OrderId, market, zFee,
				setting.MarketRelated, zFeeRelated, priceX, lastOrder.RefreshType,
				fmb1, tick.Bids[0].Price, p1, fmba, tick.Bids[0].Amount, tick.Asks[0].Amount))
		}
	} else if fms1-priceDistance <= calcAmtPriceSellNew+priceX && fmsaNew >= setting.RefreshLimitLow {
		amount := math.Min(math.Min(0.8*fmsaNew, a2), setting.GridAmount)
		if amount > 0 {
			api.PlaceOrder(``, ``, model.OrderSideBuy, model.OrderTypeLimit, setting.MarketRelated,
				symbol, ``, setting.AccountType, ``, calcAmtPriceSellNew, amount, true)
			lastOrder = api.PlaceOrder(``, ``, model.OrderSideSell, model.OrderTypeLimit, market,
				symbol, ``, setting.AccountType, ``, tick.Bids[0].Price*0.999, amount, true)
			lastOrder.RefreshType = ``
			if lastOrder != nil && lastOrder.OrderId != `` && lastOrder.Status != model.CarryStatusFail {
				time.Sleep(time.Millisecond * 500)
				api.RefreshAccount(``, ``, setting.MarketRelated)
			}
			util.Notice(fmt.Sprintf(`情况 3 %f amount %f return %s %s money price: %f %s:%f px:%f orderParam:%s
				relatedS1:%f, b0:%f p2:%f`, lastOrder.Price, lastOrder.Amount, lastOrder.OrderId, market,
				zFee, setting.MarketRelated, zFeeRelated, priceX, lastOrder.RefreshType, fms1,
				tick.Bids[0].Price, p2))
		}
	} else if fms1-priceDistance <= calcAmtPriceSell+priceX && fmsa >= setting.RefreshLimitLow &&
		tick.Asks[0].Amount*7 < tick.Bids[0].Amount && tick.Bids[0].Amount > 700000 {
		amount := math.Min(math.Min(fmsa*0.8, a2), setting.GridAmount)
		if amount > 1 {
			go api.PlaceOrder(``, ``, model.OrderSideBuy, model.OrderTypeLimit, setting.MarketRelated,
				symbol, ``, setting.AccountType, ``, calcAmtPriceSell, amount, true)
			lastOrder = api.PlaceOrder(``, ``, model.OrderSideSell, model.OrderTypeLimit, market,
				symbol, ``, setting.AccountType, PostOnly,
				tick.Asks[0].Price, amount, true)
			lastOrder.RefreshType = PostOnly
			if lastOrder != nil && lastOrder.OrderId != `` && lastOrder.Status != model.CarryStatusFail {
				time.Sleep(time.Millisecond * 500)
				api.RefreshAccount(``, ``, setting.MarketRelated)
			}
			util.Notice(fmt.Sprintf(`情况4 %f amount %f return %s %s money price %f %s:%f px:%f 
				orderParam:%s Ask0:%f relatedS1:%f p2:%f relatedSa:%f B0Amt:%f A0Amt:%f`,
				lastOrder.Price, lastOrder.Amount, lastOrder.OrderId, market, zFee,
				setting.MarketRelated, zFeeRelated, priceX, lastOrder.RefreshType,
				tick.Asks[0].Price, fms1, p2, fmsa, tick.Bids[0].Amount, tick.Asks[0].Amount))
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
