package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strings"
	"sync"
	"time"
)

var marketSymbolGrid = make(map[string]map[string]*grid)
var snycGrid = make(chan interface{}, 10)
var gridLock sync.Mutex

type grid struct {
	griding             bool
	sellOrder, buyOrder *model.Order
	lastPrice           float64
	lastSide            string
	sameSide            int64
}

func setGriding(market, symbol string, ing bool) {
	gridLock.Lock()
	defer gridLock.Unlock()
	if marketSymbolGrid[market] == nil {
		marketSymbolGrid[market] = make(map[string]*grid)
	}
	if marketSymbolGrid[market][symbol] == nil {
		marketSymbolGrid[market][symbol] = &grid{griding: ing}
	}
	marketSymbolGrid[market][symbol].griding = ing
}

func getGrid(market, symbol string) (gridType *grid) {
	gridLock.Lock()
	defer gridLock.Unlock()
	if marketSymbolGrid[market] == nil {
		marketSymbolGrid[market] = make(map[string]*grid)
	}
	if marketSymbolGrid[market][symbol] == nil {
		marketSymbolGrid[market][symbol] = &grid{}
	}
	return marketSymbolGrid[market][symbol]
}

func placeGridOrders(key, secret, market, symbol string, bidAsk *model.BidAsk) (result bool) {
	coins := strings.Split(symbol, `_`)
	if len(coins) != 2 {
		util.Notice(`symbol format not supported ` + symbol)
		return false
	}
	grid := getGrid(market, symbol)
	setting := model.GetSetting(model.FunctionGrid, market, symbol)
	if grid.lastPrice == 0 {
		grid.lastPrice = (bidAsk.Bids[0].Price + bidAsk.Asks[0].Price) / 2
	}
	usdtSymbol := coins[0] + `_usdt`
	result, tick := model.AppMarkets.GetBidAsk(usdtSymbol, market)
	if !result {
		util.Notice(fmt.Sprintf(`%s 没有usdt价格 %s`, symbol, usdtSymbol))
		return false
	}
	priceDistance := 1 / math.Pow(10, api.GetPriceDecimal(market, symbol))
	priceSell := math.Max(math.Min(bidAsk.Asks[0].Price*(1+setting.GridPriceDistance),
		grid.lastPrice*(1+setting.GridPriceDistance+0.0007)), bidAsk.Bids[0].Price+priceDistance)
	priceBuy := priceSell * (1 - 2*setting.GridPriceDistance)
	if grid.lastSide == model.OrderSideBuy {
		priceBuy = math.Min(math.Max(bidAsk.Bids[0].Price*(1-setting.GridPriceDistance),
			grid.lastPrice*(1-setting.GridPriceDistance-0.0007)), bidAsk.Asks[0].Price-priceDistance)
		priceSell = priceBuy * (1 + 2*setting.GridPriceDistance)
	}
	amountBuy := setting.GridAmount / tick.Bids[0].Price
	amountSell := setting.GridAmount / tick.Asks[0].Price
	if usdtSymbol == symbol {
		amountBuy = setting.GridAmount / priceBuy
		amountSell = setting.GridAmount / priceSell
	}
	leftFree, rightFree, _, _, err := getBalance(key, secret, market, symbol, setting.AccountType)
	if err != nil || (leftFree < amountSell && rightFree < setting.GridAmount) {
		util.Notice(fmt.Sprintf(`balance not enough %s %s %f %f %f %f`,
			market, symbol, leftFree, rightFree, amountSell, setting.GridAmount))
		return false
	}
	grid.buyOrder = nil
	grid.sellOrder = nil
	go placeGridOrder(key, secret, model.OrderSideSell, market, symbol, setting.AccountType, priceSell, amountSell)
	go placeGridOrder(key, secret, model.OrderSideBuy, market, symbol, setting.AccountType, priceBuy, amountBuy)
	return true
}

func placeGridOrder(key, secret, orderSide, market, symbol, accountType string, price, amount float64) {
	if price <= 0 {
		return
	}
	order := &model.Order{OrderSide: orderSide, OrderType: model.OrderTypeLimit, Market: market, Symbol: symbol,
		AmountType: ``, Price: price, Amount: amount, OrderId: ``, ErrCode: ``,
		Status: model.CarryStatusFail, DealAmount: 0, DealPrice: price}
	order = api.PlaceOrder(key, secret, orderSide, model.OrderTypeLimit, market, symbol, ``,
		accountType, price, amount, true)
	order.Function = model.FunctionGrid
	grid := getGrid(order.Market, order.Symbol)
	if order.OrderSide == model.OrderSideBuy {
		grid.buyOrder = order
	}
	if order.OrderSide == model.OrderSideSell {
		grid.sellOrder = order
	}
	snycGrid <- struct{}{}
}

func handleOrderDeal(key, secret string, grid *grid, order *model.Order, market, orderSide string) {
	if grid.lastSide == orderSide {
		grid.sameSide++
		if grid.sameSide > 15 {
			setting := model.GetSetting(model.FunctionGrid, market, order.Symbol)
			setting.GridPriceDistance = setting.GridPriceDistance * 2
			model.AppDB.Save(setting)
			model.LoadSettings()
			grid.sameSide = 0
		}
	} else {
		grid.sameSide = 0
	}
	grid.lastSide = orderSide
	grid.lastPrice = order.Price
	order.DealPrice = order.Price
	order.DealAmount = order.Amount
	order.Function = model.FunctionGrid
	util.Notice(fmt.Sprintf(`set buyId %s sellId %s to nil `, grid.buyOrder.OrderId, grid.sellOrder.OrderId))
	grid.sellOrder = nil
	grid.buyOrder = nil
	if order.OrderId != `` {
		api.RefreshAccount(key, secret, market)
	}
}

var ProcessGrid = func(market, symbol string) {
	grid := getGrid(market, symbol)
	if grid.griding || model.AppConfig.Handle != `1` || model.AppConfig.HandleGrid != `1` || model.AppPause {
		return
	}
	setGriding(market, symbol, true)
	defer setGriding(market, symbol, false)
	result, tick := model.AppMarkets.GetBidAsk(symbol, market)
	if !result {
		return
	}
	delay := util.GetNowUnixMillion() - int64(tick.Ts)
	if delay > 100 {
		util.Notice(fmt.Sprintf(`[delay too long] %d`, delay))
		return
	}
	if grid.sellOrder == nil || grid.buyOrder == nil {
		if placeGridOrders(``, ``, market, symbol, tick) {
			for true {
				<-snycGrid
				if grid.sellOrder != nil && grid.buyOrder != nil {
					break
				}
			}
		}
	} else if grid.sellOrder != nil && grid.sellOrder.Price < tick.Asks[0].Price {
		util.Notice(fmt.Sprintf(` sell id %s at price %f < %f`, grid.sellOrder.OrderId, grid.sellOrder.Price,
			tick.Asks[0].Price))
		handleOrderDeal(``, ``, grid, grid.sellOrder, market, model.OrderSideSell)
	} else if grid.buyOrder != nil && grid.buyOrder.Price > tick.Bids[0].Price {
		util.Notice(fmt.Sprintf(`buy id %s at price %f < %f`, grid.buyOrder.OrderId, grid.buyOrder.Price,
			tick.Bids[0].Price))
		handleOrderDeal(``, ``, grid, grid.buyOrder, market, model.OrderSideBuy)
	}
	time.Sleep(time.Microsecond * 100)
}
