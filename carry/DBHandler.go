package carry

import (
	"fmt"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/api"
	"hello/model"
	"hello/util"
	"time"
)

var accountServing = false

var WSErrHandler = func(err error) {
	print(err)
	util.SocketInfo(`get error ` + err.Error())
}

func AccountHandlerServe() {
	for true {
		accounts := <-model.AccountChannel
		if accountServing {
			continue
		}
		accountServing = true
		cleared := false
		for _, value := range accounts {
			current := util.GetNow()
			if !cleared {
				//util.Info(`remove accounts ` + value.Market + util.GetNow().Format("2006-01-02"))
				model.AppDB.Delete(model.Account{}, "market = ? AND date(created_at) > ?",
					value.Market, current.Format("2006-01-02"))
				cleared = true
			}
			model.AppDB.Create(value)
		}
		accountServing = false
	}
}

var CancelAllOrders = func() {
	previousHandle := model.AppConfig.Handle
	model.AppConfig.Handle = `0`
	time.Sleep(time.Second)
	markets := model.GetMarkets()
	for _, market := range markets {
		symbols := model.GetMarketSymbols(market)
		for symbol := range symbols {
			util.Notice(fmt.Sprintf(`[cancel old orders] %s %s`, market, symbol))
			orders := api.QueryOrders(market, symbol, model.CarryStatusWorking)
			for orderId := range orders {
				if orderId != `` {
					result, errCode, msg := api.CancelOrder(market, symbol, orderId)
					util.Notice(fmt.Sprintf(`[cancel old]%v %s %s`, result, errCode, msg))
					time.Sleep(time.Millisecond * 100)
				}
			}
		}
	}
	model.AppConfig.Handle = previousHandle
}

//func RefreshAccounts() {
//	for true {
//		if model.AppConfig.Handle == `1` {
//			model.AppConfig.Handle = `0`
//			time.Sleep(time.Second * 5)
//			model.AppConfig.Handle = `1`
//		}
//		markets := model.GetMarkets()
//		timestamp := util.GetNow()
//		for _, value := range markets {
//			api.RefreshAccount(value)
//			if model.AppAccounts.Data[value] == nil {
//				continue
//			}
//			//accounts.MarketTotal[marketName] = 0
//			symbols := model.GetMarketSymbols(value)
//			for key, account := range model.AppAccounts.Data[value] {
//				if symbols[key+"_usdt"] || key == `usdt` {
//					account.PriceInUsdt, _ = api.GetPrice(key + "_usdt")
//					account.Timestamp = timestamp
//				}
//			}
//			model.AccountChannel <- model.AppAccounts.Data[value]
//		}
//		time.Sleep(time.Hour)
//	}
//}

var feeIndex int

func MaintainTransFee() {
	for true {
		d, _ := time.ParseDuration("-48h")
		lastDays2 := util.GetNow().Add(d)
		d, _ = time.ParseDuration(`-1h`)
		lastHour := util.GetNow().Add(d)
		var orders []model.Order
		for true {
			model.AppDB.Limit(100).Offset(feeIndex).Where(
				`fee=? and fee_income=? and date(order_time)>? and status=?`,
				0, 0, lastDays2, model.CarryStatusWorking).Find(&orders)
			if len(orders) == 0 {
				break
			}
			feeIndex += len(orders)
			for _, value := range orders {
				order := api.QueryOrderById(value.Market, value.Symbol, value.OrderId)
				if order == nil {
					continue
				}
				value.Fee = order.Fee
				value.FeeIncome = order.FeeIncome
				value.DealAmount = order.DealAmount
				value.Status = order.Status
				model.AppDB.Save(&value)
				if model.AppConfig.Handle == `1` {
					time.Sleep(time.Second)
				} else {
					time.Sleep(time.Millisecond * 120)
				}
			}
		}
		feeIndex = 0
		// deal fee check
		var setting model.Setting
		symbolEarn := make(map[string]float64)  // symbol - earn
		symbolInall := make(map[string]float64) // symbol - in all
		strTime := fmt.Sprintf(`%d-%d-%d %d:%d:%d`, lastHour.Year(), lastHour.Month(), lastHour.Day(),
			lastHour.Hour(), lastHour.Minute(), lastHour.Second())
		rows, _ := model.AppDB.Model(&orders).Select(`symbol, order_side,round(sum(fee),4), 
			round(sum(fee_income),4),round(sum(price*deal_amount)/sum(deal_amount),4),
			round(sum(price*deal_amount),0)`).
			Where(`deal_amount>? and status != ? and order_time>?`, 0, `fail`, strTime).
			Group(`order_side, symbol`).Rows()
		for rows.Next() {
			var symbol, side string
			var fee, feeIncome, price, inAll float64
			_ = rows.Scan(&symbol, &side, &fee, &feeIncome, &price, &inAll)
			if side == model.OrderSideBuy {
				fee = fee * price
			}
			if side == model.OrderSideSell {
				feeIncome = feeIncome * price
			}
			symbolEarn[symbol] += feeIncome - fee
			symbolInall[symbol] += inAll
		}
		for key, value := range symbolInall {
			if value != 0 {
				rate := symbolEarn[key] / value
				msg := fmt.Sprintf(`[check fee rate]%s %f %f %f`, key, symbolEarn[key], value, rate)
				util.Notice(msg)
				if rate < -0.0001 {
					_ = util.SendMail(model.AppConfig.Mail, `手续费异常`, msg)
					model.AppDB.Model(&setting).Where("market= ? and symbol= ? and function= ?",
						model.Fcoin, key, model.FunctionRefresh).
						Updates(map[string]interface{}{`refresh_same_time`: `1`})
					model.LoadSettings()
				}
			}
		}
		time.Sleep(time.Minute * 5)
	}
}

func createMarketDepthServer(markets *model.Markets, market string) chan struct{} {
	util.SocketInfo(" create depth chan for " + market)
	var channel chan struct{}
	var err error
	switch market {
	case model.Huobi:
		channel, err = api.WsDepthServeHuobi(markets, WSErrHandler)
	case model.OKEX:
		channel, err = api.WsDepthServeOkex(markets, WSErrHandler)
	case model.OKFUTURE:
		channel, err = api.WsDepthServeOKFuture(markets, WSErrHandler)
	case model.Binance:
		channel, err = api.WsDepthServeBinance(markets, WSErrHandler)
	case model.Fcoin:
		channel, err = api.WsDepthServeFcoin(markets, WSErrHandler)
	case model.Coinpark:
		channel, err = api.WsDepthServeCoinpark(markets, WSErrHandler)
	case model.Coinbig:
		channel, err = api.WsDepthServeCoinbig(markets, WSErrHandler)
	case model.Bitmex:
		channel, err = api.WsDepthServeBitmex(WSErrHandler)
	}
	if err != nil {
		util.SocketInfo(market + ` can not create depth server ` + err.Error())
	}
	return channel
}

var socketMaintaining = false

func MaintainMarketChan() {
	if socketMaintaining {
		return
	}
	socketMaintaining = true
	for _, market := range model.GetMarkets() {
		symbols := model.GetMarketSymbols(market)
		for symbol := range symbols {
			for index := 0; index < model.AppConfig.Channels; index++ {
				channel := model.AppMarkets.GetDepthChan(market, index)
				if channel == nil {
					model.AppMarkets.PutDepthChan(market, index, createMarketDepthServer(model.AppMarkets, market))
					util.SocketInfo(market + " create new depth channel " + symbol)
				} else if model.AppMarkets.RequireDepthChanReset(market, symbol) {
					CancelRefreshHang(market, symbol)
					model.AppMarkets.PutDepthChan(market, index, nil)
					channel <- struct{}{}
					close(channel)
					model.AppMarkets.PutDepthChan(market, index, createMarketDepthServer(model.AppMarkets, market))
					util.SocketInfo(market + " reset depth channel " + symbol)
				}
			}
			break
		}
	}
	socketMaintaining = false
}

func Maintain() {
	util.Notice("start carrying")
	var err error
	model.AppDB, err = gorm.Open("postgres", model.AppConfig.DBConnection)
	if err != nil {
		util.Notice(err.Error())
		return
	}
	model.HandlerMap[model.FunctionMaker] = ProcessMake
	model.HandlerMap[model.FunctionGrid] = ProcessGrid
	//model.HandlerMap[model.FunctionArbitrary] = ProcessContractArbitrage
	model.HandlerMap[model.FunctionRefresh] = ProcessRefresh
	model.HandlerMap[model.FunctionCarry] = ProcessCarry
	model.HandlerMap[model.FunctionHang] = ProcessHang
	defer model.AppDB.Close()
	model.AppDB.AutoMigrate(&model.Account{})
	model.AppDB.AutoMigrate(&model.Setting{})
	model.AppDB.AutoMigrate(&model.Order{})
	model.LoadSettings()
	go CancelOldMakers()
	go AccountHandlerServe()
	//go RefreshAccounts()
	go MaintainTransFee()
	go util.StartMidNightTimer(CancelAllOrders)
	for true {
		go MaintainMarketChan()
		time.Sleep(time.Duration(model.AppConfig.ChannelSlot) * time.Millisecond)
	}
}

//func createAccountInfoServer(marketName string) chan struct{} {
//	util.SocketInfo(` create account info chan for ` + marketName)
//	var channel chan struct{}
//	var err error
//	switch marketName {
//	case model.OKFUTURE:
//		channel, err = api.WsAccountServeOKFuture(WSErrHandler)
//	}
//	if err != nil {
//		util.SocketInfo(marketName + ` can not create server ` + err.Error())
//	}
//	return channel
//}

////每个小时查询过去一天内有没有从这种setting里面离开的，
////       如果有，那么这是一个勤快的setting
////              如果离开利润低于百分之1.5那么买入利润增加万分之一，
////              如果进入利润高于千分之1.5，那么进入利润减万分之一
////       如果没有，那么这是一个懒惰的setting，在懒惰的里面寻找所有持仓量最大的setting
////              如果进入利润低于百分之1.5，那么进入利润加万分之一
////              如果离开利润大于万分之5，那么离开利润减万分之一
//func dealDiligentSettings() {
//	createdAt := util.GetNow().Add(time.Duration(-3600) * time.Second)
//	settings := model.LoadDiligentSettings(model.OKFUTURE, model.CarryTypeFuture, createdAt)
//	for _, setting := range settings {
//		if setting == nil {
//			continue
//		}
//		util.Notice(fmt.Sprintf(`[modify setting]%s %s`, setting.Market, setting.Symbol))
//		if setting.OpenShortMargin < 0.015 {
//			util.Notice(fmt.Sprintf(`open margin %f < 0.015, + 0.0001`, setting.OpenShortMargin))
//			setting.OpenShortMargin += 0.0001
//		}
//		if setting.CloseShortMargin > 0.0015 {
//			util.Notice(fmt.Sprintf(`close margin %f > 0.0015, - 0.0001`, setting.CloseShortMargin))
//			setting.CloseShortMargin -= 0.0001
//		}
//		model.AppDB.Save(setting)
//	}
//}

//func dealLazySettings() {
//	createdAt := util.GetNow().Add(time.Duration(-86400) * time.Second)
//	symbols := model.GetMarketSymbols(model.OKFUTURE)
//	diligentSettings := model.LoadDiligentSettings(model.OKFUTURE, model.CarryTypeFuture, createdAt)
//	openShort := 0.0
//	var setting *model.Setting
//	for symbol := range symbols {
//		if diligentSettings[symbol] != nil {
//			continue
//		}
//		futureAccount, _ := api.GetPositionOkfuture(model.OKFUTURE, symbol)
//		if futureAccount != nil {
//			short := futureAccount.OpenedShort
//			if strings.Contains(futureAccount.Symbol, `btc`) {
//				short = short * 10
//			}
//			if openShort < short {
//				openShort = short
//				setting = model.GetSetting(model.FunctionArbitrary, model.OKFUTURE, symbol)
//			}
//		}
//	}
//	if setting != nil {
//		changed := false
//		util.Notice(fmt.Sprintf(`[modify setting]%s %s`, setting.Market, setting.Symbol))
//		if setting.CloseShortMargin < 0.015 {
//			changed = true
//			util.Notice(fmt.Sprintf(`close margin %f < 0.015, + 0.0001`, setting.CloseShortMargin))
//			setting.CloseShortMargin += 0.0001
//		}
//		if setting.OpenShortMargin > 0.0005 {
//			changed = true
//			util.Notice(fmt.Sprintf(`open margin %f > 0.0005, - 0.0001`, setting.OpenShortMargin))
//			setting.OpenShortMargin -= 0.0001
//		}
//		if changed {
//			model.AppDB.Save(setting)
//		}
//	}
//}
//
//func MaintainArbitrarySettings() {
//	for true {
//		dealLazySettings()
//		dealDiligentSettings()
//		time.Sleep(time.Hour)
//	}
//}
