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

// fcoin:// 下單返回1016 資金不足// 下单返回1002 系统繁忙// 返回426 調用次數太頻繁
// coinpark://4003 调用次数繁忙 //2085 最小下单数量限制 //2027 可用余额不足
var bidAskTimes int64
var lastRefreshTime int64
var processing = false
var refreshing = false
var syncRefresh = make(chan interface{}, 10)
var refreshOrders = &RefreshOrders{}

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
		refreshOrders.bidOrders[order.Market][order.Symbol] =
			append(refreshOrders.bidOrders[order.Market][order.Symbol], order)
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
		refreshOrders.askOrders[order.Market][order.Symbol] =
			append(refreshOrders.askOrders[order.Market][order.Symbol], order)
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
		if value.Price < bidPrice {
			util.Notice(fmt.Sprintf(`[try cancel]bid %f < %f`, value.Price, bidPrice))
			go api.MustCancel(value.Market, value.Symbol, value.OrderId, true)
		} else if value.Price >= bidPrice && value.Status == model.CarryStatusWorking {
			bidOrders = append(bidOrders, value)
		}
	}
	for _, value := range refreshOrders.askOrders[market][symbol] {
		if value.Price > askPrice {
			util.Notice(fmt.Sprintf(`[try cancel]ask %f > %f`, value.Price, askPrice))
			go api.MustCancel(value.Market, value.Symbol, value.OrderId, true)
		} else if value.Price <= askPrice && value.Status == model.CarryStatusWorking {
			askOrders = append(askOrders, value)
		}
	}
	refreshOrders.bidOrders[market][symbol] = bidOrders
	refreshOrders.askOrders[market][symbol] = askOrders
}

func setRefreshing(value bool) {
	refreshing = value
}

func getSidePrice(market, symbol string, amount, priceDistance float64) (price float64) {
	totalAmount := 0.0
	ticks := model.AppMarkets.BidAsks[symbol][market].Bids
	side := model.OrderSideBuy
	if model.AppMarkets.BidAsks[symbol][market].Bids[0].Amount > model.AppMarkets.BidAsks[symbol][market].Asks[0].Amount {
		ticks = model.AppMarkets.BidAsks[symbol][market].Asks
		side = model.OrderSideSell
	}
	for _, tick := range ticks {
		totalAmount += tick.Amount
		if totalAmount > amount*0.0005 {
			if totalAmount < amount*0.02 {
				price = tick.Price
			} else {
				if side == model.OrderSideSell {
					price = tick.Price - priceDistance
				} else if side == model.OrderSideBuy {
					price = tick.Price + priceDistance
				}
			}
			break
		}
	}
	if side == model.OrderSideBuy {
		return math.Max(price, ticks[0].Price*0.9998)
	} else {
		return math.Min(price, ticks[0].Price*1.0002)
	}
}

var ProcessRefresh = func(market, symbol string) {
	if model.AppConfig.Handle != `1` || model.AppConfig.HandleRefresh != `1` || processing || refreshing {
		return
	}
	setRefreshing(true)
	defer setRefreshing(false)
	currencies := strings.Split(symbol, "_")
	leftAccount := model.AppAccounts.GetAccount(market, currencies[0])
	if leftAccount == nil || util.GetNowUnixMillion()-lastRefreshTime > 15000 {
		util.Notice(`nil account or 5min refresh ` + market + currencies[0])
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
	case model.FunRefreshSide:
		amount002 := amount * 0.02
		if ((price-bidPrice) <= priceDistance || (askPrice-price) <= priceDistance) &&
			((amount002 > bidAmount && amount002 < askAmount) || (amount002 > askAmount && amount002 < bidAmount)) {
			price = getSidePrice(market, symbol, amount, priceDistance)
			doRefresh(market, symbol, price, amount)
		}
	case model.FunRefreshSeparate:
		lastSell := refreshOrders.GetLastOrder(market, symbol, model.OrderSideSell)
		lastBuy := refreshOrders.GetLastOrder(market, symbol, model.OrderSideBuy)
		if lastBuy == nil && lastSell == nil {
			if price-bidPrice <= priceDistance || askPrice-price <= priceDistance {
				util.Notice(fmt.Sprintf(
					`[原始单] bid amount:%f ask amount: %f amount: %f bid price: %f ask price: %f %f`,
					bidAmount, askAmount, amount, bidPrice, askPrice, price))
				if askAmount > 1.5*bidAmount && bidAmount > 0.005*amount &&
					bidAmount < model.AppConfig.RefreshLimit*amount {
					if !placeSeparateOrder(model.OrderSideBuy, market, symbol, bidPrice, amount) {
						api.RefreshAccount(market)
					}
				} else if askAmount <= 1.5*bidAmount && askAmount > 0.005*amount &&
					askAmount < model.AppConfig.RefreshLimit*amount {
					if !placeSeparateOrder(model.OrderSideSell, market, symbol, askPrice, amount) {
						api.RefreshAccount(market)
					}
				}
			}
		} else if lastBuy == nil && lastSell != nil {
			if lastSell.Price-askPrice < priceDistance && askAmount < amount*(1.01+model.AppConfig.RefreshLimit) {
				if !placeSeparateOrder(model.OrderSideBuy, market, symbol, lastSell.Price, lastSell.Amount) {
					go api.MustCancel(market, symbol, lastSell.OrderId, true)
					refreshOrders.SetLastOrder(market, symbol, model.OrderSideSell, nil)
					time.Sleep(time.Second * 2)
					api.RefreshAccount(market)
				}
			} else {
				go api.MustCancel(market, symbol, lastSell.OrderId, true)
				refreshOrders.SetLastOrder(market, symbol, model.OrderSideSell, nil)
			}
			refreshOrders.SetLastOrder(market, symbol, model.OrderSideSell, nil)
			refreshOrders.SetLastOrder(market, symbol, model.OrderSideBuy, nil)
		} else if lastBuy != nil && lastSell == nil {
			if bidPrice-lastBuy.Price < priceDistance && bidAmount < amount*(1.01+model.AppConfig.RefreshLimit) {
				if !placeSeparateOrder(model.OrderSideSell, market, symbol, lastBuy.Price, lastBuy.Amount) {
					go api.MustCancel(market, symbol, lastBuy.OrderId, true)
					refreshOrders.SetLastOrder(market, symbol, model.OrderSideBuy, nil)
					time.Sleep(time.Second * 2)
					api.RefreshAccount(market)
				}
			} else {
				go api.MustCancel(market, symbol, lastBuy.OrderId, true)
				refreshOrders.SetLastOrder(market, symbol, model.OrderSideBuy, nil)
			}
			refreshOrders.SetLastOrder(market, symbol, model.OrderSideSell, nil)
			refreshOrders.SetLastOrder(market, symbol, model.OrderSideBuy, nil)
		}
	}
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
	refreshOrders.Add(market, symbol, orderSide, order)
	refreshOrders.SetLastOrder(market, symbol, orderSide, order)
	model.AppDB.Save(order)
	syncRefresh <- struct{}{}
}

func placeSeparateOrder(orderSide, market, symbol string, price, amount float64) (result bool) {
	lastRefreshTime = util.GetNowUnixMillion()
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, price, amount)
	if order.OrderId == `` || order.Status == model.CarryStatusFail {
		order = api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, price, amount)
	}
	if order.Status == model.CarryStatusWorking {
		order.Function = model.FunctionRefresh
		refreshOrders.Add(market, symbol, orderSide, order)
		refreshOrders.SetLastOrder(market, symbol, orderSide, order)
		model.AppDB.Save(order)
		return true
	}
	return false
}
