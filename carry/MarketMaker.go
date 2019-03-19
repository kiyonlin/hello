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
var orderCount int64

func setMarketMaking(making bool) {
	marketMaking = making
}

var ProcessMake = func(market, symbol string) {
	if model.AppConfig.Handle != `1` || model.AppConfig.HandleMaker != `1` || marketMaking {
		return
	}
	setMarketMaking(true)
	defer setMarketMaking(false)
	bidAsk := model.AppMarkets.BidAsks[symbol][market]
	if len(bidAsk.Asks) == 0 || bidAsk.Bids.Len() == 0 {
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
	amount := math.Min(leftBalance, rightBalance/bidAsk.Bids[0].Price) * model.AppConfig.MakerAmountRate
	price := (bidAsk.Asks[0].Price + bidAsk.Bids[0].Price) / 2
	priceDistance := 0.5 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	if bidAsk.Asks[0].Price-price < priceDistance || price-bidAsk.Bids[0].Price < priceDistance {
		util.Notice(fmt.Sprintf(`[maker price crash]%f - %f`, bidAsk.Bids[0].Price, bidAsk.Asks[0].Price))
		return
	}
	orderSide := model.OrderSideBuy
	if lastMaker != nil && lastMaker.OrderSide == model.OrderSideBuy {
		orderSide = model.OrderSideSell
	}
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, price, amount)
	if order == nil {
		return
	}
	time.Sleep(time.Millisecond * 500)
	tempOrder := api.QueryOrderById(market, symbol, lastMaker.OrderId)
	if tempOrder != nil {
		order = tempOrder
	}
	if order.Status == model.CarryStatusWorking {
		api.CancelOrder(market, symbol, lastMaker.OrderId)
	}
	order.Function = model.FunctionMaker
	lastMaker = order
	model.AppDB.Save(lastMaker)
	time.Sleep(time.Millisecond * time.Duration(model.AppConfig.WaitMaker))
	orderCount++
	if orderCount%30 == 0 {
		api.RefreshAccount(market)
	}
}
