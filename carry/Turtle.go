package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strings"
	"time"
)

type TurtleData struct {
	turtleTime time.Time
	highDays10 float64
	lowDays10  float64
	highDays20 float64
	lowDays20  float64
	highDays5  float64
	lowDays5   float64
	end1       float64
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
	switch market {
	case model.Bitmex:
		p := api.GetBtcBalance(``, ``, market)
		switch symbol {
		case `btcusd_p`:
			amount = 0.02 * p / n * price * price
		case `ethusd_p`:
			amount = 15000 * p / n
		}
	case model.Ftx:
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
	case model.OKFUTURE:
		api.RefreshAccount(``, ``, model.OKFUTURE)
		account := model.AppAccounts.GetAccount(model.OKFUTURE, symbol)
		if account != nil {
			p := account.Free * price
			if strings.Contains(strings.ToLower(symbol), `btc`) {
				amount = 0.01 * p / n / model.OKEXBTCContractFaceValue
			} else {
				amount = 0.01 * p / n / model.OKEXOtherContractFaceValue
			}
		}
	}
	return amount
}

func GetTurtleData(setting *model.Setting) (turtleData *TurtleData) {
	today := time.Now().In(time.UTC)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	duration, _ := time.ParseDuration(`-24h`)
	yesterday := today.Add(duration)
	todayStr := today.String()[0:10]
	yesterdayStr := yesterday.String()[0:10]
	if dataSet[setting.Market] == nil {
		dataSet[setting.Market] = make(map[string]map[string]*TurtleData)
	}
	if dataSet[setting.Market][setting.Symbol] == nil {
		dataSet[setting.Market][setting.Symbol] = make(map[string]*TurtleData)
	}
	if dataSet[setting.Market][setting.Symbol][todayStr] != nil {
		return dataSet[setting.Market][setting.Symbol][todayStr]
	}
	turtleYesterday := dataSet[setting.Market][setting.Symbol][yesterdayStr]
	util.Notice(`need to create turtle ` + setting.Market + setting.Symbol)
	turtleData = &TurtleData{turtleTime: today}
	var orderLong, orderShort model.Order
	model.AppDB.Model(&orderLong).Where(
		"market= ? and symbol= ? and refresh_type= ? and amount>deal_amount and status=? and order_side=?",
		setting.Market, setting.Symbol, model.FunctionTurtle, model.CarryStatusWorking, model.OrderSideBuy).Last(&orderLong)
	model.AppDB.Model(&orderShort).Where(
		"market= ? and symbol= ? and refresh_type= ? and amount>deal_amount and status=? and order_side=?",
		setting.Market, setting.Symbol, model.FunctionTurtle, model.CarryStatusWorking, model.OrderSideSell).Last(&orderShort)
	util.Notice(fmt.Sprintf(`load orders from db %s %s long: %s short: %s and to cancel`,
		setting.Market, setting.Symbol, orderLong.OrderId, orderShort.OrderId))
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
	instrument := api.GetCurrentInstrument(setting.Market, setting.Symbol)
	cross := false
	if (orderShort.Instrument != `` && instrument != orderShort.Instrument) || (orderLong.Instrument != `` && instrument != orderLong.Instrument) {
		cross = true
	}
	if orderLong.OrderId != `` {
		if !cross || setting.Chance < 0 {
			api.MustCancel(model.KeyDefault, model.SecretDefault, setting.Market, setting.Symbol, orderLong.Instrument,
				orderLong.OrderType, orderLong.OrderId, true)
		} else {
			util.Notice(fmt.Sprintf(`%s %s keep quarter long when chance %f`,
				setting.Market, setting.Symbol, setting.Chance))
		}
	}
	if orderShort.OrderId != `` {
		if !cross || setting.Chance > 0 {
			api.MustCancel(model.KeyDefault, model.SecretDefault, setting.Market, setting.Symbol, orderShort.Instrument,
				orderShort.OrderType, orderShort.OrderId, true)
		} else {
			util.Notice(fmt.Sprintf(`%s %s keep quarter short when chance %f`,
				setting.Market, setting.Symbol, setting.Chance))
		}
	}
	if cross {
		setting.Chance = 0
		channel := model.AppMarkets.GetDepthChan(setting.Market, 0)
		if channel == nil {
			ResetChannel(setting.Market, channel)
		}
		model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
			setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{`chance`: 0})
		util.Notice(fmt.Sprintf(`%s need to go cross %s from %s_%s to %s set chance 0`,
			setting.Market, setting.Symbol, orderLong.Instrument, orderShort.Instrument, instrument))
	}
	for i := 1; i < 21; i++ {
		duration, _ := time.ParseDuration(fmt.Sprintf(`%dh`, -24*i))
		day := today.Add(duration)
		candle := api.GetDayCandle(model.KeyDefault, model.SecretDefault, setting.Market, setting.Symbol, instrument, day)
		if candle == nil {
			continue
		}
		if i == 1 {
			turtleData.end1 = candle.PriceClose
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
		if candle.PriceHigh > turtleData.highDays5 && i < 6 {
			turtleData.highDays5 = candle.PriceHigh
		}
		if (turtleData.lowDays5 == 0 || turtleData.lowDays5 > candle.PriceLow) && i < 6 {
			turtleData.lowDays5 = candle.PriceLow
		}
		if i == 1 {
			turtleData.n = candle.N
			turtleData.amount = calcTurtleAmount(setting.Market, setting.Symbol, candle.PriceOpen, turtleData.n)
		}
	}
	if turtleData.amount > 0 && turtleData.n > 0 {
		dataSet[setting.Market][setting.Symbol][todayStr] = turtleData
		util.Notice(fmt.Sprintf(`%s %s set turtle data: amount:%f n:%f end1:%f 20:%f %f 10:%f %f 5:%f %f`,
			setting.Market, setting.Symbol, turtleData.amount, turtleData.n, turtleData.end1, turtleData.lowDays20,
			turtleData.highDays20, turtleData.lowDays10, turtleData.highDays10, turtleData.lowDays5, turtleData.highDays5))
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
	turtleData := GetTurtleData(setting)
	if turtleData == nil || turtleData.n == 0 || turtleData.amount == 0 {
		return
	}
	currentN := model.GetCurrentN(setting)
	showMsg := fmt.Sprintf("%s_%s_%s", model.FunctionTurtle, setting.Market, setting.Symbol)
	model.SetCarryInfo(showMsg, fmt.Sprintf("[海龟参数]%s %s 加仓次数限制:%f 当前已经持仓数量:%f 上一次开仓的价格:%f\n"+
		"20日最高:%f 20日最低:%f 10日最高:%f 10日最低:%f n:%f 数量:%f %s持仓数:%f 总持仓数%f",
		turtleData.turtleTime.String()[0:10], showMsg, setting.AmountLimit, setting.GridAmount, setting.PriceX,
		turtleData.highDays20, turtleData.lowDays20, turtleData.highDays10, turtleData.lowDays10, turtleData.n,
		turtleData.amount, setting.Symbol, setting.Chance, currentN))
	priceLong := turtleData.highDays20
	priceShort := turtleData.lowDays20
	amount := turtleData.amount
	if setting.Chance == 0 { // 开初始仓
		priceShort, priceLong = placeTurtleOrders(setting.Market, setting.Symbol, turtleData, setting, currentN,
			priceShort, priceLong, amount, amount, tick)
		if tick.Asks[0].Price >= priceLong {
			if handleBreak(setting, turtleData, model.OrderSideBuy) {
				setting.Chance = 1
				setting.GridAmount = amount
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(
					`破20日高点 %s %s chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
					setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, currentN, priceShort,
					priceLong, setting.PriceX, turtleData.n))
			}
		}
		if tick.Bids[0].Price <= priceShort {
			if handleBreak(setting, turtleData, model.OrderSideSell) {
				setting.Chance = -1
				setting.GridAmount = amount
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(
					`破20日低点 %s %s chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
					setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, currentN, priceShort,
					priceLong, setting.PriceX, turtleData.n))
			}
		}
	} else if setting.Chance > 0 {
		priceLong = math.Max(turtleData.highDays20, setting.PriceX+turtleData.n/2)
		priceShort = math.Max(turtleData.lowDays10, setting.PriceX-2*turtleData.n)
		priceShort, priceLong = placeTurtleOrders(setting.Market, setting.Symbol, turtleData, setting, currentN,
			priceShort, priceLong, setting.GridAmount, amount, tick)
		// 加仓一个单位
		if tick.Asks[0].Price >= priceLong {
			if handleBreak(setting, turtleData, model.OrderSideBuy) {
				setting.Chance = setting.Chance + 1
				setting.GridAmount = setting.GridAmount + amount
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(`加多 %s %s chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
					setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, currentN, priceShort, priceLong,
					setting.PriceX, turtleData.n))
			}
		} // 平多
		if tick.Bids[0].Price <= priceShort {
			if handleBreak(setting, turtleData, model.OrderSideSell) {
				setting.Chance = 0
				setting.GridAmount = 0
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(`平多 %s %s chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
					setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, currentN, priceShort, priceLong,
					setting.PriceX, turtleData.n))
			}
		}
	} else if setting.Chance < 0 {
		priceLong = math.Min(turtleData.highDays10, setting.PriceX+2*turtleData.n)
		priceShort = math.Min(turtleData.lowDays20, setting.PriceX-turtleData.n/2)
		priceShort, priceLong = placeTurtleOrders(setting.Market, setting.Symbol, turtleData, setting, currentN,
			priceShort, priceLong, amount, setting.GridAmount, tick)
		// 加仓一个单位
		if tick.Bids[0].Price <= priceShort {
			if handleBreak(setting, turtleData, model.OrderSideSell) {
				setting.Chance = setting.Chance - 1
				setting.GridAmount = setting.GridAmount + amount
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(`加空 %s %s chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
					setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, currentN, priceShort, priceLong,
					setting.PriceX, turtleData.n))
			}
		} // 平空
		if tick.Asks[0].Price >= priceLong {
			if handleBreak(setting, turtleData, model.OrderSideBuy) {
				setting.Chance = 0
				setting.GridAmount = 0
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(`平空 %s %s chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
					setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, currentN, priceShort, priceLong,
					setting.PriceX, turtleData.n))
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
			setting.Market, setting.Symbol, orderQuery.Instrument, orderQuery.OrderType, orderQuery.OrderId)
		if order != nil && order.DealPrice > 0 && order.Status == model.CarryStatusSuccess {
			isBreak = true
			setting.PriceX = order.DealPrice
			turtleData.orderLong = nil
			turtleData.orderShort = nil
			//turtleData.amount = calcTurtleAmount(setting.Market, setting.Symbol, orderQuery.Price, turtleData.n)
			time.Sleep(time.Second * 3)
			break
		} else {
			util.Notice(`not yet break, approaching`)
			return false
		}
	}
	for orderCancel != nil && isBreak {
		canceled, _ := api.MustCancel(``, ``, setting.Market, setting.Symbol, orderCancel.Instrument,
			orderCancel.OrderType, orderCancel.OrderId, true)
		if canceled {
			break
		}
	}
	return isBreak
}

func placeTurtleOrders(market, symbol string, turtleData *TurtleData, setting *model.Setting,
	currentN, priceShort, priceLong, amountShort, amountLong float64, tick *model.BidAsk) (short, long float64) {
	if setting.Chance > 0 && turtleData.end1/turtleData.highDays20 < 0.87 {
		priceShort = math.Max(turtleData.lowDays5, setting.PriceX-2*turtleData.n)
		util.Notice(fmt.Sprintf(`提前止盈 chance:%f, end1:%f h20:%f`,
			setting.Chance, turtleData.end1, turtleData.highDays20))
	}
	if setting.Chance < 0 && turtleData.end1/turtleData.lowDays20 > 1.13 {
		priceLong = math.Min(turtleData.highDays5, setting.PriceX+2*turtleData.n)
		util.Notice(fmt.Sprintf(`提前止盈 chance: %f, end1:%f l20:%f`,
			setting.Chance, turtleData.end1, turtleData.lowDays20))
	}
	instrument := api.GetCurrentInstrument(market, symbol)
	long = priceLong
	short = priceShort
	if turtleData.orderLong == nil && currentN < setting.AmountLimit {
		orderSide := model.OrderSideBuy
		typeLong := model.OrderTypeStop
		if setting.Chance < 0 && setting.Market == model.OKFUTURE {
			orderSide = model.OrderSideLiquidateShort
		}
		if priceLong < tick.Asks[0].Price {
			util.Notice(fmt.Sprintf(`fatal issue: (stop long price)%f < %f(market price)`,
				priceLong, tick.Asks[0].Price))
			typeLong = model.OrderTypeMarket
			long = tick.Asks[0].Price
		}
		util.Notice(fmt.Sprintf(`%s %s place stop long chance:%f amount:%f price:%f currentN-limit:%f %f 
			orderSide:%s end1:%f h20:%f h10:%f h5:%f l20:%f l10:%f l5%f`,
			setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, setting.PriceX, currentN,
			setting.AmountLimit, orderSide, turtleData.end1, turtleData.highDays20, turtleData.highDays10,
			turtleData.highDays5, turtleData.lowDays20, turtleData.lowDays10, turtleData.lowDays5))
		order := api.PlaceOrder(model.KeyDefault, model.SecretDefault, orderSide, typeLong, market, symbol, instrument,
			``, setting.AccountType, ``, model.FunctionTurtle, priceLong, amountLong, true)
		if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
			turtleData.orderLong = order
			if order.Price > 0 {
				long = order.Price
			}
		}
	} else if turtleData.orderLong != nil && currentN >= setting.AmountLimit {
		api.MustCancel(model.KeyDefault, model.SecretDefault, market, symbol, turtleData.orderLong.Instrument,
			turtleData.orderLong.OrderType, turtleData.orderLong.OrderId, true)
	}
	if turtleData.orderShort == nil && currentN > -1*setting.AmountLimit {
		orderSide := model.OrderSideSell
		typeShort := model.OrderTypeStop
		if setting.Chance > 0 && setting.Market == model.OKFUTURE {
			orderSide = model.OrderSideLiquidateLong
		}
		if priceShort > tick.Bids[0].Price {
			util.Notice(fmt.Sprintf(`fatal issue: (stop short price)%f > %f(market price)`,
				priceShort, tick.Bids[0].Price))
			typeShort = model.OrderTypeMarket
			short = tick.Bids[0].Price
		}
		util.Notice(fmt.Sprintf(`%s %s place stop short chance:%f amount:%f price:%f currentN-limit:%f %f 
			orderSide:%s end1:%f h20:%f h10:%f h5:%f l20:%f l10:%f l5%f`,
			setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, setting.PriceX, currentN,
			setting.AmountLimit, orderSide, turtleData.end1, turtleData.highDays20, turtleData.highDays10,
			turtleData.highDays5, turtleData.lowDays20, turtleData.lowDays10, turtleData.lowDays5))
		order := api.PlaceOrder(model.KeyDefault, model.SecretDefault, orderSide, typeShort, market, symbol, instrument,
			``, setting.AccountType, ``, model.FunctionTurtle, priceShort,
			amountShort, true)
		if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
			turtleData.orderShort = order
			if order.Price > 0 {
				short = order.Price
			}
		}
	} else if turtleData.orderShort != nil && currentN <= -1*setting.AmountLimit {
		api.MustCancel(model.KeyDefault, model.SecretDefault, market, symbol, turtleData.orderShort.Instrument,
			turtleData.orderShort.OrderType, turtleData.orderShort.OrderId, true)
	}
	return
}
