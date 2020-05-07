package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strconv"
	"strings"
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

func calcTurtleAmount(market, symbol string, price, n float64) (amount float64) {
	if market == model.Bitmex {
		p := api.GetBtcBalance(``, ``, market)
		switch symbol {
		case `btcusd_p`:
			amount = 0.02 * p / n * price * price
		case `ethusd_p`:
			amount = 15000 * p / n
		}
	} else if market == model.Ftx {
		p := api.GetUSDBalance(``, ``, market)
		amount = 0.01 * p / n
		switch symbol {
		case `btcusd_p`:
			amount *= 2
		case `ethusd_p`, `eosusd_p`:
			amount *= 1.5
		case `htusd_p`, `okbusd_p`, `bnbusd_p`, `btmxusd_p`:
			amount *= 0.5
		}
	}
	return amount
}

func GetTurtleData(market, symbol string) (turtleData *TurtleData) {
	today := time.Now().In(time.UTC)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	duration, _ := time.ParseDuration(`-24h`)
	yesterday := today.Add(duration)
	todayStr := today.String()[0:10]
	yesterdayStr := yesterday.String()[0:10]
	if dataSet[market] == nil {
		dataSet[market] = make(map[string]map[string]*TurtleData)
	}
	if dataSet[market][symbol] == nil {
		dataSet[market][symbol] = make(map[string]*TurtleData)
	}
	if dataSet[market][symbol][todayStr] != nil {
		return dataSet[market][symbol][todayStr]
	}
	turtleYesterday := dataSet[market][symbol][yesterdayStr]
	util.Notice(`need to create turtle ` + market + symbol)
	turtleData = &TurtleData{turtleTime: today}
	var orderLong, orderShort model.Order
	model.AppDB.Model(&orderLong).Where(
		"market= ? and symbol= ? and refresh_type= ? and amount>deal_amount and status=? and order_side=?",
		market, symbol, model.FunctionTurtle, model.CarryStatusWorking, model.OrderSideBuy).Last(&orderLong)
	model.AppDB.Model(&orderShort).Where(
		"market= ? and symbol= ? and refresh_type= ? and amount>deal_amount and status=? and order_side=?",
		market, symbol, model.FunctionTurtle, model.CarryStatusWorking, model.OrderSideSell).Last(&orderShort)
	util.Notice(fmt.Sprintf(`load orders from db %s %s long: %s short: %s and to cancel`,
		market, symbol, orderLong.OrderId, orderShort.OrderId))
	if turtleYesterday != nil {
		if orderLong.OrderId == `` && turtleYesterday.orderLong != nil {
			orderLong = *turtleYesterday.orderLong
			util.Notice(fmt.Sprintf(`set today order long from yesterday %s`, orderLong.OrderId))
		}
		if orderShort.OrderId == `` && turtleYesterday.orderShort != nil {
			orderShort = *turtleYesterday.orderShort
			util.Notice(fmt.Sprintf(`set today order short from yesterday %s`, orderShort.OrderId))
		}
	}
	if orderLong.OrderId != `` {
		api.MustCancel(model.KeyDefault, model.SecretDefault, market, symbol, orderLong.OrderType, orderLong.OrderId,
			true)
	}
	if orderShort.OrderId != `` {
		api.MustCancel(model.KeyDefault, model.SecretDefault, market, symbol, orderShort.OrderType, orderShort.OrderId,
			true)
	}
	for i := 1; i < 19; i++ {
		duration, _ := time.ParseDuration(fmt.Sprintf(`%dh`, -24*i))
		day := today.Add(duration)
		candle := api.GetDayCandle(model.KeyDefault, model.SecretDefault, market, symbol, day)
		if candle == nil {
			continue
		}
		if candle.PriceHigh > turtleData.highDays20 {
			turtleData.highDays20 = candle.PriceHigh
		}
		if turtleData.lowDays20 == 0 || turtleData.lowDays20 > candle.PriceLow {
			turtleData.lowDays20 = candle.PriceLow
		}
		if candle.PriceHigh > turtleData.highDays10 && i < 10 {
			turtleData.highDays10 = candle.PriceHigh
		}
		if (turtleData.lowDays10 == 0 || turtleData.lowDays10 > candle.PriceLow) && i < 10 {
			turtleData.lowDays10 = candle.PriceLow
		}
		if i == 1 {
			turtleData.n = candle.N
			turtleData.amount = calcTurtleAmount(market, symbol, candle.PriceOpen, turtleData.n)
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
var ProcessTurtle = func(setting *model.Setting) {
	result, tick := model.AppMarkets.GetBidAsk(setting.Symbol, setting.Market)
	now := util.GetNowUnixMillion()
	if !result || tick == nil || tick.Asks == nil || tick.Bids == nil || model.AppConfig.Handle != `1` ||
		model.AppPause || now-int64(tick.Ts) > 1000 {
		//if tick != nil {
		//util.Info(fmt.Sprintf(`[tick not good]%s %s %d`, market, symbol, now-int64(tick.Ts)))
		//}
		return
	}
	if setting == nil || turtling {
		return
	}
	if setting.Chance != 0 && setting.PriceX == 0 {
		showMsg := fmt.Sprintf("%s_%s_%s", model.FunctionTurtle, setting.Market, setting.Symbol)
		model.SetCarryInfo(showMsg, fmt.Sprintf("[海龟参数]%s %s 缺少上次成交价 chance：%f priceX:%f\n",
			setting.Market, setting.Symbol, setting.Chance, setting.PriceX))
		return
	}
	setTurtling(true)
	defer setTurtling(false)
	turtleData := GetTurtleData(setting.Market, setting.Symbol)
	if turtleData == nil || turtleData.n == 0 || turtleData.amount == 0 {
		return
	}
	if moveQuarter(setting) {
		return
	}
	currentN := model.GetCurrentN(setting)
	showMsg := fmt.Sprintf("%s_%s_%s", model.FunctionTurtle, setting.Market, setting.Symbol)
	model.SetCarryInfo(showMsg, fmt.Sprintf("[海龟参数]%s %s 加仓次数限制:%f 当前已经持仓数量:%f 上一次开仓的价格:%f\n"+
		"20日最高:%f 20日最低:%f 10日最高:%f 10日最低:%f n:%f 数量:%f %s持仓数:%f 总持仓数%f",
		turtleData.turtleTime.String()[0:10], showMsg, setting.AmountLimit, setting.GridAmount, setting.PriceX,
		turtleData.highDays20, turtleData.lowDays20, turtleData.highDays10, turtleData.lowDays10, turtleData.n,
		turtleData.amount, setting.Symbol, setting.Chance, currentN))
	priceLong := 0.0
	priceShort := 0.0
	amount := turtleData.amount
	if setting.Chance == 0 { // 开初始仓
		priceLong = turtleData.highDays20
		priceShort = turtleData.lowDays20
		placeTurtleOrders(setting.Market, setting.Symbol, turtleData, setting, currentN,
			priceShort, priceLong, amount, amount)
		if tick.Asks[0].Price >= priceLong {
			if handleBreak(setting, turtleData, model.OrderSideBuy) {
				setting.Chance = 1
				setting.GridAmount = amount
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(`破20日高点 chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
					setting.Chance, setting.GridAmount, currentN, priceShort, priceLong, setting.PriceX, turtleData.n))
			}
		}
		if tick.Bids[0].Price <= priceShort {
			if handleBreak(setting, turtleData, model.OrderSideSell) {
				setting.Chance = -1
				setting.GridAmount = amount
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(`破20日低点 chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
					setting.Chance, setting.GridAmount, currentN, priceShort, priceLong, setting.PriceX, turtleData.n))
			}
		}
	} else if setting.Chance > 0 {
		priceLong = setting.PriceX + turtleData.n/2
		priceShort = math.Max(turtleData.lowDays10, setting.PriceX-2*turtleData.n)
		placeTurtleOrders(setting.Market, setting.Symbol, turtleData, setting, currentN,
			priceShort, priceLong, setting.GridAmount, amount)
		// 加仓一个单位
		if tick.Asks[0].Price >= priceLong {
			if handleBreak(setting, turtleData, model.OrderSideBuy) {
				setting.Chance = setting.Chance + 1
				setting.GridAmount = setting.GridAmount + amount
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(`加多 chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
					setting.Chance, setting.GridAmount, currentN, priceShort, priceLong, setting.PriceX, turtleData.n))
			}
		} // 平多
		if tick.Bids[0].Price <= priceShort {
			if handleBreak(setting, turtleData, model.OrderSideSell) {
				setting.Chance = 0
				setting.GridAmount = 0
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(`平多 chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
					setting.Chance, setting.GridAmount, currentN, priceShort, priceLong, setting.PriceX, turtleData.n))
			}
		}
	} else if setting.Chance < 0 {
		priceLong = math.Min(turtleData.highDays10, setting.PriceX+2*turtleData.n)
		priceShort = setting.PriceX - turtleData.n/2
		placeTurtleOrders(setting.Market, setting.Symbol, turtleData, setting, currentN,
			priceShort, priceLong, amount, setting.GridAmount)
		// 加仓一个单位
		if tick.Bids[0].Price <= priceShort {
			if handleBreak(setting, turtleData, model.OrderSideSell) {
				setting.Chance = setting.Chance - 1
				setting.GridAmount = setting.GridAmount + amount
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(`加空 chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
					setting.Chance, setting.GridAmount, currentN, priceShort, priceLong, setting.PriceX, turtleData.n))
			}
		} // 平空
		if tick.Asks[0].Price >= priceLong {
			if handleBreak(setting, turtleData, model.OrderSideBuy) {
				setting.Chance = 0
				setting.GridAmount = 0
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(`平空 chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
					setting.Chance, setting.GridAmount, currentN, priceShort, priceLong, setting.PriceX, turtleData.n))
			}
		}
	}
}

func handleBreak(setting *model.Setting, turtleData *TurtleData, orderSide string) (isBreak bool) {
	if turtleData == nil {
		//util.Notice(fmt.Sprintf(`fatal error, nil order to break`))
		return
	}
	orderCancel := turtleData.orderLong
	orderQuery := turtleData.orderShort
	if orderSide == model.OrderSideBuy {
		orderCancel = turtleData.orderShort
		orderQuery = turtleData.orderLong
	}
	for orderQuery != nil {
		util.Notice(fmt.Sprintf(`query turtle break %s %s`, orderSide, orderQuery.OrderId))
		order := api.QueryOrderById(``, ``,
			setting.Market, setting.Symbol, orderQuery.OrderType, orderQuery.OrderId)
		if order != nil && order.DealPrice > 0 && order.Status == model.CarryStatusSuccess {
			isBreak = true
			setting.PriceX = order.DealPrice
			turtleData.orderLong = nil
			turtleData.orderShort = nil
			turtleData.amount = calcTurtleAmount(setting.Market, setting.Symbol, orderQuery.Price, turtleData.n)
			time.Sleep(time.Second * 3)
			break
		} else {
			currentN := model.GetCurrentN(setting)
			if math.Abs(currentN) >= setting.AmountLimit {
				api.MustCancel(``, ``, setting.Market, setting.Symbol, orderQuery.OrderType,
					orderQuery.OrderId, true)
				if turtleData.orderLong != nil && turtleData.orderLong.OrderId == orderQuery.OrderId {
					turtleData.orderLong = nil
				}
				if turtleData.orderShort != nil && turtleData.orderShort.OrderId == orderQuery.OrderId {
					turtleData.orderShort = nil
				}
				break
			} else {
				util.Notice(`not yet break, approaching`)
				return false
			}
		}
	}
	for orderCancel != nil && isBreak {
		canceled, _ := api.MustCancel(``, ``, setting.Market, setting.Symbol, orderCancel.OrderType,
			orderCancel.OrderId, true)
		if canceled {
			break
		}
	}
	return isBreak
}

func placeTurtleOrders(market, symbol string, turtleData *TurtleData, setting *model.Setting,
	currentN, priceShort, priceLong, amountShort, amountLong float64) {
	if turtleData.orderLong == nil && currentN < setting.AmountLimit {
		orderSide := model.OrderSideBuy
		if setting.Chance < 0 && setting.Market == model.OKFUTURE {
			orderSide = model.OrderSideLiquidateShort
		}
		util.Notice(fmt.Sprintf(`place stop long chance:%f amount:%f price:%f currentN-limit:%f %f orderSide:%s`,
			setting.Chance, setting.GridAmount, setting.PriceX, currentN, setting.AmountLimit, orderSide))
		order := api.PlaceOrder(model.KeyDefault, model.SecretDefault, orderSide, model.OrderTypeStop, market,
			symbol, ``, setting.AccountType, ``, model.FunctionTurtle, priceLong, amountLong, true)
		if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
			turtleData.orderLong = order
		}
	}
	if turtleData.orderShort == nil && currentN > -1*setting.AmountLimit {
		orderSide := model.OrderSideSell
		if setting.Chance > 0 && setting.Market == model.OKFUTURE {
			orderSide = model.OrderSideLiquidateLong
		}
		util.Notice(fmt.Sprintf(`place stop short chance:%f amount:%f price:%f currentN-limit:%f %f orderSide:%s`,
			setting.Chance, setting.GridAmount, setting.PriceX, currentN, setting.AmountLimit, orderSide))
		order := api.PlaceOrder(model.KeyDefault, model.SecretDefault, orderSide, model.OrderTypeStop,
			market, symbol, ``, setting.AccountType, ``, model.FunctionTurtle, priceShort,
			amountShort, true)
		if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
			turtleData.orderShort = order
		}
	}
}

func getInstrument(symbol string) (alias string) {
	instrumentQuarter := model.OKFutureSymbols[symbol][`quarter`]
	index := strings.LastIndex(instrumentQuarter, `-`)
	year, _ := strconv.ParseInt(instrumentQuarter[index+1:index+3], 10, 64)
	month, _ := strconv.ParseInt(instrumentQuarter[index+3:index+5], 10, 64)
	day, _ := strconv.ParseInt(instrumentQuarter[index+5:index+7], 10, 64)
	today := time.Now().In(time.UTC)
	duration, _ := time.ParseDuration(`120h`)
	days5 := today.Add(duration)
	dateQuarter := time.Date(int(year), time.Month(month), int(day), 0, 0, 0, 0, today.Location())
	if days5.Before(dateQuarter) {
		return `quarter`
	} else {
		return `bi_quarter`
	}
}

func moveQuarter(setting *model.Setting) (moved bool) {
	alias := getInstrument(setting.Symbol)
	if setting.Market != model.OKFUTURE || model.OKFutureSymbols[setting.Symbol] == nil || alias == `quarter` {
		return false
	}
	instrument := model.OKFutureSymbols[setting.Symbol][alias]
	if setting.MarketRelated != `` && setting.MarketRelated != instrument {
		if setting.GridAmount > 0 {
			orderSide := model.OrderSideLiquidateLong
			if setting.Chance < 0 {
				orderSide = model.OrderSideLiquidateShort
			}
			fmt.Println(orderSide)
			//api.PlaceOrder(model.AppConfig.OkexKey, model.AppConfig.OkexSecret, orderSide, model.)
		}
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
			`chance`: 0, `grid_amount`: 0, `market_related`: instrument})
		util.Notice(fmt.Sprintf(`切换%s->%s chance:%f amount:%f `, setting.MarketRelated, instrument,
			setting.Chance, setting.GridAmount))
		setting.Chance = 0
		setting.GridAmount = 0
		setting.MarketRelated = instrument
		return true
	}
	return false
}
