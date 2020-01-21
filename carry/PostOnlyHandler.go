package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"time"
)

var PostonlyHandler = func(market, symbol string, order interface{}) {
	startTime := util.GetNowUnixMillion()
	//_, tick := model.AppMarkets.GetBidAsk(symbol, market)
	if order == nil || model.AppConfig.Handle != `1` {
		return
	}
	//if tick == nil || tick.Bids == nil || tick.Asks == nil || tick.Bids.Len() < 1 || tick.Asks.Len() < 1 {
	//	util.Notice(`error1 tick absent`)
	//	return
	//}
	//if tick.Bids[0].Price >= tick.Asks[0].Price {
	//	util.Notice(fmt.Sprintf(`error2 bad tick %f-%f`, tick.Bids[0].Price, tick.Asks[0].Price))
	//	return
	//}
	//if int(startTime)-tick.Ts > 200 {
	//	util.Notice(fmt.Sprintf(`error3 delay too long %d %d`, startTime, tick.Ts))
	//	return
	//}
	orderPostonly := order.(*model.Order)
	if orderPostonly.RefreshType != model.PostOnly || orderPostonly.Amount-orderPostonly.DealAmount < 1 ||
		orderPostonly.OrderId == `` || orderPostonly.Status != model.CarryStatusFail {
		if orderPostonly.RefreshType == model.PostOnly {
			util.Notice(fmt.Sprintf(`leave alone: %s %s %s`,
				orderPostonly.OrderId, orderPostonly.Status, orderPostonly.RefreshType))
		}
		return
	}
	orderSide := orderPostonly.OrderSide
	price := orderPostonly.Price
	_, restBid, restAsk := api.GetOrderBook(``, ``, symbol)
	for restBid == nil || restAsk == nil {
		_, restBid, restAsk = api.GetOrderBook(``, ``, symbol)
	}
	price = restAsk.Price - api.GetPriceDistance(orderPostonly.Market, orderPostonly.Symbol)
	if orderSide == model.OrderSideSell {
		price = restBid.Price + api.GetPriceDistance(orderPostonly.Market, orderPostonly.Symbol)
	}
	//if tick != nil && tick.Asks != nil && tick.Bids != nil && tick.Asks.Len() > 0 && tick.Bids.Len() > 0 {
	//	price = tick.Asks[0].Price - api.GetPriceDistance(orderPostonly.Market, orderPostonly.Symbol)
	//	if orderSide == model.OrderSideSell {
	//		price = tick.Bids[0].Price + api.GetPriceDistance(orderPostonly.Market, orderPostonly.Symbol)
	//	}
	//} else {
	//	util.Notice(`fatal tick error`)
	//}
	amount := orderPostonly.Amount - orderPostonly.DealAmount
	for true {
		orderPostonly = api.PlaceOrder(``, ``, orderSide, model.OrderTypeLimit, market, symbol,
			``, ``, model.PostOnly, price, amount, true)
		if orderPostonly != nil && orderPostonly.OrderId != `` {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	util.Notice(fmt.Sprintf(`reorder: %s order %s %s %s %f %f orderParam:<%s> delay:%d`, market,
		orderSide, model.OrderTypeLimit, symbol, price, amount, model.PostOnly, util.GetNowUnixMillion()-startTime))
}
