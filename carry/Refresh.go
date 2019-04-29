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

// coinpark://4003 调用次数繁忙 //2085 最小下单数量限制 //2027 可用余额不足
var bidAskTimes int64
var refreshing = false

//var btcusdtBigTime *time.Time
var syncRefresh = make(chan interface{}, 10)
var LastRefreshTime = make(map[string]int64) // market - int64
var refreshOrders = &RefreshOrders{}
var lastOrign1016 = false

//var lastTickBid, lastTickAsk *model.Tick
var canceling = false

type RefreshOrders struct {
	lock             sync.Mutex
	bidOrders        map[string]map[string][]*model.Order             // market - symbol - orders
	askOrders        map[string]map[string][]*model.Order             // market - symbol - orders
	lastBid          map[string]map[string]*model.Order               // market - symbol - order
	lastAsk          map[string]map[string]*model.Order               // market - symbol - order
	recentOrders     map[string]map[string]map[float64][]*model.Order // market - symbol - price - orders
	amountLimit      map[string]map[string]map[int]float64            // market - symbol - time start point - amount
	lastRefreshPrice map[string]map[string]float64                    // market - symbol - price
}

func (refreshOrders *RefreshOrders) SetLastRefreshPrice(market, symbol string, price float64) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if refreshOrders.lastRefreshPrice == nil {
		refreshOrders.lastRefreshPrice = make(map[string]map[string]float64)
	}
	if refreshOrders.lastRefreshPrice[market] == nil {
		refreshOrders.lastRefreshPrice[market] = make(map[string]float64)
	}
	refreshOrders.lastRefreshPrice[market][symbol] = price
}

func (refreshOrders *RefreshOrders) CheckLastRefreshPrice(market, symbol string, price, priceDistance float64) (same bool) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if refreshOrders.lastRefreshPrice == nil || refreshOrders.lastRefreshPrice[market] == nil {
		return false
	}
	return math.Abs(refreshOrders.lastRefreshPrice[market][symbol]-price) < priceDistance
}

func (refreshOrders *RefreshOrders) CheckAmountLimit(market, symbol string, amountLimit float64) (underLimit bool) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if refreshOrders.amountLimit == nil || refreshOrders.amountLimit[market] == nil ||
		refreshOrders.amountLimit[market][symbol] == nil {
		return true
	}
	now := util.GetNow()
	slotNum := int((now.Hour()*3600 + now.Minute()*60 + now.Second()) / model.RefreshTimeSlot)
	refreshOrders.amountLimit[market][symbol][slotNum+1] = 0
	refreshOrders.amountLimit[market][symbol][slotNum-1] = 0
	if refreshOrders.amountLimit[market][symbol][slotNum] < amountLimit {
		return true
	}
	util.Notice(fmt.Sprintf(`[limit full]%s %s %d %f`, market, symbol, slotNum, refreshOrders.amountLimit[market][symbol][slotNum]))
	return false
}

func (refreshOrders *RefreshOrders) AddRefreshAmount(market, symbol string, amountInUsdt float64) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if refreshOrders.amountLimit == nil {
		refreshOrders.amountLimit = make(map[string]map[string]map[int]float64)
	}
	if refreshOrders.amountLimit[market] == nil {
		refreshOrders.amountLimit[market] = make(map[string]map[int]float64)
	}
	if refreshOrders.amountLimit[market][symbol] == nil {
		refreshOrders.amountLimit[market][symbol] = make(map[int]float64)
	}
	now := util.GetNow()
	slotNum := int((now.Hour()*3600 + now.Minute()*60 + now.Second()) / model.RefreshTimeSlot)
	refreshOrders.amountLimit[market][symbol][slotNum] += amountInUsdt
	util.Notice(fmt.Sprintf(`[+limit amount]%s %s %d %f`, market, symbol, slotNum, amountInUsdt))
}

func (refreshOrders *RefreshOrders) AddRecentOrder(market, symbol string, order *model.Order) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if order.OrderId == `` || market != model.Fcoin || symbol != `btc_usdt` {
		return
	}
	if refreshOrders.recentOrders == nil {
		refreshOrders.recentOrders = make(map[string]map[string]map[float64][]*model.Order)
	}
	if refreshOrders.recentOrders[market] == nil {
		refreshOrders.recentOrders[market] = make(map[string]map[float64][]*model.Order)
	}
	if refreshOrders.recentOrders[market][symbol] == nil {
		refreshOrders.recentOrders[market][symbol] = make(map[float64][]*model.Order)
	}
	if refreshOrders.recentOrders[market][symbol][order.Price] == nil {
		refreshOrders.recentOrders[market][symbol][order.Price] = make([]*model.Order, 0)
	}
	refreshOrders.recentOrders[market][symbol][order.Price] =
		append(refreshOrders.recentOrders[market][symbol][order.Price], order)
}

func (refreshOrders *RefreshOrders) CheckRecentOrder(market, symbol string, price float64) bool {
	if market != model.Fcoin || symbol != `btc_usdt` {
		return true
	}
	if refreshOrders.recentOrders == nil || refreshOrders.recentOrders[market] == nil ||
		refreshOrders.recentOrders[market][symbol] == nil {
		return true
	}
	price, _ = util.FormatNum(price, api.GetPriceDecimal(market, symbol))
	array := make([]*model.Order, 0)
	now := util.GetNow()
	for _, value := range refreshOrders.recentOrders[market][symbol][price] {
		if now.Unix()-value.OrderTime.Unix() < 10 {
			array = append(array, value)
		}
	}
	refreshOrders.recentOrders[market][symbol][price] = array
	//return len(array) <= 3
	return true
}

func (refreshOrders *RefreshOrders) SetLastOrder(market, symbol, orderSide string, order *model.Order) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if orderSide == model.OrderSideSell {
		if refreshOrders.lastAsk == nil {
			refreshOrders.lastAsk = make(map[string]map[string]*model.Order)
			if refreshOrders.lastAsk[market] == nil {
				refreshOrders.lastAsk[market] = make(map[string]*model.Order)
			}
		}
		refreshOrders.lastAsk[market][symbol] = order
	}
	if orderSide == model.OrderSideBuy {
		if refreshOrders.lastBid == nil {
			refreshOrders.lastBid = make(map[string]map[string]*model.Order)
			if refreshOrders.lastBid[market] == nil {
				refreshOrders.lastBid[market] = make(map[string]*model.Order)
			}
		}
		refreshOrders.lastBid[market][symbol] = order
	}
}

func (refreshOrders *RefreshOrders) GetLastOrder(market, symbol, orderSide string) (lastOrder *model.Order) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if orderSide == model.OrderSideSell {
		if refreshOrders.lastAsk == nil {
			refreshOrders.lastAsk = make(map[string]map[string]*model.Order)
			if refreshOrders.lastAsk[market] == nil {
				refreshOrders.lastAsk[market] = make(map[string]*model.Order)
			}
		}
		return refreshOrders.lastAsk[market][symbol]
	}
	if orderSide == model.OrderSideBuy {
		if refreshOrders.lastBid == nil {
			refreshOrders.lastBid = make(map[string]map[string]*model.Order)
			if refreshOrders.lastBid[market] == nil {
				refreshOrders.lastBid[market] = make(map[string]*model.Order)
			}
		}
		return refreshOrders.lastBid[market][symbol]
	}
	return nil
}

func (refreshOrders *RefreshOrders) Add(market, symbol, orderSide string, order *model.Order) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if orderSide == model.OrderSideBuy {
		if refreshOrders.bidOrders == nil {
			refreshOrders.bidOrders = make(map[string]map[string][]*model.Order)
		}
		if refreshOrders.bidOrders[market] == nil {
			refreshOrders.bidOrders[market] = make(map[string][]*model.Order)
		}
		if refreshOrders.bidOrders[market][symbol] == nil {
			refreshOrders.bidOrders[market][symbol] = make([]*model.Order, 0)
		}
		priceEqual := false
		for key, value := range refreshOrders.bidOrders[order.Market][order.Symbol] {
			if value.Price == order.Price {
				priceEqual = true
				if value.OrderTime.Before(order.OrderTime) {
					refreshOrders.bidOrders[order.Market][order.Symbol][key] = order
				}
			}
		}
		if !priceEqual {
			refreshOrders.bidOrders[order.Market][order.Symbol] =
				append(refreshOrders.bidOrders[order.Market][order.Symbol], order)
		}
	}
	if orderSide == model.OrderSideSell {
		if refreshOrders.askOrders == nil {
			refreshOrders.askOrders = make(map[string]map[string][]*model.Order)
		}
		if refreshOrders.askOrders[market] == nil {
			refreshOrders.askOrders[market] = make(map[string][]*model.Order)
		}
		if refreshOrders.askOrders[market][symbol] == nil {
			refreshOrders.askOrders[market][symbol] = make([]*model.Order, 0)
		}
		priceEqual := false
		for key, value := range refreshOrders.askOrders[order.Market][order.Symbol] {
			if value.Price == order.Price {
				priceEqual = true
				if value.OrderTime.Before(order.OrderTime) {
					refreshOrders.askOrders[order.Market][order.Symbol][key] = order
				}
			}
		}
		if !priceEqual {
			refreshOrders.askOrders[order.Market][order.Symbol] =
				append(refreshOrders.askOrders[order.Market][order.Symbol], order)
		}
	}
}

func (refreshOrders *RefreshOrders) CancelRefreshOrders(market, symbol string, bidPrice, askPrice float64, process bool) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	canceling = true
	if refreshOrders.askOrders == nil {
		refreshOrders.askOrders = make(map[string]map[string][]*model.Order)
	}
	if refreshOrders.askOrders[market] == nil {
		refreshOrders.askOrders[market] = make(map[string][]*model.Order)
	}
	if refreshOrders.askOrders[market][symbol] == nil {
		refreshOrders.askOrders[market][symbol] = make([]*model.Order, 0)
	}
	if refreshOrders.bidOrders == nil {
		refreshOrders.bidOrders = make(map[string]map[string][]*model.Order)
	}
	if refreshOrders.bidOrders[market] == nil {
		refreshOrders.bidOrders[market] = make(map[string][]*model.Order)
	}
	if refreshOrders.bidOrders[market][symbol] == nil {
		refreshOrders.bidOrders[market][symbol] = make([]*model.Order, 0)
	}
	bidOrders := make([]*model.Order, 0)
	askOrders := make([]*model.Order, 0)
	d, _ := time.ParseDuration("-3601s")
	timeLine := util.GetNow().Add(d)
	for _, value := range refreshOrders.bidOrders[market][symbol] {
		if value.Price < bidPrice && value.OrderTime.Before(timeLine) && process { // 大于等于卖一的买单已经成交，无需取消
			util.Notice(fmt.Sprintf(`[try cancel]bid %f < %f`, value.Price, bidPrice))
			api.MustCancel(value.Market, value.Symbol, value.OrderId, true)
			time.Sleep(time.Millisecond * 100)
		} else if value.Price < askPrice && value.Status == model.CarryStatusWorking {
			bidOrders = append(bidOrders, value)
		}
	}
	for _, value := range refreshOrders.askOrders[market][symbol] {
		if value.Price > askPrice && value.OrderTime.Before(timeLine) && process { // 小于等于买一的卖单已经成交，无需取消
			util.Notice(fmt.Sprintf(`[try cancel]ask %f > %f`, value.Price, askPrice))
			api.MustCancel(value.Market, value.Symbol, value.OrderId, true)
			time.Sleep(time.Millisecond * 100)
		} else if value.Price > bidPrice && value.Status == model.CarryStatusWorking {
			askOrders = append(askOrders, value)
		}
	}
	refreshOrders.bidOrders[market][symbol] = bidOrders
	refreshOrders.askOrders[market][symbol] = askOrders
	canceling = false
}

func setRefreshing(value bool) {
	refreshing = value
}

var ProcessRefresh = func(market, symbol string) {
	if model.AppConfig.Handle != `1` || model.AppConfig.HandleRefresh != `1` || refreshing {
		//|| (symbol == `btc_usdt` && btcusdtBigTime != nil && util.GetNow().Unix()-btcusdtBigTime.Unix() < 900) {
		return
	}
	pointGetTick := time.Now().UnixNano()
	setting := model.GetSetting(model.FunctionRefresh, market, symbol)
	setRefreshing(true)
	defer setRefreshing(false)
	//currencies := strings.Split(symbol, "_")
	now := util.GetNowUnixMillion()
	if now-LastRefreshTime[market] > 15000 {
		util.Notice(`15 seconds past, refresh and return ` + market + symbol)
		api.RefreshAccount(market)
		LastRefreshTime[market] = now
		return
	}
	leftBalance, rightBalance, _, _, err := getBalance(market, symbol, setting.AccountType)
	if err != nil {
		return
	}
	if model.AppMarkets.BidAsks[symbol] == nil || model.AppMarkets.BidAsks[symbol][market] == nil ||
		len(model.AppMarkets.BidAsks[symbol][market].Bids) == 0 ||
		len(model.AppMarkets.BidAsks[symbol][market].Asks) == 0 {
		util.Notice(`nil bid-ask price for ` + symbol)
		return
	}
	bidPrice := model.AppMarkets.BidAsks[symbol][market].Bids[0].Price
	askPrice := model.AppMarkets.BidAsks[symbol][market].Asks[0].Price
	bidAmount := model.AppMarkets.BidAsks[symbol][market].Bids[0].Amount
	askAmount := model.AppMarkets.BidAsks[symbol][market].Asks[0].Amount
	amount := math.Min(leftBalance, rightBalance/askPrice) * model.AppConfig.AmountRate
	priceDistance := 0.9 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	delay := util.GetNowUnixMillion() - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
	binanceResult, binancePrice := getBinanceInfo(symbol)
	if delay > 200 || !binanceResult {
		util.Info(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
	//if symbol == `btc_usdt` {
	//	if (lastTickBid != nil && lastTickBid.Amount >= 100 && lastTickBid.Price == bidPrice && bidAmount >= 100) ||
	//		(lastTickAsk != nil && lastTickAsk.Amount >= 100 && lastTickAsk.Price == askPrice && askAmount >= 100) {
	//		util.Notice(`[someone refreshing] sleep 15 minutes`)
	//		myTime := util.GetNow()
	//		btcusdtBigTime = &myTime
	//		lastTickAsk = nil
	//		lastTickBid = nil
	//		return
	//	}
	//	lastTickAsk = &model.AppMarkets.BidAsks[symbol][market].Asks[0]
	//	lastTickBid = &model.AppMarkets.BidAsks[symbol][market].Bids[0]
	//}
	if canceling {
		util.Notice(`[refreshing waiting for canceling]`)
		return
	}
	switch setting.FunctionParameter {
	case model.FunRefreshMiddle:
		price, _ := util.FormatNum((bidPrice+askPrice)/2, api.GetPriceDecimal(market, symbol))
		if (price-bidPrice) <= priceDistance || (askPrice-price) <= priceDistance {
			if askAmount > bidAmount {
				price = bidPrice
				if bidAmount*20 > amount {
					util.Notice(fmt.Sprintf(`[refresh crash]bid:%f - %f`, bidAmount, amount))
					return
				}
			} else {
				price = askPrice
				if askAmount*20 > amount {
					util.Notice(fmt.Sprintf(`[refresh crash]ask:%f - %f`, askAmount, amount))
					return
				}
			}
		}
		doRefresh(market, symbol, price, amount)
	case model.FunRefreshSeparate:
		haveAmount := refreshOrders.CheckAmountLimit(market, symbol, setting.AmountLimit)
		if !haveAmount {
			return
		}
		util.Info(fmt.Sprintf(`[depth %s] price %f %f amount %f %f`, symbol, bidPrice,
			askPrice, bidAmount, askAmount))
		_, orderSide, reverseSide, orderPrice := preDeal(market, symbol, binancePrice, amount)
		if refreshOrders.CheckLastRefreshPrice(market, symbol, orderPrice, priceDistance) {
			util.Notice(fmt.Sprintf(`[jump over] %s %s %f`, market, symbol, orderPrice))
			orderSide = ``
		}
		if orderSide != `` {
			if !refreshOrders.CheckRecentOrder(market, symbol, orderPrice) {
				util.Notice(fmt.Sprintf(`[same price 3] %s %f`, symbol, orderPrice))
				return
			}
			LastRefreshTime[market] = util.GetNowUnixMillion()
			pointBefore1 := time.Now().UnixNano()
			orderResult, order := placeSeparateOrder(orderSide, market, symbol, setting.AccountType,
				orderPrice, amount, 1, 2)
			pointAfter1 := time.Now().UnixNano()
			if orderResult {
				refreshOrders.AddRecentOrder(market, symbol, order)
				pointBefore2 := time.Now().UnixNano()
				reverseResult, reverseOrder :=
					placeSeparateOrder(reverseSide, market, symbol, setting.AccountType,
						orderPrice, amount, 1, 1)
				pointAfter2 := time.Now().UnixNano()
				util.Notice(fmt.Sprintf(`>>>>>>>>>>>%d %d %d %d %d`, pointGetTick,
					(pointBefore1-pointGetTick)/int64(time.Millisecond),
					(pointAfter1-pointBefore1)/int64(time.Millisecond),
					(pointBefore2-pointAfter1)/int64(time.Millisecond),
					(pointAfter2-pointAfter1)/int64(time.Millisecond)))
				if !reverseResult {
					api.MustCancel(market, symbol, order.OrderId, true)
					time.Sleep(time.Second * 2)
					if reverseOrder != nil && reverseOrder.ErrCode == `1016` {
						time.Sleep(time.Second)
						api.RefreshAccount(market)
					}
				} else {
					priceInSymbol, _ := api.GetPrice(symbol)
					refreshOrders.AddRefreshAmount(market, symbol, 2*amount*priceInSymbol)
					refreshOrders.SetLastRefreshPrice(market, symbol, orderPrice)
				}
			} else if order != nil && order.ErrCode == `1016` {
				if lastOrign1016 {
					lastOrign1016 = false
					time.Sleep(time.Second * 3)
					api.RefreshAccount(market)
				} else {
					lastOrign1016 = true
				}
			}
		} else {
			refreshOrders.SetLastRefreshPrice(market, symbol, 0.0)
		}
	}
}

func preDeal(market, symbol string, binancePrice, amount float64) (
	result bool, orderSide, reverseSide string, orderPrice float64) {
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	tick := model.AppMarkets.BidAsks[symbol][market]
	if tick.Bids[0].Price > binancePrice*(1+model.AppConfig.BinanceDisMin) &&
		tick.Bids[0].Price < binancePrice*(1+model.AppConfig.BinanceDisMax) {
		if tick.Bids[0].Price < binancePrice*(1+model.AppConfig.PreDealDis) {
			if tick.Bids[0].Amount < amount*model.AppConfig.RefreshLimit &&
				tick.Bids[0].Amount > amount*model.AppConfig.RefreshLimitLow &&
				tick.Asks[0].Amount > 2*tick.Bids[0].Amount {
				return true, model.OrderSideBuy, model.OrderSideSell, tick.Bids[0].Price
			}
		} else if tick.Bids[0].Price > binancePrice*(1+model.AppConfig.PreDealDis) {
			for orderPrice = binancePrice * (1 + model.AppConfig.BinanceOrderDis); orderPrice < tick.Bids[0].Price+0.1*priceDistance; orderPrice += priceDistance {
				bidAmount := 0.0
				for i := 0; i < tick.Bids.Len() && tick.Bids[i].Price > orderPrice-0.1*priceDistance; i++ {
					bidAmount += tick.Bids[i].Amount
				}
				if bidAmount < amount*model.AppConfig.RefreshLimit &&
					bidAmount > amount*model.AppConfig.RefreshLimitLow &&
					tick.Asks[0].Amount > 2*bidAmount {
					if orderPrice > tick.Bids[0].Price {
						util.Notice(fmt.Sprintf(`[price error] order price: %f bid1: %f`,
							orderPrice, tick.Bids[0].Price))
						orderPrice = tick.Bids[0].Price
					}
					return true, model.OrderSideBuy, model.OrderSideSell, orderPrice
				}
			}
		}
	} else if tick.Asks[0].Price > binancePrice*(1-model.AppConfig.BinanceDisMax) &&
		tick.Asks[0].Price < binancePrice*(1-model.AppConfig.BinanceDisMin) {
		if tick.Asks[0].Price > binancePrice*(1-model.AppConfig.PreDealDis) {
			if tick.Asks[0].Amount < amount*model.AppConfig.RefreshLimit &&
				tick.Asks[0].Amount > amount*model.AppConfig.RefreshLimitLow &&
				tick.Bids[0].Amount > 2*tick.Asks[0].Amount {
				return true, model.OrderSideSell, model.OrderSideBuy, tick.Asks[0].Price
			}
		} else if tick.Asks[0].Price < binancePrice*(1-model.AppConfig.PreDealDis) {
			for orderPrice = binancePrice * (1 - model.AppConfig.BinanceOrderDis); orderPrice > tick.Asks[0].Price-0.1*priceDistance; orderPrice -= priceDistance {
				askAmount := 0.0
				for i := 0; i < tick.Asks.Len() && tick.Asks[i].Price < orderPrice+0.1*priceDistance; i++ {
					askAmount += tick.Asks[i].Amount
				}
				if askAmount < amount*model.AppConfig.RefreshLimit &&
					askAmount > amount*model.AppConfig.RefreshLimitLow &&
					tick.Bids[0].Amount > 2*askAmount {
					if orderPrice < tick.Asks[0].Price {
						util.Notice(fmt.Sprintf(`[price error] order price: %f ask1 %f`, orderPrice, tick.Asks[0].Price))
						orderPrice = tick.Asks[0].Price
					}
					return true, model.OrderSideSell, model.OrderSideBuy, orderPrice
				}
			}
		}
	}
	return false, ``, ``, 0
}

func getBinanceInfo(symbol string) (result bool, binancePrice float64) {
	binanceBidAsks := model.AppMarkets.BidAsks[symbol][model.Binance]
	if binanceBidAsks == nil || binanceBidAsks.Bids == nil || binanceBidAsks.Asks == nil ||
		binanceBidAsks.Bids.Len() == 0 || binanceBidAsks.Asks.Len() == 0 {
		return false, 0
	}
	delay := util.GetNowUnixMillion() - int64(binanceBidAsks.Ts)
	if delay > 1000 {
		util.Notice(fmt.Sprintf(`[binance %s]delay %d`, symbol, delay))
		return false, 0
	}
	return true, (binanceBidAsks.Bids[0].Price + binanceBidAsks.Asks[0].Price) / 2
}

//getPriceFromDepth 卖一上数量不足百分之一的，价格往上，直到累积卖单数量超过百一的价格上下单，如果累积数量直接超过了百分之三，
// 则在此价格的前面一个单位上下单
func _(market, symbol string, amount float64) (bidPrice, askPrice float64) {
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	asks := model.AppMarkets.BidAsks[symbol][market].Asks
	bids := model.AppMarkets.BidAsks[symbol][market].Bids
	bidAmount := 0.0
	askAmount := 0.0
	for i := 0; i < len(bids); i++ {
		bidAmount += bids[i].Amount
		if bidAmount > amount*0.002 {
			bidPrice = bids[i].Price
			if bidAmount > amount*0.03 {
				bidPrice = bids[i].Price + priceDistance
			}
			break
		}
	}
	bidLimit := model.AppMarkets.BidAsks[symbol][market].Bids[0].Price * 0.9999
	if bidPrice < bidLimit {
		bidPrice = bidLimit
	}
	for i := 0; i < len(asks); i++ {
		askAmount += asks[i].Amount
		if askAmount > amount*0.002 {
			askPrice = asks[i].Price
			if askAmount > amount*0.03 {
				askPrice = asks[i].Price - priceDistance
			}
			break
		}
	}
	askLimit := model.AppMarkets.BidAsks[symbol][market].Asks[0].Price * 1.0001
	if askPrice > askLimit {
		askPrice = askLimit
	}
	return bidPrice, askPrice
}

func doRefresh(market, symbol string, price, amount float64) {
	LastRefreshTime[market] = util.GetNowUnixMillion()
	refreshOrders.SetLastOrder(market, symbol, model.OrderSideSell, nil)
	refreshOrders.SetLastOrder(market, symbol, model.OrderSideBuy, nil)
	go placeRefreshOrder(model.OrderSideSell, market, symbol, price, amount)
	go placeRefreshOrder(model.OrderSideBuy, market, symbol, price, amount)
	for true {
		<-syncRefresh
		refreshLastBid := refreshOrders.GetLastOrder(market, symbol, model.OrderSideSell)
		refreshLastAsk := refreshOrders.GetLastOrder(market, symbol, model.OrderSideBuy)
		if refreshLastBid != nil && refreshLastAsk != nil {
			if refreshLastBid.Status == model.CarryStatusWorking && refreshLastAsk.Status == model.CarryStatusFail {
				api.MustCancel(refreshLastBid.Market, refreshLastBid.Symbol, refreshLastBid.OrderId, true)
			} else if refreshLastAsk.Status == model.CarryStatusWorking && refreshLastBid.Status == model.CarryStatusFail {
				api.MustCancel(refreshLastAsk.Market, refreshLastAsk.Symbol, refreshLastAsk.OrderId, true)
			}
			break
		}
	}
	bidAskTimes++
	if bidAskTimes%7 == 0 {
		api.RefreshAccount(market)
		//rebalance(leftAccount, rightAccount, carry)
	}
}

func placeRefreshOrder(orderSide, market, symbol string, price, amount float64) {
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, ``, price, amount)
	order.Function = model.FunctionRefresh
	if order.Status == model.CarryStatusWorking {
		refreshOrders.Add(market, symbol, orderSide, order)
	}
	refreshOrders.SetLastOrder(market, symbol, orderSide, order)
	model.AppDB.Save(order)
	syncRefresh <- struct{}{}
}

func placeSeparateOrder(orderSide, market, symbol, accountType string, price, amount float64, try, insufficient int) (
	result bool, order *model.Order) {
	insufficientTimes := 0
	for i := 0; i < try; {
		order = api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, accountType, price, amount)
		if order == nil {
			break
		}
		if order.ErrCode == `1016` {
			insufficientTimes++
			if insufficientTimes >= insufficient {
				return false, order
			}
			continue
		} else if order.Status == model.CarryStatusWorking {
			order.Function = model.FunctionRefresh
			//refreshOrders.Add(market, symbol, orderSide, order)
			model.AppDB.Save(order)
			return true, order
		}
		i++
		time.Sleep(time.Millisecond * 100)
	}
	return false, order
}
