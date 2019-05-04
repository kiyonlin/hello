package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
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
	if model.AppMarkets.BidAsks[symbol] == nil {
		return
	}
	tick := model.AppMarkets.BidAsks[symbol][market]
	if tick == nil || tick.Asks == nil || tick.Bids == nil || tick.Asks.Len() < 15 || tick.Bids.Len() < 15 {
		util.Notice(fmt.Sprintf(`[tick not good]%s %s`, market, symbol))
		return
	}
	delay := util.GetNowUnixMillion() - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
	if delay > 200 {
		util.Notice(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
	setting := model.GetSetting(model.FunctionHang, market, symbol)
	if validHang(market, symbol, tick) {
		time.Sleep(time.Second)
		api.RefreshAccount(market)
		return
	}
	hang(market, symbol, setting.AccountType, tick)
}

func hang(market, symbol, accountType string, tick *model.BidAsk) {
	leftFree, rightFree, leftFroze, rightFroze, err := getBalance(market, symbol, accountType)
	if err != nil {
		return
	}
	rightFree = rightFree / tick.Asks[0].Price
	rightFroze = rightFroze / tick.Asks[0].Price
	if rightFroze+leftFroze < (leftFree+leftFroze+rightFree+rightFroze)*model.AppConfig.AmountRate {
		placeHangOrder(model.OrderSideSell, market, symbol, accountType,
			tick.Asks[3].Price, leftFree*model.AppConfig.AmountRate/2)
		placeHangOrder(model.OrderSideSell, market, symbol, accountType,
			tick.Asks[9].Price, leftFree*model.AppConfig.AmountRate/2)
		placeHangOrder(model.OrderSideBuy, market, symbol, accountType,
			tick.Bids[3].Price, rightFree*model.AppConfig.AmountRate/2)
		placeHangOrder(model.OrderSideBuy, market, symbol, accountType,
			tick.Bids[9].Price, rightFree*model.AppConfig.AmountRate/2)
	}
}

func validHang(market, symbol string, tick *model.BidAsk) (needCancel bool) {
	needCancel = false
	if hangingOrders == nil || hangingOrders[symbol] == nil {
		return needCancel
	}
	newHangOrders := make([]*model.Order, 0)
	for _, value := range hangingOrders[symbol] {
		if value != nil && value.OrderSide == model.OrderSideBuy {
			if value.Price > tick.Bids[14].Price && value.Price < tick.Bids[1].Price {
				newHangOrders = append(newHangOrders, value)
			} else {
				needCancel = true
				api.MustCancel(market, symbol, value.OrderId, true)
			}
		}
		if value != nil && value.OrderSide == model.OrderSideSell {
			if value.Price < tick.Asks[14].Price && value.Price > tick.Asks[1].Price {
				newHangOrders = append(newHangOrders, value)
			} else {
				needCancel = true
				api.MustCancel(market, symbol, value.OrderId, true)
			}
		}
	}
	refreshOrders.setRefreshHang(symbol, newHangOrders)
	return needCancel
}

func placeHangOrder(orderSide, market, symbol, accountType string, price, amount float64) {
	if amount*price < 5 {
		return
	}
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``,
		accountType, price, amount)
	if order != nil && order.Status != model.CarryStatusFail && order.OrderId != `` {
		model.AppDB.Save(order)
		time.Sleep(time.Millisecond * 20)
		hangingOrders := refreshOrders.getRefreshHang(symbol)
		hangingOrders = append(hangingOrders, order)
		refreshOrders.setRefreshHang(symbol, hangingOrders)
	}
}
