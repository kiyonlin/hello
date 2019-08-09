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
			setting.AccountType, setting.AccountType, tick.Bids[2].Price,
			math.Min(tick.Bids[2].Amount, rightFree*0.9/tick.Bids[2].Price))
		if bid != nil && bid.OrderId != `` {
			bid.Function = model.FunctionHang
			model.AppDB.Save(&bid)
			hangStatus.setBid(symbol, bid)
		}
	} else {
		if bid.Price < tick.Bids[5].Price || bid.Price > tick.Bids[1].Price {
			didSmth = true
			util.Notice(fmt.Sprintf(`---0 cancel bid %f < %f or > %f`,
				bid.Price, tick.Bids[5].Price, tick.Bids[1].Price))
			api.MustCancel(``, ``, market, symbol, bid.OrderId, true)
			hangStatus.setBid(symbol, nil)
		}
	}
	if ask == nil || ask.Price < tick.Asks[0].Price {
		didSmth = true
		ask = api.PlaceOrder(``, ``, model.OrderSideSell, model.OrderTypeLimit, market, symbol,
			setting.AccountType, setting.AccountType, tick.Asks[2].Price, math.Min(tick.Asks[2].Amount, leftFree*0.9))
		if ask != nil && ask.OrderId != `` {
			ask.Function = model.FunctionHang
			model.AppDB.Save(&ask)
			hangStatus.setAsk(symbol, ask)
		}
	} else {
		if ask.Price > tick.Asks[5].Price || ask.Price < tick.Asks[1].Price {
			didSmth = true
			util.Notice(fmt.Sprintf(`---0 cancel ask %f > %f or < %f`,
				ask.Price, tick.Asks[5].Price, tick.Asks[1].Price))
			api.MustCancel(``, ``, market, symbol, ask.OrderId, true)
			hangStatus.setAsk(symbol, nil)
		}
	}
	if didSmth {
		time.Sleep(time.Second)
		api.RefreshAccount(``, ``, market)
	} else {
		util.Notice(`nothing happened`)
	}
}
