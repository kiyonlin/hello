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

func (markets *Markets) NewBalanceTurtle(market, symbol string, leftAccount, rightAccount *Account,
	currentPrice, lastPrice float64) (*Carry, error) {
	setting := GetSetting(market, symbol)
	if setting == nil {
		return nil, errors.New(fmt.Sprintf(market + ` has no setting for ` + symbol))
	}
	leftAmount := leftAccount.Free + leftAccount.Frozen
	rightAmount := rightAccount.Free + rightAccount.Frozen
	lastBalance := leftAmount*lastPrice + rightAmount
	leftRate := leftAmount * lastPrice / lastBalance
	rightRate := rightAmount / lastBalance
	if leftRate < 0.5 {
		leftRate += (0.5 - leftRate) / 10
	} else {
		leftRate -= (leftRate - 0.5) / 10
	}
	rightRate = 1 - leftRate
	askPrice := lastPrice * (1 + 2*setting.TurtleBalanceRate)
	askBalance := askPrice*leftAmount + rightAmount
	askAmount := rightRate * (askBalance - lastBalance) / askPrice
	bidPrice := lastPrice * (1 - 2*setting.TurtleBalanceRate)
	bidBalance := bidPrice*leftAmount + rightAmount
	bidAmount := leftRate * (lastBalance - bidBalance) / bidPrice
	util.Notice(fmt.Sprintf(`比例coin - money %f - %f`, leftRate, rightRate))
	now := util.GetNowUnixMillion()
	return &Carry{Symbol: symbol, BidWeb: market, AskWeb: market, BidAmount: bidAmount, AskAmount: askAmount,
		BidPrice: bidPrice, AskPrice: askPrice, SideType: CarryTypeBalance, BidTime: now, AskTime: now}, nil
}

//func (markets *Markets) NewTurtleCarry(symbol, market string) (*Carry, error) {
//	if markets.BidAsks[symbol] == nil {
//		return nil, errors.New("no market data " + symbol)
//	}
//	setting := GetSetting(market, symbol)
//	if setting == nil {
//		return nil, errors.New(fmt.Sprintf(`no setting`))
//	}
//	bidAsks := markets.BidAsks[symbol][market]
//	var bidPrice, askPrice, bidAmount, askAmount float64
//	turtleStatus := GetTurtleStatus(market, symbol)
//	if turtleStatus != nil && turtleStatus.LastDealPrice != 0{
//		util.Notice(fmt.Sprintf(`get status when creating turtle extra bid %f - extra ask %f price %f`,
//			turtleStatus.ExtraBid, turtleStatus.ExtraAsk, turtleStatus.LastDealPrice))
//		bidPrice = turtleStatus.LastDealPrice - setting.TurtlePriceWidth
//		askPrice = turtleStatus.LastDealPrice + setting.TurtlePriceWidth
//		bidAmount = setting.TurtleLeftAmount - turtleStatus.ExtraBid
//		askAmount = setting.TurtleLeftAmount - turtleStatus.ExtraAsk
//	} else {
//		bidPrice = bidAsks.Asks[0].Price - setting.TurtlePriceWidth
//		askPrice = bidAsks.Asks[0].Price + setting.TurtlePriceWidth
//		bidAmount = setting.TurtleLeftAmount
//		askAmount = setting.TurtleLeftAmount
//	}
//	strBidAmount := strconv.FormatFloat(bidAmount, 'f', 2, 64)
//	strAskAmount := strconv.FormatFloat(askAmount, 'f', 2, 64)
//	bidAmount, _ = strconv.ParseFloat(strBidAmount, 64)
//	askAmount, _ = strconv.ParseFloat(strAskAmount, 64)
//	carry := Carry{AskWeb: market, BidWeb: market, Symbol: symbol, BidAmount: bidAmount, AskAmount: askAmount,
//		Amount: setting.TurtleLeftAmount, BidPrice: bidPrice, AskPrice: askPrice, DealBidStatus: CarryStatusWorking,
//		DealAskStatus: CarryStatusWorking, BidTime: int64(bidAsks.Ts), AskTime: int64(bidAsks.Ts), SideType: CarryTypeTurtle}
//	return &carry, nil
//}

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
		markets.marketWS[marketName] = make([]chan struct{}, AppConfig.Channels)
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
		markets.marketWS[marketName] = make([]chan struct{}, AppConfig.Channels)
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
			delay := AppConfig.ChannelSlot
			if math.Abs(float64(util.GetNowUnixMillion()-int64(bidAsk.Ts))) < delay {
				return false
			}
		}
	}
	return true
}
