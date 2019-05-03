package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	//"sync"
	"time"
)

// coinpark://4003 调用次数繁忙 //2085 最小下单数量限制 //2027 可用余额不足
var refreshing = false

var syncRefresh = make(chan interface{}, 20)
var LastRefreshTime = make(map[string]int64) // market - int64
var refreshOrders = &RefreshOrders{}
var canceling = false

type RefreshOrders struct {
	//lock             sync.Mutex
	samePriceCount   map[string]map[string]int             // market - symbol - continue same price count
	samePriceTime    map[string]map[string]*time.Time      // market - symbol - first time new order price
	bidOrders        map[string]map[string][]*model.Order  // market - symbol - orders
	askOrders        map[string]map[string][]*model.Order  // market - symbol - orders
	lastBid          map[string]map[string]*model.Order    // market - symbol - order
	lastAsk          map[string]map[string]*model.Order    // market - symbol - order
	amountLimit      map[string]map[string]map[int]float64 // market - symbol - time start point - amount
	lastChancePrice  map[string]map[string]float64         // market - symbol - chance price
	lastRefreshPrice map[string]map[string]float64         // market - symbol - refresh price
}

func (refreshOrders *RefreshOrders) SetLastChancePrice(market, symbol string, price float64) {
	//refreshOrders.lock.Lock()
	//defer refreshOrders.lock.Unlock()
	if refreshOrders.lastChancePrice == nil {
		refreshOrders.lastChancePrice = make(map[string]map[string]float64)
	}
	if refreshOrders.lastChancePrice[market] == nil {
		refreshOrders.lastChancePrice[market] = make(map[string]float64)
	}
	refreshOrders.lastChancePrice[market][symbol] = price
}

func (refreshOrders *RefreshOrders) CheckLastChancePrice(market, symbol string, price, priceDistance float64) (same bool) {
	//refreshOrders.lock.Lock()
	//defer refreshOrders.lock.Unlock()
	if refreshOrders.lastChancePrice == nil || refreshOrders.lastChancePrice[market] == nil {
		return false
	}
	if math.Abs(refreshOrders.lastChancePrice[market][symbol]-price) < priceDistance {
		util.Info(fmt.Sprintf(`[jump 1] %s %s %f`, market, symbol, price))
		return true
	}
	return false
}

func (refreshOrders *RefreshOrders) SetLastRefreshPrice(market, symbol string, price, priceDistance float64) {
	//refreshOrders.lock.Lock()
	//defer refreshOrders.lock.Unlock()
	if refreshOrders.lastRefreshPrice == nil {
		refreshOrders.lastRefreshPrice = make(map[string]map[string]float64)
		refreshOrders.samePriceCount = make(map[string]map[string]int)
		refreshOrders.samePriceTime = make(map[string]map[string]*time.Time)
	}
	if refreshOrders.lastRefreshPrice[market] == nil {
		refreshOrders.lastRefreshPrice[market] = make(map[string]float64)
		refreshOrders.samePriceCount[market] = make(map[string]int)
		refreshOrders.samePriceTime[market] = make(map[string]*time.Time)
	}
	if math.Abs(refreshOrders.lastRefreshPrice[market][symbol]-price) < priceDistance {
		refreshOrders.samePriceCount[market][symbol]++
	} else {
		refreshOrders.lastRefreshPrice[market][symbol] = price
		refreshOrders.samePriceCount[market][symbol] = 1
		samePriceTime := util.GetNow()
		refreshOrders.samePriceTime[market][symbol] = &samePriceTime
	}
}

func (refreshOrders *RefreshOrders) CheckLastRefreshPrice(market, symbol string, price, priceDistance float64) (over bool) {
	//refreshOrders.lock.Lock()
	//defer refreshOrders.lock.Unlock()
	if refreshOrders.lastRefreshPrice == nil || refreshOrders.lastRefreshPrice[market] == nil {
		return false
	}
	d, _ := time.ParseDuration("-601s")
	timeLine := util.GetNow().Add(d)
	if refreshOrders.samePriceTime[market][symbol] != nil && refreshOrders.samePriceTime[market][symbol].Before(timeLine) {
		util.Notice(fmt.Sprintf(`[10min clear]%s %f %d`,
			symbol, price, refreshOrders.samePriceTime[market][symbol].Unix()))
		refreshOrders.lastRefreshPrice[market][symbol] = 0
		refreshOrders.samePriceTime[market][symbol] = nil
		refreshOrders.samePriceCount[market][symbol] = 0
		return false
	}
	if math.Abs(refreshOrders.lastRefreshPrice[market][symbol]-price) < priceDistance &&
		refreshOrders.samePriceCount[market][symbol] >= 1 {
		util.Info(fmt.Sprintf(`[jump 2] %s %s %f`, market, symbol, price))
		return true
	}
	return false
}

func (refreshOrders *RefreshOrders) CheckAmountLimit(market, symbol string, amountLimit float64) (underLimit bool) {
	//refreshOrders.lock.Lock()
	//defer refreshOrders.lock.Unlock()
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
	//refreshOrders.lock.Lock()
	//defer refreshOrders.lock.Unlock()
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

func (refreshOrders *RefreshOrders) SetLastOrder(market, symbol, orderSide string, order *model.Order) {
	//refreshOrders.lock.Lock()
	//defer refreshOrders.lock.Unlock()
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
	//refreshOrders.lock.Lock()
	//defer refreshOrders.lock.Unlock()
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

func (refreshOrders *RefreshOrders) AddRefreshOrders(market, symbol, orderSide string, order *model.Order) {
	//refreshOrders.lock.Lock()
	//defer refreshOrders.lock.Unlock()
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
	//refreshOrders.lock.Lock()
	//defer refreshOrders.lock.Unlock()
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
		return
	}
	point1 := time.Now().UnixNano()
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
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	delay := util.GetNowUnixMillion() - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
	binanceResult, binancePrice := getBinanceInfo(symbol)
	if delay > 200 || !binanceResult {
		util.Info(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
	if canceling {
		util.Notice(`[refreshing waiting for canceling]`)
		return
	}
	haveAmount := refreshOrders.CheckAmountLimit(market, symbol, setting.AmountLimit)
	if haveAmount {
		if model.AppConfig.Env == `simon` {
			cancelHang(market, symbol)
		}
		util.Info(fmt.Sprintf(`[depth %s] price %f %f amount %f %f`, symbol, bidPrice,
			askPrice, bidAmount, askAmount))
		refreshAble, orderSide, orderReverse, orderPrice := preDeal(setting, market, symbol, binancePrice, amount)
		if refreshOrders.CheckLastChancePrice(market, symbol, orderPrice, 0.9*priceDistance) {
			refreshOrders.SetLastChancePrice(market, symbol, 0)
			refreshAble = false
		} else if refreshOrders.CheckLastRefreshPrice(market, symbol, orderPrice, 0.9*priceDistance) {
			refreshAble = false
		}
		if refreshAble {
			point2 := time.Now().UnixNano()
			doRefresh(setting, market, symbol, setting.AccountType, orderSide, orderReverse, orderPrice,
				0.9*priceDistance, amount)
			point3 := time.Now().UnixNano()
			util.Notice(fmt.Sprintf(`[speed]%d %d price:%f %f amount:%f %f`,
				point2-point1, point3-point2, bidPrice, askPrice, bidAmount, askAmount))
		}
	} else if model.AppConfig.Env == `simon` {
		util.Notice(fmt.Sprintf(`[hang] %s %s`, market, symbol))
		bidAsk := model.AppMarkets.BidAsks[symbol][market]
		hang(market, symbol, setting.AccountType, bidAsk)
	}
}

func preDeal(setting *model.Setting, market, symbol string, binancePrice, amount float64) (
	result bool, orderSide, reverseSide string, orderPrice float64) {
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	tick := model.AppMarkets.BidAsks[symbol][market]
	if math.Abs(tick.Bids[0].Price-tick.Asks[0].Price) > priceDistance*1.1 {
		return false, "", "", 0
	}
	if tick.Bids[0].Price > binancePrice*(1+setting.BinanceDisMin) &&
		tick.Bids[0].Price < binancePrice*(1+setting.BinanceDisMax) {
		if tick.Bids[0].Price <= binancePrice*(1+model.AppConfig.BinanceOrderDis) {
			if tick.Bids[0].Amount < amount*setting.RefreshLimit &&
				tick.Bids[0].Amount > amount*setting.RefreshLimitLow &&
				tick.Asks[0].Amount > 2*tick.Bids[0].Amount &&
				tick.Asks[0].Amount < model.AppConfig.PreDealDis*tick.Bids[0].Amount {
				return true, model.OrderSideBuy, model.OrderSideSell, tick.Bids[0].Price
			}
		} else {
			bidAmount := tick.Bids[0].Amount
			orderPrice = tick.Bids[0].Price - priceDistance
			if len(tick.Bids) > 1 && tick.Bids[0].Price-tick.Bids[1].Price < 1.1*priceDistance {
				bidAmount += tick.Bids[1].Amount
			}
			if bidAmount < amount*setting.RefreshLimit && bidAmount > amount*setting.RefreshLimitLow &&
				tick.Asks[0].Amount > 2*tick.Bids[0].Amount &&
				tick.Asks[0].Amount < model.AppConfig.PreDealDis*tick.Bids[0].Amount {
				if orderPrice > tick.Bids[0].Price {
					util.Notice(fmt.Sprintf(`[price error] order price: %f bid1: %f`,
						orderPrice, tick.Bids[0].Price))
					orderPrice = tick.Bids[0].Price
				}
				return true, model.OrderSideBuy, model.OrderSideSell, orderPrice
			}
		}
	}
	if tick.Asks[0].Price > binancePrice*(1-setting.BinanceDisMax) &&
		tick.Asks[0].Price < binancePrice*(1-setting.BinanceDisMin) {
		if tick.Asks[0].Price >= binancePrice*(1-model.AppConfig.BinanceOrderDis) {
			if tick.Asks[0].Amount < amount*setting.RefreshLimit &&
				tick.Asks[0].Amount > amount*setting.RefreshLimitLow &&
				tick.Bids[0].Amount > 2*tick.Asks[0].Amount &&
				tick.Bids[0].Amount < model.AppConfig.PreDealDis*tick.Asks[0].Amount {
				return true, model.OrderSideSell, model.OrderSideBuy, tick.Asks[0].Price
			}
		} else {
			askAmount := tick.Asks[0].Amount
			orderPrice = tick.Asks[0].Price + priceDistance
			if len(tick.Asks) > 1 && tick.Asks[1].Price-tick.Asks[0].Price < 1.1*priceDistance {
				askAmount += tick.Asks[1].Amount
			}
			if askAmount < amount*setting.RefreshLimit && askAmount > amount*setting.RefreshLimitLow &&
				tick.Bids[0].Amount > 2*tick.Asks[0].Amount &&
				tick.Bids[0].Amount < model.AppConfig.PreDealDis*tick.Asks[0].Amount {
				if orderPrice < tick.Asks[0].Price {
					util.Notice(fmt.Sprintf(`[price error] order price: %f ask1 %f`,
						orderPrice, tick.Asks[0].Price))
					orderPrice = tick.Asks[0].Price
				}
				return true, model.OrderSideSell, model.OrderSideBuy, orderPrice
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
	if delay > 5000 {
		util.Notice(fmt.Sprintf(`[binance %s]delay %d`, symbol, delay))
		return false, 0
	}
	return true, (binanceBidAsks.Bids[0].Price + binanceBidAsks.Asks[0].Price) / 2
}

func doRefresh(setting *model.Setting, market, symbol, accountType, orderSide, orderReverse string,
	price, priceDistance, amount float64) {
	go receiveRefresh(market, symbol, price, priceDistance, amount)
	LastRefreshTime[market] = util.GetNowUnixMillion()
	refreshOrders.SetLastOrder(market, symbol, model.OrderSideSell, nil)
	refreshOrders.SetLastOrder(market, symbol, model.OrderSideBuy, nil)
	if setting.RefreshSameTime == 1 {
		go placeRefreshOrder(orderSide, market, symbol, accountType, price, amount)
		go placeRefreshOrder(orderReverse, market, symbol, accountType, price, amount)
	} else {
		placeRefreshOrder(orderSide, market, symbol, accountType, price, amount*0.9999)
		time.Sleep(time.Millisecond * time.Duration(model.AppConfig.Between))
		placeRefreshOrder(orderReverse, market, symbol, accountType, price, amount)
	}
}

func receiveRefresh(market, symbol string, price, priceDistance, amount float64) {
	for true {
		//util.Notice(fmt.Sprintf(`[before receive]%s %s %f %f`, market, symbol, price, amount))
		_ = <-syncRefresh
		//util.Notice(fmt.Sprintf(`[after receive]%s %s %f %f`, market, symbol, price, amount))
		refreshLastBid := refreshOrders.GetLastOrder(market, symbol, model.OrderSideSell)
		refreshLastAsk := refreshOrders.GetLastOrder(market, symbol, model.OrderSideBuy)
		if refreshLastBid != nil && refreshLastAsk != nil {
			if refreshLastBid.Status == model.CarryStatusWorking &&
				refreshLastAsk.Status == model.CarryStatusWorking {
				priceInSymbol, _ := api.GetPrice(symbol)
				refreshOrders.AddRefreshAmount(market, symbol, 2*amount*priceInSymbol)
				refreshOrders.SetLastChancePrice(market, symbol, price)
				refreshOrders.SetLastRefreshPrice(market, symbol, price, priceDistance)
			} else {
				if refreshLastBid.Status == model.CarryStatusWorking &&
					refreshLastAsk.Status == model.CarryStatusFail {
					api.MustCancel(refreshLastBid.Market, refreshLastBid.Symbol, refreshLastBid.OrderId, true)
				} else if refreshLastAsk.Status == model.CarryStatusWorking &&
					refreshLastBid.Status == model.CarryStatusFail {
					api.MustCancel(refreshLastAsk.Market, refreshLastAsk.Symbol, refreshLastAsk.OrderId, true)
				}
				time.Sleep(time.Second)
				if refreshLastAsk.ErrCode == `1016` || refreshLastBid.ErrCode == `1016` {
					time.Sleep(time.Second * 1)
					api.RefreshAccount(market)
				}
			}
			break
		}
	}
}

func placeRefreshOrder(orderSide, market, symbol, accountType string, price, amount float64) {
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, accountType, price, amount)
	if order.Status == model.CarryStatusFail && order.ErrCode == `1002` {
		time.Sleep(time.Millisecond * 500)
		order = api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, accountType, price, amount)
	}
	order.Function = model.FunctionRefresh
	refreshOrders.SetLastOrder(market, symbol, orderSide, order)
	//util.Notice(fmt.Sprintf(`[before send]%s %s %f %f`, orderSide, symbol, price, amount))
	syncRefresh <- struct{}{}
	//util.Notice(fmt.Sprintf(`[after send]%s %s %f %f`, orderSide, symbol, price, amount))
	model.AppDB.Save(order)
}
