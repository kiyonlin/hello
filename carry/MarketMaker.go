package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strings"
)

var marketMaking = false
var marketOrders = make(map[string]*model.Order)
var marketMakeChannel = make(chan model.Order, 50)

func setMarketMaking(making bool) {
	marketMaking = making
}

//1. 买卖单同时下单，下单数量分别为帐户可买总量和可卖总量的1/7（可调参数），
// if 买单数量>卖单数量，卖单价格为卖1价格，买单价格=卖1价格-0.01；else 买单价格=买1价格，卖单价格=买一价格+0.01
//2. 当买一价或卖一价位上没有自己的委托单时（已成交或者价格变动造成），撤掉所有不在买一和卖一价位上的委托单，重新下单
func placeMarketMakers(market, symbol string, bidAsk *model.BidAsk) {
	coins := strings.Split(symbol, `_`)
	if len(coins) != 2 {
		util.Notice(`symbol format not supported ` + symbol)
		return
	}
	api.RefreshAccount(market)
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	priceSell := bidAsk.Asks[0].Price
	priceBuy := bidAsk.Bids[0].Price
	if bidAsk.Bids[0].Amount < bidAsk.Asks[0].Amount {
		priceSell = bidAsk.Bids[0].Price + priceDistance
	} else {
		priceBuy = bidAsk.Asks[0].Price - priceDistance
	}
	amountSell := model.AppAccounts.Data[market][coins[0]].Free * model.AppConfig.MakerRate
	amountBuy := model.AppAccounts.Data[market][coins[1]].Free / priceSell * model.AppConfig.MakerRate
	go placeMarketMaker(model.OrderSideSell, market, symbol, priceSell, amountSell)
	go placeMarketMaker(model.OrderSideBuy, market, symbol, priceBuy, amountBuy)
}

func placeMarketMaker(orderSide, market, symbol string, price, amount float64) {
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, price, amount)
	marketMakeChannel <- *order
}

func updateMarketMaker(order *model.Order) (done bool) {
	if order.Status == model.CarryStatusWorking {
		return false
	}
	util.Notice(fmt.Sprintf(`market order result: %s status:%s with fee: %f, fee income: %f`,
		order.OrderId, order.Status, order.Fee, order.FeeIncome))
	model.AppDB.Save(&order)
	return true
}

func needPlaceOrders() (need bool) {
	if model.AppConfig.Handle == 0 {
		return false
	}
	sellExist := false
	buyExist := false
	for _, value := range marketOrders {
		if value.OrderId != `` && value.OrderSide == model.OrderSideSell {
			sellExist = true
		}
		if value.OrderId != `` && value.OrderSide == model.OrderSideBuy {
			buyExist = true
		}
	}
	return sellExist && buyExist
}

var ProcessMake = func(market, symbol string) {
	if marketMaking == true {
		return
	}
	setMarketMaking(true)
	defer setMarketMaking(false)
	bidAsk := model.AppMarkets.BidAsks[symbol][market]
	if len(bidAsk.Asks) == 0 || bidAsk.Bids.Len() == 0 {
		return
	}
	util.Notice(fmt.Sprintf(`----------- %f - %f`, bidAsk.Bids[0].Price, bidAsk.Asks[0].Price))
	if marketOrders == nil {
		workingOrders := api.QueryOrders(market, symbol, model.CarryStatusWorking)
		for _, value := range workingOrders {
			api.CancelOrder(market, symbol, value.OrderId)
		}
	}
	priceHalfDistance := 0.5 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	if needPlaceOrders() {
		placeMarketMakers(market, symbol, bidAsk)
	} else {
		for key, value := range marketOrders {
			if (value.OrderSide == model.OrderSideSell && value.Price-bidAsk.Asks[0].Price > priceHalfDistance) ||
				(value.OrderSide == model.OrderSideBuy && bidAsk.Bids[0].Price-value.Price > priceHalfDistance) {
				api.CancelOrder(market, symbol, key)
			}
			if updateMarketMaker(api.QueryOrderById(market, symbol, key)) {
				delete(marketOrders, key)
			}
		}
	}
}

func MarketMakeServe() {
	for true {
		order := <-marketMakeChannel
		marketOrders[order.OrderId] = &order
		//util.Notice(fmt.Sprintf(`make market %s %s %s`, order.OrderSide, order.OrderId, order.Status))
	}
}
