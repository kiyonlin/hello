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

var contractArbitraging = false

func setContractArbitraging(status bool) {
	contractArbitraging = status
}

func closeShort(symbol, market, futureSymbol, futureMarket string, asks, bids *model.BidAsk) {
	if model.AppFutureAccount[futureMarket] == nil ||
		model.AppFutureAccount[futureMarket][futureSymbol] == nil {
		util.Notice(futureMarket + ` fail to get future account ` + futureSymbol)
		return
	}
	futureAccount := model.AppFutureAccount[futureMarket][futureSymbol]
	if futureAccount == nil || futureAccount.OpenedShort < 1 {
		util.Notice(`[No opened short]`)
	}
	carry := &model.Carry{}
	carry.Symbol = futureSymbol
	carry.AskWeb = market
	carry.BidWeb = futureMarket
	carry.AskPrice = asks.Asks[0].Price
	carry.BidPrice = bids.Bids[0].Price
	carry.AskTime = int64(asks.Ts)
	carry.BidTime = int64(bids.Ts)
	checkTime, msg := carry.CheckWorthCarryTime()
	if !checkTime {
		util.Notice(msg.Error())
		return
	}
	faceValue := model.OKEXOtherContractFaceValue
	if strings.Contains(symbol, `btc`) {
		faceValue = model.OKEXBTCContractFaceValue
	}
	carry.Amount = futureAccount.OpenedShort * faceValue / carry.AskPrice
	carry.AskAmount = carry.Amount
	carry.BidAmount = carry.Amount
	util.Notice(`[close short]` + carry.ToString())
	carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus, carry.BidAmount, carry.BidPrice =
		api.PlaceOrder(model.OrderSideLiquidateShort, model.OrderTypeMarket, futureMarket, futureSymbol, carry.AskPrice, carry.BidAmount)
	if carry.DealBidOrderId == `` || carry.DealBidOrderId == `0` {
		util.Notice(fmt.Sprintf(`[bid fail]%s %s price%f amount%f`, futureMarket, futureSymbol, carry.BidPrice, carry.BidAmount))
		return
	}
	time.Sleep(time.Second)
	api.RefreshAccount(futureMarket)
	carry.DealBidAmount, carry.BidPrice, _ = api.QueryOrderById(futureMarket, futureSymbol, carry.DealBidOrderId)
	if carry.DealBidAmount > 0 {
		transferAmount := 0.999 * carry.DealBidAmount * faceValue / carry.BidPrice
		transfer, errCode := api.FundTransferOkex(symbol, transferAmount, `3`, `1`)
		if transfer {
			carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskStatus, carry.AskAmount, carry.AskPrice =
				api.PlaceOrder(model.OrderSideSell, model.OrderTypeMarket, market, symbol, carry.BidPrice, transferAmount)
			time.Sleep(time.Second)
			if carry.DealAskOrderId != `` && carry.DealAskOrderId != `0` {
				api.RefreshAccount(market)
				carry.DealAskAmount, carry.AskPrice, _ = api.QueryOrderById(market, symbol, carry.DealAskOrderId)
			} else {
				util.Notice(`[!!Ask Fail]` + carry.DealAskErrCode + carry.DealAskStatus)
			}
		} else {
			util.Notice(`[transfer fail]` + errCode)
		}
	}
	model.CarryChannel <- *carry
}

func openShort(symbol, market, futureSymbol, futureMarket string, asks, bids *model.BidAsk) {
	carry := &model.Carry{}
	carry.Symbol = futureSymbol
	carry.AskWeb = futureMarket
	carry.BidWeb = market
	carry.AskPrice = asks.Asks[0].Price
	carry.BidPrice = bids.Bids[0].Price
	carry.AskTime = int64(asks.Ts)
	carry.BidTime = int64(bids.Ts)
	checkTime, msg := carry.CheckWorthCarryTime()
	if !checkTime {
		util.Notice(msg.Error())
		return
	}
	faceValue := model.OKEXOtherContractFaceValue
	if strings.Contains(symbol, `btc`) {
		faceValue = model.OKEXBTCContractFaceValue
	}
	account := model.AppAccounts.GetAccount(market, `usdt`)
	if account == nil {
		util.Notice(`account nil`)
	}
	carry.Amount = faceValue * math.Floor(account.Free/faceValue/(1+1/model.OKLever)) / carry.AskPrice
	if carry.Amount <= 0 {
		util.Info(fmt.Sprintf(`账户usdt余额usdt%f不够买%f个%s`, account.Free, carry.Amount+1, symbol))
		return
	}
	carry.BidAmount = carry.Amount
	carry.AskAmount = carry.Amount
	util.Notice(`[open short]` + carry.ToString())
	carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus, carry.BidAmount, carry.BidPrice =
		api.PlaceOrder(model.OrderSideBuy, model.OrderTypeMarket, market, symbol, carry.AskPrice, carry.BidAmount)
	if carry.DealBidOrderId == `` || carry.DealBidOrderId == `0` {
		util.Notice(fmt.Sprintf(`[bid fail]%s %s price%f amount%f`, market, symbol, carry.AskPrice, carry.BidAmount))
		return
	}
	time.Sleep(time.Second)
	api.RefreshAccount(market)
	carry.DealBidAmount, carry.BidPrice, _ = api.QueryOrderById(market, symbol, carry.DealBidOrderId)
	if carry.DealBidAmount > 0 {
		transfer, _ := api.FundTransferOkex(symbol, carry.DealBidAmount, `1`, `3`)
		if transfer {
			carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskStatus, carry.AskAmount, carry.AskPrice =
				api.PlaceOrder(model.OrderSideSell, model.OrderTypeMarket, futureMarket, futureSymbol, carry.BidPrice, carry.AskAmount)
			time.Sleep(time.Second)
			if carry.DealAskOrderId != `` && carry.DealAskOrderId != `0` {
				api.RefreshAccount(futureMarket)
				carry.DealAskAmount, carry.AskPrice, _ = api.QueryOrderById(futureMarket, futureSymbol, carry.DealAskOrderId)
			} else {
				util.Notice(`[!!Ask Fail]` + carry.DealAskErrCode + carry.DealAskStatus)
			}
		} else {
			util.Notice(`[transfer fail]`)
		}
	}
	model.CarryChannel <- *carry
}

func getSymbol(symbol string) string {
	index := strings.Index(symbol, `_`)
	if index > 0 {
		return symbol[0:index] + `_usdt`
	}
	return ``
}

var ProcessContractArbitrage = func(futureSymbol, futureMarket string) {
	if contractArbitraging || futureMarket != model.OKFUTURE {
		return
	}
	setContractArbitraging(true)
	defer setContractArbitraging(false)
	symbol := getSymbol(futureSymbol)
	if model.AppMarkets.BidAsks[symbol] == nil || model.AppMarkets.BidAsks[symbol][model.OKEX] == nil ||
		model.AppMarkets.BidAsks[futureSymbol] == nil || model.AppMarkets.BidAsks[futureSymbol][futureMarket] == nil {
		util.Notice(`data not available`)
		return
	}
	index := strings.Index(symbol, `_`)
	if index <= 0 {
		util.Notice(`wrong symbol without _`)
		return
	}
	setting := model.GetSetting(futureMarket, futureSymbol)
	bidAsk := model.AppMarkets.BidAsks[symbol][model.OKEX]
	futureBidAsk := model.AppMarkets.BidAsks[futureSymbol][futureMarket]
	openShortMargin := (futureBidAsk.Bids[0].Price - bidAsk.Asks[0].Price) / bidAsk.Asks[0].Price
	closeShortMargin := (futureBidAsk.Asks[0].Price - bidAsk.Bids[0].Price) / bidAsk.Bids[0].Price
	util.Info(fmt.Sprintf(`[open short %t]%f - %f [close short %t] %f - %f`,
		setting.OpenShortMargin < openShortMargin, openShortMargin, setting.OpenShortMargin,
		setting.CloseShortMargin > closeShortMargin, closeShortMargin, setting.CloseShortMargin))
	if setting.OpenShortMargin < openShortMargin {
		openShort(symbol, model.OKEX, futureSymbol, futureMarket, futureBidAsk, bidAsk)
	} else if setting.CloseShortMargin > closeShortMargin {
		closeShort(symbol, model.OKEX, futureSymbol, futureMarket, bidAsk, futureBidAsk)
	}
}