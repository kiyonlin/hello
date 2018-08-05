package carry

import (
	"fmt"
	"github.com/jinzhu/configor"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/api"
	"hello/controller"
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

var turtleCarrying = false

func setTurtleCarrying(status bool) {
	turtleCarrying = status
}

func placeTurtle(market, symbol string, carry *model.Carry) {
	util.Notice(`begin to place turtle ` + carry.ToString())
	_, _, coinLimit := model.GetTurtleSetting(market, symbol)
	currencies := strings.Split(carry.Symbol, `_`)
	if len(currencies) != 2 {
		util.Notice(`wrong symbol format ` + carry.Symbol)
		return
	}
	leftAccount := model.ApplicationAccounts.GetAccount(market, currencies[0])
	rightAccount := model.ApplicationAccounts.GetAccount(market, currencies[1])
	if leftAccount == nil || rightAccount == nil {
		api.RefreshAccount(market)
		return
	}
	coin := leftAccount.Free
	money := rightAccount.Free
	askSide := model.OrderSideSell
	bidSide := model.OrderSideBuy
	carry.SideType = model.CarryTypeTurtle
	if carry.AskAmount > coin {
		util.Notice(fmt.Sprintf(`[both buy]coin %f - ask %f %f - %f`, coin, carry.AskAmount,
			carry.BidPrice, carry.AskPrice))
		askSide = model.OrderSideBuy
		bidSide = model.OrderSideBuy
		carry.SideType = model.CarryTypeTurtleBothBuy
	} else if carry.BidAmount > money/carry.BidPrice || coin > coinLimit {
		util.Notice(fmt.Sprintf(`[both sell] [coin %f - limit %f] [bid %f - can %f] %f - %f`,
			coin, coinLimit, carry.BidAmount, money/carry.BidPrice, carry.BidPrice, carry.AskPrice))
		askSide = model.OrderSideSell
		bidSide = model.OrderSideSell
		carry.SideType = model.CarryTypeTurtleBothSell
	}
	carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskStatus = api.PlaceOrder(askSide,
		model.OrderTypeLimit, market, symbol, carry.AskPrice, carry.AskAmount)
	carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus = api.PlaceOrder(bidSide,
		model.OrderTypeLimit, market, symbol, carry.BidPrice, carry.BidAmount)
	if carry.DealAskStatus == model.CarryStatusWorking && carry.DealBidStatus == model.CarryStatusWorking {
		util.Notice(`set new carry ` + carry.ToString())
		model.SetTurtleCarry(market, symbol, carry)
		if carry.SideType == model.CarryTypeTurtleBothBuy || carry.SideType == model.CarryTypeTurtleBothSell {
			util.Notice(`[急漲急跌，休息1分鐘]`)
			time.Sleep(time.Minute * 1)
		}
	} else {
		if carry.DealAskOrderId != `` && carry.DealAskOrderId != `0` {
			api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
			api.RefreshAccount(carry.AskWeb)
		}
		if carry.DealBidOrderId != `` && carry.DealBidOrderId != `0` {
			api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
			api.RefreshAccount(carry.BidWeb)
		}
	}
	model.CarryChannel <- *carry
}

func handleTurtle(market, symbol string, carry *model.Carry) {
	turtleStatus := model.GetTurtleStatus(market, symbol)
	if turtleStatus == nil {
		util.Notice(`[error] try to handle nil turtle carry`)
	}
	marketBidPrice := model.ApplicationMarkets.BidAsks[symbol][market].Bids[0].Price
	marketAskPrice := model.ApplicationMarkets.BidAsks[symbol][market].Asks[0].Price
	if marketAskPrice < carry.BidPrice {
		api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
		carry.DealBidStatus = model.CarryStatusSuccess
		carry.DealAskStatus = model.CarryStatusFail
		carry.DealBidAmount = carry.BidAmount
		carry.DealAskAmount, _, _ = api.QueryOrderById(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
		model.CarryChannel <- *carry
		util.Info(fmt.Sprintf(`[%s捕获Turtle][取消ASK]min:%f - max:%f amount:%f bid:%f - ask:%f`,
			carry.Symbol, carry.BidPrice, carry.AskPrice, carry.Amount, marketBidPrice, marketAskPrice))
		dealAmount, _, _ := api.QueryOrderById(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
		turtleStatus = &model.TurtleStatus{LastDealPrice: carry.BidPrice, ExtraAsk: dealAmount, ExtraBid: 0}
		model.SetTurtleStatus(market, symbol, turtleStatus)
		model.SetTurtleCarry(market, symbol, nil)
	} else if marketBidPrice > carry.AskPrice {
		api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
		carry.DealBidStatus = model.CarryStatusFail
		carry.DealAskStatus = model.CarryStatusSuccess
		carry.DealAskAmount = carry.AskAmount
		carry.DealBidAmount, _, _ = api.QueryOrderById(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
		model.CarryChannel <- *carry
		util.Info(fmt.Sprintf(`[%s捕获Turtle][取消BID]min:%f - max:%f amount:%f  bid:%f - ask:%f`, carry.Symbol,
			carry.BidPrice, carry.AskPrice, carry.Amount, marketBidPrice, marketAskPrice))
		dealAmount, _, _ := api.QueryOrderById(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
		turtleStatus = &model.TurtleStatus{LastDealPrice: carry.AskPrice, ExtraAsk: 0, ExtraBid: dealAmount}
		model.SetTurtleStatus(market, symbol, turtleStatus)
		model.SetTurtleCarry(market, symbol, nil)
	} else if (marketAskPrice == carry.BidPrice || marketBidPrice == carry.AskPrice) &&
		util.GetNowUnixMillion()-turtleStatus.CarryTime > 10000 {
		dealBidAmount, _, _ := api.QueryOrderById(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
		if dealBidAmount == carry.BidAmount {
			dealAskAmount, _, _ := api.QueryOrderById(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
			if dealAskAmount == carry.AskAmount {
				carry.DealBidStatus = model.CarryStatusSuccess
				carry.DealAskStatus = model.CarryStatusSuccess
				carry.DealAskAmount = dealAskAmount
				carry.DealBidAmount = dealBidAmount
				turtleStatus = &model.TurtleStatus{LastDealPrice: turtleStatus.LastDealPrice, ExtraBid: 0, ExtraAsk: 0}
				model.SetTurtleStatus(market, symbol, turtleStatus)
				model.SetTurtleCarry(market, symbol, nil)
				model.CarryChannel <- *carry
				util.Info(fmt.Sprintf(`[hill wait]%s min:%f - max:%f bid:%f - ask:%f`, carry.Symbol,
					carry.BidPrice, carry.AskPrice, carry.BidAmount, carry.AskAmount))
			}
		}
	}
}

func handleTurtleBothSell(market, symbol string, carry *model.Carry) {
	_, priceWidth, _ := model.GetTurtleSetting(market, symbol)
	marketBidPrice := model.ApplicationMarkets.BidAsks[symbol][market].Bids[0].Price
	if marketBidPrice < carry.BidPrice { // 價格未能夾住
		api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
		api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
		carry.DealBidStatus = model.CarryStatusFail
		carry.DealAskStatus = model.CarryStatusFail
		model.CarryChannel <- *carry
		model.SetTurtleCarry(market, symbol, nil)
		askDealAmount, _, _ := api.QueryOrderById(market, symbol, carry.DealAskOrderId)
		bidDealAmount, _, _ := api.QueryOrderById(market, symbol, carry.DealBidOrderId)
		turtleStatus := &model.TurtleStatus{LastDealPrice: carry.BidPrice - priceWidth, ExtraAsk: askDealAmount,
			ExtraBid: bidDealAmount}
		model.SetTurtleStatus(market, symbol, turtleStatus)
		api.RefreshAccount(market)
	} else if marketBidPrice > carry.AskPrice {
		carry.DealBidStatus = model.CarryStatusSuccess
		carry.DealAskStatus = model.CarryStatusSuccess
		model.CarryChannel <- *carry
		model.SetTurtleCarry(market, symbol, nil)
		turtleStatus := &model.TurtleStatus{LastDealPrice: carry.AskPrice, ExtraAsk: 0, ExtraBid: 0}
		model.SetTurtleStatus(market, symbol, turtleStatus)
		api.RefreshAccount(market)
	} else if marketBidPrice > carry.BidPrice {
		api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
		carry.DealBidStatus = model.CarryStatusSuccess
		carry.DealAskStatus = model.CarryStatusFail
		model.CarryChannel <- *carry
		model.SetTurtleCarry(market, symbol, nil)
		askDealAmount, _, _ := api.QueryOrderById(market, symbol, carry.DealAskOrderId)
		turtleStatus := &model.TurtleStatus{LastDealPrice: carry.BidPrice, ExtraAsk: askDealAmount, ExtraBid: 0}
		model.SetTurtleStatus(market, symbol, turtleStatus)
		api.RefreshAccount(market)
	}
}

func handleTurtleBothBuy(market, symbol string, carry *model.Carry) {
	_, priceWidth, _ := model.GetTurtleSetting(market, symbol)
	marketAskPrice := model.ApplicationMarkets.BidAsks[symbol][market].Asks[0].Price
	if marketAskPrice > carry.AskPrice {
		api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
		api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
		carry.DealBidStatus = model.CarryStatusFail
		carry.DealAskStatus = model.CarryStatusFail
		model.CarryChannel <- *carry
		model.SetTurtleCarry(market, symbol, nil)
		askDealAmount, _, _ := api.QueryOrderById(market, symbol, carry.DealAskOrderId)
		bidDealAmount, _, _ := api.QueryOrderById(market, symbol, carry.DealBidOrderId)
		turtleStatus := &model.TurtleStatus{LastDealPrice: carry.AskPrice+priceWidth, ExtraAsk: askDealAmount,
			ExtraBid: bidDealAmount}
		model.SetTurtleStatus(market, symbol, turtleStatus)
		api.RefreshAccount(market)
	} else if marketAskPrice < carry.BidPrice {
		carry.DealBidStatus = model.CarryStatusSuccess
		carry.DealAskStatus = model.CarryStatusSuccess
		model.CarryChannel <- *carry
		model.SetTurtleCarry(market, symbol, nil)
		turtleStatus := &model.TurtleStatus{LastDealPrice: carry.BidPrice, ExtraAsk: 0, ExtraBid: 0}
		model.SetTurtleStatus(market, symbol, turtleStatus)
		api.RefreshAccount(market)
	} else if marketAskPrice < carry.AskPrice {
		api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
		carry.DealBidStatus = model.CarryStatusFail
		carry.DealAskStatus = model.CarryStatusSuccess
		model.CarryChannel <- *carry
		model.SetTurtleCarry(market, symbol, nil)
		bidDealAmount, _, _ := api.QueryOrderById(market, symbol, carry.DealBidOrderId)
		turtleStatus := &model.TurtleStatus{LastDealPrice: carry.AskPrice, ExtraAsk: 0, ExtraBid: bidDealAmount}
		model.SetTurtleStatus(market, symbol, turtleStatus)
		api.RefreshAccount(market)
	}
}

var ProcessTurtle = func(symbol, market string) {
	if turtleCarrying {
		return
	}
	setTurtleCarrying(true)
	defer setTurtleCarrying(false)
	carry, err := model.ApplicationMarkets.NewTurtleCarry(symbol, market)
	if err != nil {
		util.Notice(`can not create turtle ` + err.Error())
		return
	}
	if !carry.CheckWorthSaveMargin() {
		util.Notice(`turtle利潤不足手續費` + carry.ToString())
	}
	timeOk, _ := carry.CheckWorthCarryTime(model.ApplicationMarkets, model.ApplicationConfig)
	if !timeOk {
		util.Info(`turtle get carry not on time` + carry.ToString())
		return
	}
	if model.GetTurtleCarry(market, symbol) == nil {
		placeTurtle(market, symbol, carry)
	} else {
		carry = model.GetTurtleCarry(market, symbol)
		switch carry.SideType {
		case model.CarryTypeTurtle:
			handleTurtle(market, symbol, carry)
		case model.CarryTypeTurtleBothSell:
			handleTurtleBothSell(market, symbol, carry)
		case model.CarryTypeTurtleBothBuy:
			handleTurtleBothBuy(market, symbol, carry)
		}
	}
}

var ProcessCarry = func(symbol, market string) {
	carry, err := model.ApplicationMarkets.NewCarry(symbol)
	if err != nil {
		util.Notice(`can not create carry ` + err.Error())
		return
	}
	if carry.AskWeb != market && carry.BidWeb != market {
		util.Notice(`do not create a carry not related to ` + market)
	}
	currencies := strings.Split(carry.Symbol, "_")
	leftBalance := 0.0
	rightBalance := 0.0
	account := model.ApplicationAccounts.GetAccount(carry.AskWeb, currencies[0])
	if account == nil {
		util.Info(`nil account ` + carry.AskWeb + currencies[0])
		return
	}
	leftBalance = account.Free
	account = model.ApplicationAccounts.GetAccount(carry.BidWeb, currencies[1])
	if account == nil {
		util.Info(`nil account ` + carry.BidWeb + currencies[1])
		return
	}
	rightBalance = account.Free
	priceInUsdt, _ := api.GetPrice(currencies[0] + "_usdt")
	minAmount := 0.0
	maxAmount := 0.0
	if priceInUsdt != 0 {
		minAmount = model.ApplicationConfig.MinUsdt / priceInUsdt
		maxAmount = model.ApplicationConfig.MaxUsdt / priceInUsdt
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
	timeOk, _ := carry.CheckWorthCarryTime(model.ApplicationMarkets, model.ApplicationConfig)
	marginOk, _ := carry.CheckWorthCarryMargin(model.ApplicationMarkets, model.ApplicationConfig)
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
			model.ApplicationMarkets.BidAsks[carry.Symbol][carry.AskWeb] = nil
			model.ApplicationMarkets.BidAsks[carry.Symbol][carry.BidWeb] = nil
			if leftBalance < minAmount {
				carry.DealAskStatus = `NotEnough`
				carry.DealBidStatus = `NotEnough`
				util.Info(fmt.Sprintf(`leftB %f rightB/bidPrice %f/%f NotEnough %f - %f %s`, account.Free,
					rightBalance, carry.BidPrice, leftBalance, minAmount, carry.ToString()))
			} else {
				if model.ApplicationConfig.Env == `test` {
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
		go api.Order(carry, model.OrderSideSell, model.OrderTypeLimit, market, symbol, carry.AskPrice, leftBalance)
		go api.Order(carry, model.OrderSideBuy, model.OrderTypeLimit, market, symbol, carry.BidPrice, leftBalance)
	} else {
		model.CarryChannel <- *carry
	}
}

func createServer(markets *model.Markets, carryHandlers []api.CarryHandler, marketName string) chan struct{} {
	util.SocketInfo(" create chan for " + marketName)
	var channel chan struct{}
	var err error
	switch marketName {
	case model.Huobi:
		channel, err = api.WsDepthServeHuobi(markets, carryHandlers, WSErrHandler)
	case model.OKEX:
		channel, err = api.WsDepthServeOkex(markets, carryHandlers, WSErrHandler)
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

func MaintainMarketChan(carryHandlers []api.CarryHandler) {
	if socketMaintaining {
		return
	}
	socketMaintaining = true
	for _, marketName := range model.GetMarkets() {
		subscribes := model.GetSubscribes(marketName)
		for _, subscribe := range subscribes {
			for index := 0; index < model.ApplicationConfig.Channels; index++ {
				channel := model.ApplicationMarkets.GetChan(marketName, index)
				if channel == nil {
					model.ApplicationMarkets.PutChan(marketName, index, createServer(model.ApplicationMarkets,
						carryHandlers, marketName))
					util.SocketInfo(marketName + " create new channel " + subscribe)
				} else if model.ApplicationMarkets.RequireChanReset(marketName, subscribe) {
					util.SocketInfo(marketName + " reset channel " + subscribe)
					model.ApplicationMarkets.PutChan(marketName, index, nil)
					channel <- struct{}{}
					close(channel)
					model.ApplicationMarkets.PutChan(marketName, index, createServer(model.ApplicationMarkets,
						carryHandlers, marketName))
				}
				util.SocketInfo(marketName + " new channel reset done")
			}
			break
		}
	}
	socketMaintaining = false
}

func Maintain() {
	util.Notice("start carrying")
	model.NewConfig()
	err := configor.Load(model.ApplicationConfig, "./config.yml")
	if err != nil {
		util.Notice(err.Error())
		return
	}
	model.ApplicationDB, err = gorm.Open("postgres", model.ApplicationConfig.DBConnection)
	if err != nil {
		util.Notice(err.Error())
		return
	}
	defer model.ApplicationDB.Close()
	model.ApplicationDB.AutoMigrate(&model.Carry{})
	model.ApplicationDB.AutoMigrate(&model.Account{})
	model.ApplicationDB.AutoMigrate(&model.Setting{})
	model.LoadSettings()
	go OuterCarryServe()
	go InnerCarryServe()
	go AccountHandlerServe()
	go controller.ParameterServe()
	go RefreshAccounts()

	carryHandlers := make([]api.CarryHandler, len(model.ApplicationConfig.Functions))
	for i, value := range model.ApplicationConfig.Functions {
		switch value {
		case `carry`:
			go MaintainOrders()
			carryHandlers[i] = ProcessCarry
		case `turtle`:
			carryHandlers[i] = ProcessTurtle
		case `refresh`:
			carryHandlers[i] = ProcessRefresh
			go RefreshCarryServe()
		}
	}
	for true {
		go MaintainMarketChan(carryHandlers)
		time.Sleep(time.Duration(model.ApplicationConfig.ChannelSlot) * time.Millisecond)
	}
}
