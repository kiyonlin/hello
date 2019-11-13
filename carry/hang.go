package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strings"
)

var ProcessHang = func(market, symbol string) {
	result, tick := model.AppMarkets.GetBidAsk(symbol, market)
	if !result || tick == nil || tick.Asks == nil || tick.Bids == nil || tick.Asks.Len() < 11 ||
		tick.Bids.Len() < 11 {
		util.Notice(fmt.Sprintf(`[tick not good]%s %s`, market, symbol))
		return
	}
	if model.AppConfig.Handle != `1` || model.AppPause {
		return
	}
	delay := util.GetNowUnixMillion() - int64(tick.Ts)
	if delay > 500 {
		util.Notice(fmt.Sprintf(`%s %s [delay too long] %d`, market, symbol, delay))
		return
	}
	priceDistance := 1 / math.Pow(10, api.GetPriceDecimal(market, symbol))
	checkDistance := priceDistance / 10
	completeTick(market, symbol, tick, priceDistance, checkDistance)
	setting := model.GetSetting(model.FunctionHang, market, symbol)
	if setting == nil {
		return
	}
	point := (setting.OpenShortMargin + setting.CloseShortMargin) / 2
	didSmth := false
	scoreBid, scoreAsk := calcHighestScore(setting, tick)
	orders := rank.getOrders(symbol)
	newOrders := make([]*model.Order, 0)
	for _, order := range orders {
		orderScore := calcOrderScore(order, setting, tick)
		if orderScore.Point > point/2 {
			newOrders = append(newOrders, order)
		} else if (order.OrderSide == model.OrderSideBuy && order.Price < tick.Bids[0].Price+checkDistance) ||
			(order.OrderSide == model.OrderSideSell && order.Price > tick.Asks[0].Price-checkDistance) {
			api.CancelOrder(``, ``, market, symbol, order.OrderId)
			didSmth = true
		}
	}
	if util.GetNowUnixMillion()-rank.getCheckTime(symbol) > 300000 {
		queryOrders := api.QueryOrders(``, ``, market, symbol, model.CarryStatusWorking, setting.AccountType,
			0, 0)
		for _, queryOrder := range queryOrders {
			for _, order := range newOrders {
				if queryOrder.OrderId == order.OrderId {
					queryOrder.RefreshType = order.RefreshType
					break
				}
			}
		}
		newOrders = queryOrders
		util.Info(fmt.Sprintf(`get working orders from api %s %d`, symbol, len(newOrders)))
		rank.setCheckTime(symbol)
	} else if !didSmth {
		coins := strings.Split(symbol, `_`)
		leftFree, rightFree, _, _, _ := getBalance(key, secret, market, symbol, setting.AccountType)
		if scoreAsk.Point > point || scoreBid.Point > point {
			if rightFree/scoreBid.Price > scoreBid.Amount {
				order := api.PlaceOrder(``, ``, scoreBid.OrderSide, model.OrderTypeLimit, market,
					symbol, ``, setting.AccountType, scoreBid.Price, scoreBid.Amount, false)
				if order.OrderId != `` {
					order.Status = model.CarryStatusWorking
					order.RefreshType = RankRebalance
					if scoreBid.Point > scoreAsk.Point {
						order.RefreshType = RankSequence
					}
					newOrders = append(newOrders, order)
					model.AppDB.Save(&order)
				}
			} else {
				util.Info(fmt.Sprintf(`--- coin influient %s %f<%f point:%f`,
					coins[1], rightFree, scoreBid.Price*scoreBid.Amount, scoreBid.Point))
			}
			if leftFree > scoreAsk.Amount {
				order := api.PlaceOrder(``, ``, scoreAsk.OrderSide, model.OrderTypeLimit, market,
					symbol, ``, setting.AccountType, scoreAsk.Price, scoreAsk.Amount, false)
				if order.OrderId != `` {
					order.Status = model.CarryStatusWorking
					order.RefreshType = RankRebalance
					if scoreBid.Point < scoreAsk.Point {
						order.RefreshType = RankSequence
					}
					newOrders = append(newOrders, order)
					model.AppDB.Save(&order)
				}
			} else {
				util.Info(fmt.Sprintf(`--- coin influient %s %f<%f point:%f`,
					coins[0], leftFree, scoreAsk.Amount, scoreAsk.Point))
			}
		}
	}
	rank.setOrders(symbol, newOrders)
}
