package carry

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"net/http"
	"strings"
)

var marketMaking, marketSelling, marketBuying bool
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
	marketSelling = true
	marketBuying = true
	go placeMarketMaker(model.OrderSideSell, market, symbol, priceSell, amountSell)
	go placeMarketMaker(model.OrderSideBuy, market, symbol, priceBuy, amountBuy)
}

func placeMarketMaker(orderSide, market, symbol string, price, amount float64) {
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, price, amount)
	marketMakeChannel <- *order
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
	return !sellExist || !buyExist
}

var ProcessMake = func(market, symbol string) {
	if marketMaking || marketSelling || marketBuying {
		return
	}
	setMarketMaking(true)
	defer setMarketMaking(false)
	bidAsk := model.AppMarkets.BidAsks[symbol][market]
	if len(bidAsk.Asks) == 0 || bidAsk.Bids.Len() == 0 {
		return
	}
	if len(marketOrders) == 0 {
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
			order := api.QueryOrderById(market, symbol, key)
			if order != nil && (order.Status == model.CarryStatusFail || order.Status == model.CarryStatusSuccess) {
				util.Notice(fmt.Sprintf(`[update %s %s] price: %f %f - %f fee: %f, fee income: %f`, key,
					order.Status, order.Price, bidAsk.Bids[0].Price, bidAsk.Asks[0].Price, order.Fee, order.FeeIncome))
				go model.AppDB.Save(&order)
				delete(marketOrders, key)
				return
			}
		}
	}
}

func GetMarketMaking(c *gin.Context) {
	status := fmt.Sprintf("making %v buying %v selling %v \r\n", marketMaking, marketBuying, marketSelling)
	status += fmt.Sprintf("market orders %d need %v \r\n", len(marketOrders), needPlaceOrders())
	for key, value := range marketOrders {
		status += fmt.Sprintf("order id: %s price: %f side: %s status: %s %s \r\n",
			key, value.Price, value.OrderSide, value.Status, value.CreatedAt.String())
	}
	c.String(http.StatusOK, status)
}

func MarketMakeServe() {
	for true {
		order := <-marketMakeChannel
		if order.OrderId != `` {
			marketOrders[order.OrderId] = &order
		}
		if order.OrderSide == model.OrderSideBuy {
			marketBuying = false
		}
		if order.OrderSide == model.OrderSideSell {
			marketSelling = false
		}
		//util.Notice(fmt.Sprintf(`make market %s %s %s`, order.OrderSide, order.OrderId, order.Status))
	}
}
