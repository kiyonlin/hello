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

func arbitraryFutureMarket(futureMarket, futureSymbol string, futureBidAsk *model.BidAsk, faceValue float64) {
	if checkPending(futureSymbol) {
		return
	}
	accountRights, _, _, accountErr := api.GetAccountOkfuture(model.AppAccounts, futureSymbol)
	allHoldings, allHoldingsErr := api.GetAllHoldings(futureSymbol)
	futureSymbolHoldings, futureSymbolHoldingErr := api.GetPositionOkfuture(model.OKFUTURE, futureSymbol)
	if futureBidAsk == nil || futureBidAsk.Bids == nil || len(futureBidAsk.Bids) < 1 || accountErr != nil ||
		allHoldingsErr != nil || futureSymbolHoldingErr != nil || futureSymbolHoldings == nil {
		return
	}
	//util.Info(fmt.Sprintf(`arbitrary future with %s %f of %f`, futureSymbol, futureSymbolHoldings.OpenedShort, allHoldings))
	arbitraryAmount := math.Floor(accountRights*futureBidAsk.Bids[0].Price/faceValue - allHoldings)
	if arbitraryAmount*faceValue > model.ArbitraryCarryUSDT {
		orderId, errCode, status, actualAmount, actualPrice := api.PlaceOrder(model.OrderSideSell, model.OrderTypeMarket,
			futureMarket, futureSymbol, model.AmountTypeContractNumber, futureBidAsk.Bids[0].Price, arbitraryAmount)
		actualAmount, actualPrice, status = api.SyncQueryOrderById(futureMarket, futureSymbol, orderId)
		if actualPrice > 0 {
			actualAmount = actualAmount * faceValue / actualPrice
		}
		util.Notice(fmt.Sprintf(`[!arbitrary future!]orderid:%s errCode:%s status:%s dealAmount:%f at price:%f`,
			orderId, errCode, status, actualAmount, actualPrice))
		carry := &model.Carry{Symbol: futureSymbol, AskWeb: futureMarket, DealAskPrice: actualPrice, DealAskStatus: status,
			AskTime: int64(futureBidAsk.Ts), SideType: model.CarryTypeArbitrarySell, DealAskAmount: actualAmount}
		model.CarryChannel <- *carry
	} else if arbitraryAmount*faceValue < -1*model.ArbitraryCarryUSDT && futureSymbolHoldings.OpenedShort*faceValue >
		model.ArbitraryCarryUSDT {
		orderId, errCode, status, actualAmount, actualPrice := api.PlaceOrder(model.OrderSideLiquidateShort,
			model.OrderTypeMarket, futureMarket, futureSymbol, model.AmountTypeContractNumber,
			futureBidAsk.Asks[0].Price, model.ArbitraryCarryUSDT/faceValue)
		actualAmount, actualPrice, status = api.SyncQueryOrderById(futureMarket, futureSymbol, orderId)
		actualAmount = actualAmount * faceValue / actualPrice
		util.Notice(fmt.Sprintf(`[!arbitrary future!]orderid:%s errCode:%s status:%s dealAmount:%f at price:%f`,
			orderId, errCode, status, actualAmount, actualPrice))
		carry := &model.Carry{Symbol: futureSymbol, AskWeb: futureMarket, DealBidPrice: actualPrice, DealBidStatus: status,
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
		carry := &model.Carry{Symbol: symbol, AskWeb: market, DealAskPrice: actualPrice, DealAskStatus: status,
			AskTime: int64(marketBidAsk.Ts), SideType: model.CarryTypeArbitrarySell, DealAskAmount: actualAmount}
		model.CarryChannel <- *carry
		api.RefreshAccount(market)
		util.Notice(fmt.Sprintf(`[!arbitrary!]orderid:%s errCode:%s status:%s dealAmount:%f at price:%f`,
			orderId, errCode, status, actualAmount, actualPrice))
	}
}

func openShort(carry *model.Carry, faceValue float64) {
	index := strings.Index(carry.BidSymbol, `_`)
	if index == -1 {
		return
	}
	currency := carry.BidSymbol[0:index]
	accountUsdt := model.AppAccounts.GetAccount(carry.BidWeb, `usdt`)
	accountCoin := model.AppAccounts.GetAccount(carry.BidWeb, currency)
	if accountUsdt == nil {
		util.Info(`account nil`)
		api.RefreshAccount(carry.BidWeb)
		return
	}
	if accountUsdt.Free <= model.AppConfig.MinUsdt || accountUsdt.Free <= model.ArbitraryCarryUSDT {
		//util.Info(fmt.Sprintf(`账户usdt余额usdt%f不够买%f个%s`, account.Free, carry.Amount+1, symbol))
		return
	}
	util.Notice(`[open short]` + carry.ToString())
	carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus, carry.DealBidAmount, carry.BidPrice = api.PlaceOrder(
		model.OrderSideBuy, model.OrderTypeMarket, carry.BidWeb, carry.BidSymbol, ``,
		carry.BidPrice, model.ArbitraryCarryUSDT)
	if carry.DealBidOrderId == `` || carry.DealBidOrderId == `0` {
		util.Notice(fmt.Sprintf(`[bid fail]%s %s price%f amount%f`, carry.BidWeb, carry.BidSymbol,
			carry.AskPrice, carry.BidAmount))
		return
	}
	carry.DealBidAmount, carry.DealBidPrice, carry.DealBidStatus = api.SyncQueryOrderById(carry.BidWeb, carry.BidSymbol,
		carry.DealBidOrderId)
	api.RefreshAccount(carry.BidWeb)
	model.CarryChannel <- *carry
	transferAmount := carry.DealBidAmount
	if accountCoin != nil {
		transferAmount += accountCoin.Free
	}
	transfer, errCode := api.MustFundTransferOkex(carry.BidSymbol, transferAmount, `1`, `3`)
	util.Notice(fmt.Sprintf(`transfer %f result %v %s`, transferAmount, transfer, errCode))
	if transfer {
		buyShort(carry, faceValue)
	}
}

func buyShort(carry *model.Carry, faceValue float64) {
	accountRights, _, _, accountErr := api.GetAccountOkfuture(model.AppAccounts, carry.AskSymbol)
	allHoldings, allHoldingErr := api.GetAllHoldings(carry.AskSymbol)
	if accountErr != nil || allHoldingErr != nil {
		return
	}
	sellAmount := accountRights - allHoldings*faceValue/carry.DealAskPrice
	if sellAmount >= 0 {
		carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskStatus, carry.DealAskAmount, carry.DealAskPrice =
			api.PlaceOrder(model.OrderSideSell, model.OrderTypeMarket, carry.AskWeb, carry.AskSymbol,
				model.AmountTypeCoinNumber, carry.AskPrice, sellAmount)
		if carry.DealAskOrderId != `` && carry.DealAskOrderId != `0` {
			carry.DealAskAmount, carry.DealAskPrice, carry.DealAskStatus = api.SyncQueryOrderById(carry.AskWeb,
				carry.AskSymbol, carry.DealAskOrderId)
			if carry.DealAskPrice > 0 {
				carry.DealAskAmount = faceValue * carry.DealAskAmount / carry.DealAskPrice
			}
		} else {
			util.Notice(`[!!Ask Fail]` + carry.DealAskErrCode + carry.DealAskStatus)
		}
	}
	model.CarryChannel <- *carry
}

func liquidShort(carry *model.Carry, faceValue float64) {
	allHoldings, allHoldingErr := api.GetAllHoldings(carry.BidSymbol)
	futureSymbolHoldings, futureSymbolHoldingErr := api.GetPositionOkfuture(model.OKFUTURE, carry.BidSymbol)
	accountRights, realProfit, unrealProfit, accountErr := api.GetAccountOkfuture(model.AppAccounts, carry.BidSymbol)
	if allHoldingErr != nil || accountErr != nil || futureSymbolHoldingErr != nil || futureSymbolHoldings == nil {
		return
	}
	keepShort := math.Round((realProfit + unrealProfit) * carry.BidPrice / faceValue)
	if allHoldings <= keepShort {
		return
	}
	liquidAmount := math.Round(accountRights * carry.BidPrice / faceValue)
	if realProfit+unrealProfit > 0 {
		liquidAmount = math.Round((accountRights - realProfit - unrealProfit) * carry.BidPrice / faceValue)
	}
	if liquidAmount > model.ArbitraryCarryUSDT/faceValue {
		liquidAmount = math.Round(model.ArbitraryCarryUSDT / faceValue)
	}
	if liquidAmount > futureSymbolHoldings.OpenedShort {
		liquidAmount = futureSymbolHoldings.OpenedShort
	}
	if liquidAmount <= 0 {
		return
	}
	util.Notice(`[close short]` + carry.ToString())
	carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus, carry.DealBidAmount, carry.BidPrice =
		api.PlaceOrder(model.OrderSideLiquidateShort, model.OrderTypeMarket, carry.BidWeb, carry.BidSymbol,
			model.AmountTypeContractNumber, carry.BidPrice, liquidAmount)
	if carry.DealBidOrderId == `` || carry.DealBidOrderId == `0` {
		util.Notice(fmt.Sprintf(`[bid fail]%s %s price%f amount%f`,
			carry.BidWeb, carry.BidSymbol, carry.BidPrice, carry.BidAmount))
		return
	}
	carry.DealBidAmount, carry.DealBidPrice, carry.DealBidStatus = api.SyncQueryOrderById(carry.BidWeb, carry.BidSymbol,
		carry.DealBidOrderId)
	if carry.BidPrice > 0 {
		carry.DealBidAmount = carry.DealBidAmount * faceValue / carry.BidPrice
	}
	model.CarryChannel <- *carry
}

func jumpShort(carry *model.Carry, faceValue float64) {
	liquidShort(carry, faceValue)
	buyShort(carry, faceValue)
}

func closeShort(carry *model.Carry, faceValue float64) {
	liquidShort(carry, faceValue)
	allHoldings, allHoldingErr := api.GetAllHoldings(carry.BidSymbol)
	accountRights, realProfit, unrealProfit, accountErr := api.GetAccountOkfuture(model.AppAccounts, carry.BidSymbol)
	if allHoldingErr != nil || accountErr != nil {
		return
	}
	transferAble := accountRights - allHoldings*faceValue/carry.BidPrice
	if transferAble > accountRights-(realProfit+unrealProfit) {
		transferAble = accountRights - (realProfit + unrealProfit)
	}
	if transferAble <= 0 {
		util.Notice(fmt.Sprintf(`transferAble %f <= 0`, transferAble))
		return
	}
	transfer, errCode := api.MustFundTransferOkex(carry.AskSymbol, transferAble, `3`, `1`)
	util.Notice(fmt.Sprintf(`transfer %f result %v %s`, transferAble, transfer, errCode))
	if transfer {
		api.RefreshAccount(carry.AskWeb)
		index := strings.Index(carry.AskSymbol, `_`)
		if index == -1 {
			return
		}
		currency := carry.AskSymbol[0:index]
		account := model.AppAccounts.GetAccount(carry.AskWeb, currency)
		carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskStatus, carry.DealAskAmount, carry.DealAskPrice =
			api.PlaceOrder(model.OrderSideSell, model.OrderTypeMarket, carry.AskWeb, carry.AskSymbol,
				model.AmountTypeCoinNumber, carry.AskPrice, account.Free)
		if carry.DealAskOrderId != `` && carry.DealAskOrderId != `0` {
			carry.DealAskAmount, carry.DealAskPrice, _ = api.SyncQueryOrderById(carry.AskWeb, carry.AskWeb, carry.DealAskOrderId)
		} else {
			util.Notice(`[!!Ask Fail]` + carry.DealAskErrCode + carry.DealAskStatus)
		}
		api.RefreshAccount(carry.AskWeb)
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

func checkPending(symbol string) bool {
	pendingAmount, _ := api.QueryPendingOrderAmount(symbol)
	if pendingAmount > 0 {
		util.Notice(fmt.Sprintf(`[wait pending future orders] %d`, pendingAmount))
		return true
	}
	return false
}

func createCarry(symbol, futureSymbol, futureMarket string) *model.Carry {
	index := strings.Index(futureSymbol, `_`)
	if futureMarket != model.OKFUTURE || index < 0 {
		return nil
	}
	settings := model.GetSettings(futureMarket, futureSymbol[0:index+1])
	symbolSetting := model.GetSetting(model.OKEX, symbol)
	if symbolSetting != nil {
		settings = append(settings, symbolSetting)
	}
	var bidSetting, askSetting *model.Setting
	bestSellPrice := 0.0
	bestBuyPrice := 0.0
	for _, value := range settings {
		if futureSymbol == value.Symbol || model.AppMarkets.BidAsks[value.Symbol] == nil ||
			model.AppMarkets.BidAsks[value.Symbol][value.Market] == nil {
			continue
		}
		worth := model.AppMarkets.BidAsks[value.Symbol][value.Market].Bids[0].Amount *
			model.AppMarkets.BidAsks[value.Symbol][value.Market].Bids[0].Price
		if worth > model.ArbitraryCarryUSDT && model.AppMarkets.BidAsks[value.Symbol][value.Market].Bids[0].Price > bestSellPrice {
			bestSellPrice = model.AppMarkets.BidAsks[value.Symbol][value.Market].Bids[0].Price
			askSetting = value
		}
		worth = model.AppMarkets.BidAsks[value.Symbol][value.Market].Asks[0].Amount *
			model.AppMarkets.BidAsks[value.Symbol][value.Market].Asks[0].Price
		if worth > model.ArbitraryCarryUSDT &&
			(bestBuyPrice == 0.0 || bestBuyPrice > model.AppMarkets.BidAsks[value.Symbol][value.Market].Asks[0].Price) {
			bidSetting = value
			bestBuyPrice = model.AppMarkets.BidAsks[value.Symbol][value.Market].Asks[0].Price
		}
	}
	if bidSetting == nil || askSetting == nil || (futureSymbol != bidSetting.Symbol && futureSymbol != askSetting.Symbol) {
		return nil
	}
	margin := (bestSellPrice - bestBuyPrice) / bestSellPrice
	if margin < -1*askSetting.CloseShortMargin || margin < bidSetting.OpenShortMargin || margin < 0.0025 {
		return nil
	}
	carry := &model.Carry{AskSymbol: askSetting.Symbol, BidSymbol: bidSetting.Symbol, AskWeb: askSetting.Market,
		BidWeb: bidSetting.Market, AskPrice: bestSellPrice, BidPrice: bestBuyPrice, SideType: model.CarryTypeFuture,
		AskTime: int64(model.AppMarkets.BidAsks[askSetting.Symbol][askSetting.Market].Ts),
		BidTime: int64(model.AppMarkets.BidAsks[bidSetting.Symbol][bidSetting.Market].Ts)}
	checkTime, msg := carry.CheckWorthCarryTime()
	if !checkTime {
		util.Notice(msg.Error())
		return nil
	}
	//util.Info(fmt.Sprintf(`%s [open short %t]%f - %f`, futureSymbol,
	//	setting.OpenShortMargin < openShortMargin, openShortMargin, setting.OpenShortMargin))
	return carry
}

var ProcessContractArbitrage = func(futureSymbol, futureMarket string) {
	if contractArbitraging || futureMarket != model.OKFUTURE || checkPending(futureSymbol) {
		return
	}
	setContractArbitraging(true)
	defer setContractArbitraging(false)
	symbol := getSymbol(futureSymbol)
	carry := createCarry(symbol, futureSymbol, futureMarket)
	if carry == nil {
		return
	}
	faceValue := model.OKEXOtherContractFaceValue
	if strings.Contains(futureSymbol, `btc`) {
		faceValue = model.OKEXBTCContractFaceValue
	}
	if carry.AskWeb == model.OKEX && carry.BidWeb == model.OKFUTURE {
		closeShort(carry, faceValue)
	} else if carry.AskWeb == model.OKFUTURE && carry.BidWeb == model.OKEX {
		openShort(carry, faceValue)
	} else if carry.AskWeb == model.OKFUTURE && carry.BidWeb == model.OKFUTURE {
		jumpShort(carry, faceValue)
	}
	if util.GetNow().Second() == 0 { //每分钟检查一次
		arbitraryMarket(model.OKEX, symbol, model.AppMarkets.BidAsks[symbol][model.OKEX])
		arbitraryFutureMarket(model.OKFUTURE, futureSymbol, model.AppMarkets.BidAsks[futureSymbol][model.OKFUTURE], faceValue)
	}
}
