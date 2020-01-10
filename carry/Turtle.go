package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"time"
)

type TurtleData struct {
	turtleTime time.Time
	highDays10 float64
	lowDays10  float64
	highDays20 float64
	lowDays20  float64
	n          float64
	amount     float64
	orderLong  *model.Order
	orderShort *model.Order
}

var turtling = false

func setTurtling(value bool) {
	turtling = value
}

var dataSet = make(map[string]map[string]map[string]*TurtleData) // market - symbol - 2019-12-06 - *turtleData

func GetTurtleData(market, symbol string) (turtleData *TurtleData) {
	today := time.Now().In(time.UTC)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	todayStr := today.String()[0:10]
	if dataSet[market] == nil {
		dataSet[market] = make(map[string]map[string]*TurtleData)
	}
	if dataSet[market][symbol] == nil {
		dataSet[market][symbol] = make(map[string]*TurtleData)
	}
	if dataSet[market][symbol][todayStr] != nil {
		return dataSet[market][symbol][todayStr]
	}
	turtleData = &TurtleData{turtleTime: today}
	var orderLong, orderShort model.Order
	model.AppDB.Model(&orderLong).Where(
		"market= ? and symbol= ? and refresh_type= ? and amount>deal_amount and status=? and order_side=?",
		market, symbol, model.FunctionTurtle, model.CarryStatusWorking, model.OrderSideBuy).Last(&orderLong)
	model.AppDB.Model(&orderShort).Where(
		"market= ? and symbol= ? and refresh_type= ? and amount>deal_amount and status=? and order_side=?",
		market, symbol, model.FunctionTurtle, model.CarryStatusWorking, model.OrderSideSell).Last(&orderShort)
	if orderLong.OrderId != `` {
		api.MustCancel(key, secret, market, symbol, orderLong.OrderId, true)
	}
	if orderShort.OrderId != `` {
		api.MustCancel(key, secret, market, symbol, orderShort.OrderId, true)
	}
	for i := 1; i <= 20; i++ {
		duration, _ := time.ParseDuration(fmt.Sprintf(`%dh`, -24*i))
		day := today.Add(duration)
		candle := api.GetDayCandle(key, secret, market, symbol, day)
		if candle == nil {
			continue
		}
		if candle.PriceHigh > turtleData.highDays20 {
			turtleData.highDays20 = candle.PriceHigh
		}
		if turtleData.lowDays20 == 0 || turtleData.lowDays20 > candle.PriceLow {
			turtleData.lowDays20 = candle.PriceLow
		}
		if candle.PriceHigh > turtleData.highDays10 && i < 11 {
			turtleData.highDays10 = candle.PriceHigh
		}
		if (turtleData.lowDays10 == 0 || turtleData.lowDays10 > candle.PriceLow) && i < 11 {
			turtleData.lowDays10 = candle.PriceLow
		}
		if i == 1 {
			turtleData.n = candle.N
			p := api.GetBtcBalance(``, ``, market)
			switch symbol {
			case `btcusd_p`:
				turtleData.amount = 0.01 * p / turtleData.n * candle.PriceOpen * candle.PriceOpen / 1000
			case `ethusd_p`:
				turtleData.amount = 10000 * p / turtleData.n / 1000
			}
		}
	}
	if turtleData.amount > 0 && turtleData.n > 0 {
		dataSet[market][symbol][todayStr] = turtleData
	}
	return
}

//setting.GridAmount 当前已经持仓数量
//setting.Chance 当前开仓的个数
//setting.PriceX 上一次开仓的价格
var ProcessTurtle = func(market, symbol string) {
	result, tick := model.AppMarkets.GetBidAsk(symbol, market)
	now := util.GetNowUnixMillion()
	if !result || tick == nil || tick.Asks == nil || tick.Bids == nil || model.AppConfig.Handle != `1` ||
		model.AppPause || now-int64(tick.Ts) > 1000 {
		//if tick != nil {
		//util.Info(fmt.Sprintf(`[tick not good]%s %s %d`, market, symbol, now-int64(tick.Ts)))
		//}
		return
	}
	setting := model.GetSetting(model.FunctionTurtle, market, symbol)
	if setting == nil || turtling {
		return
	}
	setTurtling(true)
	defer setTurtling(false)
	turtleData := GetTurtleData(market, symbol)
	if turtleData == nil || turtleData.n == 0 || turtleData.amount == 0 {
		return
	}
	currentN := model.GetCurrentN(model.FunctionTurtle)
	if currentN == float64(model.AppConfig.TurtleLimitMain) && turtleData.orderLong != nil {
		if api.IsValid(turtleData.orderLong) {
			api.MustCancel(key, secret, market, symbol, turtleData.orderLong.OrderId, true)
		}
		turtleData.orderLong = nil
		return
	} else if currentN == -1*float64(model.AppConfig.TurtleLimitMain) && turtleData.orderShort != nil {
		if api.IsValid(turtleData.orderShort) {
			api.MustCancel(key, secret, market, symbol, turtleData.orderShort.OrderId, true)
		}
		turtleData.orderShort = nil
		return
	}
	showMsg := fmt.Sprintf("%s_%s_%s", model.FunctionTurtle, market, symbol)
	model.SetCarryInfo(showMsg, fmt.Sprintf("[海龟参数]%s %s 加仓次数限制:%d 当前已经持仓数量:%f 上一次开仓的价格:%f\n"+
		"20日最高:%f 20日最低:%f 10日最高:%f 10日最低:%f n:%f 数量:%f 当前开仓个数:%f %f",
		turtleData.turtleTime.String()[0:10], showMsg, model.AppConfig.TurtleLimitMain, setting.GridAmount, setting.PriceX,
		turtleData.highDays20, turtleData.lowDays20, turtleData.highDays10, turtleData.lowDays10, turtleData.n,
		turtleData.amount, setting.Chance, currentN))
	priceLong := 0.0
	priceShort := 0.0
	amountLong := turtleData.amount
	amountShort := turtleData.amount
	updateSetting := false
	if setting.Chance == 0 { // 开初始多仓
		priceLong = turtleData.highDays20
		priceShort = turtleData.lowDays20
		if tick.Asks[0].Price > priceLong {
			setting.Chance = 1
			setting.GridAmount = turtleData.amount
			setting.PriceX = priceLong
			updateSetting = true
			util.Notice(fmt.Sprintf(`破20日高点 chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
				setting.Chance, setting.GridAmount, currentN, priceShort, priceLong, setting.PriceX, turtleData.n))
		}
		if tick.Bids[0].Price < priceShort {
			setting.Chance = -1
			setting.GridAmount = turtleData.amount
			setting.PriceX = priceShort
			updateSetting = true
			util.Notice(fmt.Sprintf(`破20日低点 chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
				setting.Chance, setting.GridAmount, currentN, priceShort, priceLong, setting.PriceX, turtleData.n))
		}
	} else if setting.Chance > 0 {
		priceLong = setting.PriceX + turtleData.n/2
		priceShort = math.Max(turtleData.lowDays10, setting.PriceX-2*turtleData.n)
		amountShort = setting.GridAmount
		// 加仓一个单位
		if tick.Asks[0].Price > priceLong && currentN < float64(model.AppConfig.TurtleLimitMain) {
			setting.PriceX = priceLong
			setting.Chance = setting.Chance + 1
			setting.GridAmount = setting.GridAmount + turtleData.amount
			updateSetting = true
			util.Notice(fmt.Sprintf(`加多 chance:%f amount:%f price:%f currentN:%f short-long:%f %f px:%f n:%f`,
				setting.Chance, setting.GridAmount, setting.PriceX, currentN, priceShort, priceLong,
				setting.PriceX, turtleData.n))
			if api.IsValid(turtleData.orderLong) {
				lastOrder := api.QueryOrderById(key, secret, market, symbol, turtleData.orderLong.OrderId)
				if api.IsValid(lastOrder) {
					setting.PriceX = lastOrder.DealPrice
					priceLong = setting.PriceX + turtleData.n/2
					priceShort = math.Max(turtleData.lowDays10, setting.PriceX-2*turtleData.n)
				}
			}
		} // 平多
		if tick.Bids[0].Price < priceShort {
			setting.Chance = 0
			setting.GridAmount = 0
			setting.PriceX = priceShort
			updateSetting = true
			util.Notice(fmt.Sprintf(`平多 chance:%f amount:%f price:%f currentN:%f short-long:%f %f px:%f n:%f`,
				setting.Chance, setting.GridAmount, setting.PriceX, currentN, priceShort, priceLong,
				setting.PriceX, turtleData.n))
			if api.IsValid(turtleData.orderShort) {
				lastOrder := api.QueryOrderById(key, secret, market, symbol, turtleData.orderShort.OrderId)
				if api.IsValid(lastOrder) {
					setting.PriceX = lastOrder.DealPrice
					priceLong = setting.PriceX + turtleData.n/2
					priceShort = math.Max(turtleData.lowDays10, setting.PriceX-2*turtleData.n)
				}
			}
		}
	} else if setting.Chance < 0 {
		priceLong = math.Min(turtleData.highDays10, setting.PriceX+2*turtleData.n)
		priceShort = setting.PriceX - turtleData.n/2
		amountLong = setting.GridAmount
		// 加仓一个单位
		if tick.Bids[0].Price < priceShort && math.Abs(currentN) < float64(model.AppConfig.TurtleLimitMain) {
			setting.Chance = setting.Chance - 1
			setting.GridAmount = setting.GridAmount + turtleData.amount
			setting.PriceX = priceShort
			updateSetting = true
			util.Notice(fmt.Sprintf(`加空 chance:%f amount:%f price:%f currentN:%f short-long:%f %f px:%f n:%f`,
				setting.Chance, setting.GridAmount, setting.PriceX, currentN, priceShort, priceLong,
				setting.PriceX, turtleData.n))
			if api.IsValid(turtleData.orderLong) {
				lastOrder := api.QueryOrderById(key, secret, market, symbol, turtleData.orderLong.OrderId)
				if api.IsValid(lastOrder) {
					setting.PriceX = lastOrder.DealPrice
					priceLong = math.Min(turtleData.highDays10, setting.PriceX+2*turtleData.n)
					priceShort = setting.PriceX - turtleData.n/2
				}
			}
		} // 平空
		if tick.Asks[0].Price > priceLong {
			setting.Chance = 0
			setting.GridAmount = 0
			setting.PriceX = priceLong
			updateSetting = true
			util.Notice(fmt.Sprintf(`平空 chance:%f amount:%f price:%f currentN:%f short-long:%f %f px:%f n:%f`,
				setting.Chance, setting.GridAmount, setting.PriceX, currentN, priceShort, priceLong,
				setting.PriceX, turtleData.n))
			if api.IsValid(turtleData.orderShort) {
				lastOrder := api.QueryOrderById(key, secret, market, symbol, turtleData.orderShort.OrderId)
				if api.IsValid(lastOrder) {
					setting.PriceX = lastOrder.DealPrice
					priceLong = math.Min(turtleData.highDays10, setting.PriceX+2*turtleData.n)
					priceShort = setting.PriceX - turtleData.n/2
				}
			}
		}
	}
	if updateSetting {
		if turtleData.orderLong != nil && turtleData.orderLong.OrderId != `` {
			api.MustCancel(key, secret, market, symbol, turtleData.orderLong.OrderId, true)
		}
		if turtleData.orderShort != nil && turtleData.orderShort.OrderId != `` {
			api.MustCancel(key, secret, market, symbol, turtleData.orderShort.OrderId, true)
		}
		turtleData.orderLong = nil
		turtleData.orderShort = nil
		setting.UpdatedAt = util.GetNow()
		model.SetSetting(model.FunctionTurtle, market, symbol, setting)
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			market, symbol, model.FunctionTurtle).Updates(map[string]interface{}{
			`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
		return
	}
	if turtleData.orderLong == nil && currentN < float64(model.AppConfig.TurtleLimitMain) {
		util.Notice(fmt.Sprintf(`place stop long chance:%f amount:%f price:%f currentN-limit:%f %d`,
			setting.Chance, setting.GridAmount, setting.PriceX, currentN, model.AppConfig.TurtleLimitMain))
		order := api.PlaceOrder(key, secret, model.OrderSideBuy, model.OrderTypeStop, market, symbol,
			``, setting.AccountType, ``, priceLong, amountLong, false)
		if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
			order.RefreshType = model.FunctionTurtle
			model.AppDB.Save(&order)
			turtleData.orderLong = order
		}
	}
	if turtleData.orderShort == nil && currentN > -1*float64(model.AppConfig.TurtleLimitMain) {
		util.Notice(fmt.Sprintf(`place stop short chance:%f amount:%f price:%f currentN-limit:%f %d`,
			setting.Chance, setting.GridAmount, setting.PriceX, currentN, model.AppConfig.TurtleLimitMain))
		order := api.PlaceOrder(key, secret, model.OrderSideSell, model.OrderTypeStop, market, symbol,
			``, setting.AccountType, ``, priceShort, amountShort, false)
		if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
			order.RefreshType = model.FunctionTurtle
			model.AppDB.Save(&order)
			turtleData.orderShort = order
		}
	}
}
