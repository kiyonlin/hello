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

//var lastMaker *model.Order
var orderCount int64

func setMarketMaking(making bool) {
	marketMaking = making
}

//func placeReverse()  {
//	model.AppMarkets.BidAsks
//}

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
	current := util.GetNowUnixMillion()
	delay := current - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
	if delay > 50 {
		util.Notice(fmt.Sprintf(`[delay too long] %d`, delay))
		return
	}
	coinPrice, _ := api.GetPrice(coins[0] + `_usdt`)
	bigOrder := 0
	lastPrice := 0.0
	lastAmount := 0.0
	lastTs := 0
	for _, deal := range model.AppMarkets.Deals[symbol][market] {
		if deal.Amount*coinPrice > 10000 && current-int64(1000*deal.Ts) < 10000 {
			bigOrder++
			if deal.Ts > lastTs {
				lastTs = deal.Ts
				lastPrice = deal.Price
				lastAmount = deal.Amount
			}
		}
	}
	if bigOrder < 3 {
		return
	}
	bidPrice := model.AppMarkets.BidAsks[symbol][market].Bids[0].Price
	askPrice := model.AppMarkets.BidAsks[symbol][market].Asks[0].Price

	if lastPrice-bidPrice < priceDistance || askPrice-lastPrice < priceDistance {
		util.Notice(fmt.Sprintf(`to order price %f already have bid: %f, ask: %f`, lastPrice, bidPrice, askPrice))
		return
	}
	amount := math.Min(leftBalance, rightBalance/bidAsk.Bids[0].Price) * model.AppConfig.MakerAmountRate
	amount = math.Min(amount, lastAmount/2)
	bidOrder := api.PlaceOrder(model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``, lastPrice, amount)
	if bidOrder.OrderId == `` || bidOrder.Status == model.CarryStatusFail {
		return
	}
	time.Sleep(time.Second)
	api.MustCancel(market, symbol, bidOrder.OrderId, true)
	time.Sleep(time.Second)
	bidOrder = api.QueryOrderById(market, symbol, bidOrder.OrderId)
	model.AppDB.Save(bidOrder)
	if bidOrder.DealAmount > 0.5*bidOrder.Amount {
		api.PlaceOrder(model.OrderSideSell, model.OrderTypeLimit, market, symbol, ``, lastPrice, amount)
	}

	//orderSide := model.OrderSideBuy
	//if lastMaker != nil && lastMaker.OrderSide == model.OrderSideBuy {
	//	orderSide = model.OrderSideSell
	//}
	//order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, price, amount)
	//if order == nil {
	//	return
	//}
	//time.Sleep(time.Millisecond * 500)
	//tempOrder := api.QueryOrderById(market, symbol, lastMaker.OrderId)
	//if tempOrder != nil {
	//	order = tempOrder
	//}
	//if order.Status == model.CarryStatusWorking {
	//	api.CancelOrder(market, symbol, lastMaker.OrderId)
	//}
	//order.Function = model.FunctionMaker
	//lastMaker = order
	//model.AppDB.Save(lastMaker)
	//time.Sleep(time.Millisecond * time.Duration(model.AppConfig.WaitMaker))
	//orderCount++
	if orderCount%30 == 0 {
		api.RefreshAccount(market)
	}
}
