package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"strings"
	"time"
)

var balanceTurtleCarrying = false

func setBalanceTurtleCarrying(status bool) {
	balanceTurtleCarrying = status
}

var ProcessBalanceTurtle = func(symbol, market string) {
	if balanceTurtleCarrying {
		return
	}
	setBalanceTurtleCarrying(true)
	defer setBalanceTurtleCarrying(false)
	currencies := strings.Split(symbol, `_`)
	if len(currencies) != 2 {
		util.Notice(`wrong symbol format ` + symbol)
		return
	}
	leftAccount := model.ApplicationAccounts.GetAccount(market, currencies[0])
	rightAccount := model.ApplicationAccounts.GetAccount(market, currencies[1])
	if leftAccount == nil || rightAccount == nil {
		api.RefreshAccount(market)
		return
	}
	currentPrice, _ := api.GetPrice(symbol)
	if model.GetBalanceTurtleCarry(market, symbol) == nil {
		carry, err := model.ApplicationMarkets.NewBalanceTurtle(market, symbol, leftAccount, rightAccount, currentPrice)
		if err != nil {
			util.Notice(`can not create turtle ` + err.Error())
			return
		}
		placeTurtle(market, symbol, carry, leftAccount, rightAccount)
	} else {
		carry := model.GetBalanceTurtleCarry(market, symbol)
		handleTurtle(market, symbol, carry)
		api.RefreshAccount(market)
	}
}

func placeTurtle(market, symbol string, carry *model.Carry, leftAccount, rightAccount *model.Account) {
	util.Notice(`begin to place turtle ` + carry.ToString())
	if carry.AskAmount > leftAccount.Free || carry.BidAmount > rightAccount.Free/carry.BidPrice {
		util.Notice(fmt.Sprintf(`金额不足coin%f-ask%f money%f-bid%f`, leftAccount.Free, carry.AskAmount,
			rightAccount.Free, carry.BidAmount))
		return
	}
	carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskStatus = api.PlaceOrder(model.OrderSideSell,
		model.OrderTypeLimit, market, symbol, carry.AskPrice, carry.AskAmount)
	carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus = api.PlaceOrder(model.OrderSideBuy,
		model.OrderTypeLimit, market, symbol, carry.BidPrice, carry.BidAmount)
	if carry.DealAskOrderId != `` && carry.DealAskOrderId != `0` &&
		carry.DealBidOrderId != `` && carry.DealBidOrderId != `0` {
		util.Notice(`set new carry ` + carry.ToString())
		model.SetBalanceTurtleCarry(market, symbol, carry)
	} else {
		if carry.DealAskOrderId != `` && carry.DealAskOrderId != `0` {
			api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
			api.RefreshAccount(carry.AskWeb)
		}
		if carry.DealBidOrderId != `` && carry.DealBidOrderId != `0` {
			api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
			api.RefreshAccount(carry.BidWeb)
		}
		util.Notice(`[下單失敗，休息1分鐘]` + carry.ToString())
		time.Sleep(time.Minute * 1)
	}
	model.CarryChannel <- *carry
}

func handleTurtle(market, symbol string, carry *model.Carry) {
	marketBidPrice := model.ApplicationMarkets.BidAsks[symbol][market].Bids[0].Price
	marketAskPrice := model.ApplicationMarkets.BidAsks[symbol][market].Asks[0].Price
	if marketAskPrice < carry.BidPrice {
		api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
		carry.DealAskAmount, _, _ = api.QueryOrderById(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
		carry.DealBidAmount = carry.BidAmount
		carry.DealBidStatus = model.CarryStatusSuccess
		carry.DealAskStatus = model.CarryStatusFail
		model.CarryChannel <- *carry
		util.Info(fmt.Sprintf(`[%s捕获Turtle][取消ASK]min:%f - max:%f bid:%f - ask:%f`,
			carry.Symbol, carry.BidPrice, carry.AskPrice, marketBidPrice, marketAskPrice))
		model.SetBalanceTurtleCarry(market, symbol, nil)
	} else if marketBidPrice > carry.AskPrice {
		api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
		carry.DealBidAmount, _, _ = api.QueryOrderById(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
		carry.DealAskAmount = carry.AskAmount
		carry.DealBidStatus = model.CarryStatusFail
		carry.DealAskStatus = model.CarryStatusSuccess
		model.CarryChannel <- *carry
		util.Info(fmt.Sprintf(`[%s捕获Turtle][取消BID]min:%f - max:%f bid:%f - ask:%f`,
			carry.Symbol, carry.BidPrice, carry.AskPrice, marketBidPrice, marketAskPrice))
		model.SetBalanceTurtleCarry(market, symbol, nil)
	}
}
