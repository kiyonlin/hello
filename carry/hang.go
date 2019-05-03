package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"time"
)

var hangingOrders = make(map[string][]*model.Order, 0) // symbol - order list
var hanging = false

func setHanging(value bool) {
	hanging = value
}

var ProcessHang = func(market, symbol string) {
	if hanging || model.AppConfig.Handle != `1` {
		return
	}
	setHanging(true)
	defer setHanging(false)
	bidAsk := model.AppMarkets.BidAsks[symbol][market]
	if len(bidAsk.Asks) == 0 || bidAsk.Bids.Len() == 0 {
		return
	}
	delay := util.GetNowUnixMillion() - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
	if delay > 200 {
		util.Notice(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
	hang(market, symbol, bidAsk)
}

func cancelHang(market, symbol string) {
	for _, value := range hangingOrders[symbol] {
		api.MustCancel(market, symbol, value.OrderId, true)
		util.Notice(fmt.Sprintf(`[cancel hang]%s %s`, market, symbol))
		time.Sleep(time.Millisecond * 20)
	}
}

func hang(market, symbol string, bidAsk *model.BidAsk) {
	now := util.GetNow()
	setting := model.GetSetting(model.FunctionHang, market, symbol)
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	d, _ := time.ParseDuration("-3610s")
	//lastMin60 := now.Add(d)
	d, _ = time.ParseDuration(`-1800s`)
	lastMin30 := now.Add(d)
	newHangingOrders := make([]*model.Order, 0)
	for _, value := range hangingOrders[symbol] {
		if value.OrderSide == model.OrderSideBuy {
			if value.Price-bidAsk.Bids[0].Price > 0.1*priceDistance {
				continue // 已经成交
			}
			if bidAsk.Bids[0].Price-13*priceDistance-value.Price > 0.1*priceDistance &&
				value.OrderTime.After(lastMin30) {
				api.MustCancel(market, symbol, value.OrderId, true)
			} else {
				newHangingOrders = append(newHangingOrders, value)
			}
		} else if value.OrderSide == model.OrderSideSell {
			if bidAsk.Asks[0].Price-value.Price > 0.1*priceDistance {
				continue
			}
			if value.Price-13*priceDistance-bidAsk.Asks[0].Price > 0.1*priceDistance &&
				value.OrderTime.After(lastMin30) {
				api.MustCancel(market, symbol, value.OrderId, true)
			} else {
				newHangingOrders = append(newHangingOrders, value)
			}
		}
	}
	hangingOrders[symbol] = newHangingOrders
	leftFree, rightFree, leftFroze, rightFroze, err := getBalance(market, symbol, setting.AccountType)
	price := bidAsk.Asks[0].Price
	if err == nil && leftFroze*price+rightFroze <
		((leftFree+leftFroze)*price+rightFree+rightFroze)*model.AppConfig.AmountRate {
		leftFree = leftFree * 0.99
		rightFree = rightFree / price * 0.99
		if bidAsk.Bids[0].Amount > bidAsk.Asks[0].Amount {
			hangBids(bidAsk, market, symbol, rightFree, priceDistance)
			hangAsks(bidAsk, market, symbol, leftFree, priceDistance)
		} else {
			hangAsks(bidAsk, market, symbol, leftFree, priceDistance)
			hangBids(bidAsk, market, symbol, rightFree, priceDistance)
		}
	}
	api.RefreshAccount(market)
}
func hangAsks(bidAsk *model.BidAsk, market, symbol string, free, priceDistance float64) {
	if bidAsk.Bids[0].Amount*2 < bidAsk.Asks[0].Amount {
		placeHangOrder(model.OrderSideSell, market, symbol, bidAsk.Asks[0].Price, free/3)
		placeHangOrder(model.OrderSideSell, market, symbol, bidAsk.Asks[0].Price+3*priceDistance, free/3)
		placeHangOrder(model.OrderSideSell, market, symbol, bidAsk.Asks[0].Price+9*priceDistance, free/3)
	} else {
		placeHangOrder(model.OrderSideSell, market, symbol, bidAsk.Asks[0].Price+3*priceDistance, free/2)
		placeHangOrder(model.OrderSideSell, market, symbol, bidAsk.Asks[0].Price+9*priceDistance, free/2)
	}
}

func hangBids(bidAsk *model.BidAsk, market, symbol string, free, priceDistance float64) {
	if bidAsk.Asks[0].Amount*2 < bidAsk.Bids[0].Amount {
		placeHangOrder(model.OrderSideBuy, market, symbol, bidAsk.Bids[0].Price, free/3)
		placeHangOrder(model.OrderSideBuy, market, symbol, bidAsk.Bids[0].Price-3*priceDistance, free/3)
		placeHangOrder(model.OrderSideBuy, market, symbol, bidAsk.Bids[0].Price-9*priceDistance, free/3)
	} else {
		placeHangOrder(model.OrderSideBuy, market, symbol, bidAsk.Bids[0].Price-3*priceDistance, free/2)
		placeHangOrder(model.OrderSideBuy, market, symbol, bidAsk.Bids[0].Price-9*priceDistance, free/2)
	}
}

func placeHangOrder(orderSide, market, symbol string, price, amount float64) {
	if amount*price < 5 {
		return
	}
	setting := model.GetSetting(model.FunctionHang, market, symbol)
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``,
		setting.AccountType, price, amount)
	if order.Status != model.CarryStatusFail && order.OrderId != `` {
		model.AppDB.Save(order)
		time.Sleep(time.Millisecond * 20)
		hangingOrders[symbol] = append(hangingOrders[symbol], order)
	}
}
