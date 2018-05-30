package model

import (
	"errors"
	"sync"
	"math"
	"hello/util"
)

type BidAsk struct {
	Ts   int
	Bids PriceAmount
	Asks PriceAmount
}

type Market struct {
	// marketName - data
	Data map[string]*BidAsk
}

type Rule struct {
	Margin float64
	Delay  float64
}

type Markets struct {
	lock     sync.Mutex
	markets  map[string]*Market       // symbol - market
	MarketWS map[string]chan struct{} // marketName - channel
}

func NewMarkets() *Markets {
	markets := &Markets{}
	markets.markets = make(map[string]*Market)
	markets.MarketWS = make(map[string]chan struct{})
	return markets
}

func (bidAsk *BidAsk) SetBidAsk(data *BidAsk) {
	if data.Bids != nil && data.Bids.Len() > 0 {
		bidAsk.Bids = data.Bids
	}
	if data.Asks != nil && data.Asks.Len() > 0 {
		bidAsk.Asks = data.Asks
	}
	bidAsk.Ts = data.Ts
}

func (markets *Markets) SetBidAsk(symbol string, marketName string, bidAsk *BidAsk) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if markets.markets[symbol] == nil {
		market := &Market{}
		market.Data = make(map[string]*BidAsk)
		markets.markets[symbol] = market
	}
	oldData := markets.markets[symbol].Data[marketName]
	if oldData == nil {
		oldData = &BidAsk{}
		markets.markets[symbol].Data[marketName] = oldData
	}
	oldData.SetBidAsk(bidAsk)
}

func (markets *Markets) GetMarket(symbol string) *Market {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	return markets.markets[symbol]
}

func (markets *Markets) SetMarket(symbol string, market *Market) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	markets.markets[symbol] = market
}

func (markets *Markets) NewCarry(symbol string) (*Carry, error) {
	if markets.GetMarket(symbol) == nil {
		return nil, errors.New("no market data" + symbol)
	}
	carry := Carry{}
	carry.Symbol = symbol
	for k, v := range markets.GetMarket(symbol).Data {
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
	return nil, errors.New(`in valid carry`)
	//if BaseCarryCost < (carry.AskPrice-carry.BidPrice)/carry.AskPrice {
	//	return &carry, nil
	//}
	//return nil, errors.New(fmt.Sprintf(`利润小于%f`, BaseCarryCost))
}

func (markets *Markets) PutChan(marketName string, channel chan struct{}) {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	if channel != nil {
		util.SocketInfo(" set channel for " + marketName)
	}
	markets.MarketWS[marketName] = channel
}

func (markets *Markets) GetChan(marketName string) chan struct{} {
	markets.lock.Lock()
	defer markets.lock.Unlock()
	return markets.MarketWS[marketName]
}

func (markets *Markets) RequireChanReset(marketName string, subscribe string) bool {
	//util.SocketInfo(marketName + ` start to check require chan reset or not`)
	symbol := GetSymbol(marketName, subscribe)
	market := markets.markets[symbol]
	if market != nil {
		bidAsk := market.Data[marketName]
		if bidAsk != nil {
			//util.SocketInfo(fmt.Sprintf(`%s time %d %d diff:%d`, marketName, util.GetNowUnixMillion(),
			//	bidAsk.Ts, util.GetNowUnixMillion()-int64(bidAsk.Ts)))
			if math.Abs(float64(util.GetNowUnixMillion()-int64(bidAsk.Ts))) < 60000 { // 1分钟
				return false
			}
		}
	}
	return true
}
