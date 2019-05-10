package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strconv"
	"strings"
	"sync"
)

var hangStatus = &HangStatus{}

type HangStatus struct {
	lock    sync.Mutex
	hanging map[string]bool                            // symbol - hanging
	orders  map[string]map[string]map[int]*model.Order // symbol - orderSide - 1/5/15 - order
}

func (hangStatus *HangStatus) setHanging(symbol string, value bool) (current bool) {
	if hangStatus.hanging == nil {
		hangStatus.hanging = make(map[string]bool)
	}
	current = hangStatus.hanging[symbol]
	hangStatus.hanging[symbol] = current
	return current
}

func (hangStatus *HangStatus) setOrder(symbol, orderSide string, index int, order *model.Order) {
	hangStatus.lock.Lock()
	defer hangStatus.lock.Unlock()
	if hangStatus.orders == nil {
		hangStatus.orders = make(map[string]map[string]map[int]*model.Order)
	}
	if hangStatus.orders[symbol] == nil {
		hangStatus.orders[symbol] = make(map[string]map[int]*model.Order)
	}
	if hangStatus.orders[symbol][orderSide] == nil {
		hangStatus.orders[symbol][orderSide] = make(map[int]*model.Order)
	}
	hangStatus.orders[symbol][orderSide][index] = order
}

func (hangStatus *HangStatus) getOrder(symbol, orderSide string, index int) (order *model.Order) {
	hangStatus.lock.Lock()
	defer hangStatus.lock.Unlock()
	if hangStatus.orders == nil || hangStatus.orders[symbol] == nil || hangStatus.orders[symbol][orderSide] == nil ||
		hangStatus.orders[symbol][orderSide][index] == nil {
		return nil
	}
	return hangStatus.orders[symbol][orderSide][index]
}

var ProcessHang = func(market, symbol string) {
	if model.AppMarkets.BidAsks[symbol] == nil {
		return
	}
	tick := model.AppMarkets.BidAsks[symbol][market]
	if tick == nil || tick.Asks == nil || tick.Bids == nil || tick.Asks.Len() < 15 || tick.Bids.Len() < 15 {
		util.Notice(fmt.Sprintf(`[tick not good]%s %s`, market, symbol))
		return
	}
	go validHang(market, symbol, tick)
	if hangStatus.setHanging(symbol, true) || model.AppConfig.Handle != `1` {
		return
	}
	defer hangStatus.setHanging(symbol, false)
	delay := util.GetNowUnixMillion() - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
	if delay > 200 {
		util.Notice(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
	setting := model.GetSetting(model.FunctionHang, market, symbol)
	hang(market, symbol, setting, tick)
}

func hang(market, symbol string, setting *model.Setting, tick *model.BidAsk) {
	amounts := strings.Split(setting.FunctionParameter, `_`)
	if len(amounts) != 3 {
		util.Notice(fmt.Sprintf(`[hang para error]%s %s len%d`, symbol, setting.FunctionParameter, len(amounts)))
		return
	}
	amount1, err1 := strconv.ParseFloat(amounts[0], 64)
	amount5, err5 := strconv.ParseFloat(amounts[1], 64)
	amount15, err15 := strconv.ParseFloat(amounts[2], 64)
	if err1 != nil || err5 != nil || err15 != nil {
		util.Notice(fmt.Sprintf(`[hang para error]%s %s`, symbol, setting.FunctionParameter))
		return
	}
	if hangStatus.getOrder(symbol, model.OrderSideBuy, 1) == nil {
		go placeHangOrder(model.OrderSideBuy, market, symbol, setting.AccountType, tick.Bids[0].Price, amount1, 1)
	}
	if hangStatus.getOrder(symbol, model.OrderSideBuy, 5) == nil {
		go placeHangOrder(model.OrderSideBuy, market, symbol, setting.AccountType, tick.Bids[4].Price, amount5, 5)
	}
	if hangStatus.getOrder(symbol, model.OrderSideBuy, 15) == nil {
		go placeHangOrder(model.OrderSideBuy, market, symbol, setting.AccountType, tick.Bids[14].Price, amount15, 15)
	}
	if hangStatus.getOrder(symbol, model.OrderSideSell, 1) == nil {
		go placeHangOrder(model.OrderSideSell, market, symbol, setting.AccountType, tick.Asks[0].Price, amount1, 1)
	}
	if hangStatus.getOrder(symbol, model.OrderSideSell, 5) == nil {
		go placeHangOrder(model.OrderSideSell, market, symbol, setting.AccountType, tick.Asks[4].Price, amount5, 5)
	}
	if hangStatus.getOrder(symbol, model.OrderSideSell, 15) == nil {
		go placeHangOrder(model.OrderSideSell, market, symbol, setting.AccountType, tick.Asks[14].Price, amount15, 15)
	}
}

func validHang(market, symbol string, tick *model.BidAsk) {
	priceDistance := 0.1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	bid1 := hangStatus.getOrder(symbol, model.OrderSideBuy, 1)
	bid5 := hangStatus.getOrder(symbol, model.OrderSideBuy, 5)
	bid15 := hangStatus.getOrder(symbol, model.OrderSideBuy, 15)
	ask1 := hangStatus.getOrder(symbol, model.OrderSideSell, 1)
	ask5 := hangStatus.getOrder(symbol, model.OrderSideSell, 5)
	ask15 := hangStatus.getOrder(symbol, model.OrderSideSell, 15)
	if bid1 != nil && math.Abs(bid1.Price-tick.Bids[0].Price) >= priceDistance {
		go api.MustCancel(bid1.Market, symbol, bid1.OrderId, true)
		hangStatus.setOrder(symbol, bid1.OrderSide, 1, nil)
	}
	if bid5 != nil && math.Abs(bid5.Price-tick.Bids[4].Price) >= priceDistance {
		go api.MustCancel(bid5.Market, symbol, bid5.OrderId, true)
		hangStatus.setOrder(symbol, bid5.OrderSide, 5, nil)
	}
	if bid15 != nil && math.Abs(bid15.Price-tick.Bids[14].Price) >= priceDistance {
		go api.MustCancel(bid15.Market, symbol, bid15.OrderId, true)
		hangStatus.setOrder(symbol, bid15.OrderSide, 15, nil)
	}
	if ask1 != nil && math.Abs(ask1.Price-tick.Asks[0].Price) >= priceDistance {
		go api.MustCancel(ask1.Market, symbol, ask1.OrderId, true)
		hangStatus.setOrder(symbol, ask1.OrderSide, 1, nil)
	}
	if ask5 != nil && math.Abs(ask5.Price-tick.Asks[4].Price) >= priceDistance {
		go api.MustCancel(ask5.Market, symbol, ask5.OrderId, true)
		hangStatus.setOrder(symbol, ask5.OrderSide, 5, nil)
	}
	if ask15 != nil && math.Abs(ask15.Price-tick.Asks[14].Price) >= priceDistance {
		go api.MustCancel(ask15.Market, symbol, ask15.OrderId, true)
		hangStatus.setOrder(symbol, ask15.OrderSide, 15, nil)
	}
}

func placeHangOrder(orderSide, market, symbol, accountType string, price, amount float64, index int) {
	if amount <= 0 {
		return
	}
	util.Notice(fmt.Sprintf(`[hang %s %s]price: %f amount: %f`, symbol, orderSide, price, amount))
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``,
		accountType, price, amount)
	if order != nil && order.Status != model.CarryStatusFail && order.OrderId != `` {
		order.Function = model.FunctionHang
		model.AppDB.Save(&order)
		hangStatus.setOrder(symbol, orderSide, index, order)
	}
}
