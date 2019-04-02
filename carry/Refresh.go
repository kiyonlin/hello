package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strings"
	"sync"
	"time"
)

// coinpark://4003 调用次数繁忙 //2085 最小下单数量限制 //2027 可用余额不足
var bidAskTimes int64
var lastRefreshTime int64
var refreshing = false
var refreshingBtcUsdt = false
var syncRefresh = make(chan interface{}, 10)
var refreshOrders = &RefreshOrders{}
var lastOrign1016 = false

type RefreshOrders struct {
	lock        sync.Mutex
	bidOrders   map[string]map[string][]*model.Order // market - symbol - orders
	askOrders   map[string]map[string][]*model.Order // market - symbol - orders
	lastBid     map[string]map[string]*model.Order   // market - symbol - order
	lastAsk     map[string]map[string]*model.Order   // market - symbol - order
	stayTimes   map[string]int                       // market - current symbol refresh times
	symbolIndex map[string]int                       // market - current refresh symbol index
}

func (refreshOrders *RefreshOrders) getCurrentSymbol(market, symbol string) (currentSymbol string) {
	settings := model.GetFunctionSettingsButBTCUSDT(model.FunctionRefresh, market, model.FunRefreshSeparate)
	if len(settings) == 0 {
		return ""
	}
	if refreshOrders.symbolIndex == nil {
		refreshOrders.symbolIndex = make(map[string]int)
	}
	index := refreshOrders.symbolIndex[market] % len(settings)
	return settings[index].Symbol
}

func (refreshOrders *RefreshOrders) addStayTimes(market, symbol string) {
	current := refreshOrders.getCurrentSymbol(market, symbol)
	if symbol != current {
		return
	}
	if refreshOrders.stayTimes == nil {
		refreshOrders.stayTimes = make(map[string]int)
	}
	refreshOrders.stayTimes[market] = refreshOrders.stayTimes[market] + 1
	limit := 10
	if refreshOrders.stayTimes[market] >= limit {
		refreshOrders.moveNextSymbol(market, symbol)
	}
}

func (refreshOrders *RefreshOrders) moveNextSymbol(market, symbol string) {
	if refreshOrders.symbolIndex == nil {
		refreshOrders.symbolIndex = make(map[string]int)
	}
	if refreshOrders.stayTimes == nil {
		refreshOrders.stayTimes = make(map[string]int)
	}
	current := refreshOrders.getCurrentSymbol(market, symbol)
	if symbol == current {
		refreshOrders.symbolIndex[market] = refreshOrders.symbolIndex[market] + 1
		refreshOrders.stayTimes[market] = 0
	}
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
					util.Notice(fmt.Sprintf(`newer order at price %f`, value.Price))
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
					util.Notice(fmt.Sprintf(`newer order at price %f`, value.Price))
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

func (refreshOrders *RefreshOrders) CancelRefreshOrders(market, symbol string, bidPrice, askPrice float64) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
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
	for _, value := range refreshOrders.bidOrders[market][symbol] {
		if value.Price < bidPrice { // 大于等于卖一的买单已经成交，无需取消
			util.Notice(fmt.Sprintf(`[try cancel]bid %f < %f`, value.Price, bidPrice))
			go api.MustCancel(value.Market, value.Symbol, value.OrderId, true)
			time.Sleep(time.Second)
		} else if value.Price < askPrice && value.Price >= bidPrice && value.Status == model.CarryStatusWorking {
			bidOrders = append(bidOrders, value)
		}
	}
	for _, value := range refreshOrders.askOrders[market][symbol] {
		if value.Price > askPrice { // 小于等于买一的卖单已经成交，无需取消
			util.Notice(fmt.Sprintf(`[try cancel]ask %f > %f`, value.Price, askPrice))
			go api.MustCancel(value.Market, value.Symbol, value.OrderId, true)
			time.Sleep(time.Second)
		} else if value.Price > bidPrice && value.Price <= askPrice && value.Status == model.CarryStatusWorking {
			askOrders = append(askOrders, value)
		}
	}
	refreshOrders.bidOrders[market][symbol] = bidOrders
	refreshOrders.askOrders[market][symbol] = askOrders
}

func setRefreshing(symbol string, value bool) {
	if symbol == `btc_usdt` {
		refreshingBtcUsdt = value
	} else {
		refreshing = value
	}
}

func getRefreshing(symbol string) (result bool) {
	if symbol == `btc_usdt` {
		return refreshingBtcUsdt
	} else {
		return refreshing
	}
}

var ProcessRefresh = func(market, symbol string) {
	//current := refreshOrders.getCurrentSymbol(market, symbol)
	if model.AppConfig.Handle != `1` || model.AppConfig.HandleRefresh != `1` || getRefreshing(symbol) {
		return
	}
	setRefreshing(symbol, true)
	defer setRefreshing(symbol, false)
	currencies := strings.Split(symbol, "_")
	leftAccount := model.AppAccounts.GetAccount(market, currencies[0])
	if leftAccount == nil || util.GetNowUnixMillion()-lastRefreshTime > 15000 {
		util.Notice(`nil account or 15 seconds refresh ` + market + ` ` + symbol)
		lastRefreshTime = util.GetNowUnixMillion()
		time.Sleep(time.Second * 2)
		api.RefreshAccount(market)
		return
	}
	leftBalance := leftAccount.Free
	rightAccount := model.AppAccounts.GetAccount(market, currencies[1])
	if rightAccount == nil {
		util.Notice(`nil account ` + market + currencies[1])
		api.RefreshAccount(market)
		return
	}
	rightBalance := rightAccount.Free
	if model.AppMarkets.BidAsks[symbol] == nil || model.AppMarkets.BidAsks[symbol][market] == nil ||
		len(model.AppMarkets.BidAsks[symbol][market].Bids) == 0 || len(model.AppMarkets.BidAsks[symbol][market].Asks) == 0 {
		util.Notice(`nil bid-ask price for ` + symbol)
		return
	}
	bidPrice := model.AppMarkets.BidAsks[symbol][market].Bids[0].Price
	askPrice := model.AppMarkets.BidAsks[symbol][market].Asks[0].Price
	bidAmount := model.AppMarkets.BidAsks[symbol][market].Bids[0].Amount
	askAmount := model.AppMarkets.BidAsks[symbol][market].Asks[0].Amount
	price := (bidPrice + askPrice) / 2
	amount := math.Min(leftBalance, rightBalance/price) * model.AppConfig.AmountRate
	priceDistance := 0.9 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	setting := model.GetSetting(model.FunctionRefresh, market, symbol)
	delay := util.GetNowUnixMillion() - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
	if delay > 50 {
		util.Info(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
	go refreshOrders.CancelRefreshOrders(market, symbol, bidPrice, askPrice)
	switch setting.FunctionParameter {
	case model.FunRefreshMiddle:
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
		refreshAble := false
		refreshDone := false
		util.Notice(fmt.Sprintf(`[depth %s] price %f %f amount %f %f`, symbol, bidPrice,
			askPrice, bidAmount, askAmount))
		if (askPrice-bidPrice-1/math.Pow(10, float64(api.GetPriceDecimal(market, symbol))))*10000 < bidPrice {
			orderSide := ``
			reverseSide := ``
			orderPrice := price
			bidPrice, askPrice = getPriceFromDepth(market, symbol, amount)
			if askAmount > 1.5*bidAmount && bidAmount < model.AppConfig.RefreshLimit*amount {
				orderSide = model.OrderSideSell
				reverseSide = model.OrderSideBuy
				orderPrice = bidPrice
			} else if askAmount <= 1.5*bidAmount && askAmount < model.AppConfig.RefreshLimit*amount {
				orderSide = model.OrderSideBuy
				reverseSide = model.OrderSideSell
				orderPrice = askPrice
			}
			if orderSide != `` {
				refreshAble = true
				orderResult, order := placeSeparateOrder(orderSide, market, symbol, orderPrice, amount)
				if orderResult {
					time.Sleep(time.Millisecond * 100)
					reverseResult, reverseOrder := placeSeparateOrder(reverseSide, market, symbol, orderPrice, amount)
					if !reverseResult {
						go api.MustCancel(market, symbol, order.OrderId, true)
						time.Sleep(time.Second * 2)
						if reverseOrder.ErrCode == `1016` {
							time.Sleep(time.Second)
							api.RefreshAccount(market)
						}
					} else {
						refreshDone = true
					}
				} else if order.ErrCode == `1016` {
					if lastOrign1016 {
						lastOrign1016 = false
						time.Sleep(time.Second * 3)
						api.RefreshAccount(market)
					} else {
						lastOrign1016 = true
					}
				}
			}
		}
		if !refreshAble {
			refreshOrders.moveNextSymbol(market, symbol)
		}
		if refreshDone {
			refreshOrders.addStayTimes(market, symbol)
		}
	}
}

//卖一上数量不足百分之一的，价格往上，直到累积卖单数量超过百一的价格上下单，如果累积数量直接超过了百分之三，则在此价格的前面一个单位上下单
func getPriceFromDepth(market, symbol string, amount float64) (bidPrice, askPrice float64) {
	priceDistance := 0.9 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	asks := model.AppMarkets.BidAsks[symbol][market].Asks
	bids := model.AppMarkets.BidAsks[symbol][market].Bids
	bidAmount := 0.0
	askAmount := 0.0
	for i := 0; i < len(bids); i++ {
		bidAmount += bids[i].Amount
		if bidAmount > amount*0.01 {
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
		if askAmount > amount*0.01 {
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
				go api.MustCancel(refreshLastBid.Market, refreshLastBid.Symbol, refreshLastBid.OrderId, true)
			} else if refreshLastAsk.Status == model.CarryStatusWorking && refreshLastBid.Status == model.CarryStatusFail {
				go api.MustCancel(refreshLastAsk.Market, refreshLastAsk.Symbol, refreshLastAsk.OrderId, true)
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
	lastRefreshTime = util.GetNowUnixMillion()
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, price, amount)
	order.Function = model.FunctionRefresh
	if order.Status == model.CarryStatusWorking {
		refreshOrders.Add(market, symbol, orderSide, order)
	}
	refreshOrders.SetLastOrder(market, symbol, orderSide, order)
	model.AppDB.Save(order)
	syncRefresh <- struct{}{}
}

func placeSeparateOrder(orderSide, market, symbol string, price, amount float64) (result bool, order *model.Order) {
	lastRefreshTime = util.GetNowUnixMillion()
	order = api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, price, amount)
	if order.ErrCode == `1016` {
		return false, order
	}
	if order.OrderId == `` || order.Status == model.CarryStatusFail {
		time.Sleep(time.Millisecond * 100)
		order = api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, price, amount)
	}
	if order.Status == model.CarryStatusWorking {
		order.Function = model.FunctionRefresh
		refreshOrders.Add(market, symbol, orderSide, order)
		//refreshOrders.SetLastOrder(market, symbol, orderSide, order)
		model.AppDB.Save(order)
		return true, order
	}
	return false, order
}
