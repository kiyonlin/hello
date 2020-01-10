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

const PostOnly = `ParticipateDoNotInitiate`

var carrySameTiming = false
var carrySameTimeLock sync.Mutex

var lastOrders = make(map[string]map[string]*model.Order)
var carryChannels = make(map[string]chan model.Order)

func setLastOrder(key, market string, order *model.Order) {
	defer carrySameTimeLock.Unlock()
	carrySameTimeLock.Lock()
	if lastOrders == nil {
		lastOrders = make(map[string]map[string]*model.Order)
	}
	if lastOrders[key] == nil {
		lastOrders[key] = make(map[string]*model.Order)
	}
	lastOrders[key][market] = order
}

func getLastOrder(key, market string) (order *model.Order) {
	defer carrySameTimeLock.Unlock()
	carrySameTimeLock.Lock()
	if lastOrders == nil {
		lastOrders = make(map[string]map[string]*model.Order)
	}
	if lastOrders[key] == nil {
		lastOrders[key] = make(map[string]*model.Order)
	}
	return lastOrders[key][market]
}

func getCarryChannel(key string) (carryChannel chan model.Order) {
	defer carrySameTimeLock.Unlock()
	carrySameTimeLock.Lock()
	if carryChannels == nil {
		carryChannels = make(map[string]chan model.Order)
	}
	if carryChannels[key] == nil {
		carryChannels[key] = make(chan model.Order, 0)
	}
	return carryChannels[key]
}

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
	if (int(startTime)-tick.Ts > 500 || int(startTime)-tickRelated.Ts > 10000) ||
		model.AppConfig.Handle != `1` || model.AppPause {
		//util.Info(fmt.Sprintf(`error4 now:%d related:%s tick_%s delta:%d tick_%s delta:%d`,
		//	startTime, setting.MarketRelated, market, int(startTime)-tick.Ts, setting.MarketRelated,
		//	int(startTime)-tickRelated.Ts))
		return
	}
	if carrySameTiming {
		return
	}
	setCarrySameTiming(true)
	defer setCarrySameTiming(false)
	key := fmt.Sprintf(`%s-%s-%s`, market, setting.MarketRelated, symbol)
	orderMarket := getLastOrder(key, market)
	orderRelated := getLastOrder(key, setting.MarketRelated)
	if (api.IsValid(orderMarket) && api.IsValid(orderRelated)) ||
		(!api.IsValid(orderMarket) && !api.IsValid(orderRelated)) {
		placeBothOrders(market, symbol, key, tick, tickRelated, accountRelated, setting)
	} else if !api.IsValid(orderMarket) {
		reOrder(key, market, orderMarket, tick, setting)
	} else if !api.IsValid(orderRelated) {
		reOrder(key, market, orderRelated, tick, setting)
	}
}

func reOrder(key, market string, lastOrder *model.Order, tick *model.BidAsk, setting *model.Setting) {
	if lastOrder.Amount-lastOrder.DealAmount < 1 {
		setLastOrder(key, market, nil)
		setLastOrder(key, setting.MarketRelated, nil)
		return
	}
	price := lastOrder.Price
	priceType := `保持价格`
	if lastOrder.RefreshType == PostOnly && lastOrder.OrderId != `` {
		price = tick.Asks[0].Price - api.GetPriceDistance(lastOrder.Market, lastOrder.Symbol)
		if lastOrder.OrderSide == model.OrderSideSell {
			price = tick.Bids[0].Price + api.GetPriceDistance(lastOrder.Market, lastOrder.Symbol)
		}
		priceType = `买卖1价格`
	}
	util.Notice(fmt.Sprintf(`---- reorder: %s order %s %s %s %s %f %f orderParam:<%s> %s`,
		lastOrder.Market, lastOrder.OrderSide, lastOrder.OrderType, lastOrder.Market, lastOrder.Symbol,
		price, lastOrder.Amount-lastOrder.DealAmount, lastOrder.RefreshType, priceType))
	lastOrder = api.PlaceOrder(``, ``, lastOrder.OrderSide, lastOrder.OrderType, lastOrder.Market,
		lastOrder.Symbol, ``, setting.AccountType, lastOrder.RefreshType,
		price, lastOrder.Amount-lastOrder.DealAmount, true)
	setLastOrder(key, lastOrder.Market, lastOrder)
}

// account.free被设置成-1 * accountRelated.free
func placeBothOrders(market, symbol, key string, tick, tickRelated *model.BidAsk, accountRelated *model.Account,
	setting *model.Setting) {
	p1 := 0.0
	p2 := 0.0
	a1 := setting.AmountLimit
	a2 := setting.AmountLimit
	zFee, expired := api.GetFundingRate(market, symbol)
	zFeeRelated, expiredRelated := api.GetFundingRate(setting.MarketRelated, symbol)
	if setting.MarketRelated == model.Bybit {
		if expired > expiredRelated {
			zFee = 0
		} else {
			zFeeRelated = 0
		}
	}
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
		fmt.Sprintf("[搬砖参数] %s %s资金费率:%f %s资金费率%f p1:%f p2:%f py:%f px:%f related free:%f %s \n",
			util.GetNow().String(), market, zFee, setting.MarketRelated, zFeeRelated,
			p1, p2, py, priceX, accountRelated.Free, util.GetNow().String()))
	priceDistance := 0.1 / math.Pow(10, api.GetPriceDecimal(setting.MarketRelated, symbol))
	calcAmtPriceBuy := tick.Asks[0].Price - api.GetPriceDistance(market, symbol) +
		setting.GridPriceDistance - p1 - priceX
	calcAmtPriceSell := tick.Bids[0].Price + api.GetPriceDistance(market, symbol) -
		setting.GridPriceDistance + p2 - priceX
	fmba := getDepthAmountBuy(calcAmtPriceBuy, 0, tickRelated)
	fmsa := getDepthAmountSell(calcAmtPriceSell, 0, tickRelated)
	calcAmtPriceBuyNew := tick.Asks[0].Price*1002/1000 + setting.GridPriceDistance - p1 - priceX
	calcAmtPriceSellNew := tick.Bids[0].Price*998/1000 - setting.GridPriceDistance + p2 - priceX
	fmbaNew := getDepthAmountBuy(calcAmtPriceBuyNew, 0, tickRelated)
	fmsaNew := getDepthAmountSell(calcAmtPriceSellNew, 0, tickRelated)
	fmb1 := tickRelated.Bids[0].Price + priceX
	fms1 := tickRelated.Asks[0].Price + priceX
	amount := 0.0
	orderSide := ``
	orderSideRelated := ``
	orderPrice := 0.0
	orderPriceRelated := 0.0
	carryType := 0
	orderParam := ``
	if fmb1+priceDistance >= calcAmtPriceBuyNew+priceX && fmbaNew >= setting.RefreshLimitLow {
		amount = math.Min(math.Min(0.8*fmbaNew, a1), setting.GridAmount)
		orderSideRelated = model.OrderSideSell
		orderSide = model.OrderSideBuy
		orderPriceRelated = calcAmtPriceBuyNew
		orderPrice = tick.Asks[0].Price * 1.001
		carryType = 1
	} else if fmb1+priceDistance >= calcAmtPriceBuy+priceX && fmba >= setting.RefreshLimitLow &&
		tick.Bids[0].Amount*9 < tick.Asks[0].Amount && tick.Asks[0].Amount > 900000 {
		amount = math.Min(math.Min(fmba*0.8, a1), setting.GridAmount)
		orderSideRelated = model.OrderSideSell
		orderPriceRelated = calcAmtPriceBuy
		orderSide = model.OrderSideBuy
		orderPrice = tick.Bids[0].Price
		orderParam = PostOnly
		carryType = 2
	} else if fms1-priceDistance <= calcAmtPriceSellNew+priceX && fmsaNew >= setting.RefreshLimitLow {
		amount = math.Min(math.Min(0.8*fmsaNew, a2), setting.GridAmount)
		orderSideRelated = model.OrderSideBuy
		orderSide = model.OrderSideSell
		orderPrice = tick.Bids[0].Price * 0.999
		orderPriceRelated = calcAmtPriceSellNew
		carryType = 3
	} else if fms1-priceDistance <= calcAmtPriceSell+priceX && fmsa >= setting.RefreshLimitLow &&
		tick.Asks[0].Amount*9 < tick.Bids[0].Amount && tick.Bids[0].Amount > 900000 {
		amount = math.Min(math.Min(fmsa*0.8, a2), setting.GridAmount)
		orderSideRelated = model.OrderSideBuy
		orderSide = model.OrderSideSell
		orderPriceRelated = calcAmtPriceSell
		orderParam = PostOnly
		orderPrice = tick.Asks[0].Price
		carryType = 4
	}
	if amount > 1 {
		util.Notice(fmt.Sprintf(`情况%d %s-%s, 资金费率: %f-%f priceX:%f fmb1:%f fms1:%f fmba:%f fmsa:%f fmbaNew:%f
			fmsaNew:%f tick price:%f-%f tick amount:%f-%f tickRelatedPrice:%f-%f tickRelatedAmount:%f-%f p1:%f p2:%f 持仓:%f`,
			carryType, market, setting.MarketRelated, zFee, zFeeRelated, priceX, fmb1, fms1, fmba, fmsa, fmbaNew,
			fmsaNew, tick.Asks[0].Price, tick.Bids[0].Price, tick.Asks[0].Amount, tick.Bids[0].Amount,
			tickRelated.Asks[0].Price, tickRelated.Bids[0].Price, tickRelated.Asks[0].Amount, tickRelated.Bids[0].Amount,
			p1, p2, accountRelated.Free))
		util.Notice(fmt.Sprintf(`------------ %d-%d %f %f %f %f %f - %f %f %f %f %f`,
			tick.Bids.Len(), tick.Asks.Len(), tick.Bids[4].Price, tick.Bids[3].Price, tick.Bids[2].Price,
			tick.Bids[1].Price, tick.Bids[0].Price, tick.Asks[0].Price, tick.Asks[1].Price, tick.Asks[2].Price,
			tick.Asks[3].Price, tick.Asks[4].Price))
		setLastOrder(key, market, nil)
		setLastOrder(key, setting.MarketRelated, nil)
		carryChannel := getCarryChannel(key)
		go api.PlaceSyncOrders(``, ``, orderSideRelated, model.OrderTypeLimit, setting.MarketRelated, symbol,
			``, setting.AccountType, ``, orderPriceRelated, amount, true, carryChannel, 10)
		go api.PlaceSyncOrders(``, ``, orderSide, model.OrderTypeLimit, market, symbol, ``,
			setting.AccountType, orderParam, orderPrice, amount, true, carryChannel, 10)
		for true {
			order := <-carryChannel
			util.Notice(fmt.Sprintf(`---- get order %s %s %s`, order.Market, order.OrderId, order.Status))
			setLastOrder(key, order.Market, &order)
			if getLastOrder(key, market) != nil && getLastOrder(key, setting.MarketRelated) != nil {
				util.Notice(`---- get both, break`)
				break
			}
		}
		orderMarket := getLastOrder(key, market)
		orderRelated := getLastOrder(key, setting.MarketRelated)
		if api.IsValid(orderMarket) && api.IsValid(orderRelated) {
			if setting.MarketRelated != model.Bybit {
				time.Sleep(time.Millisecond * 500)
				api.RefreshAccount(``, ``, setting.MarketRelated)
			} else {
				if orderRelated.OrderSide == model.OrderSideSell {
					accountRelated.Free -= orderRelated.Amount
				} else {
					accountRelated.Free += orderRelated.Amount
				}
				model.AppAccounts.SetAccount(setting.MarketRelated, symbol, accountRelated)
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
