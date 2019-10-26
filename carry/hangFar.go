package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"strconv"
	"strings"
	"sync"
)

type HangFarOrders struct {
	lock       sync.Mutex
	hangingFar bool
	bidOrders  map[string]map[string]*model.Order // symbol - position - orders
	askOrders  map[string]map[string]*model.Order // symbol - position - orders
}

var hangFarOrders = &HangFarOrders{}

func (hangFarOrders *HangFarOrders) setInHangingFar(in bool) {
	hangFarOrders.hangingFar = in
}

func (hangFarOrders *HangFarOrders) getFarOrders(symbol string) (bidOrders, askOrders map[string]*model.Order) {
	defer hangFarOrders.lock.Unlock()
	hangFarOrders.lock.Lock()
	if hangFarOrders.bidOrders == nil {
		hangFarOrders.bidOrders = make(map[string]map[string]*model.Order)
	}
	if hangFarOrders.askOrders == nil {
		hangFarOrders.askOrders = make(map[string]map[string]*model.Order)
	}
	if hangFarOrders.bidOrders[symbol] == nil {
		hangFarOrders.bidOrders[symbol] = make(map[string]*model.Order)
	}
	if hangFarOrders.askOrders[symbol] == nil {
		hangFarOrders.askOrders[symbol] = make(map[string]*model.Order)
	}
	return hangFarOrders.bidOrders[symbol], hangFarOrders.askOrders[symbol]
}

func (hangFarOrders *HangFarOrders) setFarOrders(symbol string, bidOrders, askOrders map[string]*model.Order) {
	defer hangFarOrders.lock.Unlock()
	hangFarOrders.lock.Lock()
	if hangFarOrders.bidOrders == nil {
		hangFarOrders.bidOrders = make(map[string]map[string]*model.Order)
	}
	if hangFarOrders.askOrders == nil {
		hangFarOrders.askOrders = make(map[string]map[string]*model.Order)
	}
	hangFarOrders.bidOrders[symbol] = bidOrders
	hangFarOrders.askOrders[symbol] = askOrders
}

var ProcessHangFar = func(market, symbol string) {
	start := util.GetNowUnixMillion()
	_, tick := model.AppMarkets.GetBidAsk(symbol, market)
	if tick == nil || tick.Asks == nil || tick.Bids == nil || tick.Asks.Len() < 15 || tick.Bids.Len() < 15 ||
		int(start)-tick.Ts > 500 {
		timeDis := 0
		if tick != nil {
			timeDis = int(start) - tick.Ts
		}
		util.Notice(fmt.Sprintf(`[tick not good time]%s %s %d`, market, symbol, timeDis))
		CancelHang(key, secret, symbol)
		return
	}
	setting := model.GetSetting(model.FunctionHangFar, market, symbol)
	parameters := strings.Split(setting.FunctionParameter, `_`)
	posStr := make([]string, len(parameters)/3)
	pos := make(map[string]float64)
	posDis := make(map[string]float64)
	amount := make(map[string]float64)
	for i := 0; i < len(parameters)/3; i++ {
		posStr[i] = parameters[i*3]
		pos[posStr[i]], _ = strconv.ParseFloat(posStr[i], 64)
		posDis[posStr[i]], _ = strconv.ParseFloat(parameters[i*3+1], 64)
		amount[posStr[i]], _ = strconv.ParseFloat(parameters[i*3+2], 64)
	}
	//priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	if util.GetNowUnixMillion()-int64(tick.Ts) > 1000 {
		util.SocketInfo(fmt.Sprintf(`socekt old tick %d %d`, util.GetNowUnixMillion(), tick.Ts))
		CancelHang(key, secret, symbol)
		return
	}
	validHang(key, secret, symbol, pos, posDis, tick)
	if model.AppConfig.Handle != `1` || model.AppPause {
		util.Notice(fmt.Sprintf(`[status]%s is pause:%v`, model.AppConfig.Handle, model.AppPause))
		CancelHang(key, secret, symbol)
		return
	}
	if hangFarOrders.hangingFar {
		return
	}
	hangFarOrders.setInHangingFar(true)
	defer hangFarOrders.setInHangingFar(false)
	if hang(key, secret, market, symbol, setting.AccountType, pos, amount, tick) {
		cancelNonHang(market, symbol)
	}
}

func cancelNonHang(market, symbol string) {
	orders := api.QueryOrders(key, secret, market, symbol, ``, ``, 0, 0)
	ordersBids, orderAsks := hangFarOrders.getFarOrders(symbol)
	for _, order := range orders {
		if order != nil && order.OrderId != `` {
			needCancel := true
			for _, value := range ordersBids {
				if value != nil && value.OrderId == order.OrderId {
					needCancel = false
				}
			}
			for _, value := range orderAsks {
				if value != nil && value.OrderId == order.OrderId {
					needCancel = false
				}
			}
			if needCancel {
				util.Notice(fmt.Sprintf(`==cancel other pending== %s`, order.OrderId))
				api.MustCancel(key, secret, market, symbol, order.OrderId, true)
			}
		}
	}
}

func hang(key, secret, market, symbol, accountType string, pos, amount map[string]float64, tick *model.BidAsk) (dosmth bool) {
	ordersBids, orderAsks := hangFarOrders.getFarOrders(symbol)
	dosmth = false
	for str, value := range pos {
		bidPrice := (tick.Bids[0].Price + tick.Asks[0].Price) / 2 * (1 - value)
		askPrice := (tick.Bids[0].Price + tick.Asks[0].Price) / 2 * (1 + value)
		if ordersBids[str] == nil {
			dosmth = true
			order := api.PlaceOrder(key, secret, model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``,
				accountType, bidPrice, amount[str])
			if order != nil && order.OrderId != `` {
				util.Notice(fmt.Sprintf(`=hang= at %s %s`, str, order.OrderId))
				ordersBids[str] = order
				model.AppDB.Save(&order)
			}
		}
		if orderAsks[str] == nil {
			dosmth = true
			order := api.PlaceOrder(key, secret, model.OrderSideSell, model.OrderTypeLimit, market, symbol, ``,
				accountType, askPrice, amount[str])
			if order != nil && order.OrderId != `` {
				util.Notice(fmt.Sprintf(`=hang= at %s %s`, str, order.OrderId))
				orderAsks[str] = order
				model.AppDB.Save(&order)
			}
		}
	}
	hangFarOrders.setFarOrders(symbol, ordersBids, orderAsks)
	return dosmth
}

func validHang(key, secret, symbol string, pos, dis map[string]float64, tick *model.BidAsk) {
	bidOrders, askOrders := hangFarOrders.getFarOrders(symbol)
	bidOrdersValid := make(map[string]*model.Order)
	askOrdersValid := make(map[string]*model.Order)
	for posStr, order := range bidOrders {
		if pos[posStr] == 0 || (pos[posStr] > 0 &&
			(order.Price <= tick.Asks[0].Price*(1-pos[posStr]-dis[posStr]) ||
				order.Price >= tick.Bids[0].Price*(1-pos[posStr]+dis[posStr]))) {
			go api.MustCancel(key, secret, model.Fmex, symbol, order.OrderId, true)
		} else {
			util.Info(fmt.Sprintf(`validate order %s %f order id %s price bid %f`,
				posStr, pos[posStr], order.OrderId, tick.Bids[0].Price))
			bidOrdersValid[posStr] = order
		}
	}
	for posStr, order := range askOrders {
		if pos[posStr] == 0 || (pos[posStr] > 0 &&
			(order.Price <= tick.Asks[0].Price*(1+pos[posStr]-dis[posStr]) ||
				order.Price >= tick.Bids[0].Price*(1+pos[posStr]+dis[posStr]))) {
			go api.MustCancel(key, secret, model.Fmex, symbol, order.OrderId, true)
		} else {
			util.Info(fmt.Sprintf(`validate order %s %f order id %s price ask %f`,
				posStr, pos[posStr], order.OrderId, tick.Asks[0].Price))
			askOrdersValid[posStr] = order
		}
	}
	hangFarOrders.setFarOrders(symbol, bidOrdersValid, askOrdersValid)
}

func CancelHang(key, secret, symbol string) {
	util.Notice(`[cancel all orders]`)
	bidOrders, askOrders := hangFarOrders.getFarOrders(symbol)
	for _, order := range bidOrders {
		if order != nil && order.OrderId != `` {
			api.MustCancel(key, secret, model.Fmex, symbol, order.OrderId, true)
		}
	}
	for _, order := range askOrders {
		if order != nil && order.OrderId != `` {
			api.MustCancel(key, secret, model.Fmex, symbol, order.OrderId, true)
		}
	}
	hangFarOrders.setFarOrders(symbol, nil, nil)
}
