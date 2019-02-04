package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strconv"
	"strings"
	"time"
)

var marketMaking = false
var marketMakeOrders []*model.Order
var marketMakeChannel = make(chan model.Order, 50)

func setMarketMaking(making bool) {
	marketMaking = making
}

func placeMarketMaker(market, symbol string, bidAsk *model.BidAsk) {
	coins := strings.Split(symbol, `_`)
	if len(coins) != 2 {
		util.Notice(`symbol format not supported ` + symbol)
		return
	}
	api.RefreshAccount(market)
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	bidPrice := bidAsk.Bids[0].Price
	askPrice := bidAsk.Asks[0].Price
	if bidAsk.Bids[0].Amount > bidAsk.Asks[0].Amount {
		bidPrice = askPrice - priceDistance
	} else {
		askPrice = bidPrice + priceDistance
	}
	leftAmount := model.AppAccounts.Data[market][coins[0]].Free * model.AppConfig.MakerRate
	rightAmount := model.AppAccounts.Data[market][coins[1]].Free / bidPrice * model.AppConfig.MakerRate
	formatStr := `%.` + strconv.Itoa(api.GetAmountDecimal(model.Fcoin, `btc_usdt`)) + `f`
	leftAmount, _ = strconv.ParseFloat(fmt.Sprintf(formatStr, leftAmount), 64)
	rightAmount, _ = strconv.ParseFloat(fmt.Sprintf(formatStr, rightAmount), 64)
	marketMakeOrders = make([]*model.Order, 0)
	go placeOrder(model.OrderSideBuy, model.OrderTypeLimit, market, symbol, bidPrice, rightAmount)
	go placeOrder(model.OrderSideSell, model.OrderTypeLimit, market, symbol, askPrice, leftAmount)
}

func placeOrder(orderSide, orderType, market, symbol string, price, amount float64) {
	order := api.PlaceOrder(orderSide, orderType, market, symbol, ``, price, amount)
	marketMakeChannel <- *order
}

func cancelMarketMaker(market, symbol string, orders map[string]*model.Order) {
	for _, value := range orders {
		if value.OrderId == `` {
			continue
		}
		api.CancelOrder(market, symbol, value.OrderId)
		//util.Notice(fmt.Sprintf(`%v to cancel order %s, %s %s`, result, value.OrderId, errCode, errMsg))
	}
}

func updateMarketMaker(market, symbol string, orders []*model.Order) {
	for _, value := range orders {
		order := api.SyncQueryOrderById(market, symbol, value.OrderId)
		if order == nil {
			continue
		}
		util.Notice(fmt.Sprintf(`order %s status:%s with fee: %f, fee income: %f`,
			order.OrderId, order.Status, order.Fee, order.FeeIncome))
		model.OrderChannel <- *order
	}
}

var ProcessMake = func(market, symbol string) {
	if marketMaking == true || model.AppConfig.Handle == 0 {
		return
	}
	setMarketMaking(true)
	defer setMarketMaking(false)
	bidAsk := model.AppMarkets.BidAsks[symbol][market]
	if len(bidAsk.Asks) == 0 || bidAsk.Bids.Len() == 0 {
		return
	}
	if marketMakeOrders == nil {
		placeMarketMaker(market, symbol, bidAsk)
	} else if len(marketMakeOrders) < 2 {
		util.Info(fmt.Sprintf(`[sync waiting] pending for another order, now %d`,
			len(marketMakeOrders)))
		time.Sleep(time.Second * 3)
	} else {
		workingOrders := api.QueryOrders(market, symbol, model.CarryStatusWorking)
		makerTaken := false
		for _, value := range marketMakeOrders {
			if value.OrderId == `` || workingOrders[value.OrderId] == nil {
				makerTaken = true
				util.Notice(fmt.Sprintf(`can not find %s from working orders, things changed`,
					value.OrderId))
				break
			}
		}
		if makerTaken {
			cancelMarketMaker(market, symbol, workingOrders)
			updateMarketMaker(market, symbol, marketMakeOrders)
			marketMakeOrders = nil
		}
	}
}

func MarketMakeServe() {
	for true {
		order := <-marketMakeChannel
		//util.Notice(fmt.Sprintf(`make market %s %s %s`, order.OrderSide, order.OrderId, order.Status))
		marketMakeOrders = append(marketMakeOrders, &order)
	}
}
