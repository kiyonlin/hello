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

//var LastRefreshTime = make(map[string]int64) // market - int64
var refreshOrders = &RefreshOrders{}

type RefreshBidAsk struct {
	lock sync.Mutex
	bid  *model.Order
	ask  *model.Order
}

func (refreshBidAsk *RefreshBidAsk) get() (bidOrder, askOrder *model.Order) {
	defer refreshBidAsk.lock.Unlock()
	refreshBidAsk.lock.Lock()
	return refreshBidAsk.bid, refreshBidAsk.ask
}

func (refreshBidAsk *RefreshBidAsk) set(bidOrder, askOrder *model.Order) {
	defer refreshBidAsk.lock.Unlock()
	refreshBidAsk.lock.Lock()
	if bidOrder != nil {
		refreshBidAsk.bid = bidOrder
	}
	if askOrder != nil {
		refreshBidAsk.ask = askOrder
	}
}

type RefreshOrders struct {
	lock             sync.Mutex
	refreshing       bool
	hanging          bool
	samePriceCount   map[string]map[string]int             // market - symbol - continue same price count
	samePriceTime    map[string]map[string]*time.Time      // market - symbol - first time new order price
	bidOrders        map[string]map[string][]*model.Order  // market - symbol - orders
	askOrders        map[string]map[string][]*model.Order  // market - symbol - orders
	amountLimit      map[string]map[string]map[int]float64 // market - symbol - time start point - amount
	lastChancePrice  map[string]map[string]float64         // market - symbol - chance price
	lastRefreshPrice map[string]map[string]float64         // market - symbol - refresh price
	fcoinHang        map[string][]*model.Order             // symbol - refresh hang order 0:bid1 1:ask1 2:bid2 3:ask2
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

func (refreshOrders *RefreshOrders) removeRefreshHang(symbol string, hangBid1, hangAsk1, hangBid2, hangAsk2 *model.Order) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.fcoinHang == nil || refreshOrders.fcoinHang[symbol] == nil {
		return
	}
	if hangBid1 != nil && refreshOrders.fcoinHang[symbol][0] != nil &&
		refreshOrders.fcoinHang[symbol][0].OrderId == hangBid1.OrderId {
		refreshOrders.fcoinHang[symbol][0] = nil
	}
	if hangAsk1 != nil && refreshOrders.fcoinHang[symbol][1] != nil &&
		refreshOrders.fcoinHang[symbol][1].OrderId == hangAsk1.OrderId {
		refreshOrders.fcoinHang[symbol][1] = nil
	}
	if hangBid2 != nil && refreshOrders.fcoinHang[symbol][2] != nil &&
		refreshOrders.fcoinHang[symbol][2].OrderId == hangBid2.OrderId {
		refreshOrders.fcoinHang[symbol][2] = nil
	}
	if hangAsk2 != nil && refreshOrders.fcoinHang[symbol][3] != nil &&
		refreshOrders.fcoinHang[symbol][3].OrderId == hangAsk2.OrderId {
		refreshOrders.fcoinHang[symbol][3] = nil
	}
}

func (refreshOrders *RefreshOrders) setRefreshHang(symbol string, hangBid1, hangAsk1, hangBid2, hangAsk2 *model.Order) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.fcoinHang == nil {
		refreshOrders.fcoinHang = make(map[string][]*model.Order)
	}
	refreshOrders.fcoinHang[symbol] = make([]*model.Order, 4)
	refreshOrders.fcoinHang[symbol][0] = hangBid1
	refreshOrders.fcoinHang[symbol][1] = hangAsk1
	refreshOrders.fcoinHang[symbol][2] = hangBid2
	refreshOrders.fcoinHang[symbol][3] = hangAsk2
}

func (refreshOrders *RefreshOrders) getRefreshHang(symbol string) (hangBid1, hangAsk1, hangBid2, hangAsk2 *model.Order) {
	defer refreshOrders.lock.Unlock()
	refreshOrders.lock.Lock()
	if refreshOrders.fcoinHang == nil {
		refreshOrders.fcoinHang = make(map[string][]*model.Order)
	}
	if refreshOrders.fcoinHang[symbol] == nil {
		refreshOrders.fcoinHang[symbol] = make([]*model.Order, 2)
	}
	return refreshOrders.fcoinHang[symbol][0], refreshOrders.fcoinHang[symbol][1], refreshOrders.fcoinHang[symbol][2],
		refreshOrders.fcoinHang[symbol][3]
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
	result, tick := model.AppMarkets.GetBidAsk(symbol, market)
	if !result {
		CancelRefreshHang(market, symbol)
		return
	}
	if tick == nil || tick.Asks == nil || tick.Bids == nil || tick.Asks.Len() < 15 || tick.Bids.Len() < 15 {
		util.Notice(fmt.Sprintf(`[tick not good]%s %s`, market, symbol))
		CancelRefreshHang(market, symbol)
		return
	}
	result, otherPrice := getOtherPrice(market, symbol, model.Huobi)
	if !result || otherPrice == 0 {
		util.Notice(fmt.Sprintf(`[get other price]%s %f`, symbol, otherPrice))
		//CancelRefreshHang(market, symbol)
		return
	}
	setting := model.GetSetting(model.FunctionRefresh, market, symbol)
	leftFree, rightFree, _, _, err := getBalance(market, symbol, setting.AccountType)
	if err != nil || (leftFree == 0 && rightFree == 0) {
		CancelRefreshHang(market, symbol)
		return
	}
	hangRate1 := 0.0
	hangRate2 := 0.0
	amountLimit := 0.0
	parameters := strings.Split(setting.FunctionParameter, `_`)
	if len(parameters) == 3 {
		hangRate1, _ = strconv.ParseFloat(parameters[0], 64)
		amountLimit, _ = strconv.ParseFloat(parameters[1], 64)
		hangRate2, _ = strconv.ParseFloat(parameters[2], 64)
	}
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	if util.GetNowUnixMillion()-int64(tick.Ts) > 1000 {
		util.SocketInfo(fmt.Sprintf(`socekt old tick %d %d`, util.GetNowUnixMillion(), tick.Ts))
		CancelRefreshHang(market, symbol)
	}
	go validRefreshHang(symbol, amountLimit, otherPrice, priceDistance, tick)
	if model.AppConfig.Handle != `1` || model.AppConfig.HandleRefresh != `1` || model.AppPause {
		util.Notice(fmt.Sprintf(`[status]%s %s %v is pause:%v`,
			model.AppConfig.Handle, model.AppConfig.HandleRefresh, refreshOrders.refreshing, model.AppPause))
		CancelRefreshHang(market, symbol)
		return
	}
	if refreshOrders.refreshing {
		return
	}
	refreshOrders.setRefreshing(true)
	defer refreshOrders.setRefreshing(false)
	delay := util.GetNowUnixMillion() - int64(tick.Ts)
	//checkDelay := model.AppConfig.Delay
	//if strings.Contains(symbol, `pax`) {
	//	checkDelay = 20 * checkDelay
	//}
	//if float64(delay) > checkDelay {
	//	util.Notice(fmt.Sprintf(`[delay long, cancel hang], %s %s`, market, symbol))
	//	CancelRefreshHang(market, symbol)
	//}
	if delay > 500 {
		util.Info(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
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
		time.Sleep(time.Second * 2)
		refreshOrders.amountIndex = index
		symbols := model.GetMarketSymbols(market)
		for key := range symbols {
			CancelRefreshHang(market, key)
			util.Notice(`[setRefreshHang nil]` + key)
			refreshOrders.setRefreshHang(key, nil, nil, nil, nil)
			refreshOrders.setInRefresh(key, false)
		}
		time.Sleep(time.Second * 2)
		api.RefreshAccount(market)
		util.Notice(fmt.Sprintf(`[after 10min canceling]`))
		return
	}
	amount := math.Min(leftFree, rightFree/tick.Asks[0].Price) * model.AppConfig.AmountRate
	util.Notice(fmt.Sprintf(`amount %f left %f right %f`, amount, leftFree, rightFree/tick.Asks[0].Price))
	refreshAble, orderSide, orderReverse, orderPrice := preDeal(setting, market, symbol, otherPrice, amount, tick)
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
				refreshHang(market, symbol, setting.AccountType, hangRate1, hangRate2, amountLimit, leftFree, rightFree,
					otherPrice, tick)
			}
		} else {
			util.Info(fmt.Sprintf(`[in hang not have amount %s]`, symbol))
			refreshHang(market, symbol, setting.AccountType, hangRate1, hangRate2, amountLimit, leftFree, rightFree,
				otherPrice, tick)
		}
	}
}

func refreshHang(market, symbol, accountType string,
	hangRate1, hangRate2, amountLimit, leftFree, rightFree, otherPrice float64, tick *model.BidAsk) {
	util.Info(fmt.Sprintf(`[refreshhang]%s`, symbol))
	if refreshOrders.hanging {
		return
	}
	defer refreshOrders.setHanging(false)
	refreshOrders.setHanging(true)
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
	hangBid1, hangAsk1, hangBid2, hangAsk2 := refreshOrders.getRefreshHang(symbol)
	if hangBid1 == nil && bidAll > amountLimit && otherPrice*1.0005 >= tick.Bids[9].Price && hangRate1 > 0 {
		hangBid1 = api.PlaceOrder(model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``,
			accountType, tick.Bids[9].Price, rightFree*hangRate1)
		if hangBid1 != nil && hangBid1.OrderId != `` && hangBid1.Status != model.CarryStatusFail {
			hangBid1.Function = model.FunctionHang
			model.AppDB.Save(&hangBid1)
		} else if hangBid1 != nil && hangBid1.ErrCode == `1016` {
			hangBid1 = nil
			coin = coins[1]
			needRefresh = true
		}
	}
	if hangAsk1 == nil && askAll > amountLimit && otherPrice*0.9995 <= tick.Asks[9].Price && hangRate1 > 0 {
		hangAsk1 = api.PlaceOrder(model.OrderSideSell, model.OrderTypeLimit, market, symbol, ``,
			accountType, tick.Asks[9].Price, leftFree*hangRate1)
		if hangAsk1 != nil && hangAsk1.OrderId != `` && hangAsk1.Status != model.CarryStatusFail {
			hangAsk1.Function = model.FunctionHang
			model.AppDB.Save(&hangAsk1)
		} else if hangAsk1 != nil && hangAsk1.ErrCode == `1016` {
			hangAsk1 = nil
			coin = coins[0]
			needRefresh = true
		}
	}
	if hangBid2 == nil && hangRate2 > 0 {
		hangBid2 = api.PlaceOrder(model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``, accountType,
			tick.Bids[0].Price*0.98, rightFree*hangRate2)
		if hangBid2 != nil && hangBid2.OrderId != `` && hangBid2.Status != model.CarryStatusFail {
			hangBid2.Function = model.FunctionHang
			model.AppDB.Save(&hangBid2)
		} else if hangBid2 != nil && hangBid2.ErrCode == `1016` {
			hangBid2 = nil
			coin = coins[1]
			needRefresh = true
		}
	}
	if hangAsk2 == nil && hangRate2 > 0 {
		hangAsk2 = api.PlaceOrder(model.OrderSideSell, model.OrderTypeLimit, market, symbol, ``, accountType,
			tick.Asks[0].Price*1.02, leftFree*hangRate2)
		if hangAsk2 != nil && hangAsk2.OrderId != `` && hangAsk2.Status != model.CarryStatusFail {
			hangAsk2.Function = model.FunctionHang
			model.AppDB.Save(&hangAsk2)
		} else if hangAsk2 != nil && hangAsk2.ErrCode == `1016` {
			hangAsk2 = nil
			coin = coins[0]
			needRefresh = true
		}
	}
	refreshOrders.setRefreshHang(symbol, hangBid1, hangAsk1, hangBid2, hangAsk2)
	if needRefresh {
		CancelRefreshHang(market, symbol)
		time.Sleep(time.Second * 2)
		api.RefreshCoinAccount(market, symbol, coin, accountType)
	}
	util.Info(fmt.Sprintf(`[refreshhang done]%s`, symbol))
}

func validRefreshHang(symbol string, amountLimit, otherPrice, priceDistance float64, tick *model.BidAsk) {
	hangBid1, hangAsk1, hangBid2, hangAsk2 := refreshOrders.getRefreshHang(symbol)
	if hangBid1 != nil {
		bidAll := 0.0
		for i := 0; i < tick.Bids.Len() && tick.Bids[i].Price-0.1*priceDistance > hangBid1.Price; i++ {
			bidAll += tick.Bids[i].Amount
		}
		if hangBid1.Price > tick.Bids[4].Price+0.1*priceDistance ||
			hangBid1.Price < tick.Bids[14].Price-0.1*priceDistance ||
			bidAll < amountLimit || hangBid1.Price > 1.0005*otherPrice {
			util.Notice(fmt.Sprintf(`[cancelhangbid1]%s %s bid %f<bid15 %f>1.0005huobi %f || amount%f<limit %f`,
				symbol, hangBid1.OrderId, hangBid1.Price, tick.Bids[15].Price, 1.0005*otherPrice, bidAll, amountLimit))
			go api.MustCancel(hangBid1.Market, symbol, hangBid1.OrderId, true)
			refreshOrders.removeRefreshHang(symbol, hangBid1, nil, nil, nil)
			refreshOrders.setWaiting(symbol, true)
		}
	}
	if hangAsk1 != nil {
		askAll := 0.0
		for i := 0; i < tick.Asks.Len() && tick.Asks[i].Price+0.1*priceDistance < hangAsk1.Price; i++ {
			askAll += tick.Asks[i].Amount
		}
		if hangAsk1.Price < tick.Asks[4].Price-0.1*priceDistance ||
			hangAsk1.Price > tick.Asks[14].Price+0.1*priceDistance ||
			askAll < amountLimit || hangAsk1.Price < 0.9995*otherPrice {
			util.Notice(fmt.Sprintf(`[cancelhangask1]%s %s ask %f>ask15 %f<0.9995huobi%f || amount%f <limit %f`,
				symbol, hangAsk1.OrderId, hangAsk1.Price, tick.Asks[14].Price, 0.9995*otherPrice, askAll, amountLimit))
			go api.MustCancel(hangAsk1.Market, symbol, hangAsk1.OrderId, true)
			refreshOrders.removeRefreshHang(symbol, nil, hangAsk1, nil, nil)
			refreshOrders.setWaiting(symbol, true)
		}
	}
	if hangBid2 != nil && hangBid2.Price > tick.Bids[0].Price*0.99 {
		util.Notice(fmt.Sprintf(`[cancelhangbid2]%s %s %f`, symbol, hangBid2.OrderId, hangBid2.Price))
		go api.MustCancel(hangBid2.Market, symbol, hangBid2.OrderId, true)
		refreshOrders.removeRefreshHang(symbol, nil, nil, hangBid2, nil)
		refreshOrders.setWaiting(symbol, true)
	}
	if hangAsk2 != nil && hangAsk2.Price < tick.Asks[0].Price*1.01 {
		util.Notice(fmt.Sprintf(`[cancelhangask2]%s %s %f`, symbol, hangAsk2.OrderId, hangAsk2.Price))
		go api.MustCancel(hangAsk2.Market, symbol, hangAsk2.OrderId, true)
		refreshOrders.removeRefreshHang(symbol, nil, nil, nil, hangAsk2)
		refreshOrders.setWaiting(symbol, true)

	}
}

func CancelRefreshHang(market, symbol string) (needCancel bool) {
	hangBid1, hangAsk1, hangBid2, hangAsk2 := refreshOrders.getRefreshHang(symbol)
	if hangBid1 != nil {
		util.Notice(fmt.Sprintf(`[---cancel hang bid1---]%s %s %s`, market, symbol, hangBid1.OrderId))
		api.MustCancel(hangBid1.Market, symbol, hangBid1.OrderId, true)
	}
	if hangAsk1 != nil {
		util.Notice(fmt.Sprintf(`[---cancel hang ask1---]%s %s %s`, market, symbol, hangAsk1.OrderId))
		api.MustCancel(hangAsk1.Market, symbol, hangAsk1.OrderId, true)
	}
	if hangBid2 != nil {
		util.Notice(fmt.Sprintf(`[---cancel hang bid2---]%s %s %s`, market, symbol, hangBid2.OrderId))
		api.MustCancel(hangBid2.Market, symbol, hangBid2.OrderId, true)
	}
	if hangAsk2 != nil {
		util.Notice(fmt.Sprintf(`[---cancel hang ask2---]%s %s %s`, market, symbol, hangAsk2.OrderId))
		api.MustCancel(hangAsk2.Market, symbol, hangAsk2.OrderId, true)
	}
	return hangBid1 != nil || hangAsk1 != nil || hangBid2 != nil || hangAsk2 != nil
}

func preDeal(setting *model.Setting, market, symbol string, otherPrice, amount float64, tick *model.BidAsk) (
	result bool, orderSide, reverseSide string, orderPrice float64) {
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
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
			} else if tick.Bids[0].Amount < amount*setting.RefreshLimit &&
				tick.Bids[0].Amount > amount*setting.RefreshLimitLow &&
				tick.Asks[0].Amount > 2*tick.Bids[0].Amount &&
				tick.Asks[0].Amount < model.AppConfig.PreDealDis*tick.Bids[0].Amount {
				return true, model.OrderSideBuy, model.OrderSideSell, tick.Bids[0].Price
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
			} else if tick.Asks[0].Amount < amount*setting.RefreshLimit &&
				tick.Asks[0].Amount > amount*setting.RefreshLimitLow &&
				tick.Bids[0].Amount > 2*tick.Asks[0].Amount &&
				tick.Bids[0].Amount < model.AppConfig.PreDealDis*tick.Asks[0].Amount {
				return true, model.OrderSideSell, model.OrderSideBuy, tick.Asks[0].Price
			}
		}
	}
	return false, ``, ``, 0
}

func getOtherPrice(market, symbol, otherMarket string) (result bool, otherPrice float64) {
	if market == model.Fcoin && symbol == `btc_pax` {
		btcUsdtResult, tickBtcUsdt := model.AppMarkets.GetBidAsk(`btc_usdt`, model.Fcoin)
		paxUsdtResult, tickPaxUsdt := model.AppMarkets.GetBidAsk(`pax_usdt`, model.Fcoin)
		if btcUsdtResult && paxUsdtResult {
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
	otherResult, otherTick := model.AppMarkets.GetBidAsk(symbol, otherMarket)
	if !otherResult {
		return false, 0
	}
	delay := util.GetNowUnixMillion() - int64(otherTick.Ts)
	if delay > 9000 {
		util.Notice(fmt.Sprintf(`[%s %s]delay %d`, otherMarket, symbol, delay))
		return false, 0
	}
	return true, (otherTick.Bids[0].Price + otherTick.Asks[0].Price) / 2
}

func doRefresh(setting *model.Setting, market, symbol, accountType, orderSide, orderReverse string,
	price, priceDistance, amount float64, tick *model.BidAsk) {
	util.Notice(fmt.Sprintf(`[doRefresh]%s %s %s %f %f`, symbol, accountType, orderSide, price, amount))
	//orders := make([]*model.Order, 2)
	orders := &RefreshBidAsk{}
	go receiveRefresh(orders, market, symbol, accountType, price, priceDistance, amount, setting.AmountLimit)
	bidAmount := amount
	askAmount := amount
	if tick.Asks[0].Price-price > priceDistance {
		bidAmount = 0.9998 * amount
	}
	if price-tick.Bids[0].Price > priceDistance {
		askAmount = 0.9998 * amount
	}
	if setting.RefreshSameTime == 1 {
		go placeRefreshOrder(orders, model.OrderSideBuy, market, symbol, accountType, price, bidAmount)
		go placeRefreshOrder(orders, model.OrderSideSell, market, symbol, accountType, price, askAmount)
	} else {
		if orderSide == model.OrderSideBuy && orderReverse == model.OrderSideSell {
			placeRefreshOrder(orders, model.OrderSideBuy, market, symbol, accountType, price, bidAmount)
			time.Sleep(time.Millisecond * time.Duration(model.AppConfig.Between))
			placeRefreshOrder(orders, model.OrderSideSell, market, symbol, accountType, price, askAmount)
		} else if orderSide == model.OrderSideSell && orderReverse == model.OrderSideBuy {
			placeRefreshOrder(orders, model.OrderSideSell, market, symbol, accountType, price, askAmount)
			time.Sleep(time.Millisecond * time.Duration(model.AppConfig.Between))
			placeRefreshOrder(orders, model.OrderSideBuy, market, symbol, accountType, price, bidAmount)
		}
	}
}

func receiveRefresh(orders *RefreshBidAsk, market, symbol, accountType string,
	price, priceDistance, amount, amountLimit float64) {
	for true {
		refreshLastBid, refreshLastAsk := orders.get()
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
		} else {
			time.Sleep(time.Millisecond * 100)
		}
	}
}

func placeRefreshOrder(orders *RefreshBidAsk, orderSide, market, symbol, accountType string, price, amount float64) {
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, accountType, price, amount)
	if order.Status == model.CarryStatusFail && order.ErrCode == `1002` {
		time.Sleep(time.Millisecond * 500)
		order = api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, accountType, price, amount)
	}
	order.Function = model.FunctionRefresh
	if orderSide == model.OrderSideBuy {
		orders.set(order, nil)
	} else {
		orders.set(nil, order)
	}
	model.AppDB.Save(&order)
}
