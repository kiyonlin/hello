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

var hangStatus = &HangStatus{bid: make(map[string]*model.Order), ask: make(map[string]*model.Order)}

type HangStatus struct {
	lock    sync.Mutex
	hanging bool
	bid     map[string]*model.Order
	ask     map[string]*model.Order
}

func (hangStatus *HangStatus) setHanging(value bool) {
	hangStatus.hanging = value
}

func (hangStatus *HangStatus) getBid(symbol string) (order *model.Order) {
	hangStatus.lock.Lock()
	defer hangStatus.lock.Unlock()
	if hangStatus.bid == nil {
		hangStatus.bid = make(map[string]*model.Order)
	}
	return hangStatus.bid[symbol]
}

func (hangStatus *HangStatus) getAsk(symbol string) (order *model.Order) {
	hangStatus.lock.Lock()
	defer hangStatus.lock.Unlock()
	if hangStatus.ask == nil {
		hangStatus.ask = make(map[string]*model.Order)
	}
	return hangStatus.ask[symbol]
}

func (hangStatus *HangStatus) setAsk(symbol string, order *model.Order) {
	hangStatus.lock.Lock()
	defer hangStatus.lock.Unlock()
	if hangStatus.ask == nil {
		hangStatus.ask = make(map[string]*model.Order)
	}
	hangStatus.ask[symbol] = order
}

func (hangStatus *HangStatus) setBid(symbol string, order *model.Order) {
	hangStatus.lock.Lock()
	defer hangStatus.lock.Unlock()
	if hangStatus.bid == nil {
		hangStatus.bid = make(map[string]*model.Order)
	}
	hangStatus.bid[symbol] = order
}

var ProcessHang = func(market, symbol string) {
	result, tick := model.AppMarkets.GetBidAsk(symbol, market)
	if !result || tick == nil || tick.Asks == nil || tick.Bids == nil || tick.Asks.Len() < 11 ||
		tick.Bids.Len() < 11 {
		util.Notice(fmt.Sprintf(`[tick not good]%s %s`, market, symbol))
		return
	}
	if hangStatus.hanging || model.AppConfig.Handle != `1` || model.AppPause {
		return
	}
	hangStatus.setHanging(true)
	defer hangStatus.setHanging(false)
	delay := util.GetNowUnixMillion() - int64(tick.Ts)
	if delay > 500 {
		util.Notice(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
	setting := model.GetSetting(model.FunctionHang, market, symbol)
	if setting == nil {
		return
	}
	leftFree, rightFree, _, _, err := getBalance(key, secret, market, symbol, setting.AccountType)
	if err != nil {
		return
	}
	bid := hangStatus.getBid(symbol)
	ask := hangStatus.getAsk(symbol)
	didSmth := false
	if bid == nil || bid.Price > tick.Bids[0].Price {
		didSmth = true
		bid = api.PlaceOrder(``, ``, model.OrderSideBuy, model.OrderTypeLimit, market, symbol,
			setting.AccountType, setting.AccountType, tick.Bids[0].Price,
			math.Min(tick.Bids[0].Amount, rightFree*0.9/tick.Bids[0].Price))
		if bid != nil && bid.OrderId != `` {
			bid.Function = model.FunctionHang
			model.AppDB.Save(&bid)
			hangStatus.setBid(symbol, bid)
		}
	} else {
		if bid.Price < tick.Bids[0].Price {
			didSmth = true
			util.Notice(fmt.Sprintf(`---0 cancel bid not on 1 %f < %f`, bid.Price, tick.Bids[0].Price))
			api.MustCancel(``, ``, market, symbol, bid.OrderId, true)
			hangStatus.setBid(symbol, nil)
		} else if bid.Price == tick.Bids[0].Price {
			bid = api.QueryOrderById(``, ``, market, symbol, bid.OrderId)
			if bid != nil {
				if bid.DealAmount > 0 && bid.Status == model.CarryStatusWorking {
					didSmth = true
					util.Notice(fmt.Sprintf(`---1 check bid price %f = %f amount %f > deal %f`,
						bid.Price, tick.Bids[0].Price, bid.Amount, bid.DealAmount))
					api.MustCancel(``, ``, market, symbol, bid.OrderId, false)
					hangStatus.setBid(symbol, nil)
				} else if bid.Status == model.CarryStatusSuccess || bid.Status == model.CarryStatusFail {
					hangStatus.setBid(symbol, nil)
				}
			}
		}
	}
	if ask == nil || ask.Price < tick.Asks[0].Price {
		didSmth = true
		ask = api.PlaceOrder(``, ``, model.OrderSideSell, model.OrderTypeLimit, market, symbol,
			setting.AccountType, setting.AccountType, tick.Asks[0].Price, math.Min(tick.Asks[0].Amount, leftFree*0.9))
		if ask != nil && ask.OrderId != `` {
			ask.Function = model.FunctionHang
			model.AppDB.Save(&ask)
			hangStatus.setAsk(symbol, ask)
		}
	} else {
		if ask.Price > tick.Asks[0].Price {
			didSmth = true
			util.Notice(fmt.Sprintf(`---0 cancel ask not on 1 %f > %f`, ask.Price, tick.Asks[0].Price))
			api.MustCancel(``, ``, market, symbol, ask.OrderId, true)
			hangStatus.setAsk(symbol, nil)
		} else if ask.Price == tick.Asks[0].Price {
			ask = api.QueryOrderById(``, ``, market, symbol, ask.OrderId)
			if ask != nil {
				if ask.DealAmount > 0 && ask.Status == model.CarryStatusWorking {
					didSmth = true
					util.Notice(fmt.Sprintf(`---1 check ask price %f = %f amount %f > deal %f`,
						ask.Price, tick.Asks[0].Price, ask.Amount, ask.DealAmount))
					api.MustCancel(``, ``, market, symbol, ask.OrderId, false)
					hangStatus.setAsk(symbol, nil)
				} else if ask.Status == model.CarryStatusSuccess || ask.Status == model.CarryStatusFail {
					hangStatus.setAsk(symbol, nil)
				}
			}
		}
	}
	if didSmth {
		time.Sleep(time.Second)
		api.RefreshAccount(``, ``, market)
	} else {
		util.Notice(`nothing happened`)
	}
}
