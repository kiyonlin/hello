package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"sync"
)

var hangStatus = &HangStatus{keys: []string{
	`6f1ccb0d31e24f42a90e1d04b933609d`,  //haoweizh@qq.com
	`a6168a5762234a03884fd0e2572ccad5`,  //yjsx_@163.com
	`c1df9c45d72342eba471cdd0c8b6e658`}, // liwei
	secrets: map[string]string{
		`6f1ccb0d31e24f42a90e1d04b933609d`: `74e551965da542d280f6abc8f5f263b8`,
		`a6168a5762234a03884fd0e2572ccad5`: `4907b7bf3b6447caa6fc91ac7a183a3a`,
		`c1df9c45d72342eba471cdd0c8b6e658`: `a85eb6ae9bde471db2384f792f73033d`}}

type HangStatus struct {
	lock    sync.Mutex
	hanging bool
	orders  map[string]*model.Order // key - order
	keys    []string
	secrets map[string]string
}

func (hangStatus *HangStatus) setHanging(value bool) {
	hangStatus.hanging = value
}

func (hangStatus *HangStatus) setOrder(key string, order *model.Order) {
	hangStatus.lock.Lock()
	defer hangStatus.lock.Unlock()
	if hangStatus.orders == nil {
		hangStatus.orders = make(map[string]*model.Order)
	}
	hangStatus.orders[key] = order
}

func (hangStatus *HangStatus) getOrder(key string) (order *model.Order) {
	hangStatus.lock.Lock()
	defer hangStatus.lock.Unlock()
	if hangStatus.orders == nil || hangStatus.orders[key] == nil {
		return nil
	}
	return hangStatus.orders[key]
}

var ProcessHang = func(market, symbol string) {
	result, tick := model.AppMarkets.GetBidAsk(symbol, market)
	if !result || tick == nil || tick.Asks == nil || tick.Bids == nil || tick.Asks.Len() < 11 ||
		tick.Bids.Len() < 11 {
		util.Notice(fmt.Sprintf(`[tick not good]%s %s`, market, symbol))
		return
	}
	go validHang(market, symbol)
	if hangStatus.hanging || model.AppConfig.Handle != `1` || model.AppPause {
		return
	}
	defer hangStatus.setHanging(false)
	delay := util.GetNowUnixMillion() - int64(tick.Ts)
	if delay > 500 {
		util.Notice(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
	hang(market, symbol, tick)
}

func hang(market, symbol string, tick *model.BidAsk) {
	second := util.GetNow().Second()
	index := int(second / 20)
	order := hangStatus.getOrder(hangStatus.keys[index])
	if order != nil {
		return
	}
	key := hangStatus.keys[index]
	secret := hangStatus.secrets[key]
	util.Notice(fmt.Sprintf(`[hang %s %s]price: %f amount: %f`, key, symbol, tick.Bids[4].Price, 10))
	order = api.PlaceOrder(key, secret, model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``,
		``, tick.Bids[4].Price, 10)
	if order != nil && order.Status != model.CarryStatusFail && order.OrderId != `` {
		order.Function = model.FunctionHang
		model.AppDB.Save(&order)
		hangStatus.setOrder(key, order)
	}
}

func validHang(market, symbol string) {
	second := util.GetNow().Second()
	index := int(second / 20)
	for _, value := range hangStatus.keys {
		order := hangStatus.getOrder(value)
		if value != hangStatus.keys[index] && order != nil {
			api.CancelOrder(value, hangStatus.secrets[value], market, symbol, order.OrderId)
			hangStatus.setOrder(value, nil)
		}
	}
}
