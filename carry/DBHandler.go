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
var lastAmountIndex = 0

var WSErrHandler = func(err error) {
	print(err)
	util.SocketInfo(`get error ` + err.Error())
}

func MaintainFcoinRank() {
	for true {
		api.RefreshAccount(``, ``, model.Fcoin)
		//settings := recalcRankLine(model.Fcoin)
		//for _, setting := range settings {
		//	model.AppDB.Model(&setting).Where(`function=? and market=? and symbol=?`,
		//		model.FunctionRank, setting.Market, setting.Symbol).
		//		Updates(model.Setting{OpenShortMargin: setting.OpenShortMargin,
		//			CloseShortMargin: setting.CloseShortMargin})
		//}
		time.Sleep(time.Minute * 3)
	}
}

//func CheckPastRefresh() {
func _() {
	start := fmt.Sprintf(`-%ds`, model.AppConfig.RefreshTimeSlot)
	d, _ := time.ParseDuration(start)
	for true {
		now := util.GetNow()
		begin := now.Add(d)
		begin = time.Date(begin.Year(), begin.Month(), begin.Day(), begin.Hour(), begin.Minute(), 0, 0,
			begin.Location())
		end := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0,
			now.Location())
		slotNum := int((now.Hour()*3600 + now.Minute()*60 + now.Second()) / model.AppConfig.RefreshTimeSlot)
		if lastAmountIndex != slotNum {
			lastAmountIndex = slotNum
			markets := model.GetMarkets()
			for _, market := range markets {
				symbols := model.GetMarketSymbols(market)
				for symbol := range symbols {
					beginStr := fmt.Sprintf(`%d-%d-%d %d:%d:%d`,
						begin.Year(), begin.Month(), begin.Day(), begin.Hour(), begin.Minute(), begin.Second())
					endStr := fmt.Sprintf(`%d-%d-%d %d:%d:%d`,
						end.Year(), end.Month(), end.Day(), end.Hour(), end.Minute(), end.Second())
					util.SocketInfo(fmt.Sprintf(`to check amount %s %s %s ~ %s`, market, symbol, beginStr, endStr))
					rows, err := model.AppDB.Table(`orders`).Select(`sum(price*amount)`).
						Where(`market=? and symbol=? and function=? and order_time>? and order_time<?`,
							market, symbol, model.FunctionRefresh, beginStr, endStr).Rows()
					if err == nil {
						if rows.Next() {
							var amount float64
							_ = rows.Scan(&amount)
							setting := model.GetSetting(model.FunctionRefresh, market, symbol)
							if setting != nil {
								util.SocketInfo(fmt.Sprintf(`check amount %s %s %f %f`, market, symbol,
									amount, setting.AmountLimit))
								if setting.AmountLimit > amount {
									body := fmt.Sprintf(`[%s~%s]%s %s amount:%f < limit%f`,
										beginStr, endStr, market, symbol, amount, setting.AmountLimit)
									err := util.SendMail(model.AppConfig.Mail,
										fmt.Sprintf(`[refresh]%s %f`, symbol, amount), body)
									if err != nil {
										util.SocketInfo(fmt.Sprintf(`%s %s发送失败`, market, symbol))
									}
								} else {
									util.SocketInfo(fmt.Sprintf(`[refresh enough]%s %s %f > limit %f`,
										market, symbol, amount, setting.AmountLimit))
								}
							}
						}
					} else {
						util.SocketInfo(`can not get amount from db ` + err.Error())
					}
					if rows != nil {
						rows.Close()
					}
				}
			}
		}
		time.Sleep(time.Minute * 9)
	}
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

//CancelAllOrders
var _ = func() {
	model.AppPause = true
	time.Sleep(time.Second)
	markets := model.GetMarkets()
	for _, market := range markets {
		symbols := model.GetMarketSymbols(market)
		for symbol := range symbols {
			util.Notice(fmt.Sprintf(`[cancel old orders] %s %s`, market, symbol))
			orders := api.QueryOrders(key, secret, market, symbol, model.CarryStatusWorking,
				model.AccountTypeLever+model.AccountTypeNormal, 0, 0)
			for _, order := range orders {
				if order != nil && order.OrderId != `` {
					result, errCode, msg, _ := api.CancelOrder(key, secret, market, symbol, order.OrderId)
					util.Notice(fmt.Sprintf(`[cancel old]%v %s %s`, result, errCode, msg))
					time.Sleep(time.Millisecond * 100)
				}
			}
		}
	}
	model.LoadSettings()
	model.AppPause = false
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

func MaintainTransFee(key, secret string) {
	for true {
		d, _ := time.ParseDuration("-48h")
		lastDays2 := util.GetNow().Add(d)
		//d, _ = time.ParseDuration(`-1h`)
		//lastHour := util.GetNow().Add(d)
		var orders []model.Order
		for true {
			model.AppDB.Limit(100).Offset(feeIndex).Where(
				`deal_amount<amount and date(order_time)>? and status!=? and status!=?`,
				lastDays2, model.CarryStatusSuccess, model.CarryStatusFail).Find(&orders)
			if len(orders) == 0 {
				break
			}
			util.Info(fmt.Sprintf(`--- get working orders %d`, len(orders)))
			feeIndex += len(orders)
			for _, value := range orders {
				order := api.QueryOrderById(key, secret, value.Market, value.Symbol, value.OrderId)
				if order == nil {
					continue
				}
				value.Fee = order.Fee
				value.FeeIncome = order.FeeIncome
				value.DealAmount = order.DealAmount
				value.Status = order.Status
				value.DealPrice = order.DealPrice
				model.AppDB.Save(&value)
				util.Info(fmt.Sprintf(`save order %s %s %s %s`,
					value.Symbol, value.OrderSide, value.OrderTime.String(), value.Status))
				time.Sleep(time.Second)
			}
		}
		feeIndex = 0
		time.Sleep(time.Minute * 5)
	}
}

func createMarketDepthServer(markets *model.Markets, market string) chan struct{} {
	model.ConnectionResetTime = 0
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
	case model.Fmex:
		channel, err = api.WsDepthServeFmex(markets, WSErrHandler)
	case model.Coinpark:
		channel, err = api.WsDepthServeCoinpark(markets, WSErrHandler)
	case model.Bitmex:
		channel, err = api.WsDepthServeBitmex(markets, WSErrHandler)
	case model.Bybit:
		channel, err = api.WsDepthServeBybit(markets, WSErrHandler)
	}
	if err != nil {
		util.SocketInfo(market + ` can not create depth server ` + err.Error())
	}
	return channel
}

var socketMaintaining = false

func ResetChannel(market string, channel chan struct{}) {
	model.AppPause = true
	model.AppMarkets.PutDepthChan(market, 0, nil)
	symbols := model.GetMarketSymbols(market)
	for symbol := range symbols {
		for function := range model.GetFunctions(model.Bitmex, symbol) {
			if model.FunctionHangContract == function {
				go CancelHangContracts(key, secret, market, symbol)
			}
			if model.FunctionRefresh == function {
				go CancelRefreshHang(key, secret, market, symbol, RefreshTypeGrid)
			}
			if model.FunctionHangFar == function {
				go CancelHang(key, secret, market, symbol)
			}
		}
	}
	channel <- struct{}{}
	close(channel)
	model.AppMarkets.PutDepthChan(market, 0, createMarketDepthServer(model.AppMarkets, market))
	model.AppPause = false
	util.SocketInfo(market + " reset depth channel ")
}

func MaintainMarketChan() {
	if socketMaintaining {
		return
	}
	socketMaintaining = true
	for _, market := range model.GetMarkets() {
		channel := model.AppMarkets.GetDepthChan(market, 0)
		if channel == nil {
			model.AppMarkets.PutDepthChan(market, 0, createMarketDepthServer(model.AppMarkets, market))
			util.Notice(fmt.Sprintf("%s create new depth channel ", market))
		} else {
			if model.AppMarkets.RequireDepthChanReset(market) {
				util.Notice(fmt.Sprintf("%s require new depth channel ", market))
				ResetChannel(market, channel)
			}
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
	//model.HandlerMap[model.FunctionCarry] = ProcessCarry
	model.HandlerMap[model.FunctionTurtle] = ProcessTurtle
	model.HandlerMap[model.FunctionCarry] = ProcessCarrySameTime
	model.HandlerMap[model.FunctionCarry+`_`+model.Fmex] = ProcessCarrySameTime
	model.HandlerMap[model.FunctionCarry+`_`+model.Bybit] = ProcessCarrySameTime
	model.HandlerMap[model.FunctionHang] = ProcessHang
	model.HandlerMap[model.FunctionRank] = ProcessRank
	model.HandlerMap[model.FunctionHangFar] = ProcessHangFar
	model.HandlerMap[model.FunctionHangContract] = ProcessHangContract
	model.HandlerMap[model.FunctionBMCarryHang] = ProcessCarryOrder
	defer model.AppDB.Close()
	model.AppDB.AutoMigrate(&model.Account{})
	model.AppDB.AutoMigrate(&model.Setting{})
	model.AppDB.AutoMigrate(&model.Order{})
	model.AppDB.AutoMigrate(&model.Score{})
	model.AppDB.AutoMigrate(&model.Candle{})
	model.LoadSettings()
	go MaintainFcoinRank()
	go CancelOldMakers(``, ``)
	go AccountHandlerServe()
	//go CheckPastRefresh()
	go MaintainTransFee(key, secret)
	//go util.StartMidNightTimer(CancelAllOrders)
	for true {
		go MaintainMarketChan()
		time.Sleep(time.Duration(model.AppConfig.ChannelSlot) * time.Millisecond)
	}
}
