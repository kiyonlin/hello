package carry

import (
	"hello/api"
	"hello/model"
	"hello/util"
	"strings"
)

var marketSymbolGrid = make(map[string]map[string]*grid)

type grid struct {
	griding, selling, buying, edging bool
	sellOrder, buyOrder              *model.Order
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
	setting := model.GetSetting(market, symbol)
	priceMiddle := (bidAsk.Bids[0].Price + bidAsk.Asks[0].Price) / 2
	priceSell := (1 + setting.GridPriceDistance) * priceMiddle
	priceBuy := (1 - setting.GridPriceDistance) * priceMiddle
	if model.AppAccounts.Data[market][coins[0]] == nil || model.AppAccounts.Data[market][coins[1]] == nil {
		api.RefreshAccount(market)
	}
	grid := getGrid(market, symbol)
	if model.AppAccounts.Data[market][coins[0]].Free < setting.GridAmount {
		grid.buying = true
		grid.edging = true
		go placeGridOrder(model.OrderSideBuy, market, symbol, priceBuy,
			setting.GridEdgeRate*model.AppAccounts.Data[market][coins[1]].Free/priceMiddle)
	} else if model.AppAccounts.Data[market][coins[1]].Free/priceMiddle < setting.GridAmount {
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
				go model.AppDB.Save(&grid.buyOrder)
				grid.buyOrder = nil
				cancelResult, _, _ := api.CancelOrder(market, symbol, grid.sellOrder.OrderId)
				if cancelResult {
					grid.sellOrder = nil
				}
			}
		} else {
			go model.AppDB.Save(&grid.sellOrder)
			grid.sellOrder = nil
			if grid.buyOrder.Status == model.CarryStatusWorking {
				cancelResult, _, _ := api.CancelOrder(market, symbol, grid.buyOrder.OrderId)
				if cancelResult {
					grid.buyOrder = nil
				}
			} else {
				go model.AppDB.Save(&grid.buyOrder)
				grid.buyOrder = nil
			}
		}
		api.RefreshAccount(market)
	} else if grid.edging {
		if grid.sellOrder != nil {
			order := api.QueryOrderById(market, symbol, grid.sellOrder.OrderId)
			if order != nil && order.OrderId != `` && order.Status != model.CarryStatusWorking {
				go model.AppDB.Save(&order)
				grid.sellOrder = nil
				grid.edging = false
			}
		}
		if grid.buyOrder != nil {
			order := api.QueryOrderById(market, symbol, grid.buyOrder.OrderId)
			if order != nil && order.OrderId != `` && order.Status != model.CarryStatusWorking {
				go model.AppDB.Save(&order)
				grid.buyOrder = nil
				grid.edging = false
			}
		}
	} else if grid.buyOrder != nil {
		cancelResult, _, _ := api.CancelOrder(market, symbol, grid.buyOrder.OrderId)
		if cancelResult {
			go model.AppDB.Save(&grid.buyOrder)
			grid.buyOrder = nil
		}
		api.RefreshAccount(market)
	} else if grid.sellOrder != nil {
		cancelResult, _, _ := api.CancelOrder(market, symbol, grid.sellOrder.OrderId)
		if cancelResult {
			go model.AppDB.Save(&grid.sellOrder)
			grid.sellOrder = nil
		}
		api.RefreshAccount(market)
	}
}

func GridServe() {
	for true {
		order := <-gridChannel
		if order.OrderId == `` {
			continue
		}
		grid := getGrid(order.Market, order.Symbol)
		if order.OrderSide == model.OrderSideBuy {
			grid.sellOrder = &order
			grid.buying = false
		}
		if order.OrderSide == model.OrderSideSell {
			grid.buyOrder = &order
			grid.selling = false
		}
	}
}
