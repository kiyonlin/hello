package model

import (
	"errors"
	"math"
	"hello/util"
	"sync"
)

type BidAsk struct {
	Ts   int
	Bids PriceAmount
	Asks PriceAmount
}

type Rule struct {
	Margin float64
	Delay  float64
}

type Markets struct {
	lock    sync.Mutex
	BidAsks map[string]map[string]*BidAsk // symbol - market - bidAsk
	marketWS map[string][]chan struct{}   // marketName - channel
}

func NewMarkets() *Markets {
	return &Markets{BidAsks: make(map[string]map[string]*BidAsk), marketWS: make(map[string][]chan struct{})}
}

func (markets *Markets) SetBidAsk(symbol string, marketName string, bidAsk *BidAsk) bool {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.BidAsks[symbol] == nil {
		markets.BidAsks[symbol] = map[string]*BidAsk{}
	}
	if markets.BidAsks[symbol][marketName] == nil || markets.BidAsks[symbol][marketName].Ts < bidAsk.Ts {
		markets.BidAsks[symbol][marketName] = bidAsk
		return true
	}
	return false
}

func (markets *Markets) NewCarry(symbol string) (*Carry, error) {
	if markets.BidAsks[symbol] == nil {
		return nil, errors.New("no market data " + symbol)
	}
	carry := Carry{}
	carry.Symbol = symbol
	for k, v := range markets.BidAsks[symbol] {
		if v == nil {
			continue
		}
		if len(v.Bids) > 0 && carry.AskPrice < v.Bids[0][0] {
			carry.AskPrice = v.Bids[0][0]
			carry.AskAmount = v.Bids[0][1]
			carry.AskWeb = k
			carry.AskTime = int64(v.Ts)
		}
		if len(v.Asks) > 0 && (carry.BidPrice == 0 || carry.BidPrice > v.Asks[0][0]) {
			carry.BidPrice = v.Asks[0][0]
			carry.BidAmount = v.Asks[0][1]
			carry.BidWeb = k
			carry.BidTime = int64(v.Ts)
		}
	}
	if carry.BidAmount < carry.AskAmount {
		carry.Amount = carry.BidAmount
	} else {
		carry.Amount = carry.AskAmount
	}
	carry.Margin, _ = ApplicationConfig.GetMargin(symbol)
	minAmount := ApplicationConfig.MinNum[symbol]
	if carry.Symbol != `` && carry.BidAmount > minAmount && carry.AskAmount > minAmount {
		return &carry, nil
	}
	return nil, errors.New(`invalid carry`)
}

func (markets *Markets) GetChan(marketName string, index int) chan struct{} {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.marketWS[marketName] == nil {
		markets.marketWS[marketName] = make([]chan struct{}, ApplicationConfig.Channels)
	}
	return markets.marketWS[marketName][index]
}

func (markets *Markets) PutChan(marketName string, index int, channel chan struct{}) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	//if channel != nil {
	//	util.SocketInfo(" set channel for " + marketName)
	//}
	if markets.marketWS[marketName] == nil {
		markets.marketWS[marketName] = make([]chan struct{}, ApplicationConfig.Channels)
	}
	markets.marketWS[marketName][index] = channel
}

func (markets *Markets) RequireChanReset(marketName string, subscribe string) bool {
	//util.SocketInfo(marketName + ` start to check require chan reset or not`)
	symbol := GetSymbol(marketName, subscribe)
	bidAsks := markets.BidAsks[symbol]
	if bidAsks != nil {
		bidAsk := bidAsks[marketName]
		if bidAsk != nil {
			delay := ApplicationConfig.ChannelSlot
			if math.Abs(float64(util.GetNowUnixMillion()-int64(bidAsk.Ts))) < delay {
				return false
			}
		}
	}
	return true
}
