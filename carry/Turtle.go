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
	longs      []*model.Order
	shorts     []*model.Order
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
			amount = 20000 * p / n
		}
	case model.Ftx:
		p := api.GetUSDBalance(``, ``, market)
		amount = 0.01 * p / n
		switch symbol {
		case `btcusd_p`, `ethusd_p`, `eosusd_p`:
			amount *= 2
		case `htusd_p`, `okbusd_p`, `bnbusd_p`, `btmxusd_p`:
			amount *= 1
		}
	case model.OKFUTURE, model.HuobiDM:
		api.RefreshAccount(``, ``, market)
		account := model.AppAccounts.GetAccount(market, symbol)
		if market == model.HuobiDM {
			currency := symbol
			if strings.Contains(symbol, `_`) {
				currency = symbol[0:strings.Index(symbol, `_`)]
			}
			account = model.AppAccounts.GetAccount(market, currency)
		}
		if account != nil {
			p := account.Free * price
			if strings.Contains(strings.ToLower(symbol), `btc`) {
				amount = 0.02 * p * price / n / model.OKEXBTCContractFaceValue
			} else {
				amount = 0.02 * p * price / n / model.OKEXOtherContractFaceValue
			}
		}
	}
	return amount
}

func GetTurtleData(setting *model.Setting) (turtleData *TurtleData) {
	today, todayStr := model.GetMarketToday(setting.Market)
	if dataSet[setting.Market] == nil {
		dataSet[setting.Market] = make(map[string]map[string]*TurtleData)
	}
	if dataSet[setting.Market][setting.Symbol] == nil {
		dataSet[setting.Market][setting.Symbol] = make(map[string]*TurtleData)
	}
	if dataSet[setting.Market][setting.Symbol][todayStr] != nil {
		return dataSet[setting.Market][setting.Symbol][todayStr]
	}
	util.Notice(`need to create turtle ` + setting.Market + setting.Symbol)
	turtleData = &TurtleData{turtleTime: today}
	var orderLong, orderShort *model.Order
	model.AppDB.Where("market= ? and symbol= ? and refresh_type= ? and amount>deal_amount and status=? and order_side=?",
		setting.Market, setting.Symbol, model.FunctionTurtle, model.CarryStatusWorking, model.OrderSideBuy).
		Order(`order_time desc`).Limit(setting.AmountLimit).Find(&turtleData.longs)
	model.AppDB.Where("market= ? and symbol= ? and refresh_type= ? and amount>deal_amount and status=? and order_side=?",
		setting.Market, setting.Symbol, model.FunctionTurtle, model.CarryStatusWorking, model.OrderSideSell).
		Order(`order_time desc`).Limit(setting.AmountLimit).Find(&turtleData.shorts)
	util.Notice(fmt.Sprintf(`load db turtle orders longs %d shorts %d`,
		len(turtleData.longs), len(turtleData.shorts)))
	for _, order := range turtleData.longs {
		if orderLong == nil || (order != nil && order.OrderId != `` && order.OrderTime.After(orderLong.OrderTime)) {
			orderLong = order
		}
	}
	for _, order := range turtleData.shorts {
		if orderShort == nil || (order != nil && order.OrderId != `` && order.OrderTime.After(orderShort.OrderTime)) {
			orderShort = order
		}
	}
	instrument, isNext := api.GetCurrentInstrument(setting.Market, setting.Symbol)
	cross := false
	if (setting.Market == model.OKFUTURE || setting.Market == model.HuobiDM) && isNext &&
		((orderShort != nil && orderShort.Instrument != `` && instrument != orderShort.Instrument) ||
			(orderLong != nil && orderLong.Instrument != `` && instrument != orderLong.Instrument)) {
		if orderLong != nil {
			util.Notice(fmt.Sprintf(`go cross %s => %s`, orderLong.Instrument, instrument))
		}
		if orderShort != nil {
			util.Notice(fmt.Sprintf(`go cross %s => %s`, orderShort.Instrument, instrument))
		}
		cross = true
	}
	if orderLong != nil && orderLong.OrderId != `` {
		if !cross || setting.Chance >= 0 {
			api.MustCancel(model.KeyDefault, model.SecretDefault, setting.Market, setting.Symbol,
				orderLong.Instrument, orderLong.OrderType, orderLong.OrderId, true)
		} else {
			util.Notice(fmt.Sprintf(`%s %s keep quarter long when chance %d`,
				setting.Market, setting.Symbol, setting.Chance))
		}
	}
	if orderShort != nil && orderShort.OrderId != `` {
		if !cross || setting.Chance <= 0 {
			api.MustCancel(model.KeyDefault, model.SecretDefault, setting.Market, setting.Symbol,
				orderShort.Instrument, orderShort.OrderType, orderShort.OrderId, true)
		} else {
			util.Notice(fmt.Sprintf(`%s %s keep quarter short when chance %d`,
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
		util.Notice(fmt.Sprintf(`%s need to go cross %s to %s set chance 0`,
			setting.Market, setting.Symbol, instrument))
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
		return
	}
	if setting == nil || turtling {
		return
	}
	if setting.Chance != 0 && setting.PriceX == 0 {
		showMsg := fmt.Sprintf("%s_%s_%s", model.FunctionTurtle, setting.Market, setting.Symbol)
		model.SetCarryInfo(showMsg, fmt.Sprintf("[海龟参数]%s %s 缺少上次成交价 chance：%d priceX:%f\n",
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
	model.SetCarryInfo(showMsg, fmt.Sprintf("[海龟参数]%s %s 加仓次数限制:%d 当前已经持仓数量:%f 上一次开仓的价格:%f"+
		"20日:%f-%f 10日:%f-%f n:%f 数量:%f %s持仓数:%d 总持仓数%d",
		turtleData.turtleTime.String()[0:10], showMsg, setting.AmountLimit, setting.GridAmount, setting.PriceX,
		turtleData.lowDays20, turtleData.highDays20, turtleData.lowDays10, turtleData.highDays10, turtleData.n,
		turtleData.amount, setting.Symbol, setting.Chance, currentN))
	priceLong := turtleData.highDays20
	priceShort := turtleData.lowDays20
	amount := turtleData.amount
	if setting.Chance == 0 { // 开初始仓
		priceShort, priceLong = placeTurtleOrders(turtleData, setting, currentN, priceShort, priceLong, amount, amount, tick)
		if tick.Asks[0].Price >= priceLong {
			if handleBreak(setting, turtleData, model.OrderSideBuy, priceLong) {
				setting.Chance = 1
				setting.GridAmount = amount
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(
					`破20日高点 %s %s chance:%d amount:%f currentN:%d short-long:%f %f px:%f n:%f`,
					setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, currentN, priceShort,
					priceLong, setting.PriceX, turtleData.n))
			}
		}
		if tick.Bids[0].Price <= priceShort {
			if handleBreak(setting, turtleData, model.OrderSideSell, priceShort) {
				setting.Chance = -1
				setting.GridAmount = amount
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(
					`破20日低点 %s %s chance:%d amount:%f currentN:%d short-long:%f %f px:%f n:%f`,
					setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, currentN, priceShort,
					priceLong, setting.PriceX, turtleData.n))
			}
		}
	} else if setting.Chance > 0 {
		priceLong = math.Max(turtleData.highDays20, setting.PriceX+turtleData.n/2)
		priceShort = math.Max(turtleData.lowDays10, setting.PriceX-2*turtleData.n)
		priceShort, priceLong = placeTurtleOrders(turtleData, setting, currentN, priceShort, priceLong, setting.GridAmount, amount, tick)
		// 加仓一个单位
		if tick.Asks[0].Price >= priceLong {
			if handleBreak(setting, turtleData, model.OrderSideBuy, priceLong) {
				setting.Chance = setting.Chance + 1
				setting.GridAmount = setting.GridAmount + amount
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(`加多 %s %s chance:%d amount:%f currentN:%d short-long:%f %f px:%f n:%f`,
					setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, currentN, priceShort, priceLong,
					setting.PriceX, turtleData.n))
			}
		} // 平多
		if tick.Bids[0].Price <= priceShort {
			if handleBreak(setting, turtleData, model.OrderSideSell, priceShort) {
				setting.Chance = 0
				setting.GridAmount = 0
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(`平多 %s %s chance:%d amount:%f currentN:%d short-long:%f %f px:%f n:%f`,
					setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, currentN, priceShort, priceLong,
					setting.PriceX, turtleData.n))
			}
		}
	} else if setting.Chance < 0 {
		priceLong = math.Min(turtleData.highDays10, setting.PriceX+2*turtleData.n)
		priceShort = math.Min(turtleData.lowDays20, setting.PriceX-turtleData.n/2)
		priceShort, priceLong = placeTurtleOrders(turtleData, setting, currentN, priceShort, priceLong, amount, setting.GridAmount, tick)
		// 加仓一个单位
		if tick.Bids[0].Price <= priceShort {
			if handleBreak(setting, turtleData, model.OrderSideSell, priceShort) {
				setting.Chance = setting.Chance - 1
				setting.GridAmount = setting.GridAmount + amount
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(`加空 %s %s chance:%d amount:%f currentN:%d short-long:%f %f px:%f n:%f`,
					setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, currentN, priceShort, priceLong,
					setting.PriceX, turtleData.n))
			}
		} // 平空
		if tick.Asks[0].Price >= priceLong {
			if handleBreak(setting, turtleData, model.OrderSideBuy, priceLong) {
				setting.Chance = 0
				setting.GridAmount = 0
				model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
					setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
					`price_x`: setting.PriceX, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
				util.Notice(fmt.Sprintf(`平空 %s %s chance:%d amount:%f currentN:%d short-long:%f %f px:%f n:%f`,
					setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, currentN, priceShort, priceLong,
					setting.PriceX, turtleData.n))
			}
		}
	}
}

func handleBreak(setting *model.Setting, turtleData *TurtleData, orderSide string, price float64) (priceBreak bool) {
	if turtleData == nil {
		//util.Notice(fmt.Sprintf(`fatal error, nil order to break`))
		return false
	}
	orderQuery := turtleData.orderShort
	if orderSide == model.OrderSideBuy {
		orderQuery = turtleData.orderLong
	}
	if orderQuery != nil {
		time.Sleep(time.Second * 3)
		util.Notice(fmt.Sprintf(`query turtle break %s %s`, orderSide, orderQuery.OrderId))
		order := api.QueryOrderById(``, ``,
			setting.Market, setting.Symbol, orderQuery.Instrument, orderQuery.OrderType, orderQuery.OrderId)
		if order != nil && order.DealPrice > 0 && order.Status == model.CarryStatusSuccess {
			setting.PriceX = order.DealPrice
		} else {
			setting.PriceX = price
			return false
		}
		priceBreak = true
		turtleData.orderLong = nil
		turtleData.orderShort = nil
		if orderSide == model.OrderSideBuy {
			for _, short := range turtleData.shorts {
				short = api.QueryOrderById(``, ``, setting.Market, setting.Symbol, short.Instrument,
					short.OrderType, short.OrderId)
				if short.Status == model.CarryStatusWorking {
					api.MustCancel(model.KeyDefault, model.SecretDefault, short.Market, short.Symbol,
						short.Instrument, short.OrderType, short.OrderId, true)
				}
			}
			util.Notice(fmt.Sprintf(`clear %s %s shorts %d`, setting.Market, setting.Symbol, len(turtleData.shorts)))
			turtleData.shorts = []*model.Order{}
		} else {
			for _, long := range turtleData.longs {
				long = api.QueryOrderById(``, ``, setting.Market, setting.Symbol, long.Instrument,
					long.OrderType, long.OrderId)
				if long.Status == model.CarryStatusWorking {
					api.MustCancel(model.KeyDefault, model.SecretDefault, long.Market, long.Symbol,
						long.Instrument, long.OrderType, long.OrderId, true)
				}
			}
			util.Notice(fmt.Sprintf(`clear %s %s longs %d`, setting.Market, setting.Symbol, len(turtleData.longs)))
			turtleData.longs = []*model.Order{}
		}
	}
	return priceBreak
}

func placeTurtleOrders(turtleData *TurtleData, setting *model.Setting,
	currentN int64, priceShort, priceLong, amountShort, amountLong float64, tick *model.BidAsk) (short, long float64) {
	if setting.Chance > 0 && turtleData.end1/turtleData.highDays20 < 0.87 && turtleData.orderShort == nil {
		priceShort = math.Max(turtleData.lowDays5, setting.PriceX-2*turtleData.n)
		//util.Notice(fmt.Sprintf(`提前止盈 chance:%f, end1:%f h20:%f`,
		//	setting.Chance, turtleData.end1, turtleData.highDays20))
	}
	if setting.Chance < 0 && turtleData.end1/turtleData.lowDays20 > 1.13 && turtleData.orderShort == nil {
		priceLong = math.Min(turtleData.highDays5, setting.PriceX+2*turtleData.n)
		//util.Notice(fmt.Sprintf(`提前止盈 chance: %f, end1:%f l20:%f`,
		//	setting.Chance, turtleData.end1, turtleData.lowDays20))
	}
	instrument, _ := api.GetCurrentInstrument(setting.Market, setting.Symbol)
	if turtleData.orderLong == nil && currentN < setting.AmountLimit && setting.Chance < setting.AmountLimit {
		orderSide := model.OrderSideBuy
		typeLong := model.OrderTypeStop
		if setting.Chance < 0 {
			api.RefreshAccount(``, ``, setting.Market)
			account := model.AppAccounts.GetAccount(setting.Market, setting.Symbol)
			if account != nil && account.Holding < 0 {
				amountLong = math.Abs(account.Holding)
			}
			util.Notice(fmt.Sprintf(
				`limit平空 %s %s chance:%d amount:%f currentN:%d short-long:%f %f px:%f n:%f`,
				setting.Market, setting.Symbol, setting.Chance, amountLong, currentN, priceShort,
				priceLong, setting.PriceX, turtleData.n))
			if setting.Market == model.OKFUTURE || setting.Market == model.HuobiDM {
				orderSide = model.OrderSideLiquidateShort
			}
		}
		if priceLong < tick.Asks[0].Price {
			util.Notice(fmt.Sprintf(`fatal issue: (stop long price)%f < %f(market price)`,
				priceLong, tick.Asks[0].Price))
			typeLong = model.OrderTypeLimit
		}
		util.Notice(fmt.Sprintf(`%s %s place stop long chance:%d amount:%f price:%f currentN-limit:%d %d 
			orderSide:%s end1:%f h20:%f h10:%f h5:%f l20:%f l10:%f l5%f`,
			setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, setting.PriceX, currentN,
			setting.AmountLimit, orderSide, turtleData.end1, turtleData.highDays20, turtleData.highDays10,
			turtleData.highDays5, turtleData.lowDays20, turtleData.lowDays10, turtleData.lowDays5))
		order := api.PlaceOrder(model.KeyDefault, model.SecretDefault, orderSide, typeLong, setting.Market,
			setting.Symbol, instrument, ``, setting.AccountType, ``, model.FunctionTurtle,
			priceLong*1.003, priceLong, amountLong, true)
		if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
			turtleData.orderLong = order
			turtleData.longs = append(turtleData.longs, order)
		}
	} else if turtleData.orderLong != nil && (currentN >= setting.AmountLimit || setting.Chance >= setting.AmountLimit) {
		api.MustCancel(model.KeyDefault, model.SecretDefault, setting.Market, setting.Symbol,
			turtleData.orderLong.Instrument, turtleData.orderLong.OrderType, turtleData.orderLong.OrderId, true)
		turtleData.orderLong = nil
	}
	if turtleData.orderShort == nil && currentN > -1*setting.AmountLimit && setting.Chance > -1*setting.AmountLimit {
		orderSide := model.OrderSideSell
		typeShort := model.OrderTypeStop
		if setting.Chance > 0 {
			api.RefreshAccount(``, ``, setting.Market)
			account := model.AppAccounts.GetAccount(setting.Market, setting.Symbol)
			if account != nil && account.Holding > 0 {
				amountShort = math.Abs(account.Holding)
			}
			util.Notice(fmt.Sprintf(
				`limit平多 %s %s chance:%d amount:%f currentN:%d short-long:%f %f px:%f n:%f`,
				setting.Market, setting.Symbol, setting.Chance, amountShort, currentN, priceShort,
				priceLong, setting.PriceX, turtleData.n))
			if setting.Market == model.OKFUTURE || setting.Market == model.HuobiDM {
				orderSide = model.OrderSideLiquidateLong
			}
		}
		if priceShort > tick.Bids[0].Price {
			util.Notice(fmt.Sprintf(`fatal issue: (stop short price)%f > %f(market price)`,
				priceShort, tick.Bids[0].Price))
			typeShort = model.OrderTypeLimit
		}
		util.Notice(fmt.Sprintf(`%s %s place stop short chance:%d amount:%f price:%f currentN-limit:%d %d 
			orderSide:%s end1:%f h20:%f h10:%f h5:%f l20:%f l10:%f l5%f`,
			setting.Market, setting.Symbol, setting.Chance, setting.GridAmount, setting.PriceX, currentN,
			setting.AmountLimit, orderSide, turtleData.end1, turtleData.highDays20, turtleData.highDays10,
			turtleData.highDays5, turtleData.lowDays20, turtleData.lowDays10, turtleData.lowDays5))
		order := api.PlaceOrder(model.KeyDefault, model.SecretDefault, orderSide, typeShort, setting.Market,
			setting.Symbol, instrument, ``, setting.AccountType, ``, model.FunctionTurtle,
			priceShort*0.997, priceShort, amountShort, true)
		if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
			turtleData.orderShort = order
			turtleData.shorts = append(turtleData.shorts, order)
		}
	} else if turtleData.orderShort != nil && (currentN <= -1*setting.AmountLimit || setting.Chance <= -1*setting.AmountLimit) {
		api.MustCancel(model.KeyDefault, model.SecretDefault, setting.Market, setting.Symbol,
			turtleData.orderShort.Instrument, turtleData.orderShort.OrderType, turtleData.orderShort.OrderId, true)
		turtleData.orderShort = nil
	}
	return priceShort, priceLong
}
