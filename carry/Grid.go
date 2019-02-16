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
	griding, selling, buying, edging bool
	sellOrder, buyOrder              *model.Order
	lastPrice                        float64
	lastSide                         string
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
		grid.lastPrice*(1+setting.GridPriceDistance)), bidAsk.Bids[0].Price+priceDistance)
	priceBuy := priceSell * (1 - 2*setting.GridPriceDistance)
	if grid.lastSide == model.OrderSideBuy {
		priceBuy = math.Min(math.Max(bidAsk.Bids[0].Price*(1-setting.GridPriceDistance),
			grid.lastPrice*(1-setting.GridPriceDistance-0.0005)), bidAsk.Asks[0].Price-priceDistance)
		priceSell = priceBuy * (1 + 2*setting.GridPriceDistance)
	}
	amountBuy := setting.GridAmount / model.AppMarkets.BidAsks[usdtSymbol][market].Bids[0].Price
	amountSell := setting.GridAmount / model.AppMarkets.BidAsks[usdtSymbol][market].Asks[0].Price
	if usdtSymbol == symbol {
		amountBuy = setting.GridAmount / priceBuy
		amountSell = setting.GridAmount / priceSell
	}
	if model.AppAccounts.Data[market][coins[0]].Free < amountSell {
		util.Notice(fmt.Sprintf(`[币光 %s %s]%f`, market, symbol, model.AppAccounts.Data[market][coins[0]].Free))
		grid.buying = true
		grid.edging = true
		go placeGridOrder(model.OrderSideBuy, market, symbol, priceBuy,
			setting.GridEdgeRate*model.AppAccounts.Data[market][coins[1]].Free/grid.lastPrice)
	} else if model.AppAccounts.Data[market][coins[1]].Free/grid.lastPrice < amountBuy {
		util.Notice(fmt.Sprintf(`[钱光 %s %s]%f`, market, symbol, model.AppAccounts.Data[market][coins[1]].Free))
		grid.selling = true
		grid.edging = true
		go placeGridOrder(model.OrderSideSell, market, symbol, priceSell,
			setting.GridEdgeRate*model.AppAccounts.Data[market][coins[0]].Free)
	} else {
		grid.selling = true
		grid.buying = true
		go placeGridOrder(model.OrderSideSell, market, symbol, priceSell, amountSell)
		go placeGridOrder(model.OrderSideBuy, market, symbol, priceBuy, amountBuy)
	}
}

func placeGridOrder(orderSide, market, symbol string, price, amount float64) {
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, price, amount)
	gridChannel <- *order
}

func cancelGridOrder(grid *grid, orderSide string) {
	if orderSide == model.OrderSideSell {
		result, _, _ := api.CancelOrder(grid.sellOrder.Market, grid.sellOrder.Symbol, grid.sellOrder.OrderId)
		if result {
			go model.AppDB.Save(grid.sellOrder)
			grid.sellOrder = nil
		}
	}
	if orderSide == model.OrderSideBuy {
		result, _, _ := api.CancelOrder(grid.buyOrder.Market, grid.buyOrder.Symbol, grid.buyOrder.OrderId)
		if result {
			go model.AppDB.Save(grid.buyOrder)
			grid.buyOrder = nil
		}
	}
}

func handleSellDeal(grid *grid, market, symbol string) {
	order := api.QueryOrderById(market, symbol, grid.sellOrder.OrderId)
	if order != nil && order.OrderId != `` {
		grid.lastSide = model.OrderSideSell
		grid.lastPrice = order.DealPrice
		go model.AppDB.Save(order)
		grid.sellOrder = nil
	}
	api.RefreshAccount(market)
}

func handleBuyDeal(grid *grid, market, symbol string) {
	order := api.QueryOrderById(market, symbol, grid.buyOrder.OrderId)
	if order != nil && order.OrderId != `` {
		grid.lastSide = model.OrderSideBuy
		grid.lastPrice = order.DealPrice
		go model.AppDB.Save(order)
		grid.buyOrder = nil
	}
	api.RefreshAccount(market)
}

var ProcessGrid = func(market, symbol string) {
	grid := getGrid(market, symbol)
	if grid.griding || grid.selling || grid.buying || model.AppConfig.Handle == 0 {
		return
	}
	setGriding(market, symbol, true)
	defer setGriding(market, symbol, false)
	bidAsk := model.AppMarkets.BidAsks[symbol][market]
	if len(bidAsk.Asks) == 0 || bidAsk.Bids.Len() == 0 {
		return
	}
	if grid.sellOrder == nil && grid.buyOrder == nil {
		placeGridOrders(market, symbol, bidAsk)
	} else if grid.buyOrder != nil && grid.sellOrder != nil {
		if grid.sellOrder.Price < bidAsk.Asks[0].Price {
			handleSellDeal(grid, market, symbol)
		}
		if grid.buyOrder.Price > bidAsk.Bids[0].Price {
			handleBuyDeal(grid, market, symbol)
		}
	} else if grid.edging {
		if grid.sellOrder != nil && grid.sellOrder.Price < bidAsk.Asks[0].Price {
			handleSellDeal(grid, market, symbol)
		}
		if grid.buyOrder != nil && grid.buyOrder.Price > bidAsk.Bids[0].Price {
			handleBuyDeal(grid, market, symbol)
		}
	} else if grid.buyOrder != nil {
		cancelGridOrder(grid, model.OrderSideBuy)
	} else if grid.sellOrder != nil {
		cancelGridOrder(grid, model.OrderSideSell)
	}
	time.Sleep(time.Microsecond * 100)
}

func GridServe() {
	for true {
		order := <-gridChannel
		grid := getGrid(order.Market, order.Symbol)
		if order.OrderSide == model.OrderSideBuy {
			if order.OrderId != `` {
				grid.buyOrder = &order
			}
			grid.buying = false
		}
		if order.OrderSide == model.OrderSideSell {
			if order.OrderId != `` {
				grid.sellOrder = &order
			}
			grid.selling = false
		}
	}
}
