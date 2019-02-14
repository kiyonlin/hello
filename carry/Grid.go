package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"strings"
	"time"
)

var marketSymbolGrid = make(map[string]map[string]*grid)

type grid struct {
	griding, selling, buying, edging bool
	sellOrder, buyOrder              *model.Order
	lastPrice                        float64
}

var gridChannel = make(chan model.Order, 50)

func setLastPrice(g *grid, orderSide string, marketPrice, dealPrice float64) {
	if orderSide == model.OrderSideBuy {
		g.lastPrice = dealPrice * 0.999
		if marketPrice > g.lastPrice {
			g.lastPrice = marketPrice
		}
	}
	if orderSide == model.OrderSideSell {
		g.lastPrice = dealPrice * 1.001
		if marketPrice < g.lastPrice {
			g.lastPrice = marketPrice
		}
	}
}

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
	priceSell := (1 + setting.GridPriceDistance) * grid.lastPrice
	priceBuy := (1 - setting.GridPriceDistance) * grid.lastPrice
	if model.AppAccounts.Data[market][coins[0]] == nil || model.AppAccounts.Data[market][coins[1]] == nil {
		api.RefreshAccount(market)
		util.Notice(fmt.Sprintf(`nil account data for %s`, symbol))
		return
	}
	if model.AppAccounts.Data[market][coins[0]].Free < setting.GridAmount {
		util.Notice(fmt.Sprintf(`[币光 %s %s]%f`, market, symbol, model.AppAccounts.Data[market][coins[0]].Free))
		grid.buying = true
		grid.edging = true
		go placeGridOrder(model.OrderSideBuy, market, symbol, priceBuy,
			setting.GridEdgeRate*model.AppAccounts.Data[market][coins[1]].Free/grid.lastPrice)
	} else if model.AppAccounts.Data[market][coins[1]].Free/grid.lastPrice < setting.GridAmount {
		util.Notice(fmt.Sprintf(`[钱光 %s %s]%f`, market, symbol, model.AppAccounts.Data[market][coins[1]].Free))
		grid.selling = true
		grid.edging = true
		go placeGridOrder(model.OrderSideSell, market, symbol, priceSell,
			setting.GridEdgeRate*model.AppAccounts.Data[market][coins[0]].Free)
	} else {
		grid.selling = true
		grid.buying = true
		go placeGridOrder(model.OrderSideSell, market, symbol, priceSell, setting.GridAmount)
		go placeGridOrder(model.OrderSideBuy, market, symbol, priceBuy, setting.GridAmount)
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
		order := api.QueryOrderById(market, symbol, grid.sellOrder.OrderId)
		if order != nil && order.OrderId != `` {
			grid.sellOrder = order
		}
		order = api.QueryOrderById(market, symbol, grid.buyOrder.OrderId)
		if order != nil && order.OrderId != `` {
			grid.buyOrder = order
		}
		if grid.sellOrder.Status == model.CarryStatusWorking {
			if grid.buyOrder.Status != model.CarryStatusWorking {
				setLastPrice(grid, model.OrderSideBuy, bidAsk.Bids[0].Price, grid.buyOrder.DealPrice)
				go model.AppDB.Save(grid.buyOrder)
				grid.buyOrder = nil
				cancelGridOrder(grid, model.OrderSideSell)
			}
		} else {
			setLastPrice(grid, model.OrderSideSell, bidAsk.Asks[0].Price, grid.sellOrder.DealPrice)
			go model.AppDB.Save(grid.sellOrder)
			grid.sellOrder = nil
			if grid.buyOrder.Status == model.CarryStatusWorking {
				cancelGridOrder(grid, model.OrderSideBuy)
			} else {
				go model.AppDB.Save(grid.buyOrder)
				grid.buyOrder = nil
			}
		}
	} else if grid.edging {
		if grid.sellOrder != nil {
			order := api.QueryOrderById(market, symbol, grid.sellOrder.OrderId)
			if order != nil && order.OrderId != `` && order.Status != model.CarryStatusWorking {
				go model.AppDB.Save(order)
				setLastPrice(grid, model.OrderSideSell, bidAsk.Asks[0].Price, order.DealPrice)
				grid.sellOrder = nil
				grid.edging = false
			}
		}
		if grid.buyOrder != nil {
			order := api.QueryOrderById(market, symbol, grid.buyOrder.OrderId)
			if order != nil && order.OrderId != `` && order.Status != model.CarryStatusWorking {
				go model.AppDB.Save(order)
				setLastPrice(grid, model.OrderSideBuy, bidAsk.Bids[0].Price, order.DealPrice)
				grid.buyOrder = nil
				grid.edging = false
			}
		}
	} else if grid.buyOrder != nil {
		cancelGridOrder(grid, model.OrderSideBuy)
	} else if grid.sellOrder != nil {
		cancelGridOrder(grid, model.OrderSideSell)
	}
	api.RefreshAccount(market)
	time.Sleep(time.Second * 1)
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
