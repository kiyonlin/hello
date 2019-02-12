package carry

import (
	"hello/api"
	"hello/model"
	"hello/util"
	"strings"
)

var griding, gridSelling, gridBuying, gridEdging bool
var gridSellOrder, gridBuyOrder *model.Order
var gridChannel = make(chan model.Order, 50)

func setGriding(ing bool) {
	griding = ing
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
	if model.AppAccounts.Data[market][coins[0]].Free < setting.GridAmount {
		gridBuying = true
		gridEdging = true
		go placeGridOrder(model.OrderSideBuy, market, symbol, priceBuy,
			setting.GridEdgeRate*model.AppAccounts.Data[market][coins[1]].Free/priceMiddle)
	} else if model.AppAccounts.Data[market][coins[1]].Free/priceMiddle < setting.GridAmount {
		gridSelling = true
		gridEdging = true
		go placeGridOrder(model.OrderSideSell, market, symbol, priceSell,
			setting.GridEdgeRate*model.AppAccounts.Data[market][coins[0]].Free)
	} else {
		gridSelling = true
		gridBuying = true
		go placeGridOrder(model.OrderSideSell, market, symbol, priceSell, setting.GridAmount)
		go placeGridOrder(model.OrderSideBuy, market, symbol, priceBuy, setting.GridAmount)
	}
}

func placeGridOrder(orderSide, market, symbol string, price, amount float64) {
	order := api.PlaceOrder(orderSide, model.OrderTypeLimit, market, symbol, ``, price, amount)
	gridChannel <- *order
}

var ProcessGrid = func(market, symbol string) {
	if griding || gridSelling || gridBuying || model.AppConfig.Handle == 0 {
		return
	}
	setGriding(true)
	defer setGriding(false)
	bidAsk := model.AppMarkets.BidAsks[symbol][market]
	if len(bidAsk.Asks) == 0 || bidAsk.Bids.Len() == 0 {
		return
	}
	if gridSellOrder == nil && gridBuyOrder == nil {
		placeGridOrders(market, symbol, bidAsk)
	} else if gridBuyOrder != nil && gridSellOrder != nil {
		order := api.QueryOrderById(market, symbol, gridSellOrder.OrderId)
		if order != nil && order.OrderId != `` {
			gridSellOrder = order
		}
		order = api.QueryOrderById(market, symbol, gridBuyOrder.OrderId)
		if order != nil && order.OrderId != `` {
			gridBuyOrder = order
		}
		if gridSellOrder.Status == model.CarryStatusWorking {
			if gridBuyOrder.Status != model.CarryStatusWorking {
				go model.AppDB.Save(&gridBuyOrder)
				gridBuyOrder = nil
				cancelResult, _, _ := api.CancelOrder(market, symbol, gridSellOrder.OrderId)
				if cancelResult {
					gridSellOrder = nil
				}
			}
		} else {
			go model.AppDB.Save(&gridSellOrder)
			gridSellOrder = nil
			if gridBuyOrder.Status == model.CarryStatusWorking {
				cancelResult, _, _ := api.CancelOrder(market, symbol, gridBuyOrder.OrderId)
				if cancelResult {
					gridBuyOrder = nil
				}
			} else {
				go model.AppDB.Save(&gridBuyOrder)
				gridBuyOrder = nil
			}
		}
		api.RefreshAccount(market)
	} else if gridEdging {
		if gridSellOrder != nil {
			order := api.QueryOrderById(market, symbol, gridSellOrder.OrderId)
			if order != nil && order.OrderId != `` && order.Status != model.CarryStatusWorking {
				go model.AppDB.Save(&order)
				gridSellOrder = nil
				gridEdging = false
			}
		}
		if gridBuyOrder != nil {
			order := api.QueryOrderById(market, symbol, gridBuyOrder.OrderId)
			if order != nil && order.OrderId != `` && order.Status != model.CarryStatusWorking {
				go model.AppDB.Save(&order)
				gridBuyOrder = nil
				gridEdging = false
			}
		}
	} else if gridBuyOrder != nil {
		cancelResult, _, _ := api.CancelOrder(market, symbol, gridBuyOrder.OrderId)
		if cancelResult {
			go model.AppDB.Save(&gridBuyOrder)
			gridBuyOrder = nil
		}
		api.RefreshAccount(market)
	} else if gridSellOrder != nil {
		cancelResult, _, _ := api.CancelOrder(market, symbol, gridSellOrder.OrderId)
		if cancelResult {
			go model.AppDB.Save(&gridSellOrder)
			gridSellOrder = nil
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
		if order.OrderSide == model.OrderSideBuy {
			gridSellOrder = &order
			gridBuying = false
		}
		if order.OrderSide == model.OrderSideSell {
			gridBuyOrder = &order
			gridSelling = false
		}
	}
}
