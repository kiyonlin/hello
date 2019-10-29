package model

import (
	"fmt"
	"github.com/gorilla/websocket"
	"hello/util"
	"strconv"
	"strings"
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
	Ts   int
	Bids Ticks
	Asks Ticks
}

type Rule struct {
	Margin float64
	Delay  float64
}

type Markets struct {
	lock      sync.Mutex
	bidAsks   map[string]map[string]*BidAsk           // symbol - market - bidAsk
	trade     map[int64]map[string]map[string]float64 // time in second - symbol - market - price
	BigDeals  map[string]map[string]*Deal             // symbol - market - Deal
	wsDepth   map[string][]chan struct{}              // market - []depth channel
	isWriting map[string]bool                         // market - writing
	conns     map[string]*websocket.Conn              // market - conn
}

func NewMarkets() *Markets {
	return &Markets{bidAsks: make(map[string]map[string]*BidAsk), wsDepth: make(map[string][]chan struct{}),
		trade: make(map[int64]map[string]map[string]float64)}
}

func (markets *Markets) SetTrade(deal *Deal) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.trade == nil {
		markets.trade = make(map[int64]map[string]map[string]float64)
	}
	second := deal.Ts / 1000
	//util.Notice(fmt.Sprintf(`= set deal %d %s`, second, deal.Market))
	if markets.trade[second] == nil {
		markets.trade[second] = make(map[string]map[string]float64)
	}
	if markets.trade[second][deal.Symbol] == nil {
		markets.trade[second][deal.Symbol] = make(map[string]float64)
	}
	markets.trade[second][deal.Symbol][deal.Market] = deal.Price
	needSave := true
	compareSecond := second - AppConfig.MonitorTime
	if markets.trade[compareSecond] == nil || markets.trade[compareSecond][deal.Symbol] == nil {
		needSave = false
	} else {
		if markets.trade[compareSecond][deal.Symbol][Fmex] <= 0 ||
			markets.trade[compareSecond][deal.Symbol][Bitmex] <= 0 {
			needSave = false
		}
		for market := range markets.trade[compareSecond][deal.Symbol] {
			fmt.Println(markets.trade[second][deal.Symbol][market])
			if markets.trade[second][deal.Symbol][market] <= 0 {
				needSave = false
			}
		}
	}
	if needSave {
		candle := &Candle{Symbol: deal.Symbol, Ts: second,
			PriceBitmex:    markets.trade[second][deal.Symbol][Bitmex],
			PriceFmex:      markets.trade[second][deal.Symbol][Fmex],
			IncreaseBitmex: markets.trade[second][deal.Symbol][Bitmex] - markets.trade[compareSecond][deal.Symbol][Bitmex],
			IncreaseFmex:   markets.trade[second][deal.Symbol][Fmex] - markets.trade[compareSecond][deal.Symbol][Fmex],
		}
		candle.Compare = candle.IncreaseBitmex - candle.IncreaseFmex
		AppDB.Save(&candle)
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

func (markets *Markets) SetBigDeal(symbol, market string, deal *Deal) bool {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	setting := GetSetting(FunctionMaker, market, symbol)
	if setting == nil {
		return false
	}
	params := strings.Split(setting.FunctionParameter, `_`)
	if len(params) != 2 {
		util.Notice(`maker param error: require d_d format param while get ` + setting.FunctionParameter)
		return false
	}
	if markets.BigDeals == nil {
		markets.BigDeals = make(map[string]map[string]*Deal)
	}
	if markets.BigDeals[symbol] == nil {
		markets.BigDeals[symbol] = make(map[string]*Deal)
	}
	bigOrderLine, err := strconv.ParseFloat(params[0], 64)
	oldDeal := markets.BigDeals[symbol][market]
	if err == nil && deal.Amount >= bigOrderLine && (oldDeal == nil || deal.Ts > oldDeal.Ts) {
		markets.BigDeals[symbol][market] = deal
		util.Notice(fmt.Sprintf(`[get big]%f-%f`, deal.Amount, bigOrderLine))
		return true
	}
	return false
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

func (markets *Markets) SetBidAsk(symbol, marketName string, bidAsk *BidAsk) bool {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if len(symbol) > 7 && symbol[0:7] == `bchabc_` {
		symbol = `bch_` + symbol[7:]
	}
	if markets.bidAsks == nil {
		markets.bidAsks = map[string]map[string]*BidAsk{}
	}
	if markets.bidAsks[symbol] == nil {
		markets.bidAsks[symbol] = map[string]*BidAsk{}
	}
	if bidAsk == nil || bidAsk.Bids == nil || bidAsk.Asks == nil || bidAsk.Bids.Len() == 0 || bidAsk.Asks.Len() == 0 {
		return false
	}
	if bidAsk.Bids[0].Price > bidAsk.Asks[0].Price {
		util.Info(fmt.Sprintf(`[fatal error]%s %s bid %f > ask %f amount %f %f`,
			symbol, marketName, bidAsk.Bids[0].Price, bidAsk.Asks[0].Price, bidAsk.Bids[0].Amount, bidAsk.Asks[0].Amount))
	}
	if markets.bidAsks[symbol][marketName] == nil || markets.bidAsks[symbol][marketName].Ts < bidAsk.Ts {
		//util.SocketInfo(fmt.Sprintf(`...%s %s socket delay %d`,
		//	symbol, marketName, util.GetNowUnixMillion()-int64(bidAsk.Ts)))
		markets.bidAsks[symbol][marketName] = bidAsk
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

func (markets *Markets) RequireDepthChanReset(market string) bool {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	needReset := true
	for symbol, value := range markets.bidAsks {
		if value[market] != nil && float64(util.GetNowUnixMillion()-int64(value[market].Ts)) < AppConfig.Delay {
			needReset = false
		}
		end := int64(util.GetNowUnixMillion() / 1000)
		for i := end - int64(AppConfig.Delay/1000); i <= end; i++ {
			if markets.trade[i] != nil && markets.trade[i][symbol] != nil && markets.trade[i][symbol][market] > 0 {
				needReset = false
				break
			}
		}
	}
	if needReset {
		util.SocketInfo(fmt.Sprintf(`socket need reset %v`, needReset))
	}
	return needReset
}
