package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"strings"
)

var contractArbitraging = false

func setContractArbitraging(status bool) {
	contractArbitraging = status
}

func arbitraryFutureMarket(futureMarket, futureSymbol string, futureBidAsk *model.BidAsk) {
	faceValue := model.OKEXOtherContractFaceValue
	if strings.Contains(futureSymbol, `btc`) {
		faceValue = model.OKEXBTCContractFaceValue
	}
	futureAccount, _ := api.GetPositionOkfuture(futureMarket, futureSymbol)
	holdings := 0.0
	if futureAccount != nil {
		holdings = futureAccount.OpenedShort
	}
	accountRights, _, _, _ := api.GetAccountOkfuture(futureSymbol)
	if futureBidAsk == nil || futureBidAsk.Bids == nil || len(futureBidAsk.Bids) < 1 {
		return
	}
	arbitraryAmount := math.Floor(accountRights*futureBidAsk.Bids[0].Price/faceValue - holdings)
	if arbitraryAmount > 0 {
		orderId, errCode, status, actualAmount, actualPrice := api.PlaceOrder(model.OrderSideSell, model.OrderTypeMarket,
			futureMarket, futureSymbol, model.AmountTypeContractNumber, futureBidAsk.Bids[0].Price, arbitraryAmount)
		actualAmount, actualPrice, status = api.SyncQueryOrderById(futureMarket, futureSymbol, orderId)
		actualAmount = actualAmount * faceValue / actualPrice
		util.Notice(fmt.Sprintf(`[!arbitrary future!]orderid:%s errCode:%s status:%s dealAmount:%f at price:%f`,
			orderId, errCode, status, actualAmount, actualPrice))
		carry := &model.Carry{Symbol: futureSymbol, AskWeb: futureMarket, AskPrice: actualPrice, DealAskStatus: status,
			AskTime: int64(futureBidAsk.Ts), SideType: model.CarryTypeArbitrarySell, DealAskAmount: actualAmount}
		model.CarryChannel <- *carry
	} else if arbitraryAmount < 0 {
		orderId, errCode, status, actualAmount, actualPrice := api.PlaceOrder(model.OrderSideLiquidateShort,
			model.OrderTypeMarket, futureMarket, futureSymbol, model.AmountTypeContractNumber,
			futureBidAsk.Asks[0].Price, -1*arbitraryAmount)
		actualAmount, actualPrice, status = api.SyncQueryOrderById(futureMarket, futureSymbol, orderId)
		actualAmount = actualAmount * faceValue / actualPrice
		util.Notice(fmt.Sprintf(`[!arbitrary future!]orderid:%s errCode:%s status:%s dealAmount:%f at price:%f`,
			orderId, errCode, status, actualAmount, actualPrice))
		carry := &model.Carry{Symbol: futureSymbol, AskWeb: futureMarket, BidPrice: actualPrice, DealBidStatus: status,
			BidTime: int64(futureBidAsk.Ts), SideType: model.CarryTypeArbitraryBuy, DealBidAmount: actualAmount}
		model.CarryChannel <- *carry
	}
}

func arbitraryMarket(market, symbol string, marketBidAsk *model.BidAsk) {
	index := strings.Index(symbol, `_`)
	if index == -1 {
		return
	}
	currency := symbol[0:index]
	accountCoin := model.AppAccounts.GetAccount(market, currency)
	if accountCoin == nil || marketBidAsk == nil || marketBidAsk.Bids == nil || marketBidAsk.Bids.Len() < 1 {
		return
	}
	if accountCoin.Free*marketBidAsk.Bids[0].Price > model.AppConfig.MinUsdt {
		orderId, errCode, status, actualAmount, actualPrice := api.PlaceOrder(model.OrderSideSell, model.OrderTypeMarket,
			market, symbol, model.AmountTypeCoinNumber, marketBidAsk.Bids[0].Price, accountCoin.Free)
		actualAmount, actualPrice, status = api.SyncQueryOrderById(market, symbol, orderId)
		carry := &model.Carry{Symbol: symbol, AskWeb: market, AskPrice: actualPrice, DealAskStatus: status,
			AskTime: int64(marketBidAsk.Ts), SideType: model.CarryTypeArbitrarySell, DealAskAmount: actualAmount}
		model.CarryChannel <- *carry
		api.RefreshAccount(market)
		util.Notice(fmt.Sprintf(`[!arbitrary!]orderid:%s errCode:%s status:%s dealAmount:%f at price:%f`,
			orderId, errCode, status, actualAmount, actualPrice))
	}
}

func openShort(symbol, market, futureSymbol, futureMarket string, futureBidAsk, bidAsk *model.BidAsk) {
	carry := &model.Carry{Symbol: futureSymbol, AskWeb: futureMarket, BidWeb: market, AskPrice: futureBidAsk.Bids[0].Price,
		BidPrice: bidAsk.Asks[0].Price, AskTime: int64(futureBidAsk.Ts), BidTime: int64(bidAsk.Ts), SideType: model.CarryTypeOpenShort}
	checkTime, msg := carry.CheckWorthCarryTime()
	if !checkTime {
		util.Notice(msg.Error())
		return
	}
	index := strings.Index(symbol, `_`)
	if index == -1 {
		return
	}
	currency := symbol[0:index]
	accountUsdt := model.AppAccounts.GetAccount(market, `usdt`)
	accountCoin := model.AppAccounts.GetAccount(market, currency)
	if accountUsdt == nil {
		util.Info(`account nil`)
		api.RefreshAccount(market)
		return
	}
	if accountUsdt.Free <= model.AppConfig.MinUsdt || accountUsdt.Free <= model.ArbitraryCarryUSDT {
		//util.Info(fmt.Sprintf(`账户usdt余额usdt%f不够买%f个%s`, account.Free, carry.Amount+1, symbol))
		return
	}
	util.Notice(`[open short]` + carry.ToString())
	carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus, carry.DealBidAmount, carry.BidPrice = api.PlaceOrder(
		model.OrderSideBuy, model.OrderTypeMarket, market, symbol, ``, carry.BidPrice, model.ArbitraryCarryUSDT)
	if carry.DealBidOrderId == `` || carry.DealBidOrderId == `0` {
		util.Notice(fmt.Sprintf(`[bid fail]%s %s price%f amount%f`, market, symbol, carry.AskPrice, carry.BidAmount))
		return
	}
	carry.DealBidAmount, carry.BidPrice, carry.DealBidStatus = api.SyncQueryOrderById(market, symbol, carry.DealBidOrderId)
	transferAmount := carry.DealBidAmount
	if accountCoin != nil {
		transferAmount += accountCoin.Free
	}
	transfer, errCode := api.MustFundTransferOkex(symbol, transferAmount, `1`, `3`)
	util.Notice(fmt.Sprintf(`transfer %f result %v %s`, transferAmount, transfer, errCode))
	if transfer {
		faceValue := model.OKEXOtherContractFaceValue
		if strings.Contains(symbol, `btc`) {
			faceValue = model.OKEXBTCContractFaceValue
		}
		accountRights, _, _, _ := api.GetAccountOkfuture(futureSymbol)
		futureAccount, _ := api.GetPositionOkfuture(futureMarket, futureSymbol)
		sellAmount := accountRights
		if futureAccount != nil {
			sellAmount = accountRights - futureAccount.OpenedShort*faceValue
		}
		carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskStatus, carry.DealAskAmount, carry.AskPrice =
			api.PlaceOrder(model.OrderSideSell, model.OrderTypeMarket, futureMarket, futureSymbol,
				model.AmountTypeCoinNumber, carry.AskPrice, sellAmount)
		if carry.DealAskOrderId != `` && carry.DealAskOrderId != `0` {
			carry.DealAskAmount, carry.AskPrice, carry.DealAskStatus = api.SyncQueryOrderById(futureMarket, futureSymbol,
				carry.DealAskOrderId)
			carry.DealAskAmount = faceValue * carry.DealAskAmount / carry.AskPrice
			//if carry.DealAskStatus == model.CarryStatusFail {
			//	arbitraryFutureMarket(model.OKFUTURE, futureSymbol, futureBidAsk)
			//}
		} else {
			util.Notice(`[!!Ask Fail]` + carry.DealAskErrCode + carry.DealAskStatus)
		}
		api.RefreshAccount(market)
	}
	model.CarryChannel <- *carry
}

func closeShort(symbol, market, futureSymbol, futureMarket string, bidAsk, futureBidAsk *model.BidAsk) {
	carry := &model.Carry{Symbol: futureSymbol, AskWeb: market, BidWeb: futureMarket, AskPrice: bidAsk.Bids[0].Price,
		BidPrice: futureBidAsk.Asks[0].Price, AskTime: int64(bidAsk.Ts), BidTime: int64(futureBidAsk.Ts), SideType: model.CarryTypeCloseShort}
	checkTime, msg := carry.CheckWorthCarryTime()
	if !checkTime {
		util.Notice(msg.Error())
		return
	}
	faceValue := model.OKEXOtherContractFaceValue
	if strings.Contains(symbol, `btc`) {
		faceValue = model.OKEXBTCContractFaceValue
	}
	futureAccount, err := api.GetPositionOkfuture(futureMarket, futureSymbol)
	if futureAccount == nil || err != nil {
		return
	}
	accountRights, realProfit, unrealProfit, _ := api.GetAccountOkfuture(futureSymbol)
	transferAble := accountRights
	if realProfit+unrealProfit > 0 {
		transferAble = accountRights - realProfit - unrealProfit
	}
	keepShort := math.Round((realProfit + unrealProfit) / faceValue)
	if futureAccount.OpenedShort <= keepShort || transferAble < model.ArbitraryCarryUSDT {
		return
	}
	util.Notice(`[close short]` + carry.ToString())
	carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus, carry.DealBidAmount, carry.BidPrice =
		api.PlaceOrder(model.OrderSideLiquidateShort, model.OrderTypeMarket, futureMarket, futureSymbol,
			model.AmountTypeContractNumber, carry.BidPrice, model.ArbitraryCarryUSDT/faceValue)
	if carry.DealBidOrderId == `` || carry.DealBidOrderId == `0` {
		util.Notice(fmt.Sprintf(`[bid fail]%s %s price%f amount%f`,
			futureMarket, futureSymbol, carry.BidPrice, carry.BidAmount))
		return
	}
	carry.DealBidAmount, carry.BidPrice, carry.DealBidStatus = api.SyncQueryOrderById(futureMarket, futureSymbol, carry.DealBidOrderId)
	carry.DealBidAmount = carry.DealBidAmount * faceValue / carry.BidPrice
	model.CarryChannel <- *carry
	//if carry.DealBidStatus != model.CarryStatusSuccess {
	//	arbitraryFutureMarket(model.OKFUTURE, futureSymbol, futureBidAsk)
	//	return
	//}
	accountRights, realProfit, unrealProfit, _ = api.GetAccountOkfuture(futureSymbol)
	futureAccount, err = api.GetPositionOkfuture(futureMarket, futureSymbol)
	if futureAccount == nil || err != nil {
		return
	}
	transferAble = accountRights - faceValue*futureAccount.OpenedShort
	if transferAble > accountRights-(realProfit+unrealProfit) {
		transferAble = accountRights - (realProfit + unrealProfit)
	}
	if transferAble < accountRights-futureAccount.OpenedShort*faceValue/futureBidAsk.Bids[0].Price {
		transferAble = accountRights - futureAccount.OpenedShort*faceValue/futureBidAsk.Bids[0].Price
	}
	if transferAble <= 0 {
		util.Notice(fmt.Sprintf(`transferAble %f <= 0`, transferAble))
	}
	transfer, errCode := api.MustFundTransferOkex(symbol, transferAble, `3`, `1`)
	util.Notice(fmt.Sprintf(`transfer %f result %v %s`, transferAble, transfer, errCode))
	if transfer {
		api.RefreshAccount(market)
		index := strings.Index(symbol, `_`)
		if index == -1 {
			return
		}
		currency := symbol[0:index]
		account := model.AppAccounts.GetAccount(market, currency)
		carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskStatus, carry.DealAskAmount, carry.AskPrice =
			api.PlaceOrder(model.OrderSideSell, model.OrderTypeMarket, market, symbol, model.AmountTypeCoinNumber,
				carry.AskPrice, account.Free)
		if carry.DealAskOrderId != `` && carry.DealAskOrderId != `0` {
			carry.DealAskAmount, carry.AskPrice, _ = api.SyncQueryOrderById(market, symbol, carry.DealAskOrderId)
		} else {
			util.Notice(`[!!Ask Fail]` + carry.DealAskErrCode + carry.DealAskStatus)
		}
		api.RefreshAccount(market)
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
	pendingAmount, _ := api.QueryPendingOrderAmount(futureSymbol)
	if pendingAmount > 0 {
		util.Notice(fmt.Sprintf(`[wait pending future orders] %d`, pendingAmount))
		return
	}
	if setting.OpenShortMargin < openShortMargin {
		openShort(symbol, model.OKEX, futureSymbol, futureMarket, futureBidAsk, bidAsk)
		if setting.CloseShortMargin > closeShortMargin {
			util.Info(fmt.Sprintf("[logic error]future market - market \n %f %f \n %f %f \n %f %f \n %f %f",
				futureBidAsk.Asks[1].Price, bidAsk.Asks[1].Price, futureBidAsk.Asks[0].Price, bidAsk.Asks[0].Price,
				futureBidAsk.Bids[0].Price, bidAsk.Bids[0].Price, futureBidAsk.Bids[1].Price, bidAsk.Bids[1].Price))
		}
	} else if setting.CloseShortMargin > closeShortMargin {
		closeShort(symbol, model.OKEX, futureSymbol, futureMarket, bidAsk, futureBidAsk)
	}
	if util.GetNow().Second() == 0 { //每分钟检查一次
		util.Info(fmt.Sprintf(`[open short %t]%f - %f [close short %t] %f - %f`,
			setting.OpenShortMargin < openShortMargin, openShortMargin, setting.OpenShortMargin,
			setting.CloseShortMargin > closeShortMargin, closeShortMargin, setting.CloseShortMargin))
		arbitraryMarket(model.OKEX, symbol, bidAsk)
		arbitraryFutureMarket(model.OKFUTURE, futureSymbol, futureBidAsk)
	}
}
