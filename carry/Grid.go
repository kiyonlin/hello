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

var marketSymbolGrid = make(map[string]map[string]*grid)

type grid struct {
	griding, selling, buying bool
	sellOrder, buyOrder      *model.Order
	lastPrice                float64
	lastSide                 string
}

var gridChannel = make(chan model.Order, 50)

func setGriding(market, symbol string, ing bool) {
	if marketSymbolGrid[market] == nil {
		marketSymbolGrid[market] = make(map[string]*grid)
	}
	if marketSymbolGrid[market][symbol] == nil {
		marketSymbolGrid[market][symbol] = &grid{griding: ing}
	}
	marketSymbolGrid[market][symbol].griding = ing
}

func getGrid(market, symbol string) (gridType *grid) {
	if marketSymbolGrid[market] == nil {
		marketSymbolGrid[market] = make(map[string]*grid)
	}
	if marketSymbolGrid[market][symbol] == nil {
		marketSymbolGrid[market][symbol] = &grid{}
	}
	return marketSymbolGrid[market][symbol]
}

func placeGridOrders(market, symbol string, bidAsk *model.BidAsk) {
	coins := strings.Split(symbol, `_`)
	if len(coins) != 2 {
		util.Notice(`symbol format not supported ` + symbol)
		return
	}
	grid := getGrid(market, symbol)
	setting := model.GetSetting(market, symbol)
	if grid.lastPrice == 0 {
		grid.lastPrice = (bidAsk.Bids[0].Price + bidAsk.Asks[0].Price) / 2
	}
	if model.AppAccounts.Data[market][coins[0]] == nil || model.AppAccounts.Data[market][coins[1]] == nil {
		api.RefreshAccount(market)
		util.Notice(fmt.Sprintf(`nil account data for %s`, symbol))
		return
	}
	usdtSymbol := coins[0] + `_usdt`
	if model.AppMarkets.BidAsks[usdtSymbol] == nil || model.AppMarkets.BidAsks[usdtSymbol][market] == nil {
		util.Notice(fmt.Sprintf(`%s 没有usdt价格 %s`, symbol, usdtSymbol))
		return
	}
	priceDistance := 1 / math.Pow(10, float64(api.GetPriceDecimal(market, symbol)))
	priceSell := math.Max(math.Min(bidAsk.Asks[0].Price*(1+setting.GridPriceDistance),
		grid.lastPrice*(1+setting.GridPriceDistance+0.0007)), bidAsk.Bids[0].Price+priceDistance)
	priceBuy := priceSell * (1 - 2*setting.GridPriceDistance)
	if grid.lastSide == model.OrderSideBuy {
		priceBuy = math.Min(math.Max(bidAsk.Bids[0].Price*(1-setting.GridPriceDistance),
			grid.lastPrice*(1-setting.GridPriceDistance-0.0007)), bidAsk.Asks[0].Price-priceDistance)
		priceSell = priceBuy * (1 + 2*setting.GridPriceDistance)
	}
	amountBuy := setting.GridAmount / model.AppMarkets.BidAsks[usdtSymbol][market].Bids[0].Price
	amountSell := setting.GridAmount / model.AppMarkets.BidAsks[usdtSymbol][market].Asks[0].Price
	if usdtSymbol == symbol {
		amountBuy = setting.GridAmount / priceBuy
		amountSell = setting.GridAmount / priceSell
	}
	if amountSell < priceDistance && amountBuy < priceDistance {
		return
	}
	grid.selling = true
	grid.buying = true
	go placeGridOrder(model.OrderSideSell, market, symbol, priceSell, amountSell)
	go placeGridOrder(model.OrderSideBuy, market, symbol, priceBuy, amountBuy)
}

func placeGridOrder(orderSide, market, symbol string, price, amount float64) {
	if price <= 0 {
		return
	}
	order := &model.Order{OrderSide: orderSide, OrderType: model.OrderTypeLimit, Market: market, Symbol: symbol,
		AmountType: ``, Price: price, Amount: amount, OrderId: ``, ErrCode: ``,
		Status: model.CarryStatusFail, DealAmount: 0, DealPrice: price}
	if model.AppConfig.Handle == `1` {
		order = api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, price, amount)
	}
	gridChannel <- *order
}

func handleOrderDeal(grid *grid, order *model.Order, market, orderSide string) {
	grid.lastSide = orderSide
	grid.lastPrice = order.Price
	order.DealPrice = order.Price
	order.DealAmount = order.Amount
	grid.sellOrder = nil
	grid.buyOrder = nil
	go model.AppDB.Save(order)
	api.RefreshAccount(market)
}

var ProcessGrid = func(market, symbol string) {
	grid := getGrid(market, symbol)
	if grid.griding || grid.selling || grid.buying {
		return
	}
	setGriding(market, symbol, true)
	defer setGriding(market, symbol, false)
	bidAsk := model.AppMarkets.BidAsks[symbol][market]
	if len(bidAsk.Asks) == 0 || bidAsk.Bids.Len() == 0 {
		return
	}
	if grid.sellOrder == nil || grid.buyOrder == nil {
		placeGridOrders(market, symbol, bidAsk)
	} else if grid.sellOrder != nil && grid.sellOrder.Price < bidAsk.Asks[0].Price {
		handleOrderDeal(grid, grid.sellOrder, market, model.OrderSideSell)
	} else if grid.buyOrder != nil && grid.buyOrder.Price > bidAsk.Bids[0].Price {
		handleOrderDeal(grid, grid.buyOrder, market, model.OrderSideBuy)
	}
	time.Sleep(time.Microsecond * 100)
}

func GridServe() {
	for true {
		order := <-gridChannel
		grid := getGrid(order.Market, order.Symbol)
		if order.OrderSide == model.OrderSideBuy {
			grid.buyOrder = &order
			grid.buying = false
		}
		if order.OrderSide == model.OrderSideSell {
			grid.sellOrder = &order
			grid.selling = false
		}
	}
}
