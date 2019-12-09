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
}

var turtling = false

func setTurtling(value bool) {
	turtling = value
}

var dataSet = make(map[string]map[string]map[string]*TurtleData) // market - symbol - 2019-12-06 - *turtleData

func GetTurtleData(market, symbol string) (turtleData *TurtleData) {
	today := util.GetNow()
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
	dataSet[market][symbol][todayStr] = turtleData
	return
}

//setting.GridAmount 当前已经持仓数量
//setting.Chance 当前开仓的个数
//setting.PriceX 上一次开仓的价格
var ProcessTurtle = func(market, symbol string) {
	result, tick := model.AppMarkets.GetBidAsk(symbol, market)
	if !result || tick == nil || tick.Asks == nil || tick.Bids == nil || model.AppConfig.Handle != `1` ||
		model.AppPause || util.GetNowUnixMillion()-int64(tick.Ts) > 500 {
		util.Notice(fmt.Sprintf(`[tick not good]%s %s`, market, symbol))
		return
	}
	setting := model.GetSetting(model.FunctionTurtle, market, symbol)
	if setting == nil || turtling {
		return
	}
	setTurtling(true)
	defer setTurtling(false)
	turtleData := GetTurtleData(market, symbol)

	currentN := model.GetCurrentN(model.FunctionTurtle)
	key := fmt.Sprintf("%s_%s_%s", model.FunctionTurtle, market, symbol)
	model.CarryInfo[key] = fmt.Sprintf("[海龟参数]%s %s 加仓次数限制:%d 当前已经持仓数量:%f 上一次开仓的价格:%f\n"+
		"20日最高:%f 20日最低:%f 10日最高:%f 10日最低:%f n:%f 数量:%f 当前开仓个数:%f",
		turtleData.turtleTime.String()[0:10], key, model.AppConfig.TurtleLimitMain, setting.GridAmount, setting.PriceX,
		turtleData.highDays20, turtleData.lowDays20, turtleData.highDays10, turtleData.lowDays10, turtleData.n,
		turtleData.amount, currentN)
	var order *model.Order
	if currentN == 0 { // 开初始多仓
		if tick.Asks[0].Price > turtleData.highDays20 {
			util.Notice(fmt.Sprintf(`开初始多仓 当前价格%f>20日最高%f %s`,
				tick.Asks[0].Price, turtleData.highDays20, model.CarryInfo[key]))
			order = api.PlaceOrder(key, secret, model.OrderSideBuy, model.OrderTypeMarket, market, symbol,
				``, setting.AccountType, ``, tick.Asks[0].Price, turtleData.amount, false)
			if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
				setting.Chance = 1
				setting.GridAmount = turtleData.amount
			}
		}
		if tick.Bids[0].Price < turtleData.lowDays20 {
			util.Notice(fmt.Sprintf(`开初始空仓 当前价格%f<20日最低%f %s`,
				tick.Bids[0].Price, turtleData.lowDays20, model.CarryInfo[key]))
			order = api.PlaceOrder(key, secret, model.OrderSideSell, model.OrderTypeMarket, market, symbol,
				``, setting.AccountType, ``, tick.Bids[0].Price, turtleData.amount, false)
			if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
				setting.Chance = -1
				setting.GridAmount = turtleData.amount
			}
		}
	} else if currentN > 0 {
		// 加仓一个单位
		if tick.Asks[0].Price > setting.PriceX+turtleData.n/2 && currentN < float64(model.AppConfig.TurtleLimitMain) {
			util.Notice(fmt.Sprintf(`加多仓 当前价格%f>%f+0.5*%f %s`,
				tick.Asks[0].Price, setting.PriceX, turtleData.n, model.CarryInfo[key]))
			order = api.PlaceOrder(key, secret, model.OrderSideBuy, model.OrderTypeMarket, market, symbol,
				``, setting.AccountType, ``, tick.Asks[0].Price, turtleData.amount, false)
			if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
				setting.Chance = setting.Chance + 1
				setting.GridAmount = setting.GridAmount + turtleData.amount
			}
		} // 平多
		if tick.Bids[0].Price < turtleData.lowDays10 || tick.Bids[0].Price < setting.PriceX-2*turtleData.n {
			util.Notice(fmt.Sprintf(`平多 当前价格%f<10日最低%f || < %f-2*%f %s`,
				tick.Bids[0].Price, turtleData.lowDays10, setting.PriceX, turtleData.n, model.CarryInfo[key]))
			order = api.PlaceOrder(key, secret, model.OrderSideSell, model.OrderTypeMarket, market, symbol,
				``, setting.AccountType, ``, tick.Bids[0].Price, setting.GridAmount, false)
			if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
				setting.Chance = 0
				setting.GridAmount = 0
			}
		}
	} else if currentN < 0 {
		// 加仓一个单位
		if tick.Bids[0].Price < setting.PriceX-turtleData.n/2 &&
			math.Abs(currentN) < float64(model.AppConfig.TurtleLimitMain) {
			util.Notice(fmt.Sprintf(`加空仓 当前价格%f<%f-0.5*%f %s`,
				tick.Bids[0].Price, setting.PriceX, turtleData.n, model.CarryInfo[key]))
			order = api.PlaceOrder(key, secret, model.OrderSideSell, model.OrderTypeMarket, market, symbol,
				``, setting.AccountType, ``, tick.Bids[0].Price, turtleData.amount, false)
			if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
				setting.Chance = setting.Chance - 1
				setting.GridAmount = setting.GridAmount + turtleData.amount
			}
		} // 平空
		if tick.Asks[0].Price > turtleData.highDays10 || tick.Asks[0].Price > setting.PriceX+2*turtleData.n {
			util.Notice(fmt.Sprintf(`平空 当前价格%f>10日最高%f || > %f-2*%f %s`,
				tick.Asks[0].Price, turtleData.highDays10, setting.PriceX, turtleData.n, model.CarryInfo[key]))
			order = api.PlaceOrder(key, secret, model.OrderSideBuy, model.OrderTypeMarket, market, symbol,
				``, setting.AccountType, ``, tick.Asks[0].Price, setting.GridAmount, false)
			if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
				setting.Chance = 0
				setting.GridAmount = 0
			}
		}
	}
	if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
		setting.PriceX = order.DealPrice
		model.SetSetting(model.FunctionTurtle, market, symbol, setting)
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			market, symbol, model.FunctionTurtle).Updates(map[string]interface{}{
			`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
		order.RefreshType = model.FunctionTurtle
		model.AppDB.Save(&order)
	}
	model.SetSetting(model.FunctionTurtle, market, symbol, setting)
}
