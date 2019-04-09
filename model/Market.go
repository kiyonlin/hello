package model

import (
	"fmt"
	"github.com/gorilla/websocket"
	"hello/util"
	"math"
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
	Ts     int
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
	BidAsks   map[string]map[string]*BidAsk // symbol - market - bidAsk
	BigDeals  map[string]map[string]*Deal   // symbol - market - Deal
	wsDepth   map[string][]chan struct{}    // market - []depth channel
	isWriting map[string]bool               // market - writing
	conns     map[string]*websocket.Conn    // market - conn
}

func NewMarkets() *Markets {
	return &Markets{BidAsks: make(map[string]map[string]*BidAsk), wsDepth: make(map[string][]chan struct{})}
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
		return true
	}
	return false
	//util.Notice(fmt.Sprintf(`[get big %v]%f:%f-%f`, bigOrderLine < deal.Amount, deal.Amount, amount, bigOrderLine))
}

func (markets *Markets) SetBidAsk(symbol, marketName string, bidAsk *BidAsk) bool {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.BidAsks[symbol] == nil {
		markets.BidAsks[symbol] = map[string]*BidAsk{}
	}
	if bidAsk == nil || bidAsk.Bids == nil || bidAsk.Asks == nil || bidAsk.Bids.Len() == 0 || bidAsk.Asks.Len() == 0 {
		return false
	}
	if bidAsk.Bids[0].Price > bidAsk.Asks[0].Price {
		util.Info(fmt.Sprintf(`[fatal error]%s %s bid %f > ask %f amount %f %f`,
			symbol, marketName, bidAsk.Bids[0].Price, bidAsk.Asks[0].Price, bidAsk.Bids[0].Amount, bidAsk.Asks[0].Amount))
	}
	if markets.BidAsks[symbol][marketName] == nil || markets.BidAsks[symbol][marketName].Ts < bidAsk.Ts {
		markets.BidAsks[symbol][marketName] = bidAsk
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

func (markets *Markets) RequireDepthChanReset(market, symbol string) bool {
	bidAsks := markets.BidAsks[symbol]
	if bidAsks != nil {
		bidAsk := bidAsks[market]
		if bidAsk != nil {
			if math.Abs(float64(util.GetNowUnixMillion()-int64(bidAsk.Ts))) < AppConfig.Delay {
				return false
			}
		}
	}
	return true
}

func (markets *Markets) RequireDealChanReset(market string, subscribe string) bool {
	symbol := GetSymbol(market, subscribe)
	deals := markets.BigDeals[symbol]
	if deals != nil {
		deal := deals[market]
		if deal != nil {
			if float64(util.GetNowUnixMillion()-int64(deal.Ts)) < AppConfig.Delay {
				return false
			}
		}
	}
	return true
}
