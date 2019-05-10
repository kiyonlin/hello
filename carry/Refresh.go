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
	"time"
)

//function_parameter:0.2_30_
// coinpark://4003 调用次数繁忙 //2085 最小下单数量限制 //2027 可用余额不足
var syncRefresh = make(chan interface{}, 20)

//var LastRefreshTime = make(map[string]int64) // market - int64
var refreshOrders = &RefreshOrders{}

type RefreshOrders struct {
	lock             sync.Mutex
	refreshing       bool
	hanging          bool
	samePriceCount   map[string]map[string]int             // market - symbol - continue same price count
	samePriceTime    map[string]map[string]*time.Time      // market - symbol - first time new order price
	bidOrders        map[string]map[string][]*model.Order  // market - symbol - orders
	askOrders        map[string]map[string][]*model.Order  // market - symbol - orders
	lastBid          map[string]map[string]*model.Order    // market - symbol - order
	lastAsk          map[string]map[string]*model.Order    // market - symbol - order
	amountLimit      map[string]map[string]map[int]float64 // market - symbol - time start point - amount
	lastChancePrice  map[string]map[string]float64         // market - symbol - chance price
	lastRefreshPrice map[string]map[string]float64         // market - symbol - refresh price
	fcoinHang        map[string][]*model.Order             // symbol - refresh hang order 0:bid 1:ask
	inRefresh        map[string]bool                       // symbol - bool
	waiting          map[string]bool                       // symbol - wait
	needReset        map[string]map[string]string          // symbol - accountType - coin
	amountIndex      int
}

func (refreshOrders *RefreshOrders) getNeedReset(symbol, accountType string) (coin string) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.needReset == nil || refreshOrders.needReset[symbol] == nil {
		return ``
	}
	return refreshOrders.needReset[symbol][accountType]
}

func (refreshOrders *RefreshOrders) setNeedReset(symbol, accountType, coin string) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.needReset == nil {
		refreshOrders.needReset = make(map[string]map[string]string)
	}
	if refreshOrders.needReset[symbol] == nil {
		refreshOrders.needReset[symbol] = make(map[string]string)
	}
	refreshOrders.needReset[symbol][accountType] = coin
}

func (refreshOrders *RefreshOrders) setWaiting(symbol string, in bool) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.waiting == nil {
		refreshOrders.waiting = make(map[string]bool)
	}
	refreshOrders.waiting[symbol] = in
}

func (refreshOrders *RefreshOrders) getWaiting(symbol string) (out bool) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.waiting == nil {
		return false
	}
	return refreshOrders.waiting[symbol]
}

func (refreshOrders *RefreshOrders) setInRefresh(symbol string, in bool) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.inRefresh == nil {
		refreshOrders.inRefresh = make(map[string]bool)
	}
	refreshOrders.inRefresh[symbol] = in
}

func (refreshOrders *RefreshOrders) getInRefresh(symbol string) (in bool) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.inRefresh == nil {
		return false
	}
	return refreshOrders.inRefresh[symbol]
}

func (refreshOrders *RefreshOrders) removeRefreshHang(symbol string, hangBid, hangAsk *model.Order) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.fcoinHang == nil || refreshOrders.fcoinHang[symbol] == nil {
		return
	}
	if hangBid != nil && refreshOrders.fcoinHang[symbol][0] != nil &&
		refreshOrders.fcoinHang[symbol][0].OrderId == hangBid.OrderId {
		refreshOrders.fcoinHang[symbol][0] = nil
	}
	if hangAsk != nil && refreshOrders.fcoinHang[symbol][1] != nil &&
		refreshOrders.fcoinHang[symbol][1].OrderId == hangAsk.OrderId {
		refreshOrders.fcoinHang[symbol][1] = nil
	}
}

func (refreshOrders *RefreshOrders) setRefreshHang(symbol string, hangBid, hangAsk *model.Order) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.fcoinHang == nil {
		refreshOrders.fcoinHang = make(map[string][]*model.Order)
	}
	refreshOrders.fcoinHang[symbol] = make([]*model.Order, 2)
	refreshOrders.fcoinHang[symbol][0] = hangBid
	refreshOrders.fcoinHang[symbol][1] = hangAsk
}

func (refreshOrders *RefreshOrders) getRefreshHang(symbol string) (hangBid, hangAsk *model.Order) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.fcoinHang == nil {
		refreshOrders.fcoinHang = make(map[string][]*model.Order)
	}
	if refreshOrders.fcoinHang[symbol] == nil {
		refreshOrders.fcoinHang[symbol] = make([]*model.Order, 2)
	}
	return refreshOrders.fcoinHang[symbol][0], refreshOrders.fcoinHang[symbol][1]
}

func (refreshOrders *RefreshOrders) SetLastChancePrice(market, symbol string, price float64) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.lastChancePrice == nil {
		refreshOrders.lastChancePrice = make(map[string]map[string]float64)
	}
	if refreshOrders.lastChancePrice[market] == nil {
		refreshOrders.lastChancePrice[market] = make(map[string]float64)
	}
	refreshOrders.lastChancePrice[market][symbol] = price
}

func (refreshOrders *RefreshOrders) CheckLastChancePrice(market, symbol string, price, priceDistance float64) (same bool) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.lastChancePrice == nil || refreshOrders.lastChancePrice[market] == nil {
		return false
	}
	if refreshOrders.lastChancePrice[market][symbol] > 0 {
		return true
	}
	return false
}

func (refreshOrders *RefreshOrders) SetLastRefreshPrice(market, symbol string, price, priceDistance float64) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.lastRefreshPrice == nil {
		refreshOrders.lastRefreshPrice = make(map[string]map[string]float64)
		refreshOrders.samePriceCount = make(map[string]map[string]int)
		refreshOrders.samePriceTime = make(map[string]map[string]*time.Time)
	}
	if refreshOrders.lastRefreshPrice[market] == nil {
		refreshOrders.lastRefreshPrice[market] = make(map[string]float64)
		refreshOrders.samePriceCount[market] = make(map[string]int)
		refreshOrders.samePriceTime[market] = make(map[string]*time.Time)
	}
	if math.Abs(refreshOrders.lastRefreshPrice[market][symbol]-price) < priceDistance {
		refreshOrders.samePriceCount[market][symbol]++
	} else {
		refreshOrders.lastRefreshPrice[market][symbol] = price
		refreshOrders.samePriceCount[market][symbol] = 1
		samePriceTime := util.GetNow()
		refreshOrders.samePriceTime[market][symbol] = &samePriceTime
	}
}

func (refreshOrders *RefreshOrders) CheckLastRefreshPrice(market, symbol string, price, priceDistance float64) (over bool) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.lastRefreshPrice == nil || refreshOrders.lastRefreshPrice[market] == nil {
		return false
	}
	d, _ := time.ParseDuration("-601s")
	timeLine := util.GetNow().Add(d)
	if refreshOrders.samePriceTime[market][symbol] != nil && refreshOrders.samePriceTime[market][symbol].Before(timeLine) {
		util.Notice(fmt.Sprintf(`[10min clear]%s %f %d`,
			symbol, price, refreshOrders.samePriceTime[market][symbol].Unix()))
		refreshOrders.lastRefreshPrice[market][symbol] = 0
		refreshOrders.samePriceTime[market][symbol] = nil
		refreshOrders.samePriceCount[market][symbol] = 0
		return false
	}
	if math.Abs(refreshOrders.lastRefreshPrice[market][symbol]-price) < priceDistance &&
		refreshOrders.samePriceCount[market][symbol] >= 2 {
		util.Info(fmt.Sprintf(`[jump 2] %s %s %f`, market, symbol, price))
		return true
	}
	return false
}

func (refreshOrders *RefreshOrders) CheckAmountLimit(market, symbol string, amountLimit float64) (
	underLimit bool, amountIndex int) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	now := util.GetNow()
	amountIndex = int((now.Hour()*3600 + now.Minute()*60 + now.Second()) / model.RefreshTimeSlot)
	if refreshOrders.amountLimit == nil || refreshOrders.amountLimit[market] == nil ||
		refreshOrders.amountLimit[market][symbol] == nil {
		return true, amountIndex
	}
	refreshOrders.amountLimit[market][symbol][amountIndex+1] = 0
	refreshOrders.amountLimit[market][symbol][amountIndex-1] = 0
	if refreshOrders.amountLimit[market][symbol][amountIndex] < amountLimit {
		return true, amountIndex
	}
	util.Notice(fmt.Sprintf(`[limit full]%s %s %d %f`, market, symbol, amountIndex,
		refreshOrders.amountLimit[market][symbol][amountIndex]))
	return false, amountIndex
}

func (refreshOrders *RefreshOrders) AddRefreshAmount(market, symbol string, amount, amountLimit float64) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.amountLimit == nil {
		refreshOrders.amountLimit = make(map[string]map[string]map[int]float64)
	}
	if refreshOrders.amountLimit[market] == nil {
		refreshOrders.amountLimit[market] = make(map[string]map[int]float64)
	}
	if refreshOrders.amountLimit[market][symbol] == nil {
		refreshOrders.amountLimit[market][symbol] = make(map[int]float64)
	}
	now := util.GetNow()
	slotNum := int((now.Hour()*3600 + now.Minute()*60 + now.Second()) / model.RefreshTimeSlot)
	refreshOrders.amountLimit[market][symbol][slotNum] += amount
	util.Notice(fmt.Sprintf(`[+limit amount]%s %s %d %f`, market, symbol, slotNum, amount))
}

func (refreshOrders *RefreshOrders) SetLastOrder(market, symbol, orderSide string, order *model.Order) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if orderSide == model.OrderSideSell {
		if refreshOrders.lastAsk == nil {
			refreshOrders.lastAsk = make(map[string]map[string]*model.Order)
			if refreshOrders.lastAsk[market] == nil {
				refreshOrders.lastAsk[market] = make(map[string]*model.Order)
			}
		}
		refreshOrders.lastAsk[market][symbol] = order
	}
	if orderSide == model.OrderSideBuy {
		if refreshOrders.lastBid == nil {
			refreshOrders.lastBid = make(map[string]map[string]*model.Order)
			if refreshOrders.lastBid[market] == nil {
				refreshOrders.lastBid[market] = make(map[string]*model.Order)
			}
		}
		refreshOrders.lastBid[market][symbol] = order
	}
}

func (refreshOrders *RefreshOrders) GetLastOrder(market, symbol, orderSide string) (lastOrder *model.Order) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if orderSide == model.OrderSideSell {
		if refreshOrders.lastAsk == nil {
			refreshOrders.lastAsk = make(map[string]map[string]*model.Order)
			if refreshOrders.lastAsk[market] == nil {
				refreshOrders.lastAsk[market] = make(map[string]*model.Order)
			}
		}
		return refreshOrders.lastAsk[market][symbol]
	}
	if orderSide == model.OrderSideBuy {
		if refreshOrders.lastBid == nil {
			refreshOrders.lastBid = make(map[string]map[string]*model.Order)
			if refreshOrders.lastBid[market] == nil {
				refreshOrders.lastBid[market] = make(map[string]*model.Order)
			}
		}
		return refreshOrders.lastBid[market][symbol]
	}
	return nil
}

func (refreshOrders *RefreshOrders) AddRefreshOrders(market, symbol, orderSide string, order *model.Order) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if orderSide == model.OrderSideBuy {
		if refreshOrders.bidOrders == nil {
			refreshOrders.bidOrders = make(map[string]map[string][]*model.Order)
		}
		if refreshOrders.bidOrders[market] == nil {
			refreshOrders.bidOrders[market] = make(map[string][]*model.Order)
		}
		if refreshOrders.bidOrders[market][symbol] == nil {
			refreshOrders.bidOrders[market][symbol] = make([]*model.Order, 0)
		}
		priceEqual := false
		for key, value := range refreshOrders.bidOrders[order.Market][order.Symbol] {
			if value.Price == order.Price {
				priceEqual = true
				if value.OrderTime.Before(order.OrderTime) {
					refreshOrders.bidOrders[order.Market][order.Symbol][key] = order
				}
			}
		}
		if !priceEqual {
			refreshOrders.bidOrders[order.Market][order.Symbol] =
				append(refreshOrders.bidOrders[order.Market][order.Symbol], order)
		}
	}
	if orderSide == model.OrderSideSell {
		if refreshOrders.askOrders == nil {
			refreshOrders.askOrders = make(map[string]map[string][]*model.Order)
		}
		if refreshOrders.askOrders[market] == nil {
			refreshOrders.askOrders[market] = make(map[string][]*model.Order)
		}
		if refreshOrders.askOrders[market][symbol] == nil {
			refreshOrders.askOrders[market][symbol] = make([]*model.Order, 0)
		}
		priceEqual := false
		for key, value := range refreshOrders.askOrders[order.Market][order.Symbol] {
			if value.Price == order.Price {
				priceEqual = true
				if value.OrderTime.Before(order.OrderTime) {
					refreshOrders.askOrders[order.Market][order.Symbol][key] = order
				}
			}
		}
		if !priceEqual {
			refreshOrders.askOrders[order.Market][order.Symbol] =
				append(refreshOrders.askOrders[order.Market][order.Symbol], order)
		}
	}
}

func (refreshOrders *RefreshOrders) setRefreshing(in bool) {
	refreshOrders.refreshing = in
}

func (refreshOrders *RefreshOrders) setHanging(in bool) {
	refreshOrders.hanging = in
}

var ProcessRefresh = func(market, symbol string) {
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
		util.Info(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
	result, otherPrice := getOtherPrice(market, symbol, model.Huobi)
	if !result || otherPrice == 0 {
		util.Notice(fmt.Sprintf(`[get other price]%s %f`, symbol, otherPrice))
		CancelAndRefresh(market)
		return
	}
	setting := model.GetSetting(model.FunctionRefresh, market, symbol)
	leftFree, rightFree, _, _, err := getBalance(market, symbol, setting.AccountType)
	if err != nil || (leftFree == 0 && rightFree == 0) {
		return
	}
	hangRate := 0.0
	amountLimit := 0.0
	parameters := strings.Split(setting.FunctionParameter, `_`)
	if len(parameters) == 2 {
		hangRate, _ = strconv.ParseFloat(parameters[0], 64)
		amountLimit, _ = strconv.ParseFloat(parameters[1], 64)
	}
	go validRefreshHang(symbol, amountLimit, otherPrice, tick)
	if model.AppConfig.Handle != `1` || model.AppConfig.HandleRefresh != `1` || refreshOrders.refreshing {
		util.Notice(fmt.Sprintf(`[status]%s %s %v`,
			model.AppConfig.Handle, model.AppConfig.HandleRefresh, refreshOrders.refreshing))
		return
	}
	refreshOrders.setRefreshing(true)
	defer refreshOrders.setRefreshing(false)
	resetCoin := refreshOrders.getNeedReset(symbol, setting.AccountType)
	if resetCoin != `` {
		time.Sleep(time.Second)
		util.Notice(fmt.Sprintf(`[reset balance]%s %s %s %s`, market, symbol, resetCoin, setting.AccountType))
		api.RefreshCoinAccount(market, symbol, resetCoin, setting.AccountType)
		refreshOrders.setNeedReset(symbol, setting.AccountType, ``)
		return
	}
	haveAmount, index := refreshOrders.CheckAmountLimit(market, symbol, setting.AmountLimit)
	if index == 0 {
		refreshOrders.amountIndex = 0
	}
	if index > refreshOrders.amountIndex {
		util.Notice(fmt.Sprintf(`[before 10min canceling]`))
		model.AppConfig.Handle = `0`
		time.Sleep(time.Second * 2)
		refreshOrders.amountIndex = index
		CancelAndRefresh(market)
		model.AppConfig.Handle = `1`
		util.Notice(fmt.Sprintf(`[after 10min canceling]`))
		return
	}
	amount := math.Min(leftFree, rightFree/tick.Asks[0].Price) * model.AppConfig.AmountRate
	util.Notice(fmt.Sprintf(`amount %f left %f right %f`, amount, leftFree, rightFree/tick.Asks[0].Price))
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	refreshAble, orderSide, orderReverse, orderPrice := preDeal(setting, market, symbol, otherPrice, amount)
	if refreshOrders.CheckLastChancePrice(market, symbol, orderPrice, 0.9*priceDistance) {
		refreshOrders.SetLastChancePrice(market, symbol, 0)
		refreshAble = false
	}
	//else if refreshOrders.CheckLastRefreshPrice(market, symbol, orderPrice, 0.9*priceDistance) {
	//	refreshAble = false
	//}
	if refreshOrders.getWaiting(symbol) {
		time.Sleep(time.Second)
		refreshOrders.setWaiting(symbol, false)
		return
	}
	if refreshOrders.getInRefresh(symbol) {
		util.Info(fmt.Sprintf(`[in refreshing %s]`, symbol))
		if haveAmount {
			if refreshAble {
				doRefresh(setting, market, symbol, setting.AccountType, orderSide, orderReverse, orderPrice,
					0.9*priceDistance, amount, tick)
			} else {
				util.Info(fmt.Sprintf(`[in refreshing not refreshable %s]`, symbol))
			}
		} else {
			refreshOrders.setInRefresh(symbol, false)
			time.Sleep(time.Second)
		}
	} else {
		util.Info(fmt.Sprintf(`[in hang %s]`, symbol))
		if haveAmount {
			if refreshAble {
				util.Info(fmt.Sprintf(`[-->refreshable]%s %s`, market, symbol))
				refreshOrders.setInRefresh(symbol, true)
				CancelRefreshHang(market, symbol)
				time.Sleep(time.Second)
				util.Info(fmt.Sprintf(`[-->set done refreshable]%s %s`, market, symbol))
			} else {
				util.Info(fmt.Sprintf(`[in hang not refreshable %s]`, symbol))
				refreshHang(market, symbol, setting.AccountType, hangRate, amountLimit, leftFree, rightFree,
					otherPrice, tick)
			}
		} else {
			util.Info(fmt.Sprintf(`[in hang not have amount %s]`, symbol))
			refreshHang(market, symbol, setting.AccountType, hangRate, amountLimit, leftFree, rightFree,
				otherPrice, tick)
		}
	}
}

func refreshHang(market, symbol, accountType string,
	hangRate, amountLimit, leftFree, rightFree, otherPrice float64, tick *model.BidAsk) {
	util.Info(fmt.Sprintf(`[refreshhang]%s`, symbol))
	if refreshOrders.hanging {
		return
	}
	defer refreshOrders.setHanging(false)
	refreshOrders.setHanging(true)
	if hangRate == 0.0 {
		return
	}
	bidAll := tick.Bids[0].Amount
	askAll := tick.Asks[0].Amount
	for i := 1; i <= 8; i++ {
		bidAll += tick.Bids[i].Amount
		askAll += tick.Asks[i].Amount
	}
	rightFree = rightFree / tick.Asks[0].Price
	needRefresh := false
	coins := strings.Split(symbol, `_`)
	if len(coins) != 2 {
		util.Notice(fmt.Sprintf(`[wrong symbol]%s`, symbol))
		return
	}
	coin := ``
	hangBid, hangAsk := refreshOrders.getRefreshHang(symbol)
	if hangAsk == nil && askAll > amountLimit && otherPrice*0.9995 <= tick.Asks[9].Price {
		hangAsk = api.PlaceOrder(model.OrderSideSell, model.OrderTypeLimit, market, symbol, ``,
			accountType, tick.Asks[9].Price, leftFree*hangRate)
		if hangAsk != nil && hangAsk.OrderId != `` && hangAsk.Status != model.CarryStatusFail {
			hangAsk.Function = model.FunctionHang
			model.AppDB.Save(&hangAsk)
		} else if hangAsk != nil && hangAsk.ErrCode == `1016` {
			hangAsk = nil
			coin = coins[0]
			needRefresh = true
		}
	}
	if hangBid == nil && bidAll > amountLimit && otherPrice*1.0005 >= tick.Bids[9].Price {
		hangBid = api.PlaceOrder(model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``,
			accountType, tick.Bids[9].Price, rightFree*hangRate)
		if hangBid != nil && hangBid.OrderId != `` && hangBid.Status != model.CarryStatusFail {
			hangBid.Function = model.FunctionHang
			model.AppDB.Save(&hangBid)
		} else if hangBid != nil && hangBid.ErrCode == `1016` {
			hangBid = nil
			coin = coins[1]
			needRefresh = true
		}
	}
	refreshOrders.setRefreshHang(symbol, hangBid, hangAsk)
	if needRefresh {
		CancelRefreshHang(market, symbol)
		time.Sleep(time.Second * 2)
		api.RefreshCoinAccount(market, symbol, coin, accountType)
	}
	util.Info(fmt.Sprintf(`[refreshhang done]%s`, symbol))
}

func validRefreshHang(symbol string, amountLimit, otherPrice float64, tick *model.BidAsk) {
	hangBid, hangAsk := refreshOrders.getRefreshHang(symbol)
	if hangBid != nil {
		bidAll := 0.0
		for i := 0; i < tick.Bids.Len() && tick.Bids[i].Price > hangBid.Price; i++ {
			bidAll += tick.Bids[i].Amount
		}
		if hangBid.Price < tick.Bids[14].Price || bidAll < amountLimit || hangBid.Price > 1.0005*otherPrice {
			util.Notice(fmt.Sprintf(`[cancelhangbid]%s %s`, symbol, hangBid.OrderId))
			go api.MustCancel(hangBid.Market, symbol, hangBid.OrderId, true)
			refreshOrders.removeRefreshHang(symbol, hangBid, nil)
			refreshOrders.setWaiting(symbol, true)
		}
	}
	if hangAsk != nil {
		askAll := 0.0
		for i := 0; i < tick.Asks.Len() && tick.Asks[i].Price < hangAsk.Price; i++ {
			askAll += tick.Asks[i].Amount
		}
		if hangAsk.Price > tick.Asks[14].Price || askAll < amountLimit || hangAsk.Price < 0.9995*otherPrice {
			util.Notice(fmt.Sprintf(`[cancelhangask]%s %s`, symbol, hangAsk.OrderId))
			go api.MustCancel(hangAsk.Market, symbol, hangAsk.OrderId, true)
			refreshOrders.removeRefreshHang(symbol, nil, hangAsk)
			refreshOrders.setWaiting(symbol, true)
		}
	}
}

func CancelRefreshHang(market, symbol string) (needCancel bool) {
	util.Notice(fmt.Sprintf(`[-----cancel hang---]%s %s`, market, symbol))
	//if refreshOrders.hanging {
	//	return
	//}
	//defer refreshOrders.setHanging(false)
	msg := `[-----cancel hang done---]`
	hangBid, hangAsk := refreshOrders.getRefreshHang(symbol)
	if hangBid != nil {
		msg += `hangbid:` + hangBid.OrderId + `;`
		api.MustCancel(hangBid.Market, symbol, hangBid.OrderId, true)
	}
	if hangAsk != nil {
		msg += `hangask:` + hangAsk.OrderId + `;`
		api.MustCancel(hangAsk.Market, symbol, hangAsk.OrderId, true)
	}
	//refreshOrders.setRefreshHang(symbol, nil, nil)
	util.Notice(fmt.Sprintf(`[-----cancel hang done---]%s %s`, market, symbol))
	return hangBid != nil || hangAsk != nil
}

func preDeal(setting *model.Setting, market, symbol string, otherPrice, amount float64) (
	result bool, orderSide, reverseSide string, orderPrice float64) {
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	tick := model.AppMarkets.BidAsks[symbol][market]
	if tick.Asks[0].Price-tick.Bids[0].Price > priceDistance*1.1 && symbol != `btc_pax` {
		return false, "", "", 0
	}
	if tick.Bids[0].Price > otherPrice*(1+setting.BinanceDisMin) &&
		tick.Bids[0].Price < otherPrice*(1+setting.BinanceDisMax) {
		if tick.Bids[0].Price <= otherPrice*(1+model.AppConfig.BinanceOrderDis) {
			if tick.Bids[0].Amount < amount*setting.RefreshLimit &&
				tick.Bids[0].Amount > amount*setting.RefreshLimitLow &&
				tick.Asks[0].Amount > 2*tick.Bids[0].Amount &&
				tick.Asks[0].Amount < model.AppConfig.PreDealDis*tick.Bids[0].Amount {
				return true, model.OrderSideBuy, model.OrderSideSell, tick.Bids[0].Price
			}
		} else {
			bidAmount := tick.Bids[0].Amount
			orderPrice = tick.Bids[0].Price - priceDistance
			if len(tick.Bids) > 1 && tick.Bids[0].Price-tick.Bids[1].Price < 1.1*priceDistance {
				bidAmount += tick.Bids[1].Amount
			}
			if bidAmount < amount*setting.RefreshLimit && bidAmount > amount*setting.RefreshLimitLow &&
				tick.Asks[0].Amount > 2*tick.Bids[0].Amount &&
				tick.Asks[0].Amount < model.AppConfig.PreDealDis*tick.Bids[0].Amount {
				if orderPrice > tick.Bids[0].Price {
					util.Notice(fmt.Sprintf(`[price error] order price: %f bid1: %f`,
						orderPrice, tick.Bids[0].Price))
					orderPrice = tick.Bids[0].Price
				}
				return true, model.OrderSideBuy, model.OrderSideSell, orderPrice
			}
		}
	}
	if tick.Asks[0].Price > otherPrice*(1-setting.BinanceDisMax) &&
		tick.Asks[0].Price < otherPrice*(1-setting.BinanceDisMin) {
		if tick.Asks[0].Price >= otherPrice*(1-model.AppConfig.BinanceOrderDis) {
			if tick.Asks[0].Amount < amount*setting.RefreshLimit &&
				tick.Asks[0].Amount > amount*setting.RefreshLimitLow &&
				tick.Bids[0].Amount > 2*tick.Asks[0].Amount &&
				tick.Bids[0].Amount < model.AppConfig.PreDealDis*tick.Asks[0].Amount {
				return true, model.OrderSideSell, model.OrderSideBuy, tick.Asks[0].Price
			}
		} else {
			askAmount := tick.Asks[0].Amount
			orderPrice = tick.Asks[0].Price + priceDistance
			if len(tick.Asks) > 1 && tick.Asks[1].Price-tick.Asks[0].Price < 1.1*priceDistance {
				askAmount += tick.Asks[1].Amount
			}
			if askAmount < amount*setting.RefreshLimit && askAmount > amount*setting.RefreshLimitLow &&
				tick.Bids[0].Amount > 2*tick.Asks[0].Amount &&
				tick.Bids[0].Amount < model.AppConfig.PreDealDis*tick.Asks[0].Amount {
				if orderPrice < tick.Asks[0].Price {
					util.Notice(fmt.Sprintf(`[price error] order price: %f ask1 %f`,
						orderPrice, tick.Asks[0].Price))
					orderPrice = tick.Asks[0].Price
				}
				return true, model.OrderSideSell, model.OrderSideBuy, orderPrice
			}
		}
	}
	return false, ``, ``, 0
}

func getOtherPrice(market, symbol, otherMarket string) (result bool, otherPrice float64) {
	if market == model.Fcoin && symbol == `btc_pax` {
		if model.AppMarkets.BidAsks[`btc_usdt`] == nil || model.AppMarkets.BidAsks[`pax_usdt`] == nil {
			return false, 0
		}
		tickBtcUsdt := model.AppMarkets.BidAsks[`btc_usdt`][model.Fcoin]
		tickPaxUsdt := model.AppMarkets.BidAsks[`pax_usdt`][model.Fcoin]
		if tickBtcUsdt != nil && tickBtcUsdt.Bids != nil && tickBtcUsdt.Asks != nil && len(tickBtcUsdt.Bids) > 0 &&
			len(tickBtcUsdt.Asks) > 0 && tickPaxUsdt != nil && tickPaxUsdt.Bids != nil && tickPaxUsdt.Asks != nil &&
			len(tickPaxUsdt.Bids) > 0 && len(tickPaxUsdt.Asks) > 0 {
			pricePaxUsdt := (tickPaxUsdt.Asks[0].Price + tickPaxUsdt.Bids[0].Price) / 2
			priceBtcUsdt := (tickBtcUsdt.Asks[0].Price + tickBtcUsdt.Bids[0].Price) / 2
			if pricePaxUsdt == 0 {
				return false, 0
			} else {
				return true, priceBtcUsdt / pricePaxUsdt
			}
		} else {
			return false, 0
		}
	}
	binanceBidAsks := model.AppMarkets.BidAsks[symbol][otherMarket]
	if binanceBidAsks == nil || binanceBidAsks.Bids == nil || binanceBidAsks.Asks == nil ||
		binanceBidAsks.Bids.Len() == 0 || binanceBidAsks.Asks.Len() == 0 {
		return false, 0
	}
	delay := util.GetNowUnixMillion() - int64(binanceBidAsks.Ts)
	if delay > 5000 {
		util.Notice(fmt.Sprintf(`[%s %s]delay %d`, otherMarket, symbol, delay))
		return false, 0
	}
	return true, (binanceBidAsks.Bids[0].Price + binanceBidAsks.Asks[0].Price) / 2
}

func doRefresh(setting *model.Setting, market, symbol, accountType, orderSide, orderReverse string,
	price, priceDistance, amount float64, tick *model.BidAsk) {
	go receiveRefresh(market, symbol, accountType, price, priceDistance, amount, setting.AmountLimit)
	refreshOrders.SetLastOrder(market, symbol, model.OrderSideSell, nil)
	refreshOrders.SetLastOrder(market, symbol, model.OrderSideBuy, nil)
	if setting.RefreshSameTime == 1 {
		bidAmount := amount
		if math.Abs(tick.Bids[0].Price-price) < priceDistance {
			bidAmount = 0.9999 * amount
		}
		go placeRefreshOrder(model.OrderSideBuy, market, symbol, accountType, price, bidAmount)
		go placeRefreshOrder(model.OrderSideSell, market, symbol, accountType, price, amount)
	} else {
		placeRefreshOrder(orderSide, market, symbol, accountType, price, amount*0.9999)
		time.Sleep(time.Millisecond * time.Duration(model.AppConfig.Between))
		placeRefreshOrder(orderReverse, market, symbol, accountType, price, amount)
	}
}

func receiveRefresh(market, symbol, accountType string, price, priceDistance, amount, amountLimit float64) {
	for true {
		//util.Notice(fmt.Sprintf(`[before receive]%s %s %f %f`, market, symbol, price, amount))
		_ = <-syncRefresh
		//util.Notice(fmt.Sprintf(`[after receive]%s %s %f %f`, market, symbol, price, amount))
		refreshLastBid := refreshOrders.GetLastOrder(market, symbol, model.OrderSideBuy)
		refreshLastAsk := refreshOrders.GetLastOrder(market, symbol, model.OrderSideSell)
		if refreshLastBid != nil && refreshLastAsk != nil {
			if refreshLastBid.Status == model.CarryStatusWorking &&
				refreshLastAsk.Status == model.CarryStatusWorking {
				refreshOrders.AddRefreshAmount(market, symbol, 2*amount*price, amountLimit)
				refreshOrders.SetLastChancePrice(market, symbol, price)
				refreshOrders.SetLastRefreshPrice(market, symbol, price, priceDistance)
			} else {
				if refreshLastBid.Status == model.CarryStatusWorking &&
					refreshLastAsk.Status == model.CarryStatusFail {
					api.MustCancel(refreshLastBid.Market, refreshLastBid.Symbol, refreshLastBid.OrderId, true)
				} else if refreshLastAsk.Status == model.CarryStatusWorking &&
					refreshLastBid.Status == model.CarryStatusFail {
					api.MustCancel(refreshLastAsk.Market, refreshLastAsk.Symbol, refreshLastAsk.OrderId, true)
				}
				if refreshLastBid.Status == model.CarryStatusFail || refreshLastAsk.Status == model.CarryStatusFail {
					refreshOrders.setWaiting(symbol, true)
				}
				coins := strings.Split(symbol, `_`)
				coin := ``
				if refreshLastAsk.ErrCode == `1016` {
					coin = coins[0]
				}
				if refreshLastBid.ErrCode == `1016` {
					coin = coins[1]
				}
				refreshOrders.setNeedReset(symbol, accountType, coin)
			}
			break
		}
	}
}

func CancelAndRefresh(market string) {
	util.Notice(fmt.Sprintf(`[cancelAndRefresh]`))
	symbols := model.GetMarketSymbols(market)
	for key := range symbols {
		CancelRefreshHang(market, key)
	}
	time.Sleep(time.Second * 2)
	api.RefreshAccount(market)
}

func placeRefreshOrder(orderSide, market, symbol, accountType string, price, amount float64) {
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, accountType, price, amount)
	if order.Status == model.CarryStatusFail && order.ErrCode == `1002` {
		time.Sleep(time.Millisecond * 500)
		order = api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, accountType, price, amount)
	}
	order.Function = model.FunctionRefresh
	refreshOrders.SetLastOrder(market, symbol, orderSide, order)
	//util.Notice(fmt.Sprintf(`[before send]%s %s %f %f`, orderSide, symbol, price, amount))
	syncRefresh <- struct{}{}
	//util.Notice(fmt.Sprintf(`[after send]%s %s %f %f`, orderSide, symbol, price, amount))
	model.AppDB.Save(&order)
}
