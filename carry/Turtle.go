package carry

//import (
//	"hello/model"
//	"hello/util"
//	"hello/api"
//	"fmt"
//	"time"
//	"strings"
//)
//
//var turtleCarrying = false
//
//func setTurtleCarrying(status bool) {
//	turtleCarrying = status
//}
//
//func placeTurtle(market, symbol string, carry *model.Carry) {
//	util.Notice(`begin to place turtle ` + carry.ToString())
//	setting := model.GetSetting(market, symbol)
//	if carry.BidPrice < setting.MinPrice || carry.AskPrice > setting.MaxPrice {
//		util.Notice(fmt.Sprintf(`超出限额min%f-max%f bid%f-ask%f`,
//			setting.MinPrice, setting.MaxPrice, carry.BidPrice, carry.AskPrice))
//		return
//	}
//	currencies := strings.Split(carry.Symbol, `_`)
//	if len(currencies) != 2 {
//		util.Notice(`wrong symbol format ` + carry.Symbol)
//		return
//	}
//	leftAccount := model.AppAccounts.GetAccount(market, currencies[0])
//	rightAccount := model.AppAccounts.GetAccount(market, currencies[1])
//	if leftAccount == nil || rightAccount == nil {
//		api.RefreshAccount(market)
//		return
//	}
//	coin := leftAccount.Free
//	money := rightAccount.Free
//	askSide := model.OrderSideSell
//	bidSide := model.OrderSideBuy
//	carry.SideType = model.CarryTypeTurtle
//	if carry.AskAmount > coin || carry.BidAmount > money/carry.BidPrice {
//		util.Notice(fmt.Sprintf(`金额不足coin%f-ask%f money%f-bid%f`, coin, carry.AskAmount, money, carry.BidAmount))
//		return
//	}
//	//if carry.AskAmount > coin {
//	//	util.Notice(fmt.Sprintf(`[both buy]coin %f - ask %f %f - %f`, coin, carry.AskAmount,
//	//		carry.BidPrice, carry.AskPrice))
//	//	askSide = model.OrderSideBuy
//	//	bidSide = model.OrderSideBuy
//	//	carry.SideType = model.CarryTypeTurtleBothBuy
//	//} else if carry.BidAmount > money/carry.BidPrice || coin > float64(setting.TurtleLeftCopy) * setting.TurtleLeftAmount {
//	//	util.Notice(fmt.Sprintf(`[both sell] [coin %f - limit %f] [bid %f - can %f] %f - %f`,
//	//		coin, float64(setting.TurtleLeftCopy) * setting.TurtleLeftAmount, carry.BidAmount,
//	//		money/carry.BidPrice, carry.BidPrice, carry.AskPrice))
//	//	askSide = model.OrderSideSell
//	//	bidSide = model.OrderSideSell
//	//	carry.SideType = model.CarryTypeTurtleBothSell
//	//}
//	if api.CheckOrderValue(currencies[0], carry.AskAmount) {
//		carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskStatus = api.PlaceOrder(askSide,
//			model.OrderTypeLimit, market, symbol, carry.AskPrice, carry.AskAmount)
//	} else {
//		carry.DealAskStatus = model.CarryStatusSuccess
//	}
//	if api.CheckOrderValue(currencies[1], carry.BidAmount) {
//		carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus = api.PlaceOrder(bidSide,
//			model.OrderTypeLimit, market, symbol, carry.BidPrice, carry.BidAmount)
//	} else {
//		carry.DealBidStatus = model.CarryStatusSuccess
//	}
//	if (carry.DealAskStatus == model.CarryStatusWorking || carry.DealAskStatus == model.CarryStatusSuccess) &&
//		(carry.DealBidStatus == model.CarryStatusWorking || carry.DealBidStatus == model.CarryStatusSuccess) {
//		util.Notice(`set new carry ` + carry.ToString())
//		model.SetTurtleCarry(market, symbol, carry)
//		if carry.SideType == model.CarryTypeTurtleBothBuy || carry.SideType == model.CarryTypeTurtleBothSell {
//			util.Notice(`[急漲急跌，休息1分鐘]`)
//			time.Sleep(time.Minute * 1)
//		}
//	} else {
//		if carry.DealAskOrderId != `` && carry.DealAskOrderId != `0` && carry.DealAskStatus == model.CarryStatusWorking {
//			api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
//			api.RefreshAccount(carry.AskWeb)
//		}
//		if carry.DealBidOrderId != `` && carry.DealBidOrderId != `0` && carry.DealBidStatus == model.CarryStatusWorking {
//			api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
//			api.RefreshAccount(carry.BidWeb)
//		}
//		util.Notice(`[下單失敗，休息1分鐘]`)
//		time.Sleep(time.Minute * 1)
//	}
//	model.CarryChannel <- *carry
//}
//
//func handleTurtle(market, symbol string, carry *model.Carry, turtleStatus *model.TurtleStatus) {
//	marketBidPrice := model.AppMarkets.BidAsks[symbol][market].Bids[0].Price
//	marketAskPrice := model.AppMarkets.BidAsks[symbol][market].Asks[0].Price
//	if marketAskPrice < carry.BidPrice {
//		if carry.DealAskStatus == model.CarryStatusWorking {
//			api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
//			carry.DealAskAmount, _, _ = api.QueryOrderById(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
//		} else if carry.DealAskStatus == model.CarryStatusSuccess {
//			carry.DealAskAmount = carry.AskAmount
//		}
//		carry.DealBidAmount = carry.BidAmount
//		carry.DealBidStatus = model.CarryStatusSuccess
//		carry.DealAskStatus = model.CarryStatusFail
//		model.CarryChannel <- *carry
//		util.Info(fmt.Sprintf(`[%s捕获Turtle][取消ASK]min:%f - max:%f amount:%f bid:%f - ask:%f`,
//			carry.Symbol, carry.BidPrice, carry.AskPrice, carry.Amount, marketBidPrice, marketAskPrice))
//		turtleStatus = &model.TurtleStatus{LastDealPrice: carry.BidPrice,
//			ExtraAsk: carry.DealAskAmount + turtleStatus.ExtraAsk, ExtraBid: 0}
//		model.SetTurtleStatus(market, symbol, turtleStatus)
//		model.SetTurtleCarry(market, symbol, nil)
//	} else if marketBidPrice > carry.AskPrice {
//		if carry.DealBidStatus == model.CarryStatusWorking {
//			api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
//			carry.DealBidAmount, _, _ = api.QueryOrderById(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
//		} else if carry.DealBidStatus == model.CarryStatusSuccess {
//			carry.DealBidAmount = carry.BidAmount
//		}
//		carry.DealBidStatus = model.CarryStatusFail
//		carry.DealAskStatus = model.CarryStatusSuccess
//		carry.DealAskAmount = carry.AskAmount
//		model.CarryChannel <- *carry
//		util.Info(fmt.Sprintf(`[%s捕获Turtle][取消BID]min:%f - max:%f amount:%f  bid:%f - ask:%f`, carry.Symbol,
//			carry.BidPrice, carry.AskPrice, carry.Amount, marketBidPrice, marketAskPrice))
//		turtleStatus = &model.TurtleStatus{LastDealPrice: carry.AskPrice,
//			ExtraAsk: 0, ExtraBid: turtleStatus.ExtraBid + carry.DealBidAmount}
//		model.SetTurtleStatus(market, symbol, turtleStatus)
//		model.SetTurtleCarry(market, symbol, nil)
//	} else if (marketAskPrice == carry.BidPrice || marketBidPrice == carry.AskPrice) &&
//		util.GetNowUnixMillion()-turtleStatus.CarryTime > 10000 {
//		turtleStatus.CarryTime = util.GetNowUnixMillion()
//		model.SetTurtleStatus(market, symbol, turtleStatus)
//		if carry.DealBidStatus == model.CarryStatusWorking {
//			carry.DealBidAmount, _, _ = api.QueryOrderById(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
//		} else if carry.DealBidStatus == model.CarryStatusSuccess {
//			carry.DealBidAmount = carry.BidAmount
//		}
//		if carry.DealBidAmount == carry.BidAmount {
//			if carry.DealAskStatus == model.CarryStatusWorking {
//				carry.DealAskAmount, _, _ = api.QueryOrderById(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
//			} else if carry.DealAskStatus == model.CarryStatusSuccess {
//				carry.DealAskAmount = carry.AskAmount
//			}
//			if carry.DealAskAmount == carry.AskAmount {
//				carry.DealBidStatus = model.CarryStatusSuccess
//				carry.DealAskStatus = model.CarryStatusSuccess
//				turtleStatus = &model.TurtleStatus{LastDealPrice: turtleStatus.LastDealPrice, ExtraBid: 0, ExtraAsk: 0}
//				model.SetTurtleStatus(market, symbol, turtleStatus)
//				model.SetTurtleCarry(market, symbol, nil)
//				model.CarryChannel <- *carry
//				util.Info(fmt.Sprintf(`[hill wait]%s min:%f - max:%f bid:%f - ask:%f`, carry.Symbol,
//					carry.BidPrice, carry.AskPrice, carry.BidAmount, carry.AskAmount))
//			}
//		}
//	}
//}
//
//func handleTurtleBothSell(market, symbol string, carry *model.Carry, turtleStatus *model.TurtleStatus) {
//	setting := model.GetSetting(market, symbol)
//	marketBidPrice := model.AppMarkets.BidAsks[symbol][market].Bids[0].Price
//	if marketBidPrice < carry.BidPrice { // 價格未能夾住
//		if carry.DealAskStatus != model.CarryStatusSuccess {
//			api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
//			carry.DealAskAmount, _, _ = api.QueryOrderById(market, symbol, carry.DealAskOrderId)
//		} else {
//			carry.DealAskAmount = carry.AskAmount
//		}
//		if carry.DealBidStatus != model.CarryStatusSuccess {
//			api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
//			carry.DealBidAmount, _, _ = api.QueryOrderById(market, symbol, carry.DealBidOrderId)
//		} else {
//			carry.DealBidAmount = carry.BidAmount
//		}
//		carry.DealBidStatus = model.CarryStatusFail
//		carry.DealAskStatus = model.CarryStatusFail
//		model.CarryChannel <- *carry
//		model.SetTurtleCarry(market, symbol, nil)
//		turtleStatus := &model.TurtleStatus{LastDealPrice: carry.BidPrice - setting.TurtlePriceWidth,
//			ExtraAsk: turtleStatus.ExtraAsk + carry.DealAskAmount, ExtraBid: turtleStatus.ExtraBid + carry.DealBidAmount}
//		model.SetTurtleStatus(market, symbol, turtleStatus)
//		api.RefreshAccount(market)
//	} else if marketBidPrice > carry.AskPrice {
//		carry.DealBidStatus = model.CarryStatusSuccess
//		carry.DealAskStatus = model.CarryStatusSuccess
//		model.CarryChannel <- *carry
//		model.SetTurtleCarry(market, symbol, nil)
//		turtleStatus := &model.TurtleStatus{LastDealPrice: carry.AskPrice, ExtraAsk: 0, ExtraBid: 0}
//		model.SetTurtleStatus(market, symbol, turtleStatus)
//		api.RefreshAccount(market)
//	} else if marketBidPrice > carry.BidPrice {
//		if carry.DealAskStatus == model.CarryStatusWorking {
//			api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
//			carry.DealAskAmount, _, _ = api.QueryOrderById(market, symbol, carry.DealAskOrderId)
//		} else {
//			carry.DealAskAmount = carry.AskAmount
//		}
//		carry.DealBidStatus = model.CarryStatusSuccess
//		carry.DealAskStatus = model.CarryStatusFail
//		model.CarryChannel <- *carry
//		model.SetTurtleCarry(market, symbol, nil)
//		turtleStatus := &model.TurtleStatus{LastDealPrice: carry.BidPrice,
//			ExtraAsk: turtleStatus.ExtraAsk + carry.DealAskAmount, ExtraBid: 0}
//		model.SetTurtleStatus(market, symbol, turtleStatus)
//		api.RefreshAccount(market)
//	}
//}
//
//func handleTurtleBothBuy(market, symbol string, carry *model.Carry, turtleStatus *model.TurtleStatus) {
//	setting := model.GetSetting(market, symbol)
//	marketAskPrice := model.AppMarkets.BidAsks[symbol][market].Asks[0].Price
//	if marketAskPrice > carry.AskPrice {
//		if carry.DealAskStatus == model.CarryStatusSuccess {
//			api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
//			carry.DealAskAmount, _, _ = api.QueryOrderById(market, symbol, carry.DealAskOrderId)
//		}
//		if carry.DealBidStatus == model.CarryStatusSuccess {
//			api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
//			carry.DealBidAmount, _, _ = api.QueryOrderById(market, symbol, carry.DealBidOrderId)
//		}
//		carry.DealBidStatus = model.CarryStatusFail
//		carry.DealAskStatus = model.CarryStatusFail
//		model.CarryChannel <- *carry
//		model.SetTurtleCarry(market, symbol, nil)
//		turtleStatus := &model.TurtleStatus{LastDealPrice: carry.AskPrice + setting.TurtlePriceWidth,
//			ExtraAsk: turtleStatus.ExtraAsk + carry.DealAskAmount, ExtraBid: turtleStatus.ExtraBid + carry.DealBidAmount}
//		model.SetTurtleStatus(market, symbol, turtleStatus)
//		api.RefreshAccount(market)
//	} else if marketAskPrice < carry.BidPrice {
//		carry.DealBidStatus = model.CarryStatusSuccess
//		carry.DealAskStatus = model.CarryStatusSuccess
//		model.CarryChannel <- *carry
//		model.SetTurtleCarry(market, symbol, nil)
//		turtleStatus := &model.TurtleStatus{LastDealPrice: carry.BidPrice, ExtraAsk: 0, ExtraBid: 0}
//		model.SetTurtleStatus(market, symbol, turtleStatus)
//		api.RefreshAccount(market)
//	} else if marketAskPrice < carry.AskPrice {
//		if carry.DealBidStatus != model.CarryStatusSuccess {
//			api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
//			carry.DealBidAmount, _, _ = api.QueryOrderById(market, symbol, carry.DealBidOrderId)
//		} else {
//			carry.DealBidAmount = carry.BidAmount
//		}
//		carry.DealBidStatus = model.CarryStatusFail
//		carry.DealAskStatus = model.CarryStatusSuccess
//		model.CarryChannel <- *carry
//		model.SetTurtleCarry(market, symbol, nil)
//		turtleStatus := &model.TurtleStatus{LastDealPrice: carry.AskPrice, ExtraAsk: 0,
//			ExtraBid: turtleStatus.ExtraBid + carry.DealBidAmount}
//		model.SetTurtleStatus(market, symbol, turtleStatus)
//		api.RefreshAccount(market)
//	}
//}
//
//var ProcessTurtle = func(symbol, market string) {
//	if turtleCarrying {
//		return
//	}
//	setTurtleCarrying(true)
//	defer setTurtleCarrying(false)
//	if model.GetTurtleCarry(market, symbol) == nil {
//		carry, err := model.AppMarkets.NewTurtleCarry(symbol, market)
//		if err != nil {
//			util.Notice(`can not create turtle ` + err.Error())
//			return
//		}
//		if !carry.CheckWorthSaveMargin() {
//			util.Notice(`turtle利潤不足手續費` + carry.ToString())
//		}
//		timeOk, _ := carry.CheckWorthCarryTime(model.AppMarkets, model.AppConfig)
//		if !timeOk {
//			util.Info(`turtle get carry not on time` + carry.ToString())
//			return
//		}
//		placeTurtle(market, symbol, carry)
//	} else {
//		carry := model.GetTurtleCarry(market, symbol)
//		turtleStatus := model.GetTurtleStatus(market, symbol)
//		if turtleStatus == nil {
//			turtleStatus = &model.TurtleStatus{}
//		}
//		switch carry.SideType {
//		case model.CarryTypeTurtle:
//			handleTurtle(market, symbol, carry, turtleStatus)
//		case model.CarryTypeTurtleBothSell:
//			handleTurtleBothSell(market, symbol, carry, turtleStatus)
//		case model.CarryTypeTurtleBothBuy:
//			handleTurtleBothBuy(market, symbol, carry, turtleStatus)
//		}
//	}
//}
