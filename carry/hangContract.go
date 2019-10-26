package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"sync"
	"time"
)

var contractHoldingUpdate = int64(0)

type HangContractOrders struct {
	lock            sync.Mutex
	hangingContract bool
	holdingLong     float64
	holdingShort    float64
	orders          map[string]map[string]*model.Order // symbol - order id - order
}

var hangContractOrders = &HangContractOrders{}

func (hangContractOrders *HangContractOrders) setInHangingContract(in bool) {
	hangContractOrders.hangingContract = in
}

func (hangContractOrders *HangContractOrders) getHangContractOrders(symbol string) (orders map[string]*model.Order) {
	defer hangContractOrders.lock.Unlock()
	hangContractOrders.lock.Lock()
	if hangContractOrders.orders == nil {
		hangContractOrders.orders = make(map[string]map[string]*model.Order)
	}
	return hangContractOrders.orders[symbol]
}

func (hangContractOrders *HangContractOrders) setHangContractOrders(symbol string, orders map[string]*model.Order) {
	defer hangContractOrders.lock.Unlock()
	hangContractOrders.lock.Lock()
	if hangContractOrders.orders == nil {
		hangContractOrders.orders = make(map[string]map[string]*model.Order)
	}
	hangContractOrders.orders[symbol] = orders
}

var ProcessHangContract = func(market, symbol string) {
	start := util.GetNowUnixMillion()
	_, tick := model.AppMarkets.GetBidAsk(symbol, market)
	if tick == nil || tick.Asks == nil || tick.Bids == nil || tick.Asks.Len() < 15 || tick.Bids.Len() < 15 ||
		int(start)-tick.Ts > 500 {
		timeDis := 0
		if tick != nil {
			timeDis = int(start) - tick.Ts
		}
		util.Notice(fmt.Sprintf(`[tick not good time]%s %s %d`, market, symbol, timeDis))
		CancelHangContract(key, secret, symbol)
		return
	}
	setting := model.GetSetting(model.FunctionHangContract, market, symbol)
	if util.GetNowUnixMillion()-int64(tick.Ts) > 1000 {
		util.SocketInfo(fmt.Sprintf(`socekt old tick %d %d`, util.GetNowUnixMillion(), tick.Ts))
		CancelHangContract(key, secret, symbol)
		return
	}
	if model.AppConfig.Handle != `1` || model.AppPause {
		util.Notice(fmt.Sprintf(`[status]%s is pause:%v`, model.AppConfig.Handle, model.AppPause))
		CancelHangContract(key, secret, symbol)
		return
	}
	if hangContractOrders.hangingContract {
		return
	}
	hangContractOrders.setInHangingContract(true)
	defer hangContractOrders.setInHangingContract(false)
	if start-contractHoldingUpdate > 3000 {
		updateContractHolding(market, symbol)
	} else {
		hangContract(key, secret, market, symbol, setting.AccountType, setting, tick)
		validHangContract(key, secret, symbol, setting, tick)
	}
}

func updateContractHolding(market, symbol string) {
	api.RefreshAccount(key, secret, market)
	account := model.AppAccounts.GetAccount(market, symbol)
	if account.Direction == model.OrderSideBuy {
		hangContractOrders.holdingLong = account.Free
	}
	if account.Direction == model.OrderSideSell {
		hangContractOrders.holdingShort = account.Free
	}
	contractHoldingUpdate = util.GetNowUnixMillion()
}

func calcPending(orders map[string]*model.Order) (pendingLong, pendingShort float64) {
	pendingLong = 0.0
	pendingShort = 0.0
	for _, order := range orders {
		if order.OrderSide == model.OrderSideBuy {
			pendingLong += order.Amount
		}
		if order.OrderSide == model.OrderSideSell {
			pendingShort += order.Amount
		}
	}
	return pendingLong, pendingShort
}

func hangContract(key, secret, market, symbol, accountType string, setting *model.Setting, tick *model.BidAsk) {
	orders := hangContractOrders.getHangContractOrders(symbol)
	pendingLong, pendingShort := calcPending(orders)
	if hangContractOrders.holdingLong+pendingLong < setting.AmountLimit {
		util.Notice(fmt.Sprintf(`=not limit= %s %f + %f < %f place at %f %f`,
			symbol, model.OrderSideBuy, hangContractOrders.holdingLong, pendingLong, setting.AmountLimit,
			tick.Bids[0].Price, tick.Asks[0].Price))
		order := api.PlaceOrder(key, secret, model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``,
			accountType, tick.Bids[0].Price, setting.GridAmount)
		if order != nil && order.OrderId != `` {
			orders[order.OrderId] = order
			model.AppDB.Save(&order)
		}
	}
	if hangContractOrders.holdingShort+pendingShort < setting.AmountLimit {
		util.Notice(fmt.Sprintf(`=not limit= %s %s %f + %f < %f place at %f %f`,
			symbol, model.OrderSideBuy, hangContractOrders.holdingLong, pendingLong, setting.AmountLimit,
			tick.Bids[0].Price, tick.Asks[0].Price))
		order := api.PlaceOrder(key, secret, model.OrderSideSell, model.OrderTypeLimit, market, symbol, ``,
			accountType, tick.Asks[0].Price, setting.GridAmount)
		if order != nil && order.OrderId != `` {
			orders[order.OrderId] = order
			model.AppDB.Save(&order)
		}
	}
	hangContractOrders.setHangContractOrders(symbol, orders)
}

func validHangContract(key, secret, symbol string, setting *model.Setting, tick *model.BidAsk) {
	orders := hangContractOrders.getHangContractOrders(symbol)
	pendingLong, pendingShort := calcPending(orders)
	orderFilterLimit := make(map[string]*model.Order)
	if pendingLong+hangContractOrders.holdingLong >= setting.AmountLimit {
		for orderId, order := range orders {
			if order.OrderSide == model.OrderSideBuy {
				util.Notice(fmt.Sprintf(`=reduce hang= %f + %f >= %f %s %s`,
					pendingLong, hangContractOrders.holdingLong, setting.AmountLimit, order.OrderSide, orderId))
				api.MustCancel(key, secret, model.Fmex, symbol, orderId, true)
				time.Sleep(time.Millisecond * 50)
			} else {
				orderFilterLimit[orderId] = order
			}
		}
	} else if pendingShort+hangContractOrders.holdingShort >= setting.AmountLimit {
		for orderId, order := range orders {
			if order.OrderSide == model.OrderSideSell {
				util.Notice(fmt.Sprintf(`=reduce hang= %f + %f >= %f %s %s`,
					pendingShort, hangContractOrders.holdingShort, setting.AmountLimit, order.OrderSide, orderId))
				api.MustCancel(key, secret, model.Fmex, symbol, orderId, true)
				time.Sleep(time.Millisecond * 50)
			} else {
				orderFilterLimit[orderId] = order
			}
		}
	}
	orderFilterPrice := make(map[string]*model.Order)
	for orderId, order := range orderFilterLimit {
		if (order.OrderSide == model.OrderSideBuy && order.Price < tick.Bids[0].Price*0.96) || (order.OrderSide == model.OrderSideSell && order.Price > tick.Asks[0].Price*1.04) {
			util.Notice(fmt.Sprintf(`=price out of range= %s %f %f-%f`,
				orderId, order.Price, tick.Bids[0].Price, tick.Asks[0].Price))
			api.MustCancel(key, secret, model.Fmex, symbol, orderId, true)
			time.Sleep(time.Millisecond * 50)
		} else {
			orderFilterPrice[orderId] = order
		}
	}
	hangContractOrders.setHangContractOrders(symbol, orderFilterPrice)
}

func CancelHangContract(key, secret, symbol string) {
	util.Notice(`cancel all hang contract`)
	orders := hangContractOrders.getHangContractOrders(symbol)
	for _, order := range orders {
		if order != nil && order.OrderId != `` {
			api.MustCancel(key, secret, model.Fmex, symbol, order.OrderId, true)
			time.Sleep(time.Millisecond * 50)
		}
	}
	hangContractOrders.setHangContractOrders(symbol, nil)
}
