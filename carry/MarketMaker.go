package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strconv"
	"strings"
)

var marketMaking = false
var marketOrderSell, marketOrderBuy *model.Order

func setMarketMaking(making bool) {
	marketMaking = making
}

//1. 买卖单同时下单，下单数量分别为帐户可买总量和可卖总量的1/7（可调参数），
// if 买单数量>卖单数量，卖单价格为卖1价格，买单价格=卖1价格-0.01；else 买单价格=买1价格，卖单价格=买一价格+0.01
//2. 当买一价或卖一价位上没有自己的委托单时（已成交或者价格变动造成），撤掉所有不在买一和卖一价位上的委托单，重新下单
func placeMarketMaker(market, symbol, orderSide string, bidAsk *model.BidAsk) (order *model.Order) {
	coins := strings.Split(symbol, `_`)
	if len(coins) != 2 {
		util.Notice(`symbol format not supported ` + symbol)
		return
	}
	var price, amount float64
	api.RefreshAccount(market)
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	formatStr := `%.` + strconv.Itoa(api.GetAmountDecimal(model.Fcoin, symbol)) + `f`
	if orderSide == model.OrderSideSell {
		price = bidAsk.Asks[0].Price
		if bidAsk.Bids[0].Amount < bidAsk.Asks[0].Amount {
			price = bidAsk.Bids[0].Price + priceDistance
		}
		amount = model.AppAccounts.Data[market][coins[0]].Free * model.AppConfig.MakerRate
	} else if orderSide == model.OrderSideBuy {
		price = bidAsk.Bids[0].Price
		if bidAsk.Bids[0].Amount > bidAsk.Asks[0].Amount {
			price = bidAsk.Asks[0].Price - priceDistance
		}
		amount = model.AppAccounts.Data[market][coins[1]].Free / price * model.AppConfig.MakerRate
	}
	amount, _ = strconv.ParseFloat(fmt.Sprintf(formatStr, amount), 64)
	return api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, price, amount)
}

func updateMarketMaker(order *model.Order) (done bool) {
	if order == nil || order.OrderId == `` {
		return false
	}
	if order.Status == model.CarryStatusWorking {
		return false
	}
	util.Notice(fmt.Sprintf(`market order result: %s status:%s with fee: %f, fee income: %f`,
		order.OrderId, order.Status, order.Fee, order.FeeIncome))
	model.AppDB.Save(&order)
	return true
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
	if marketOrderSell == nil && marketOrderBuy == nil {
		workingOrders := api.QueryOrders(market, symbol, model.CarryStatusWorking)
		for _, value := range workingOrders {
			api.CancelOrder(market, symbol, value.OrderId)
		}
	}
	priceHalfDistance := 0.5 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	if marketOrderBuy == nil && model.AppConfig.Handle > 0 {
		marketOrderBuy = placeMarketMaker(market, symbol, model.OrderSideBuy, bidAsk)
	} else if marketOrderSell == nil && model.AppConfig.Handle > 0 {
		marketOrderSell = placeMarketMaker(market, symbol, model.OrderSideSell, bidAsk)
	} else if marketOrderSell.Price-bidAsk.Asks[0].Price > priceHalfDistance {
		api.CancelOrder(market, symbol, marketOrderSell.OrderId)
		util.Notice(fmt.Sprintf(`卖单价格%f>市场最低卖价%f，取消卖单, 买%v-卖%v`, marketOrderSell.Price,
			bidAsk.Asks[0].Price, marketOrderBuy != nil, marketOrderSell != nil))
		if updateMarketMaker(api.SyncQueryOrderById(market, symbol, marketOrderSell.OrderId)) {
			marketOrderSell = nil
		}
	} else if bidAsk.Bids[0].Price-marketOrderBuy.Price > priceHalfDistance {
		api.CancelOrder(market, symbol, marketOrderBuy.OrderId)
		util.Notice(fmt.Sprintf(`买单价格%f<市场最高买价%f，取消买单, 买%v-卖%v`, marketOrderBuy.Price,
			bidAsk.Bids[0].Price, marketOrderBuy != nil, marketOrderSell != nil))
		if updateMarketMaker(api.SyncQueryOrderById(market, symbol, marketOrderBuy.OrderId)) {
			marketOrderBuy = nil
		}
	} else if bidAsk.Bids[0].Price-marketOrderSell.Price > priceHalfDistance {
		if updateMarketMaker(api.QueryOrderById(market, symbol, marketOrderSell.OrderId)) {
			marketOrderSell = nil
		}
	} else if marketOrderBuy.Price-bidAsk.Asks[0].Price > priceHalfDistance {
		if updateMarketMaker(api.QueryOrderById(market, symbol, marketOrderBuy.OrderId)) {
			marketOrderBuy = nil
		}
	} else if math.Abs(bidAsk.Asks[0].Price-marketOrderSell.Price) < priceHalfDistance {
		if updateMarketMaker(api.QueryOrderById(market, symbol, marketOrderSell.OrderId)) {
			marketOrderSell = nil
		}
	} else if math.Abs(bidAsk.Bids[0].Price-marketOrderBuy.Price) < priceHalfDistance {
		if updateMarketMaker(api.QueryOrderById(market, symbol, marketOrderBuy.OrderId)) {
			marketOrderBuy = nil
		}
	}
}
