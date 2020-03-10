package model

import (
	"fmt"
	"github.com/gorilla/websocket"
	"hello/util"
	"sync"
)

type KLinePoint struct {
	TS            int64
	EndPrice      float64
	HighPrice     float64
	LowPrice      float64
	RSI           float64
	RSIExpectBuy  float64
	RSIExpectSell float64
}

type Deal struct {
	Ts     int64
	Market string
	Symbol string
	Amount float64
	Id     string
	Side   string
	Price  float64
}

type BidAsk struct {
	Ts   int // time in unix epoch million seconds
	Bids Ticks
	Asks Ticks
}

type Rule struct {
	Margin float64
	Delay  float64
}

type Markets struct {
	lock            sync.Mutex
	bmPendingOrders map[string]*Order                     // bm中的orderId-order
	TrendEnd        map[string]map[string]*Deal           // symbol - market - deal
	TrendStart      map[string]map[string]*Deal           // symbol - market - deal
	bidAsks         map[string]map[string]*BidAsk         // symbol - market - bidAsk
	lastUp          map[string]map[string]int             // symbol - market - time in million second
	lastDown        map[string]map[string]int             // symbol - market - time in million second
	trade           map[int64]map[string]map[string]*Deal // time in second - symbol - market - deal
	BigDeals        map[string]map[string]*Deal           // symbol - market - Deal
	wsDepth         map[string][]chan struct{}            // market - []depth channel
	isWriting       map[string]bool                       // market - writing
	conns           map[string]*websocket.Conn            // market - conn
}

func NewMarkets() *Markets {
	return &Markets{bidAsks: make(map[string]map[string]*BidAsk), wsDepth: make(map[string][]chan struct{}),
		trade: make(map[int64]map[string]map[string]*Deal)}
}

func (markets *Markets) GetBmPendingOrders() (orders map[string]*Order) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.bmPendingOrders == nil {
		markets.bmPendingOrders = make(map[string]*Order)
	}
	return markets.bmPendingOrders
}

func (markets *Markets) RemoveBmPendingOrder() (order *Order) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.bmPendingOrders == nil {
		return nil
	}
	for orderId, item := range markets.bmPendingOrders {
		delete(markets.bmPendingOrders, orderId)
		return item
	}
	return nil
}

func (markets *Markets) AddBMPendingOrder(order *Order) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.bmPendingOrders == nil {
		markets.bmPendingOrders = make(map[string]*Order)
	}
	markets.bmPendingOrders[order.OrderId] = order
}

func (markets *Markets) GetTrends(symbol string) (start, end map[string]*Deal) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.TrendStart != nil {
		start = markets.TrendStart[symbol]
	}
	if markets.TrendEnd != nil {
		end = markets.TrendEnd[symbol]
	}
	return start, end
}

func (markets *Markets) SetTrade(deal *Deal) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.trade == nil {
		markets.trade = make(map[int64]map[string]map[string]*Deal)
	}
	second := deal.Ts / 1000
	symbol := deal.Symbol
	if len(markets.trade[second]) > 1000 {
		markets.trade = nil
		markets.TrendStart = nil
		util.Notice(fmt.Sprintf(`clear trade map and trend point`))
	}
	if markets.trade[second] == nil {
		markets.trade[second] = make(map[string]map[string]*Deal)
	}
	if markets.trade[second][symbol] == nil {
		markets.trade[second][symbol] = make(map[string]*Deal)
	}
	if markets.trade[second][symbol][deal.Market] != nil {
		return
	}
	markets.trade[second][symbol][deal.Market] = deal
	if markets.trade[second] != nil && markets.trade[second][symbol] != nil &&
		markets.trade[second][symbol][Bitmex] != nil && markets.trade[second][symbol][Fmex] != nil {
		chance := 15.0
		compareSecond := second - int64(chance)
		compare := markets.trade[compareSecond]
		if compare != nil && compare[symbol] != nil && compare[symbol][Bitmex] != nil &&
			compare[symbol][Fmex] != nil {
			markets.TrendStart = compare
			markets.TrendEnd = markets.trade[second]
		}
		delete(markets.trade, compareSecond)
	}
}

func (markets *Markets) GetIsWriting(market string) bool {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.isWriting == nil {
		markets.isWriting = make(map[string]bool)
	}
	return markets.isWriting[market]
}

func (markets *Markets) SetIsWriting(market string, isWriting bool) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.isWriting == nil {
		markets.isWriting = make(map[string]bool)
	}
	markets.isWriting[market] = isWriting
}

func (markets *Markets) SetConn(market string, conn *websocket.Conn) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.conns == nil {
		markets.conns = make(map[string]*websocket.Conn)
	}
	markets.conns[market] = conn
}

func (markets *Markets) GetConn(market string) *websocket.Conn {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.conns == nil {
		markets.conns = make(map[string]*websocket.Conn)
	}
	return markets.conns[market]
}

func (markets *Markets) GetBigDeal(symbol, market string) (deal *Deal) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.BigDeals == nil {
		markets.BigDeals = make(map[string]map[string]*Deal)
	}
	if markets.BigDeals[symbol] == nil {
		markets.BigDeals[symbol] = make(map[string]*Deal)
	}
	return markets.BigDeals[symbol][market]
}

func (markets *Markets) GetPrice(symbol string) (result bool, price float64) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	for _, bidAsks := range AppMarkets.bidAsks[symbol] {
		if bidAsks != nil && bidAsks.Bids != nil {
			return true, bidAsks.Bids[0].Price
		}
	}
	return false, 0
}

func (markets *Markets) ToStringBidAsk(bidAsk *BidAsk) (result string) {
	if bidAsk == nil || bidAsk.Bids == nil || bidAsk.Asks == nil {
		return ``
	}
	for i := bidAsk.Bids.Len() - 1; i >= 0; i-- {
		result += fmt.Sprintf(`%f,`, bidAsk.Bids[i].Price)
	}
	result += `--|--`
	for i := 0; i < bidAsk.Asks.Len(); i++ {
		result += fmt.Sprintf(`%f,`, bidAsk.Asks[i].Price)
	}
	return
}

func (markets *Markets) CopyBidAsk(symbol, market string) (result bool, bidAsk *BidAsk) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.bidAsks == nil || markets.bidAsks[symbol] == nil || markets.bidAsks[symbol][market] == nil ||
		markets.bidAsks[symbol][market].Asks == nil || markets.bidAsks[symbol][market].Bids == nil ||
		markets.bidAsks[symbol][market].Asks.Len() == 0 || markets.bidAsks[symbol][market].Bids.Len() == 0 {
		return false, nil
	}
	bidAsk = &BidAsk{}
	bidAsk.Bids = make([]Tick, markets.bidAsks[symbol][market].Bids.Len())
	bidAsk.Asks = make([]Tick, markets.bidAsks[symbol][market].Asks.Len())
	for key, value := range markets.bidAsks[symbol][market].Bids {
		bidAsk.Bids[key] = value
	}
	for key, value := range markets.bidAsks[symbol][market].Asks {
		bidAsk.Asks[key] = value
	}
	return true, bidAsk
}

func (markets *Markets) GetBidAsk(symbol, market string) (result bool, bidAsk *BidAsk) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.bidAsks == nil || markets.bidAsks[symbol] == nil || markets.bidAsks[symbol][market] == nil ||
		markets.bidAsks[symbol][market].Asks == nil || markets.bidAsks[symbol][market].Bids == nil ||
		markets.bidAsks[symbol][market].Asks.Len() == 0 || markets.bidAsks[symbol][market].Bids.Len() == 0 {
		return false, nil
	}
	return true, markets.bidAsks[symbol][market]
}

func (markets *Markets) GetLastUpDown(symbol, marketName string) (up, down int) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.lastUp == nil || markets.lastDown == nil || markets.lastUp[symbol] == nil ||
		markets.lastDown[symbol] == nil {
		return 0, 0
	}
	return markets.lastUp[symbol][marketName], markets.lastDown[symbol][marketName]
}

func (markets *Markets) SetBidAsk(symbol, marketName string, bidAsk *BidAsk) bool {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if len(symbol) > 7 && symbol[0:7] == `bchabc_` {
		symbol = `bch_` + symbol[7:]
	}
	if markets.bidAsks == nil {
		markets.bidAsks = make(map[string]map[string]*BidAsk)
	}
	if markets.bidAsks[symbol] == nil {
		markets.bidAsks[symbol] = make(map[string]*BidAsk)
	}
	if bidAsk == nil || bidAsk.Bids == nil || bidAsk.Asks == nil || bidAsk.Bids.Len() == 0 || bidAsk.Asks.Len() == 0 {
		util.SocketInfo(`do not set nil or empty bid ask`)
		return false
	}
	last := markets.bidAsks[symbol][marketName]
	if last == nil || markets.bidAsks[symbol][marketName].Ts <= bidAsk.Ts {
		markets.bidAsks[symbol][marketName] = bidAsk
		if last != nil && last.Bids[0].Price > bidAsk.Bids[0].Price {
			if markets.lastDown == nil {
				markets.lastDown = make(map[string]map[string]int)
			}
			if markets.lastDown[symbol] == nil {
				markets.lastDown[symbol] = make(map[string]int)
			}
			markets.lastDown[symbol][marketName] = bidAsk.Ts
		}
		if last != nil && last.Asks[0].Price < bidAsk.Asks[0].Price {
			if markets.lastUp == nil {
				markets.lastUp = make(map[string]map[string]int)
			}
			if markets.lastUp[symbol] == nil {
				markets.lastUp[symbol] = make(map[string]int)
			}
			markets.lastUp[symbol][marketName] = bidAsk.Ts
		}
		current := util.GetNow()
		AppMetric.addTick(marketName, symbol, current, int(current.UnixNano()/1000000)-bidAsk.Ts)
		return true
	}
	return false
}

func (markets *Markets) GetDepthChan(marketName string, index int) chan struct{} {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.wsDepth[marketName] == nil {
		markets.wsDepth[marketName] = make([]chan struct{}, AppConfig.Channels)
	}
	return markets.wsDepth[marketName][index]
}

func (markets *Markets) PutDepthChan(marketName string, index int, channel chan struct{}) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.wsDepth[marketName] == nil {
		markets.wsDepth[marketName] = make([]chan struct{}, AppConfig.Channels)
	}
	markets.wsDepth[marketName][index] = channel
}

func (markets *Markets) GetSymbols() (symbols map[string]bool) {
	symbols = make(map[string]bool)
	for symbol := range markets.bidAsks {
		symbols[symbol] = true
	}
	return
}
