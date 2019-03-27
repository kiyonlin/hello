package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strings"
	"time"
)

var marketMaking bool

var lastMaker *model.Order

func setMarketMaking(making bool) {
	marketMaking = making
}

func placeMaker(market, symbol string) {
	coins := strings.Split(symbol, `_`)
	leftAccount := model.AppAccounts.GetAccount(market, coins[0])
	if leftAccount == nil {
		util.Notice(`nil account ` + market + coins[0])
		//go getAccount()
		return
	}
	leftBalance := leftAccount.Free
	rightAccount := model.AppAccounts.GetAccount(market, coins[1])
	if rightAccount == nil {
		util.Notice(`nil account ` + market + coins[1])
		//go getAccount()
		return
	}
	rightBalance := rightAccount.Free
	priceDistance := 0.9 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	coinPrice, _ := api.GetPrice(coins[0] + `_usdt`)
	bigOrder := 0
	lastPrice := 0.0
	lastAmount := 0.0
	lastTs := 0
	lastSide := ``
	for _, deal := range model.AppMarkets.Deals[symbol][market] {
		if deal.Amount*coinPrice > 10000 && util.GetNowUnixMillion()-int64(1000*deal.Ts) < 10000 {
			bigOrder++
			if deal.Ts > lastTs {
				lastTs = deal.Ts
				lastPrice = deal.Price
				lastAmount = deal.Amount
				lastSide = deal.Side
			}
		}
	}
	if bigOrder < 3 || model.AppMarkets.BidAsks[symbol][market].Bids[0].Price-lastPrice > priceDistance ||
		lastPrice-model.AppMarkets.BidAsks[symbol][market].Asks[0].Price > priceDistance {
		return
	}
	amount := math.Min(leftBalance, rightBalance/lastPrice) * model.AppConfig.MakerAmountRate
	amount = math.Min(amount, lastAmount/2)
	side := ``
	if lastPrice-model.AppMarkets.BidAsks[symbol][market].Bids[0].Price < priceDistance {
		side = model.OrderSideBuy
	} else if model.AppMarkets.BidAsks[symbol][market].Asks[0].Price-lastPrice < priceDistance {
		side = model.OrderSideSell
	} else if lastSide == model.OrderSideSell {
		side = model.OrderSideBuy
	} else if lastSide == model.OrderSideBuy {
		side = model.OrderSideSell
	}
	order := api.PlaceOrder(side, model.OrderTypeLimit, market, symbol, ``, lastPrice, amount)
	if order.OrderId != `` {
		time.Sleep(time.Second)
		api.MustCancel(market, symbol, order.OrderId, true)
		time.Sleep(time.Second)
		order = api.QueryOrderById(market, symbol, order.OrderId)
		order.OrderType = model.FunctionMaker
		lastMaker = order
		model.AppDB.Save(order)
	}
}

func placeMakerReverse(market, symbol string) {
	asks := model.AppMarkets.BidAsks[symbol][market].Asks
	bids := model.AppMarkets.BidAsks[symbol][market].Bids
	amount := 0.0
	var order *model.Order
	priceDistance := 0.9 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	if lastMaker.OrderSide == model.OrderSideSell {
		for i := 0; i < len(asks); i++ {
			if asks[i].Price-lastMaker.DealPrice < priceDistance {
				amount += asks[i].Amount
			} else {
				break
			}
		}
		if amount > 0.2*lastMaker.Amount {
			order = api.PlaceOrder(model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``,
				lastMaker.DealPrice, lastMaker.Amount)
		}
	} else if lastMaker.OrderSide == model.OrderSideBuy {
		for i := 0; i < len(bids); i++ {
			if lastMaker.DealPrice-bids[i].Price < priceDistance {
				amount += bids[i].Amount
			} else {
				break
			}
		}
		if amount > 0.2*lastMaker.Amount {
			order = api.PlaceOrder(model.OrderSideSell, model.OrderTypeLimit, market, symbol, ``,
				lastMaker.DealPrice, lastMaker.Amount)
		}
	}
	if order != nil && order.Status == model.CarryStatusWorking {
		time.Sleep(time.Second)
		api.MustCancel(market, symbol, order.OrderId, true)
		order = api.QueryOrderById(market, symbol, order.OrderId)
		order.OrderType = model.FunctionMaker
		model.AppDB.Save(order)
		lastMaker = nil
		api.RefreshAccount(market)
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
	if lastMaker == nil {
		placeMaker(market, symbol)
	} else {
		placeMakerReverse(market, symbol)
	}
}
