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
	if lastMaker == nil {
		lastMaker = api.PlaceOrder(model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``, price, amount)
	} else if lastMaker.OrderSide == model.OrderSideSell {
		order := api.PlaceOrder(model.OrderSideBuy, model.OrderTypeLimit, market, symbol, ``, price, amount)
		time.Sleep(time.Millisecond * 500)
		api.MustCancel(market, symbol, lastMaker.OrderId, true)
		lastMaker = order
	} else if lastMaker.OrderSide == model.OrderSideBuy {
		order := api.PlaceOrder(model.OrderSideSell, model.OrderTypeLimit, market, symbol, ``, price, amount)
		time.Sleep(time.Millisecond * 500)
		api.MustCancel(market, symbol, lastMaker.OrderId, true)
		lastMaker = order
	}
	model.AppDB.Save(order)
	time.Sleep(time.Second * 2)
	api.RefreshAccount(market)
}
