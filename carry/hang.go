package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"time"
)

var hangStatus = HangStatus{}

type HangStatus struct {
	hanging map[string]bool // symbol - *time
	//lastHangTime map[string]*time.Time   // symbol - *time
	bid map[string][]*model.Order // symbol - *orders
	ask map[string][]*model.Order // symbol - *orders
}

func (hangStatus *HangStatus) getHangOrders(symbol string) (bid, ask []*model.Order) {
	if hangStatus.bid == nil || hangStatus.ask == nil {
		return nil, nil
	}
	return hangStatus.bid[symbol], hangStatus.ask[symbol]
}

func (hangStatus *HangStatus) setHangOrders(symbol string, bid, ask []*model.Order) {
	if hangStatus.bid == nil {
		hangStatus.bid = make(map[string][]*model.Order)
	}
	if hangStatus.ask == nil {
		hangStatus.ask = make(map[string][]*model.Order)
	}
	hangStatus.bid[symbol] = bid
	hangStatus.ask[symbol] = ask
}

func (hangStatus *HangStatus) getHanging(symbol string) bool {
	if hangStatus.hanging == nil {
		return false
	}
	return hangStatus.hanging[symbol]
}

func (hangStatus *HangStatus) setHanging(symbol string, value bool) {
	if hangStatus.hanging == nil {
		hangStatus.hanging = make(map[string]bool)
	}
	hangStatus.hanging[symbol] = value
}

var ProcessHang = func(market, symbol string) {
	if hangStatus.getHanging(symbol) || model.AppConfig.Handle != `1` {
		return
	}
	hangStatus.setHanging(symbol, true)
	defer hangStatus.setHanging(symbol, false)
	setting := model.GetSetting(model.FunctionHang, market, symbol)
	bidAsk := model.AppMarkets.BidAsks[symbol][market]
	if len(bidAsk.Asks) == 0 || bidAsk.Bids.Len() == 0 {
		return
	}
	delay := util.GetNowUnixMillion() - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
	if delay > 500 {
		util.Notice(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	now := util.GetNow()
	d, _ := time.ParseDuration("-3610s")
	lastMin60 := now.Add(d)
	d, _ = time.ParseDuration(`-1800`)
	lastMin30 := now.Add(d)
	newBids := make([]*model.Order, 0)
	newAsks := make([]*model.Order, 0)
	bids, asks := hangStatus.getHangOrders(symbol)
	for _, value := range bids {
		if value.Price-bidAsk.Bids[0].Price > 0.1*priceDistance {
			continue // 已经成交
		}
		if (bidAsk.Bids[0].Price-13*priceDistance-value.Price > 0.1*priceDistance &&
			value.OrderTime.After(lastMin30)) || value.OrderTime.Before(lastMin60) {
			api.MustCancel(market, symbol, value.OrderId, true)
		} else {
			newBids = append(newBids, value)
		}
	}
	for _, value := range asks {
		if bidAsk.Asks[0].Price-value.Price > 0.1*priceDistance {
			continue
		}
		if (value.Price-13*priceDistance-bidAsk.Asks[0].Price > 0.1*priceDistance &&
			value.OrderTime.After(lastMin30)) || value.OrderTime.Before(lastMin60) {
			api.MustCancel(market, symbol, value.OrderId, true)
		} else {
			newAsks = append(newAsks, value)
		}
	}
	leftFree, rightFree, leftFroze, rightFroze, err := getBalance(market, symbol, setting.AccountType)
	leftFree = leftFree * model.AppConfig.AmountRate
	rightFree = rightFree / bidAsk.Bids[0].Price * model.AppConfig.AmountRate
	if err == nil {
		if bidAsk.Bids[0].Amount > bidAsk.Asks[0].Amount {
			hangBids(newBids, bidAsk, market, symbol, rightFree, rightFroze, priceDistance)
			hangAsks(newAsks, bidAsk, market, symbol, leftFree, leftFroze, priceDistance)
		} else {
			hangAsks(newAsks, bidAsk, market, symbol, leftFree, leftFroze, priceDistance)
			hangBids(newBids, bidAsk, market, symbol, rightFree, rightFroze, priceDistance)
		}
	}
	hangStatus.setHangOrders(symbol, newBids, newAsks)
	api.RefreshAccount(market)
}

func hangAsks(orders []*model.Order, bidAsk *model.BidAsk, market, symbol string, free, froze, priceDistance float64) {
	if free > (1-model.AppConfig.AmountRate)*froze {
		if bidAsk.Bids[0].Amount*2 < bidAsk.Asks[0].Amount {
			placeHangOrder(orders, model.OrderSideSell, market, symbol, bidAsk.Asks[0].Price, free/3)
			placeHangOrder(orders, model.OrderSideSell, market, symbol, bidAsk.Asks[0].Price+3*priceDistance, free/3)
			placeHangOrder(orders, model.OrderSideSell, market, symbol, bidAsk.Asks[0].Price+9*priceDistance, free/3)
		} else {
			placeHangOrder(orders, model.OrderSideSell, market, symbol, bidAsk.Asks[0].Price+3*priceDistance, free/2)
			placeHangOrder(orders, model.OrderSideSell, market, symbol, bidAsk.Asks[0].Price+9*priceDistance, free/2)
		}
	}
}

func hangBids(orders []*model.Order, bidAsk *model.BidAsk, market, symbol string, free, froze, priceDistance float64) {
	if free > (1-model.AppConfig.AmountRate)*froze {
		if bidAsk.Asks[0].Amount*2 < bidAsk.Bids[0].Amount {
			placeHangOrder(orders, model.OrderSideBuy, market, symbol, bidAsk.Bids[0].Price, free/3)
			placeHangOrder(orders, model.OrderSideBuy, market, symbol, bidAsk.Bids[0].Price-3*priceDistance, free/3)
			placeHangOrder(orders, model.OrderSideBuy, market, symbol, bidAsk.Bids[0].Price-9*priceDistance, free/3)
		} else {
			placeHangOrder(orders, model.OrderSideBuy, market, symbol, bidAsk.Bids[0].Price-3*priceDistance, free/2)
			placeHangOrder(orders, model.OrderSideBuy, market, symbol, bidAsk.Bids[0].Price-9*priceDistance, free/2)
		}
	}
}

func placeHangOrder(orders []*model.Order, orderSide, market, symbol string, price, amount float64) {
	if amount*price < 5 {
		return
	}
	setting := model.GetSetting(model.FunctionHang, market, symbol)
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``,
		setting.AccountType, price, amount)
	if order.Status != model.CarryStatusFail && order.OrderId != `` {
		orders = append(orders, order)
		model.AppDB.Save(order)
		time.Sleep(time.Millisecond * 20)
	}
}
