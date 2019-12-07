package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"time"
)

type TurtleData struct {
	initTime   time.Time
	highDays10 float64
	lowDays10  float64
	highDays20 float64
	lowDays20  float64
	n          float64
	amount     float64
}

var dataSet = make(map[string]map[string]map[string]*TurtleData) // market - symbol - 2019-12-06 - *turtleData

func getTurtleData(market, symbol string) (turtleData *TurtleData) {
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
	turtleData = &TurtleData{}
	for i := 1; i <= 20; i++ {
		duration, _ := time.ParseDuration(fmt.Sprintf(`%dh`, -24*i))
		day := today.Add(duration)
		candle := model.GetCandle(market, symbol, `1d`, day)
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
				turtleData.amount = 0.01 * p / turtleData.n * candle.PriceOpen * candle.PriceOpen
			case `ethusd_p`:
				turtleData.amount = 10000 * p / turtleData.n
			}
		}
	}
	return
}

//setting.AmountLimit 同一方向上最多加仓的N的倍数
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
	turtleData := getTurtleData(market, symbol)
	setting := model.GetSetting(model.FunctionTurtle, market, symbol)
	if setting == nil {
		return
	}
	fmt.Println(turtleData.amount)

	model.SetSetting(model.FunctionTurtle, market, symbol, setting)
}
