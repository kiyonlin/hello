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
	str := fmt.Sprintf(`%f`, originalAmount)
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

//func extraOrder(symbol, market, orderSide, orderType string, amount float64) {
//	if extraBidAskDone {
//		api.RefreshAccount(market)
//		return
//	}
//	turtleExtraDoing = true
//	price := fmt.Sprintf(`%f`, model.ApplicationMarkets.BidAsks[symbol][market].Asks[0][0])
//	orderId, _, _ := api.PlaceOrder(orderSide, orderType, market, symbol, price, fmt.Sprintf(`%f`, amount))
//	if orderId != `` && orderId != `0` {
//		extraBidAskDone = true
//		for true {
//			dealAmount, dealPrice, _ := api.QueryOrderById(market, symbol, orderId)
//			if dealAmount >= amount*0.9 {
//				if dealPrice != 0 {
//					model.SetTurtleDealPrice(market, symbol, dealPrice)
//				}
//				util.Notice(fmt.Sprintf(`[extra price]%f`, dealPrice))
//				break
//			}
//			time.Sleep(time.Second * 3)
//		}
//		if orderSide == model.OrderSideSell {
//			model.CarryChannel <- model.Carry{Symbol: symbol, AskWeb: market, AskAmount: amount,
//				AskPrice: model.ApplicationMarkets.BidAsks[symbol][market].Asks[0][0], DealAskStatus: model.CarryStatusWorking}
//		} else if orderSide == model.OrderSideBuy {
//			model.CarryChannel <- model.Carry{Symbol: symbol, BidWeb: market, BidAmount: amount,
//				BidPrice: model.ApplicationMarkets.BidAsks[symbol][market].Asks[0][0], DealBidStatus: model.CarryStatusWorking}
//		}
//	}
//	turtleExtraDoing = false
//}

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
		_, _, coinLimit := model.GetTurtleSetting(market, symbol)
		bidAmount := fmt.Sprintf(`%f`, carry.BidAmount)
		askAmount := fmt.Sprintf(`%f`, carry.AskAmount)
		bidPrice := fmt.Sprintf(`%f`, carry.BidPrice)
		askPrice := fmt.Sprintf(`%f`, carry.AskPrice)
		askSide := model.OrderSideSell
		bidSide := model.OrderSideBuy
		carry.SideType = model.CarryTypeTurtle
		if carry.AskAmount > coin {
			util.Notice(fmt.Sprintf(`[三倍buy]coin %f - ask %f %f - %f`, coin, carry.AskAmount,
				carry.BidPrice, carry.AskPrice))
			askSide = model.OrderSideBuy
			bidSide = model.OrderSideBuy
			askAmount = fmt.Sprintf(`%f`, carry.AskAmount*3)
			carry.SideType = model.CarryTypeTurtleBothBuy
		} else if carry.BidAmount > money/carry.BidPrice || coin > coinLimit {
			util.Notice(fmt.Sprintf(`[三倍sell] [coin %f - limit %f] [bid %f - can %f] %f - %f`,
				coin, coinLimit, carry.BidAmount, money/carry.BidPrice, carry.BidPrice, carry.AskPrice))
			askSide = model.OrderSideSell
			bidSide = model.OrderSideSell
			bidAmount = fmt.Sprintf(`%f`, carry.BidAmount*3)
			carry.SideType = model.CarryTypeTurtleBothSell
		}
		carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskStatus = api.PlaceOrder(askSide,
			model.OrderTypeLimit, market, symbol, askPrice, askAmount)
		carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus = api.PlaceOrder(bidSide,
			model.OrderTypeLimit, market, symbol, bidPrice, bidAmount)
		if carry.DealAskStatus == model.CarryStatusWorking && carry.DealBidStatus == model.CarryStatusWorking {
			util.Notice(`set new carry ` + carry.ToString())
			model.SetTurtleCarry(market, symbol, carry)
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
	} else {
		carry = model.GetTurtleCarry(market, symbol)
		marketBidPrice := model.ApplicationMarkets.BidAsks[symbol][market].Bids[0][0]
		marketAskPrice := model.ApplicationMarkets.BidAsks[symbol][market].Asks[0][0]
		util.Notice(fmt.Sprintf(`carry bid %f - carry ask %f; market bid %f - market ask %f`,
			carry.BidPrice, carry.AskPrice, marketBidPrice, marketAskPrice))
		if carry.SideType == model.CarryTypeTurtleBothSell && marketBidPrice < carry.BidPrice { // 價格未能夾住
			api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
			api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
			model.SetTurtleDealPrice(market, symbol, carry.BidPrice)
			model.SetTurtleCarry(market, symbol, nil)
			api.RefreshAccount(market)
		} else if carry.SideType == model.CarryTypeTurtleBothBuy && marketAskPrice > carry.AskPrice {
			api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
			api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
			model.SetTurtleDealPrice(market, symbol, carry.AskPrice)
			model.SetTurtleCarry(market, symbol, nil)
			api.RefreshAccount(market)
		} else if marketAskPrice < carry.BidPrice {
			api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
			model.SetTurtleDealPrice(carry.BidWeb, symbol, carry.BidPrice)
			model.SetTurtleCarry(market, symbol, nil)
			util.Info(fmt.Sprintf(`[%s捕获Turtle][取消ASK]min:%f - max:%f amount:%f bid:%f - ask:%f`, carry.Symbol,
				carry.BidPrice, carry.AskPrice, carry.Amount, marketBidPrice, marketAskPrice))
		} else if marketBidPrice > carry.AskPrice {
			api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
			model.SetTurtleDealPrice(carry.AskWeb, symbol, carry.AskPrice)
			model.SetTurtleCarry(market, symbol, nil)
			util.Info(fmt.Sprintf(`[%s捕获Turtle][取消BID]min:%f - max:%f amount:%f  bid:%f - ask:%f`, carry.Symbol,
				carry.BidPrice, carry.AskPrice, carry.Amount, marketBidPrice, marketAskPrice))
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
	minAmount := model.ApplicationConfig.MinUsdt / priceInUsdt
	maxAmount := model.ApplicationConfig.MaxUsdt / priceInUsdt
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
	strLeftBalance := fmt.Sprintf(`%f`, leftBalance)
	strAskPrice := fmt.Sprintf(`%f`, carry.AskPrice)
	strBidPrice := fmt.Sprintf(`%f`, carry.BidPrice)

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
		go api.Order(carry, model.OrderSideSell, model.OrderTypeLimit, market, symbol, strAskPrice, strLeftBalance)
		go api.Order(carry, model.OrderSideBuy, model.OrderTypeLimit, market, symbol, strBidPrice, strLeftBalance)
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
					util.SocketInfo(marketName + " create new channel" + subscribe)
				} else if model.ApplicationMarkets.RequireChanReset(marketName, subscribe) {
					util.SocketInfo(marketName + " reset channel" + subscribe)
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
	go RefreshAccounts()
	go OuterCarryServe()
	go InnerCarryServe()
	go AccountHandlerServe()
	model.ApplicationMarkets = model.NewMarkets()
	go controller.ParameterServe()

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
