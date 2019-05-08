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
var canceling = false

type RefreshOrders struct {
	lock             sync.Mutex
	refreshing       bool
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
	amountIndex      int
}

func (refreshOrders *RefreshOrders) setInRefresh(symbol string, in bool) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if refreshOrders.inRefresh == nil {
		refreshOrders.inRefresh = make(map[string]bool)
	}
	refreshOrders.inRefresh[symbol] = in
}

func (refreshOrders *RefreshOrders) getInRefresh(symbol string) (in bool) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if refreshOrders.inRefresh == nil {
		return false
	}
	return refreshOrders.inRefresh[symbol]
}

func (refreshOrders *RefreshOrders) setRefreshHang(symbol string, hangBid, hangAsk *model.Order) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if refreshOrders.fcoinHang == nil {
		refreshOrders.fcoinHang = make(map[string][]*model.Order)
	}
	refreshOrders.fcoinHang[symbol] = make([]*model.Order, 2)
	refreshOrders.fcoinHang[symbol][0] = hangBid
	refreshOrders.fcoinHang[symbol][1] = hangAsk
}

func (refreshOrders *RefreshOrders) getRefreshHang(symbol string) (hangBid, hangAsk *model.Order) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if refreshOrders.fcoinHang == nil {
		refreshOrders.fcoinHang = make(map[string][]*model.Order)
	}
	if refreshOrders.fcoinHang[symbol] == nil {
		refreshOrders.fcoinHang[symbol] = make([]*model.Order, 2)
	}
	return refreshOrders.fcoinHang[symbol][0], refreshOrders.fcoinHang[symbol][1]
}

func (refreshOrders *RefreshOrders) SetLastChancePrice(market, symbol string, price float64) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if refreshOrders.lastChancePrice == nil {
		refreshOrders.lastChancePrice = make(map[string]map[string]float64)
	}
	if refreshOrders.lastChancePrice[market] == nil {
		refreshOrders.lastChancePrice[market] = make(map[string]float64)
	}
	refreshOrders.lastChancePrice[market][symbol] = price
}

func (refreshOrders *RefreshOrders) CheckLastChancePrice(market, symbol string, price, priceDistance float64) (same bool) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if refreshOrders.lastChancePrice == nil || refreshOrders.lastChancePrice[market] == nil {
		return false
	}
	if refreshOrders.lastChancePrice[market][symbol] > 0 {
		return true
	}
	return false
}

func (refreshOrders *RefreshOrders) SetLastRefreshPrice(market, symbol string, price, priceDistance float64) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
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
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
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
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	if refreshOrders.amountLimit == nil || refreshOrders.amountLimit[market] == nil ||
		refreshOrders.amountLimit[market][symbol] == nil {
		return true, -1
	}
	now := util.GetNow()
	amountIndex = int((now.Hour()*3600 + now.Minute()*60 + now.Second()) / model.RefreshTimeSlot)
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
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
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
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
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
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
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
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
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

func (refreshOrders *RefreshOrders) CancelRefreshOrders(market, symbol string, bidPrice, askPrice float64, process bool) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	canceling = true
	if refreshOrders.askOrders == nil {
		refreshOrders.askOrders = make(map[string]map[string][]*model.Order)
	}
	if refreshOrders.askOrders[market] == nil {
		refreshOrders.askOrders[market] = make(map[string][]*model.Order)
	}
	if refreshOrders.askOrders[market][symbol] == nil {
		refreshOrders.askOrders[market][symbol] = make([]*model.Order, 0)
	}
	if refreshOrders.bidOrders == nil {
		refreshOrders.bidOrders = make(map[string]map[string][]*model.Order)
	}
	if refreshOrders.bidOrders[market] == nil {
		refreshOrders.bidOrders[market] = make(map[string][]*model.Order)
	}
	if refreshOrders.bidOrders[market][symbol] == nil {
		refreshOrders.bidOrders[market][symbol] = make([]*model.Order, 0)
	}
	bidOrders := make([]*model.Order, 0)
	askOrders := make([]*model.Order, 0)
	d, _ := time.ParseDuration("-3601s")
	timeLine := util.GetNow().Add(d)
	for _, value := range refreshOrders.bidOrders[market][symbol] {
		if value.Price < bidPrice && value.OrderTime.Before(timeLine) && process { // 大于等于卖一的买单已经成交，无需取消
			util.Notice(fmt.Sprintf(`[try cancel]bid %f < %f`, value.Price, bidPrice))
			api.MustCancel(value.Market, value.Symbol, value.OrderId, true)
			time.Sleep(time.Millisecond * 100)
		} else if value.Price < askPrice && value.Status == model.CarryStatusWorking {
			bidOrders = append(bidOrders, value)
		}
	}
	for _, value := range refreshOrders.askOrders[market][symbol] {
		if value.Price > askPrice && value.OrderTime.Before(timeLine) && process { // 小于等于买一的卖单已经成交，无需取消
			util.Notice(fmt.Sprintf(`[try cancel]ask %f > %f`, value.Price, askPrice))
			api.MustCancel(value.Market, value.Symbol, value.OrderId, true)
			time.Sleep(time.Millisecond * 100)
		} else if value.Price > bidPrice && value.Status == model.CarryStatusWorking {
			askOrders = append(askOrders, value)
		}
	}
	refreshOrders.bidOrders[market][symbol] = bidOrders
	refreshOrders.askOrders[market][symbol] = askOrders
	canceling = false
}

func (refreshOrders *RefreshOrders) setRefreshing(market, symbol string, refreshing bool) (current bool) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
	current = refreshOrders.refreshing
	refreshOrders.refreshing = refreshing
	return current
}

func (refreshOrders *RefreshOrders) getCurrencies() (currencies map[string]bool) {
	if refreshOrders.fcoinHang == nil {
		return make(map[string]bool)
	}
	currencies = make(map[string]bool)
	for key := range refreshOrders.fcoinHang {
		bid, ask := refreshOrders.getRefreshHang(key)
		if bid != nil || ask != nil {
			coins := strings.Split(key, `_`)
			if len(coins) >= 2 {
				currencies[coins[0]] = true
				currencies[coins[1]] = true
			}
		}
	}
	return currencies
}

var ProcessRefresh = func(market, symbol string) {
	delay := util.GetNowUnixMillion() - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
	if model.AppMarkets.BidAsks[symbol] == nil {
		return
	}
	tick := model.AppMarkets.BidAsks[symbol][market]
	if tick == nil || tick.Asks == nil || tick.Bids == nil || tick.Asks.Len() < 15 || tick.Bids.Len() < 15 {
		util.Notice(fmt.Sprintf(`[tick not good]%s %s`, market, symbol))
		return
	}
	binanceResult, binancePrice := getBinanceInfo(market, symbol)
	if delay > 200 || !binanceResult {
		util.Info(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
	setting := model.GetSetting(model.FunctionRefresh, market, symbol)
	leftFree, rightFree, _, _, err := getBalance(market, symbol, setting.AccountType)
	if err != nil {
		return
	}
	hangRate := 0.0
	amountLimit := 0.0
	parameters := strings.Split(setting.FunctionParameter, `_`)
	if len(parameters) == 2 {
		hangRate, _ = strconv.ParseFloat(parameters[0], 64)
		amountLimit, _ = strconv.ParseFloat(parameters[1], 64)
	}
	go validRefreshHang(market, symbol, amountLimit, binancePrice, tick)
	if model.AppConfig.Handle != `1` || model.AppConfig.HandleRefresh != `1` ||
		refreshOrders.setRefreshing(market, symbol, true) {
		return
	}
	defer refreshOrders.setRefreshing(market, symbol, false)
	haveAmount, index := refreshOrders.CheckAmountLimit(market, symbol, setting.AmountLimit)
	if index == 0 {
		refreshOrders.amountIndex = 0
	}
	if index > refreshOrders.amountIndex {
		refreshOrders.amountIndex = index
		CancelAndRefresh(market)
		return
	}
	amount := math.Min(leftFree, rightFree/tick.Asks[0].Price) * model.AppConfig.AmountRate
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	if canceling {
		util.Notice(`[refreshing waiting for canceling]`)
		return
	}
	refreshAble, orderSide, orderReverse, orderPrice := preDeal(setting, market, symbol, binancePrice, amount)
	if refreshOrders.CheckLastChancePrice(market, symbol, orderPrice, 0.9*priceDistance) {
		refreshOrders.SetLastChancePrice(market, symbol, 0)
		refreshAble = false
	} else if refreshOrders.CheckLastRefreshPrice(market, symbol, orderPrice, 0.9*priceDistance) {
		refreshAble = false
	}
	if refreshOrders.getInRefresh(symbol) {
		if haveAmount {
			if refreshAble {
				doRefresh(setting, market, symbol, setting.AccountType, orderSide, orderReverse, orderPrice,
					0.9*priceDistance, amount, tick)
			}
		} else {
			refreshOrders.setInRefresh(symbol, false)
			time.Sleep(time.Second)
		}
	} else {
		if haveAmount {
			if refreshAble {
				refreshOrders.setInRefresh(symbol, true)
				CancelRefreshHang(market, symbol)
			} else {
				refreshOrders.refreshHang(market, symbol, setting.AccountType, hangRate, amountLimit, leftFree, rightFree,
					binancePrice, tick)
			}
		} else {
			refreshOrders.refreshHang(market, symbol, setting.AccountType, hangRate, amountLimit, leftFree, rightFree,
				binancePrice, tick)
		}
	}
}

func (refreshOrders *RefreshOrders) refreshHang(market, symbol, accountType string,
	hangRate, amountLimit, leftFree, rightFree, binancePrice float64, tick *model.BidAsk) {
	refreshOrders.lock.Lock()
	defer refreshOrders.lock.Unlock()
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
	if hangAsk == nil && askAll > amountLimit && binancePrice*0.9997 <= tick.Asks[9].Price {
		hangAsk = api.PlaceOrder(model.OrderSideSell, model.OrderTypeLimit, market, symbol, ``,
			accountType, tick.Asks[9].Price, leftFree*hangRate)
		if hangAsk != nil && hangAsk.OrderId != `` && hangAsk.Status != model.CarryStatusFail {
			hangAsk.Function = model.FunctionHang
			model.AppDB.Save(&hangAsk)
		} else if hangAsk != nil && hangAsk.ErrCode == `1016` {
			coin = coins[0]
			needRefresh = true
		}
	}
	if hangBid == nil && bidAll > amountLimit && binancePrice*1.0003 >= tick.Bids[9].Price {
		hangBid = api.PlaceOrder(model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``,
			accountType, tick.Bids[9].Price, rightFree*hangRate)
		if hangBid != nil && hangBid.OrderId != `` && hangBid.Status != model.CarryStatusFail {
			hangBid.Function = model.FunctionHang
			model.AppDB.Save(&hangBid)
		} else if hangBid != nil && hangBid.ErrCode == `1016` {
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
}

func validRefreshHang(market, symbol string, amountLimit, binancePrice float64, tick *model.BidAsk) {
	needCancel := false
	hangBid, hangAsk := refreshOrders.getRefreshHang(symbol)
	if hangBid != nil {
		bidAll := 0.0
		for i := 0; i < tick.Bids.Len() && tick.Bids[i].Price > hangBid.Price; i++ {
			bidAll += tick.Bids[i].Amount
		}
		if hangBid.Price < tick.Bids[14].Price || bidAll < amountLimit || hangBid.Price > 1.0003*binancePrice {
			util.Notice(fmt.Sprintf(`[cancelhangbid]%s %f <bid15:%f bidall:%f < amount:%f price %f > binance %f`,
				symbol, hangBid.Price, tick.Bids[14].Price, bidAll, amountLimit, hangBid.Price, binancePrice))
			needCancel = true
			api.MustCancel(market, symbol, hangBid.OrderId, true)
			hangBid = nil
		}
	}
	if hangAsk != nil {
		askAll := 0.0
		for i := 0; i < tick.Asks.Len() && tick.Asks[i].Price < hangAsk.Price; i++ {
			askAll += tick.Asks[i].Amount
		}
		if hangAsk.Price > tick.Asks[14].Price || askAll < amountLimit || hangAsk.Price < 0.9997*binancePrice {
			util.Notice(fmt.Sprintf(`[cancelhangask]%s %f >ask15:%f askall:%f < amount:%f price %f < binance %f`,
				symbol, hangAsk.Price, tick.Asks[14].Price, askAll, amountLimit, hangAsk.Price, binancePrice))
			needCancel = true
			api.MustCancel(market, symbol, hangAsk.OrderId, true)
			hangAsk = nil
		}
	}
	refreshOrders.setRefreshHang(symbol, hangBid, hangAsk)
	if needCancel {
		util.Notice(fmt.Sprintf(`[hang] %s %s need cancel`, market, symbol))
	}
}

func CancelRefreshHang(market, symbol string) (needCancel bool) {
	hangBid, hangAsk := refreshOrders.getRefreshHang(symbol)
	if hangBid != nil {
		api.MustCancel(market, symbol, hangBid.OrderId, true)
	}
	if hangAsk != nil {
		api.MustCancel(market, symbol, hangAsk.OrderId, true)
	}
	refreshOrders.setRefreshHang(symbol, nil, nil)
	return hangBid != nil || hangAsk != nil
}

func preDeal(setting *model.Setting, market, symbol string, binancePrice, amount float64) (
	result bool, orderSide, reverseSide string, orderPrice float64) {
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	tick := model.AppMarkets.BidAsks[symbol][market]
	if math.Abs(tick.Bids[0].Price-tick.Asks[0].Price) > priceDistance*1.1 && symbol != `btc_pax` {
		return false, "", "", 0
	}
	if tick.Bids[0].Price > binancePrice*(1+setting.BinanceDisMin) &&
		tick.Bids[0].Price < binancePrice*(1+setting.BinanceDisMax) {
		if tick.Bids[0].Price <= binancePrice*(1+model.AppConfig.BinanceOrderDis) {
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
	if tick.Asks[0].Price > binancePrice*(1-setting.BinanceDisMax) &&
		tick.Asks[0].Price < binancePrice*(1-setting.BinanceDisMin) {
		if tick.Asks[0].Price >= binancePrice*(1-model.AppConfig.BinanceOrderDis) {
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

func getBinanceInfo(market, symbol string) (result bool, binancePrice float64) {
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
	binanceBidAsks := model.AppMarkets.BidAsks[symbol][model.Binance]
	if binanceBidAsks == nil || binanceBidAsks.Bids == nil || binanceBidAsks.Asks == nil ||
		binanceBidAsks.Bids.Len() == 0 || binanceBidAsks.Asks.Len() == 0 {
		return false, 0
	}
	delay := util.GetNowUnixMillion() - int64(binanceBidAsks.Ts)
	if delay > 5000 {
		util.Notice(fmt.Sprintf(`[binance %s]delay %d`, symbol, delay))
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
		refreshLastBid := refreshOrders.GetLastOrder(market, symbol, model.OrderSideSell)
		refreshLastAsk := refreshOrders.GetLastOrder(market, symbol, model.OrderSideBuy)
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
				time.Sleep(time.Second)
				coins := strings.Split(symbol, `_`)
				coin := ``
				if refreshLastAsk.ErrCode == `1016` {
					coin = coins[0]
				}
				if refreshLastBid.ErrCode == `1016` {
					coin = coins[1]
				}
				if coin != `` {
					api.RefreshCoinAccount(market, symbol, coin, accountType)
				}
			}
			break
		}
	}
}

func CancelAndRefresh(market string) {
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
