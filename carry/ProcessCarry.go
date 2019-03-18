package carry

import (
	"fmt"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/api"
	"hello/model"
	"hello/util"
	"strconv"
	"strings"
	"time"
)

var WSErrHandler = func(err error) {
	print(err)
	util.SocketInfo(`get error ` + err.Error())
}

// 只取第一位
func calcAmount(originalAmount float64) (num float64, err error) {
	str := strconv.FormatFloat(originalAmount, 'f', util.GetPrecision(originalAmount), 64)
	bytes := []byte(str)
	startReplace := false
	for i, v := range bytes {
		if startReplace && v != '.' {
			bytes[i] = '0'
		}
		if v != '0' && v != '.' {
			startReplace = true
		}
	}
	return strconv.ParseFloat(string(bytes), 64)
}

var ProcessCarry = func(market, symbol string) {
	carry, err := model.AppMarkets.NewCarry(symbol)
	if err != nil {
		util.Notice(`can not create carry ` + err.Error())
		return
	}
	if carry.AskWeb != market && carry.BidWeb != market {
		util.Notice(`do not create a carry not related to ` + market)
	}
	currencies := strings.Split(carry.BidSymbol, "_")
	leftBalance := 0.0
	rightBalance := 0.0
	account := model.AppAccounts.GetAccount(carry.AskWeb, currencies[0])
	if account == nil {
		util.Info(`nil account ` + carry.AskWeb + currencies[0])
		return
	}
	leftBalance = account.Free
	account = model.AppAccounts.GetAccount(carry.BidWeb, currencies[1])
	if account == nil {
		util.Info(`nil account ` + carry.BidWeb + currencies[1])
		return
	}
	rightBalance = account.Free
	priceInUsdt, _ := api.GetPrice(currencies[0] + "_usdt")
	minAmount := 0.0
	maxAmount := 0.0
	if priceInUsdt != 0 {
		minAmount = model.AppConfig.MinUsdt / priceInUsdt
		maxAmount = model.AppConfig.MaxUsdt / priceInUsdt
	}
	if carry.Amount > maxAmount {
		carry.Amount = maxAmount
	}
	if leftBalance > carry.Amount {
		leftBalance = carry.Amount
	}
	if leftBalance*carry.BidPrice > rightBalance {
		leftBalance = rightBalance / carry.BidPrice
	}
	planAmount, _ := calcAmount(carry.Amount)
	carry.Amount = planAmount
	leftBalance, _ = calcAmount(leftBalance)
	timeOk, _ := carry.CheckWorthCarryTime()
	marginOk, _ := carry.CheckWorthCarryMargin(model.AppMarkets)
	if !carry.CheckWorthSaveMargin() {
		// no need to save carry with margin < base cost
		return
	}
	doCarry := false
	if !timeOk {
		carry.DealAskStatus = `NotOnTime`
		carry.DealBidStatus = `NotOnTime`
		util.Info(`get carry not on time` + carry.ToString())
	} else {
		if !marginOk {
			carry.DealAskStatus = `NotWorth`
			carry.DealBidStatus = `NotWorth`
			util.Info(`get carry no worth` + carry.ToString())
		} else {
			model.AppMarkets.BidAsks[carry.AskSymbol][carry.AskWeb] = nil
			model.AppMarkets.BidAsks[carry.BidSymbol][carry.BidWeb] = nil
			if leftBalance < minAmount {
				carry.DealAskStatus = `NotEnough`
				carry.DealBidStatus = `NotEnough`
				util.Info(fmt.Sprintf(`leftB %f rightB/bidPrice %f/%f NotEnough %f - %f %s`, account.Free,
					rightBalance, carry.BidPrice, leftBalance, minAmount, carry.ToString()))
			} else {
				if model.AppConfig.Env == `test` {
					carry.DealAskStatus = `NotDo`
					carry.DealBidStatus = `NotDo`
				} else {
					util.Notice(`[worth carry]` + carry.ToString())
					doCarry = true
				}
			}
		}
	}
	if doCarry {
		go order(carry, model.OrderSideSell, model.OrderTypeLimit, market, symbol, carry.AskPrice, leftBalance)
		go order(carry, model.OrderSideBuy, model.OrderTypeLimit, market, symbol, carry.BidPrice, leftBalance)
	} else {
		model.CarryChannel <- *carry
	}
}

func order(carry *model.Carry, orderSide, orderType, market, symbol string, price, amount float64) {
	if orderSide == model.OrderSideSell {
		order := api.PlaceOrder(orderSide, orderType, market, symbol, ``, price, amount)
		carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskStatus, carry.AskAmount, carry.AskPrice =
			order.OrderId, order.ErrCode, order.Status, order.DealPrice, order.DealPrice
	} else if orderSide == model.OrderSideBuy {
		order := api.PlaceOrder(orderSide, orderType, market, symbol, ``, price, amount)
		carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus, carry.BidAmount, carry.BidPrice =
			order.OrderId, order.ErrCode, order.Status, order.DealAmount, order.DealPrice
	}
	model.InnerCarryChannel <- *carry
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

func createMarketDepthServer(markets *model.Markets, carryHandlers []api.CarryHandler, marketName string) chan struct{} {
	util.SocketInfo(" create depth chan for " + marketName)
	var channel chan struct{}
	var err error
	switch marketName {
	case model.Huobi:
		channel, err = api.WsDepthServeHuobi(markets, carryHandlers, WSErrHandler)
	case model.OKEX:
		channel, err = api.WsDepthServeOkex(markets, carryHandlers, WSErrHandler)
	case model.OKFUTURE:
		channel, err = api.WsDepthServeOKFuture(markets, carryHandlers, WSErrHandler)
	case model.Binance:
		channel, err = api.WsDepthServeBinance(markets, carryHandlers, WSErrHandler)
	case model.Fcoin:
		channel, err = api.WsDepthServeFcoin(markets, carryHandlers, WSErrHandler)
	case model.Coinpark:
		channel, err = api.WsDepthServeCoinpark(markets, carryHandlers, WSErrHandler)
	case model.Coinbig:
		channel, err = api.WsDepthServeCoinbig(markets, carryHandlers, WSErrHandler)
	case model.Bitmex:
		channel, err = api.WsDepthServeBitmex(markets, carryHandlers, WSErrHandler)
	}
	if err != nil {
		util.SocketInfo(marketName + ` can not create server ` + err.Error())
	}
	return channel
}

var socketMaintaining = false

func MaintainMarketDepthChan(carryHandlers []api.CarryHandler) {
	if socketMaintaining {
		return
	}
	socketMaintaining = true
	for _, marketName := range model.GetMarkets() {
		subscribes := model.GetDepthSubscribes(marketName)
		for _, subscribe := range subscribes {
			for index := 0; index < model.AppConfig.Channels; index++ {
				channel := model.AppMarkets.GetChan(marketName, index)
				if channel == nil {
					model.AppMarkets.PutChan(marketName, index, createMarketDepthServer(model.AppMarkets,
						carryHandlers, marketName))
					util.SocketInfo(marketName + " create new channel " + subscribe)
				} else if model.AppMarkets.RequireChanReset(marketName, subscribe) {
					model.AppMarkets.PutChan(marketName, index, nil)
					channel <- struct{}{}
					close(channel)
					model.AppMarkets.PutChan(marketName, index, createMarketDepthServer(model.AppMarkets,
						carryHandlers, marketName))
					util.SocketInfo(marketName + " reset channel " + subscribe)
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
	defer model.AppDB.Close()
	model.AppDB.AutoMigrate(&model.Carry{})
	model.AppDB.AutoMigrate(&model.Account{})
	model.AppDB.AutoMigrate(&model.Setting{})
	model.AppDB.AutoMigrate(&model.Order{})
	model.LoadSettings()
	go OuterCarryServe()
	go InnerCarryServe()
	go AccountHandlerServe()
	go RefreshAccounts()
	go CancelOldWorkingOrders()
	needWS := true
	carryHandlers := make([]api.CarryHandler, len(model.AppConfig.Functions))
	for i, value := range model.AppConfig.Functions {
		switch value {
		case model.FunctionCarry:
			go MaintainOrders()
			carryHandlers[i] = ProcessCarry
		case model.FunctionBalanceTurtle:
			carryHandlers[i] = ProcessBalanceTurtle
		case model.FunctionRefresh:
			carryHandlers[i] = ProcessRefresh
			go RefreshCarryServe()
		case model.FunctionArbitrary:
			go MaintainArbitrarySettings()
			carryHandlers[i] = ProcessContractArbitrage
		case model.FunctionRsi:
			needWS = false
			go ProcessInform()
		case model.FunctionMaker:
			carryHandlers[i] = ProcessMake
		case model.FunctionGrid:
			carryHandlers[i] = ProcessGrid
			go GridServe()
		}
	}
	//go MaintainAccountChan()
	for true {
		if needWS {
			go MaintainMarketDepthChan(carryHandlers)
		}
		time.Sleep(time.Duration(model.AppConfig.ChannelSlot) * time.Millisecond)
	}
}
