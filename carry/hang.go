package carry

import (
	"fmt"
	"hello/model"
	"hello/util"
	"sync"
	"time"
)

var lastHangTime = 0
var hanging = false

type hangStatus struct {
	lock         sync.Mutex
	hanging      bool
	lastHangTime map[string]*time.Time // symbol - *time
}

var ProcessHang = func(market, symbol string) {
	grid := getGrid(market, symbol)
	if grid.griding || model.AppConfig.Handle != `1` || model.AppConfig.HandleGrid != `1` {
		return
	}
	setGriding(market, symbol, true)
	defer setGriding(market, symbol, false)
	bidAsk := model.AppMarkets.BidAsks[symbol][market]
	if len(bidAsk.Asks) == 0 || bidAsk.Bids.Len() == 0 {
		return
	}
	delay := util.GetNowUnixMillion() - int64(model.AppMarkets.BidAsks[symbol][market].Ts)
	if delay > 50 {
		util.Notice(fmt.Sprintf(`[delay too long] %d`, delay))
		return
	}
	if grid.sellOrder == nil || grid.buyOrder == nil {
		placeGridOrders(market, symbol, bidAsk)
		for true {
			<-snycGrid
			if grid.sellOrder != nil && grid.buyOrder != nil {
				break
			}
		}

	} else if grid.sellOrder != nil && grid.sellOrder.Price < bidAsk.Asks[0].Price {
		util.Notice(fmt.Sprintf(` sell id %s at price %f < %f`, grid.sellOrder.OrderId, grid.sellOrder.Price, bidAsk.Asks[0].Price))
		handleOrderDeal(grid, grid.sellOrder, market, model.OrderSideSell)
	} else if grid.buyOrder != nil && grid.buyOrder.Price > bidAsk.Bids[0].Price {
		util.Notice(fmt.Sprintf(`buy id %s at price %f < %f`, grid.buyOrder.OrderId, grid.buyOrder.Price, bidAsk.Bids[0].Price))
		handleOrderDeal(grid, grid.buyOrder, market, model.OrderSideBuy)
	}
	CancelOldGridOrders()
	time.Sleep(time.Microsecond * 100)
}
