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
var refreshingBtcUsdt = false
var syncRefresh = make(chan interface{}, 10)
var refreshOrders = &RefreshOrders{}
var lastOrign1016 = false

type RefreshOrders struct {
	lock      sync.Mutex
	bidOrders map[string]map[string][]*model.Order // market - symbol - orders
	askOrders map[string]map[string][]*model.Order // market - symbol - orders
	lastBid   map[string]map[string]*model.Order   // market - symbol - order
	lastAsk   map[string]map[string]*model.Order   // market - symbol - order
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

func setRefreshing(value bool) {
	refreshing = value
}

var ProcessRefresh = func(market, symbol string) {
	//current := refreshOrders.getCurrentSymbol(market, symbol)
	if model.AppConfig.Handle != `1` || model.AppConfig.HandleRefresh != `1` || refreshing ||
		(symbol != `btc_usdt` && refreshingBtcUsdt) {
		return
	}
	setting := model.GetSetting(model.FunctionRefresh, market, symbol)
	setRefreshing(true)
	defer setRefreshing(false)
	//currencies := strings.Split(symbol, "_")
	leftBalance, rightBalance, err := getBalance(market, symbol, setting.AccountType)
	if err != nil {
		return
	}
	if model.AppMarkets.BidAsks[symbol] == nil || model.AppMarkets.BidAsks[symbol][market] == nil ||
		len(model.AppMarkets.BidAsks[symbol][market].Bids) == 0 || len(model.AppMarkets.BidAsks[symbol][market].Asks) == 0 {
		util.Notice(`nil bid-ask price for ` + symbol)
		return
	}
	bidPrice := model.AppMarkets.BidAsks[symbol][market].Bids[0].Price
	askPrice := model.AppMarkets.BidAsks[symbol][market].Asks[0].Price
	bidAmount := model.AppMarkets.BidAsks[symbol][market].Bids[0].Amount
	askAmount := model.AppMarkets.BidAsks[symbol][market].Asks[0].Amount
	if symbol == `btc_usdt` && (bidAmount >= 100 || askAmount >= 100) {
		util.Notice(`[someone refreshing] sleep 30 minutes`)
		time.Sleep(time.Minute * 30)
	}
	price := (bidPrice + askPrice) / 2
	amount := math.Min(leftBalance, rightBalance/price) * model.AppConfig.AmountRate
	priceDistance := 0.9 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	delay := util.GetNowUnixMillion() - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
	binanceResult, binancePrice := getBinanceInfo(symbol)
	if delay > 50 || !binanceResult {
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
		util.Notice(fmt.Sprintf(`[depth %s] price %f %f amount %f %f`, symbol, bidPrice,
			askPrice, bidAmount, askAmount))
		orderSide := ``
		reverseSide := ``
		orderPrice := price
		if (price-bidPrice) <= priceDistance || (askPrice-price) <= priceDistance {
			//bidPrice, askPrice = getPriceFromDepth(market, symbol, amount)
			if askAmount > 1.5*bidAmount &&
				bidAmount < amount*model.AppConfig.RefreshLimit &&
				bidAmount > amount*model.AppConfig.RefreshLimitLow &&
				(1-model.AppConfig.BinanceDisMin)*price > binancePrice &&
				(1-model.AppConfig.BinanceDisMax)*price < binancePrice {
				orderSide = model.OrderSideBuy
				reverseSide = model.OrderSideSell
				orderPrice = bidPrice
			} else if 1.5*askAmount <= bidAmount &&
				askAmount < amount*model.AppConfig.RefreshLimit &&
				askAmount > amount*model.AppConfig.RefreshLimitLow &&
				(1+model.AppConfig.BinanceDisMax)*price > binancePrice &&
				(1+model.AppConfig.BinanceDisMin)*price < binancePrice {
				orderSide = model.OrderSideSell
				reverseSide = model.OrderSideBuy
				orderPrice = askPrice
			}
		} else if symbol == `btc_usdt` {
			if price > binancePrice && (1-model.AppConfig.BinanceDisMin)*price > binancePrice &&
				(1-model.AppConfig.BinanceDisMax)*price < binancePrice {
				orderSide = model.OrderSideBuy
				reverseSide = model.OrderSideSell
				orderPrice = (price + bidPrice) / 2
			} else if price <= binancePrice && (1+model.AppConfig.BinanceDisMax)*price > binancePrice &&
				(1+model.AppConfig.BinanceDisMin)*price < binancePrice {
				orderSide = model.OrderSideSell
				reverseSide = model.OrderSideBuy
				orderPrice = (price + askPrice) / 2
			}
		}
		if orderSide != `` {
			refreshAble = true
			orderResult, order := placeSeparateOrder(orderSide, market, symbol, setting.AccountType,
				orderPrice, amount, 1, 2)
			if orderResult {
				time.Sleep(time.Millisecond * 15)
				reverseResult, reverseOrder :=
					placeSeparateOrder(reverseSide, market, symbol, setting.AccountType,
						orderPrice, amount, 4, 1)
				if !reverseResult {
					go api.MustCancel(market, symbol, order.OrderId, true)
					time.Sleep(time.Second * 2)
					if reverseOrder.ErrCode == `1016` {
						time.Sleep(time.Second)
						api.RefreshAccount(market)
					}
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
		if symbol == `btc_usdt` {
			refreshingBtcUsdt = refreshAble
		}
	}
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
	priceDistance := 0.9 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
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
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, ``, price, amount)
	order.Function = model.FunctionRefresh
	if order.Status == model.CarryStatusWorking {
		refreshOrders.Add(market, symbol, orderSide, order)
	}
	refreshOrders.SetLastOrder(market, symbol, orderSide, order)
	model.AppDB.Save(order)
	syncRefresh <- struct{}{}
}

func placeSeparateOrder(orderSide, market, symbol, accountType string, price, amount float64, retry, insufficient int) (
	result bool, order *model.Order) {
	insufficientTimes := 0
	for i := 0; i < retry; i++ {
		order = api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, accountType, price, amount)
		if order.ErrCode == `1016` {
			insufficientTimes++
			if insufficient <= insufficientTimes {
				return false, order
			}
		} else if order.Status == model.CarryStatusWorking {
			order.Function = model.FunctionRefresh
			refreshOrders.Add(market, symbol, orderSide, order)
			//refreshOrders.SetLastOrder(market, symbol, orderSide, order)
			model.AppDB.Save(order)
			return true, order
		}
		time.Sleep(time.Millisecond * 100)
	}
	return false, order
}
