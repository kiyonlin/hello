package model

import (
	"errors"
	"math"
	"hello/util"
)

type BidAsk struct {
	Ts   int
	Bids PriceAmount
	Asks PriceAmount
}

//type Market struct {
//	// marketName - data
//	Data map[string]*BidAsk
//}

type Rule struct {
	Margin float64
	Delay  float64
}

type Markets struct {
	BidAsks  map[string]map[string]*BidAsk // symbol - market - bidask
	MarketWS map[string]chan struct{}      // marketName - channel
}

func NewMarkets() *Markets {
	return &Markets{BidAsks: make(map[string]map[string]*BidAsk), MarketWS: make(map[string]chan struct{})}
}

//func (bidAsk *BidAsk) SetBidAsk(data *BidAsk) {
//	if data.Bids != nil && data.Bids.Len() > 0 {
//		bidAsk.Bids = data.Bids
//	}
//	if data.Asks != nil && data.Asks.Len() > 0 {
//		bidAsk.Asks = data.Asks
//	}
//	bidAsk.Ts = data.Ts
//}

func (markets *Markets) SetBidAsk(symbol string, marketName string, bidAsk *BidAsk) {
	if markets.BidAsks[symbol] == nil {
		markets.BidAsks[symbol] = map[string]*BidAsk{}
	}
	markets.BidAsks[symbol][marketName] = bidAsk
	//oldData := markets.markets[symbol].Data[marketName]
	//if oldData == nil {
	//	oldData = &BidAsk{}
	//	markets.markets[symbol].Data[marketName] = oldData
	//}
	//oldData.SetBidAsk(bidAsk)
}

func (markets *Markets) NewCarry(symbol string) (*Carry, error) {
	if markets.BidAsks[symbol] == nil {
		return nil, errors.New("no market data" + symbol)
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
	if carry.Symbol != `` && carry.BidAmount > 0 && carry.AskAmount > 0 {
		return &carry, nil
	}
	return nil, errors.New(`invalid carry`)
	//if BaseCarryCost < (carry.AskPrice-carry.BidPrice)/carry.AskPrice {
	//	return &carry, nil
	//}
	//return nil, errors.New(fmt.Sprintf(`利润小于%f`, BaseCarryCost))
}

func (markets *Markets) PutChan(marketName string, channel chan struct{}) {
	if channel != nil {
		util.SocketInfo(" set channel for " + marketName)
	}
	markets.MarketWS[marketName] = channel
}

func (markets *Markets) RequireChanReset(marketName string, subscribe string) bool {
	//util.SocketInfo(marketName + ` start to check require chan reset or not`)
	symbol := GetSymbol(marketName, subscribe)
	bidAsks := markets.BidAsks[symbol]
	if bidAsks != nil {
		bidAsk := bidAsks[marketName]
		if bidAsk != nil {
			//util.SocketInfo(fmt.Sprintf(`%s time %d %d diff:%d`, marketName, util.GetNowUnixMillion(),
			//	bidAsk.Ts, util.GetNowUnixMillion()-int64(bidAsk.Ts)))
			delay, _ := ApplicationConfig.GetDelay(symbol)
			if math.Abs(float64(util.GetNowUnixMillion()-int64(bidAsk.Ts))) < delay {
				return false
			}
		}
	}
	return true
}
