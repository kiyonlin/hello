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

var hangStatus = HangStatus{}

type HangStatus struct {
	lock         sync.Mutex
	hanging      map[string]bool         // symbol - *time
	lastHangTime map[string]*time.Time   // symbol - *time
	bid          map[string]*model.Order // symbol - *order
	ask          map[string]*model.Order // symbol - *order
}

func (hangStatus *HangStatus) getHangOrders(symbol string) (bid, ask *model.Order) {
	hangStatus.lock.Lock()
	defer hangStatus.lock.Unlock()
	if hangStatus.bid == nil || hangStatus.ask == nil {
		return nil, nil
	}
	return hangStatus.bid[symbol], hangStatus.ask[symbol]
}

func (hangStatus *HangStatus) setHangOrders(symbol string, bid, ask *model.Order) {
	hangStatus.lock.Lock()
	defer hangStatus.lock.Unlock()
	if hangStatus.bid == nil {
		hangStatus.bid = make(map[string]*model.Order)
	}
	if hangStatus.ask == nil {
		hangStatus.ask = make(map[string]*model.Order)
	}
	hangStatus.bid[symbol] = bid
	hangStatus.ask[symbol] = ask
}

func (hangStatus *HangStatus) getHanging(symbol string) bool {
	hangStatus.lock.Lock()
	defer hangStatus.lock.Unlock()
	if hangStatus.hanging == nil {
		return false
	}
	return hangStatus.hanging[symbol]
}

func (hangStatus *HangStatus) setHanging(symbol string, value bool) {
	hangStatus.lock.Lock()
	defer hangStatus.lock.Unlock()
	if hangStatus.hanging == nil {
		hangStatus.hanging = make(map[string]bool)
	}
	hangStatus.hanging[symbol] = value
}

func (hangStatus *HangStatus) getLastHangTime(symbol string) (lasthangTime *time.Time) {
	hangStatus.lock.Lock()
	defer hangStatus.lock.Unlock()
	if hangStatus.lastHangTime == nil {
		return nil
	}
	return hangStatus.lastHangTime[symbol]
}

func (hangStatus *HangStatus) setLastHangTime(symbol string, hangTime *time.Time) {
	hangStatus.lock.Lock()
	defer hangStatus.lock.Unlock()
	if hangStatus.lastHangTime == nil {
		hangStatus.lastHangTime = make(map[string]*time.Time)
	}
	hangStatus.lastHangTime[symbol] = hangTime
}

// 暂停刷单，撤上次挂单，等待3秒，查可用余额，预留刷单资金，剩余的挂单，启动刷单
func rehang(market, symbol string, midPrice float64, accountType string, hangDis float64, reserveAmount float64) {
	model.AppConfig.HandleRefresh = `0`
	bid, ask := hangStatus.getHangOrders(symbol)
	if bid != nil && bid.OrderId != `` {
		api.MustCancel(market, symbol, bid.OrderId, true)
	}
	if ask != nil && ask.OrderId != `` {
		api.MustCancel(market, symbol, ask.OrderId, true)
	}
	time.Sleep(time.Second * 3)
	api.RefreshAccount(market)
	hangStatus.setHangOrders(symbol, nil, nil)
	left, right, _, _, err := getBalance(market, symbol, accountType)
	if err != nil || midPrice == 0 {
		return
	}
	minAmount := math.Min(left, right/midPrice)
	minAmount = math.Min(minAmount, reserveAmount)
	for i := 0; i < 3; i++ {
		bid = api.PlaceOrder(model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``, accountType,
			midPrice*(1-hangDis), (right/midPrice)-minAmount)
		if bid != nil && bid.Status == model.CarryStatusWorking && bid.OrderId != `` {
			bid.Function = model.FunctionHang
			model.AppDB.Save(bid)
			break
		}
	}
	for i := 0; i < 3; i++ {
		ask = api.PlaceOrder(model.OrderSideSell, model.OrderTypeLimit, market, symbol, ``, accountType,
			midPrice*(1+hangDis), left-minAmount)
		if ask != nil && ask.Status == model.CarryStatusWorking && ask.OrderId != `` {
			ask.Function = model.FunctionHang
			model.AppDB.Save(ask)
			break
		}
	}
	now := util.GetNow()
	hangStatus.setLastHangTime(symbol, &now)
	hangStatus.setHangOrders(symbol, bid, ask)
	model.AppConfig.HandleRefresh = `1`
	util.Notice(fmt.Sprintf(`[rehang]%s %s midprice %f left %f right %f`, market, symbol, midPrice, left, right))
}

var ProcessHang = func(market, symbol string) {
	if hangStatus.getHanging(symbol) || model.AppConfig.Handle != `1` {
		return
	}
	hangStatus.setHanging(symbol, true)
	defer hangStatus.setHanging(symbol, false)
	now := util.GetNow()
	if now.Hour() == 0 && now.Minute() == 0 && now.Second() < 10 {
		hangStatus.setLastHangTime(symbol, nil)
		return
	}
	setting := model.GetSetting(model.FunctionHang, market, symbol)
	bidAsk := model.AppMarkets.BidAsks[symbol][market]
	if len(bidAsk.Asks) == 0 || bidAsk.Bids.Len() == 0 {
		return
	}
	delay := util.GetNowUnixMillion() - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
	if delay > 200 {
		util.Notice(fmt.Sprintf(`[delay too long] %d`, delay))
		return
	}
	params := strings.Split(setting.FunctionParameter, `_`)
	if len(params) != 3 {
		util.Notice(fmt.Sprintf(`[param wrong format] %s %s %s`, market, symbol, setting.FunctionParameter))
	}
	hangDis, _ := strconv.ParseFloat(params[0], 64)
	rehangDis, _ := strconv.ParseFloat(params[1], 64)
	reserveAmount, _ := strconv.ParseFloat(params[2], 64)
	bid, ask := hangStatus.getHangOrders(symbol)
	d, _ := time.ParseDuration("-3610s")
	timeLine := now.Add(d)
	lastHang := hangStatus.getLastHangTime(symbol)
	if (bid != nil && bidAsk.Bids[0].Price*(1-rehangDis) < bid.Price) ||
		(ask != nil && bidAsk.Asks[0].Price*(1+rehangDis) > ask.Price) || lastHang == nil || lastHang.Before(timeLine) {
		price := (bidAsk.Bids[0].Price + bidAsk.Asks[0].Price) / 2
		rehang(market, symbol, price, setting.AccountType, hangDis, reserveAmount)
	}
}
