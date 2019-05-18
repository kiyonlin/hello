package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"sync"
)

var carrying bool
var targetCarries = &TargetCarries{}

type TargetCarries struct {
	lock    sync.Mutex
	targets map[string]map[string]map[string]*model.Order // symbol - market - targetMarket - order
}

func (targetCarries *TargetCarries) set(market, targetMarket, symbol string, order *model.Order) {
	targetCarries.lock.Lock()
	defer targetCarries.lock.Unlock()
	if targetCarries.targets == nil {
		targetCarries.targets = make(map[string]map[string]map[string]*model.Order)
	}
	if targetCarries.targets[symbol] == nil {
		targetCarries.targets[symbol] = make(map[string]map[string]*model.Order)
	}
	if targetCarries.targets[symbol][market] == nil {
		targetCarries.targets[symbol][market] = make(map[string]*model.Order)
	}
	targetCarries.targets[symbol][market][targetMarket] = order
}

func (targetCarries *TargetCarries) get(market, targetMarket, symbol string) (order *model.Order) {
	targetCarries.lock.Lock()
	defer targetCarries.lock.Unlock()
	if targetCarries.targets == nil || targetCarries.targets[symbol] == nil ||
		targetCarries.targets[symbol][market] == nil {
		return nil
	}
	return targetCarries.targets[symbol][market][targetMarket]
}

func setCarrying(value bool) {
	carrying = value
}

var ProcessCarry = func(market, symbol string) {
	if carrying {
		return
	}
	setCarrying(true)
	defer setCarrying(false)
	setting := model.GetSetting(model.FunctionCarry, market, symbol)
	if setting == nil || setting.FunctionParameter == `` {
		return
	}
	result, tick := model.AppMarkets.GetBidAsk(symbol, market)
	targetResult, targetTick := model.AppMarkets.GetBidAsk(symbol, setting.FunctionParameter)
	if !result || !targetResult || targetTick == nil {
		return
	}
	targetOrder := targetCarries.get(market, setting.FunctionParameter, symbol)
	if targetOrder != nil {
		queryOrder := api.QueryOrderById(setting.FunctionParameter, symbol, targetOrder.OrderId)
		orderSide := model.OrderSideSell
		if targetOrder.OrderSide == model.OrderSideSell {
			orderSide = model.OrderSideBuy
		}
		amount := queryOrder.DealAmount - targetOrder.DealAmount
		order := api.PlaceOrder(orderSide, model.OrderTypeMarket, market, symbol, ``,
			setting.AccountType, targetOrder.Price, amount)
		if order.Status == model.CarryStatusWorking && order.OrderId != `` {
			targetCarries.set(market, setting.FunctionParameter, symbol, order)
		}
	} else {
		var targetOrderSide string
		var targetPrice, targetAmount float64
		now := util.GetNowUnixMillion()
		if now-int64(targetTick.Ts) > 1000 || now-int64(tick.Ts) > 1000 {
			util.Notice(fmt.Sprintf(`[dealy too long]%d - %d`, now-int64(targetTick.Ts), now-int64(tick.Ts)))
			return
		}
		if (tick.Bids[0].Price-targetTick.Bids[0].Price)/tick.Bids[0].Price > model.AppConfig.CarryDistance {
			targetOrderSide = model.OrderSideBuy
			targetPrice = targetTick.Bids[0].Price
			targetAmount = targetTick.Bids[0].Amount
		} else if (targetTick.Bids[0].Price-tick.Bids[0].Price)/tick.Bids[0].Price > model.AppConfig.CarryDistance {
			targetOrderSide = model.OrderSideSell
			targetPrice = targetTick.Asks[0].Price
			targetAmount = targetTick.Asks[0].Amount
		}
		if targetOrderSide == `` {
			return
		}
		order := api.PlaceOrder(targetOrderSide, model.OrderTypeLimit, setting.FunctionParameter, symbol,
			``, ``, targetPrice, targetAmount)
		if order.Status == model.CarryStatusWorking && order.OrderId != `` {
			targetCarries.set(market, setting.FunctionParameter, symbol, order)
		}
	}
}
