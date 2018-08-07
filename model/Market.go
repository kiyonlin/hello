package model

import (
	"errors"
	"fmt"
	"hello/util"
	"math"
	"strings"
	"sync"
)

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
	lock     sync.Mutex
	BidAsks  map[string]map[string]*BidAsk // symbol - market - bidAsk
	marketWS map[string][]chan struct{}    // marketName - channel
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

func (markets *Markets) NewTurtleCarry(symbol, market string) (*Carry, error) {
	if markets.BidAsks[symbol] == nil {
		return nil, errors.New("no market data " + symbol)
	}
	amount, priceWidth, _ := GetTurtleSetting(market, symbol)
	if amount == 0 || priceWidth == 0 {
		return nil, errors.New(fmt.Sprintf(`no amount: %f or no price width: %f`, amount, priceWidth))
	}
	bidAsks := markets.BidAsks[symbol][market]
	var bidPrice, askPrice, bidAmount, askAmount float64
	turtleStatus := GetTurtleStatus(market, symbol)
	util.Notice(fmt.Sprintf(`get status when creating turtle extra bid %f - extra ask %f price %f`,
		turtleStatus.ExtraBid, turtleStatus.ExtraAsk, turtleStatus.LastDealPrice))
	if turtleStatus != nil && turtleStatus.LastDealPrice != 0{
		bidPrice = turtleStatus.LastDealPrice - priceWidth
		askPrice = turtleStatus.LastDealPrice + priceWidth
		bidAmount = amount - turtleStatus.ExtraBid
		askAmount = amount - turtleStatus.ExtraAsk
	} else {
		bidPrice = bidAsks.Asks[0].Price - priceWidth
		askPrice = bidAsks.Asks[0].Price + priceWidth
		bidAmount = amount
		askAmount = amount
	}
	carry := Carry{AskWeb: market, BidWeb: market, Symbol: symbol, BidAmount: bidAmount, AskAmount: askAmount,
		Amount: amount, BidPrice: bidPrice, AskPrice: askPrice, DealBidStatus: CarryStatusWorking,
		DealAskStatus: CarryStatusWorking, BidTime: int64(bidAsks.Ts), AskTime: int64(bidAsks.Ts), SideType: CarryTypeTurtle}
	return &carry, nil
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
		if len(v.Bids) > 0 && carry.AskPrice < v.Bids[0].Price {
			carry.AskPrice = v.Bids[0].Price
			carry.AskAmount = v.Bids[0].Amount
			carry.AskWeb = k
			carry.AskTime = int64(v.Ts)
		}
		if len(v.Asks) > 0 && (carry.BidPrice == 0 || carry.BidPrice > v.Asks[0].Price) {
			carry.BidPrice = v.Asks[0].Price
			carry.BidAmount = v.Asks[0].Amount
			carry.BidWeb = k
			carry.BidTime = int64(v.Ts)
		}
	}
	currencies := strings.Split(carry.Symbol, `_`)
	if len(currencies) != 2 {
		return nil, errors.New(`invalid carry symbol`)
	}
	carry.Margin = GetMargin(symbol)
	if carry.BidAmount < carry.AskAmount {
		carry.Amount = carry.BidAmount
	} else {
		carry.Amount = carry.AskAmount
	}
	if carry.Amount == 0 {
		return nil, errors.New(`0 amount carry`)
	}
	return &carry, nil
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
