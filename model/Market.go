package model

import (
	"errors"
	"fmt"
	"hello/util"
	"math"
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
	lock    sync.Mutex
	BidAsks map[string]map[string]*BidAsk // symbol - market - bidAsk
	Deals   map[string]map[string][]Deal  // symbol - market - []Deal
	wsDepth map[string][]chan struct{}    // market - []depth channel
	wsDeal  map[string]chan struct{}      // market - deal channel
}

func NewMarkets() *Markets {
	return &Markets{BidAsks: make(map[string]map[string]*BidAsk), wsDepth: make(map[string][]chan struct{}),
		wsDeal: make(map[string]chan struct{})}
}

func (markets *Markets) SetBidAsk(symbol string, marketName string, bidAsk *BidAsk) bool {
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

func (markets *Markets) NewBalanceTurtle(market, symbol string, leftAccount, rightAccount *Account,
	currentPrice, lastPrice float64) (*Carry, error) {
	setting := GetSetting(FunctionBalanceTurtle, market, symbol)
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
	return &Carry{BidSymbol: symbol, AskSymbol: symbol, BidWeb: market, AskWeb: market, BidAmount: bidAmount,
		AskAmount: askAmount, BidPrice: bidPrice, AskPrice: askPrice, SideType: CarryTypeBalance, BidTime: now,
		AskTime: now}, nil
}

func (markets *Markets) NewCarry(symbol string) (*Carry, error) {
	if markets.BidAsks[symbol] == nil {
		return nil, errors.New("no market data " + symbol)
	}
	carry := Carry{}
	carry.BidSymbol = symbol
	carry.AskSymbol = symbol
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
	currencies := strings.Split(carry.BidSymbol, `_`)
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

func (markets *Markets) GetDepthChan(marketName string, index int) chan struct{} {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.wsDepth[marketName] == nil {
		markets.wsDepth[marketName] = make([]chan struct{}, AppConfig.Channels)
	}
	return markets.wsDepth[marketName][index]
}

func (markets *Markets) GetDealChan(market string) chan struct{} {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	return markets.wsDeal[market]
}

func (markets *Markets) PutDealChan(market string, channel chan struct{}) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	markets.wsDeal[market] = channel
}

func (markets *Markets) PutDepthChan(marketName string, index int, channel chan struct{}) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.wsDepth[marketName] == nil {
		markets.wsDepth[marketName] = make([]chan struct{}, AppConfig.Channels)
	}
	markets.wsDepth[marketName][index] = channel
}

func (markets *Markets) RequireDepthChanReset(marketName string, subscribe string) bool {
	//util.SocketInfo(marketName + ` start to check require chan reset or not`)
	symbol := GetSymbol(marketName, subscribe)
	bidAsks := markets.BidAsks[symbol]
	if bidAsks != nil {
		bidAsk := bidAsks[marketName]
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
	deals := markets.Deals[symbol]
	if deals != nil {
		deal := deals[market]
		if deal != nil && len(deal) > 0 {
			if float64(util.GetNowUnixMillion()-int64(deal[0].Ts)) < AppConfig.Delay {
				return false
			}
		}
	}
	return true
}
