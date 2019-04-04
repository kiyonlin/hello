package carry

import (
	"fmt"
	"github.com/pkg/errors"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strings"
	"sync"
	"time"
)

var marketMaking bool

type MakerStatus struct {
	lock        sync.Mutex
	lastBigTime map[string]map[string]int64 // market - symbol - unix time
}

func setMarketMaking(making bool) {
	marketMaking = making
}

func getBalance(market, symbol, accountType string) (left, right float64, err error) {
	if accountType == model.AccountTypeLever {
		market = fmt.Sprintf(`%s_%s_%s`, market, model.AccountTypeLever, symbol)
	}
	coins := strings.Split(symbol, `_`)
	leftAccount := model.AppAccounts.GetAccount(market, coins[0])
	if util.GetNowUnixMillion()-api.LastRefreshTime > 15000 {
		util.Notice(`15 seconds past, refresh and return ` + market + symbol)
		return 0, 0, errors.New(`data older than 15 seconds`)
	}
	if leftAccount == nil {
		util.Notice(`nil account ` + market + coins[0])
		time.Sleep(time.Second * 2)
		api.RefreshAccount(market)
		return 0, 0, errors.New(`no left balance`)
	}
	rightAccount := model.AppAccounts.GetAccount(market, coins[1])
	if rightAccount == nil {
		util.Notice(`nil account ` + market + coins[1])
		time.Sleep(time.Second * 2)
		api.RefreshAccount(market)
		return 0, 0, errors.New(`no right balance`)
	}
	return leftAccount.Free, rightAccount.Free, nil
}

func placeMaker(market, symbol string) {
	coins := strings.Split(symbol, `_`)
	priceDistance := 0.9 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	coinPrice, _ := api.GetPrice(coins[0] + `_btc`)
	bigOrder := 0
	lastPrice := 0.0
	lastAmount := 0.0
	lastTs := 0
	for _, deal := range model.AppMarkets.Deals[symbol][market] {
		if deal.Amount*coinPrice > 200 && util.GetNowUnixMillion()-int64(deal.Ts) < 10000 {
			bigOrder++
			if deal.Ts > lastTs {
				lastTs = deal.Ts
				lastPrice = deal.Price
				lastAmount = deal.Amount
				//util.Notice(fmt.Sprintf(`%s %s time: %d, amount: %f`, market, symbol, lastTs, lastAmount))
			}
		}
	}
	model.AppMarkets.Deals[symbol][market] = nil
	if bigOrder < 3 {
		return
	}
	price := (model.AppMarkets.BidAsks[symbol][market].Bids[0].Price +
		model.AppMarkets.BidAsks[symbol][market].Asks[0].Price) / 2
	if price-model.AppMarkets.BidAsks[symbol][market].Bids[0].Price < priceDistance &&
		model.AppMarkets.BidAsks[symbol][market].Asks[0].Price-price < priceDistance {
		price = lastPrice
	}
	left, right, err := getBalance(market, symbol, ``)
	if err != nil {
		return
	}
	side := model.OrderSideBuy
	amount := math.Floor(math.Min(right/model.AppMarkets.BidAsks[symbol][market].Asks[0].Price, lastAmount/2))
	if left > right/model.AppMarkets.BidAsks[symbol][market].Asks[0].Price {
		side = model.OrderSideSell
		amount = math.Floor(math.Min(left, lastAmount/2))
		if 0.2*amount < model.AppMarkets.BidAsks[symbol][market].Bids[0].Amount {
			return
		}
	} else {
		if 0.2*amount < model.AppMarkets.BidAsks[symbol][market].Asks[0].Amount {
			return
		}
	}
	order := api.PlaceOrder(side, model.OrderTypeLimit, market, symbol, ``, ``, price, amount)
	if order.OrderId != `` {
		time.Sleep(time.Second)
		api.MustCancel(market, symbol, order.OrderId, true)
		time.Sleep(time.Second)
		api.QueryOrder(order)
		order.OrderType = model.FunctionMaker
		model.AppDB.Save(order)
	}
}

var ProcessMake = func(market, symbol string) {
	if model.AppConfig.Handle != `1` || model.AppConfig.HandleMaker != `1` || marketMaking {
		return
	}
	setMarketMaking(true)
	defer setMarketMaking(false)
	bidAsk := model.AppMarkets.BidAsks[symbol][market]
	if len(bidAsk.Asks) == 0 || bidAsk.Bids.Len() == 0 || model.AppMarkets.Deals[symbol] == nil ||
		model.AppMarkets.Deals[symbol][market] == nil {
		return
	}
	coins := strings.Split(symbol, `_`)
	if len(coins) != 2 {
		util.Notice(`symbol format not supported ` + symbol)
		return
	}
	if model.AppAccounts.Data[market][coins[0]] == nil || model.AppAccounts.Data[market][coins[1]] == nil {
		api.RefreshAccount(market)
		return
	}
	delay := util.GetNowUnixMillion() - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
	if delay > 50 {
		util.Notice(fmt.Sprintf(`[delay too long] %d`, delay))
		return
	}
	placeMaker(market, symbol)
	api.RefreshAccount(market)
}
