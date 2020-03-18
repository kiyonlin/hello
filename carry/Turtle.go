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
		api.MustCancel(model.KeyDefault, model.SecretDefault, market, symbol, orderLong.OrderId, true)
	}
	if orderShort.OrderId != `` {
		api.MustCancel(model.KeyDefault, model.SecretDefault, market, symbol, orderShort.OrderId, true)
	}
	for i := 1; i <= 20; i++ {
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
		if candle.PriceHigh > turtleData.highDays10 && i < 11 {
			turtleData.highDays10 = candle.PriceHigh
		}
		if (turtleData.lowDays10 == 0 || turtleData.lowDays10 > candle.PriceLow) && i < 11 {
			turtleData.lowDays10 = candle.PriceLow
		}
		if i == 1 {
			turtleData.n = candle.N
			if market == model.Bitmex {
				p := api.GetBtcBalance(``, ``, market)
				switch symbol {
				case `btcusd_p`:
					turtleData.amount = 0.02 * p / turtleData.n * candle.PriceOpen * candle.PriceOpen
				case `ethusd_p`:
					turtleData.amount = 15000 * p / turtleData.n
				}
			} else if market == model.Ftx {
				p := api.GetUSDBalance(``, ``, market)
				turtleData.amount = 0.01 * p / turtleData.n
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
	currentN := model.GetCurrentN(model.FunctionTurtle)
	if int(currentN) >= model.GetTurtleLimit(setting.Symbol) && turtleData.orderLong != nil {
		if api.IsValid(turtleData.orderLong) {
			api.MustCancel(model.KeyDefault, model.SecretDefault, setting.Market, setting.Symbol,
				turtleData.orderLong.OrderId, true)
		}
		turtleData.orderLong = nil
		return
	} else if int(currentN) <= -1*model.GetTurtleLimit(setting.Symbol) && turtleData.orderShort != nil {
		if api.IsValid(turtleData.orderShort) {
			api.MustCancel(model.KeyDefault, model.SecretDefault, setting.Market, setting.Symbol,
				turtleData.orderShort.OrderId, true)
		}
		turtleData.orderShort = nil
		return
	}
	showMsg := fmt.Sprintf("%s_%s_%s", model.FunctionTurtle, setting.Market, setting.Symbol)
	model.SetCarryInfo(showMsg, fmt.Sprintf("[海龟参数]%s %s 加仓次数限制:%d 当前已经持仓数量:%f 上一次开仓的价格:%f\n"+
		"20日最高:%f 20日最低:%f 10日最高:%f 10日最低:%f n:%f 数量:%f %s持仓数:%f 总持仓数%f",
		turtleData.turtleTime.String()[0:10], showMsg, model.GetTurtleLimit(setting.Symbol), setting.GridAmount, setting.PriceX,
		turtleData.highDays20, turtleData.lowDays20, turtleData.highDays10, turtleData.lowDays10, turtleData.n,
		turtleData.amount, setting.Symbol, setting.Chance, currentN))
	priceLong := 0.0
	priceShort := 0.0
	amountLong := turtleData.amount
	amountShort := turtleData.amount
	if setting.Chance == 0 { // 开初始仓
		priceLong = turtleData.highDays20
		priceShort = turtleData.lowDays20
		placeTurtleOrders(setting.Market, setting.Symbol, turtleData, setting, currentN,
			priceShort, priceLong, amountShort, amountLong)
		if tick.Asks[0].Price > priceLong {
			setting.Chance = 1
			setting.GridAmount = turtleData.amount
			handleBreak(setting, turtleData, model.OrderSideBuy)
			util.Notice(fmt.Sprintf(`破20日高点 chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
				setting.Chance, setting.GridAmount, currentN, priceShort, priceLong, setting.PriceX, turtleData.n))
		}
		if tick.Bids[0].Price < priceShort {
			setting.Chance = -1
			setting.GridAmount = turtleData.amount
			handleBreak(setting, turtleData, model.OrderSideSell)
			util.Notice(fmt.Sprintf(`破20日低点 chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
				setting.Chance, setting.GridAmount, currentN, priceShort, priceLong, setting.PriceX, turtleData.n))
		}
	} else if setting.Chance > 0 {
		priceLong = setting.PriceX + turtleData.n/2
		priceShort = math.Max(turtleData.lowDays10, setting.PriceX-2*turtleData.n)
		amountShort = setting.GridAmount
		placeTurtleOrders(setting.Market, setting.Symbol, turtleData, setting, currentN,
			priceShort, priceLong, amountShort, amountLong)
		// 加仓一个单位
		if tick.Bids[0].Price > priceLong && int(currentN) < model.GetTurtleLimit(setting.Symbol) {
			setting.Chance = setting.Chance + 1
			setting.GridAmount = setting.GridAmount + turtleData.amount
			handleBreak(setting, turtleData, model.OrderSideBuy)
			util.Notice(fmt.Sprintf(`加多 chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
				setting.Chance, setting.GridAmount, currentN, priceShort, priceLong, setting.PriceX, turtleData.n))
		} // 平多
		if tick.Asks[0].Price < priceShort {
			setting.Chance = 0
			setting.GridAmount = 0
			handleBreak(setting, turtleData, model.OrderSideSell)
			util.Notice(fmt.Sprintf(`平多 chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
				setting.Chance, setting.GridAmount, currentN, priceShort, priceLong, setting.PriceX, turtleData.n))
		}
	} else if setting.Chance < 0 {
		priceLong = math.Min(turtleData.highDays10, setting.PriceX+2*turtleData.n)
		priceShort = setting.PriceX - turtleData.n/2
		amountLong = setting.GridAmount
		placeTurtleOrders(setting.Market, setting.Symbol, turtleData, setting, currentN,
			priceShort, priceLong, amountShort, amountLong)
		// 加仓一个单位
		if tick.Asks[0].Price < priceShort && int(currentN) > -1*model.GetTurtleLimit(setting.Symbol) {
			setting.Chance = setting.Chance - 1
			setting.GridAmount = setting.GridAmount + turtleData.amount
			handleBreak(setting, turtleData, model.OrderSideSell)
			util.Notice(fmt.Sprintf(`加空 chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
				setting.Chance, setting.GridAmount, currentN, priceShort, priceLong, setting.PriceX, turtleData.n))
		} // 平空
		if tick.Bids[0].Price > priceLong {
			setting.Chance = 0
			setting.GridAmount = 0
			handleBreak(setting, turtleData, model.OrderSideBuy)
			util.Notice(fmt.Sprintf(`平空 chance:%f amount:%f currentN:%f short-long:%f %f px:%f n:%f`,
				setting.Chance, setting.GridAmount, currentN, priceShort, priceLong, setting.PriceX, turtleData.n))
		}
	}
}

func handleBreak(setting *model.Setting, turtleData *TurtleData, orderSide string) (dealPrice float64) {
	if turtleData == nil || turtleData.orderLong == nil || turtleData.orderShort == nil {
		util.Notice(fmt.Sprintf(`fata error, nil order to break`))
		return
	}
	orderIdCancel := turtleData.orderLong.OrderId
	orderIdQuery := turtleData.orderShort.OrderId
	if orderSide == model.OrderSideBuy {
		orderIdCancel = turtleData.orderShort.OrderId
		orderIdQuery = turtleData.orderLong.OrderId
	}
	for true {
		canceled, _ := api.MustCancel(``, ``, setting.Market, setting.Symbol, orderIdCancel, true)
		if canceled {
			break
		}
	}
	for true {
		util.Notice(fmt.Sprintf(`query turtle break %s %s`, orderSide, orderIdQuery))
		order := api.QueryOrderById(``, ``, setting.Market, setting.Symbol, orderIdQuery)
		if order != nil && order.DealPrice > 0 && order.Status == model.CarryStatusSuccess {
			setting.UpdatedAt = util.GetNow()
			model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
				setting.Market, setting.Symbol, model.FunctionTurtle).Updates(map[string]interface{}{
				`price_x`: order.DealPrice, `chance`: setting.Chance, `grid_amount`: setting.GridAmount})
			turtleData.orderLong = nil
			turtleData.orderShort = nil
			return order.DealPrice
		}
		time.Sleep(time.Second * 3)
	}
	return setting.PriceX
}

func placeTurtleOrders(market, symbol string, turtleData *TurtleData, setting *model.Setting,
	currentN, priceShort, priceLong, amountShort, amountLong float64) {
	if turtleData.orderLong == nil && int(currentN) < model.GetTurtleLimit(symbol) {
		util.Notice(fmt.Sprintf(`place stop long chance:%f amount:%f price:%f currentN-limit:%f %d`,
			setting.Chance, setting.GridAmount, setting.PriceX, currentN, model.GetTurtleLimit(symbol)))
		order := api.PlaceOrder(model.KeyDefault, model.SecretDefault, model.OrderSideBuy, model.OrderTypeStop, market,
			symbol, ``, setting.AccountType, ``, model.FunctionTurtle, priceLong, amountLong, true)
		if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
			turtleData.orderLong = order
		}
	}
	if turtleData.orderShort == nil && int(currentN) > -1*model.GetTurtleLimit(symbol) {
		util.Notice(fmt.Sprintf(`place stop short chance:%f amount:%f price:%f currentN-limit:%f %d`,
			setting.Chance, setting.GridAmount, setting.PriceX, currentN, model.GetTurtleLimit(symbol)))
		order := api.PlaceOrder(model.KeyDefault, model.SecretDefault, model.OrderSideSell, model.OrderTypeStop,
			market, symbol, ``, setting.AccountType, ``, model.FunctionTurtle, priceShort,
			amountShort, true)
		if order != nil && order.OrderId != `` && order.Status != model.CarryStatusFail {
			turtleData.orderShort = order
		}
	}
}
