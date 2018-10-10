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

func recordCarry(carry *model.Carry) {
	if carry.SideType == model.CarryTypeFuture && carry.DealAskPrice > 0 && carry.DealBidPrice > carry.DealAskPrice {
		delta := (carry.DealAskPrice - carry.DealBidPrice) / carry.DealAskPrice
		openSetting := model.GetSetting(carry.BidWeb, carry.BidSymbol)
		closeSetting := model.GetSetting(carry.AskWeb, carry.AskSymbol)
		util.Notice(fmt.Sprintf(`[modify setting]open: %s %s %f+%f close: %s %s %f+%f`,
			carry.BidWeb, carry.BidSymbol, openSetting.OpenShortMargin, delta,
			carry.AskWeb, carry.AskSymbol, closeSetting.CloseShortMargin, delta))
		openSetting.OpenShortMargin += delta
		closeSetting.CloseShortMargin += delta
		model.AppDB.Save(openSetting)
		model.AppDB.Save(closeSetting)
		model.LoadSettings()
	}
	model.CarryChannel <- *carry
}

func arbitraryFutureMarket(futureSymbol string, futureBidAsk *model.BidAsk, faceValue float64) {
	if checkPending(futureSymbol) {
		return
	}
	util.Info(fmt.Sprintf(`check to arbitrary %s`, futureSymbol))
	accountRights, realProfit, unrealProfit, accountErr := api.GetAccountOkfuture(model.AppAccounts, futureSymbol)
	allHoldings, allHoldingsErr := api.GetAllHoldings(futureSymbol)
	if futureBidAsk == nil || futureBidAsk.Bids == nil || len(futureBidAsk.Bids) < 1 ||
		accountErr != nil || allHoldingsErr != nil {
		util.Notice(fmt.Sprintf(`fail to get allholdings and position and holding`))
		return
	}
	transferAble := accountRights - allHoldings*faceValue/futureBidAsk.Bids[0].Price
	if transferAble > 0 {
		if transferAble > accountRights-(realProfit+unrealProfit) {
			transferAble = accountRights - (realProfit + unrealProfit)
		}
		if transferAble*futureBidAsk.Bids[0].Price <= model.AppConfig.MinUsdt {
			util.Notice(fmt.Sprintf(`%s transferAble %f <= %f`, futureSymbol, transferAble, model.AppConfig.MinUsdt))
			return
		}
		transfer, errCode := api.MustFundTransferOkex(futureSymbol, transferAble, `3`, `1`)
		util.Notice(fmt.Sprintf(`[arbitrary transfer]%f %s 3->1 %v %s`, transferAble, futureSymbol, transfer, errCode))
	} else if transferAble*(-1) < faceValue {
		bidAmount := getBidAmount(model.OKFUTURE, futureSymbol, faceValue, futureBidAsk.Asks[0].Price)
		if bidAmount > 0 {
			util.Notice(fmt.Sprintf(`[%s]%s price at %f transferable %f facevalue %f`,
				model.CarryTypeArbitraryBuy, futureSymbol, futureBidAsk.Asks[0].Price, transferAble, faceValue))
			carry := &model.Carry{BidSymbol: futureSymbol, BidWeb: model.OKFUTURE, BidPrice: futureBidAsk.Asks[0].Price,
				BidTime: int64(futureBidAsk.Ts), BidAmount: 1, SideType: model.CarryTypeArbitraryBuy}
			if liquidShort(carry, faceValue) {
				recordCarry(carry)
			}
		}
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
		if orderId == `` || orderId == `0` {
			util.Notice(fmt.Sprintf(`[arbitrary sell fail]%s %s price%f amount%f`, market, symbol,
				marketBidAsk.Bids[0].Price, accountCoin.Free))
			return
		}
		actualAmount, actualPrice, status = api.SyncQueryOrderById(market, symbol, orderId)
		carry := &model.Carry{AskSymbol: symbol, AskWeb: market, DealAskPrice: actualPrice,
			AskPrice: marketBidAsk.Bids[0].Price, DealAskStatus: status, AskTime: int64(marketBidAsk.Ts),
			SideType: model.CarryTypeArbitrarySell, DealAskAmount: actualAmount}
		recordCarry(carry)
		api.RefreshAccount(market)
		util.Notice(fmt.Sprintf(`[!arbitrary!]orderid:%s errCode:%s status:%s dealAmount:%f at price:%f`,
			orderId, errCode, status, actualAmount, actualPrice))
	}
}

func getBidAmount(market, symbol string, faceValue, bidPrice float64) (amount float64) {
	if market == model.OKEX {
		index := strings.Index(symbol, `_`)
		if index == -1 {
			return 0
		}
		accountUsdt := model.AppAccounts.GetAccount(market, `usdt`)
		if accountUsdt == nil {
			util.Info(`account nil`)
			api.RefreshAccount(market)
			return 0
		}
		if accountUsdt.Free <= model.AppConfig.MinUsdt || accountUsdt.Free <= model.ArbitraryCarryUSDT {
			//util.Info(fmt.Sprintf(`账户usdt余额usdt%f不够买%f个%s`, account.Free, carry.Amount+1, symbol))
			return 0
		}
		return model.ArbitraryCarryUSDT
	} else if market == model.OKFUTURE {
		allHoldings, allHoldingErr := api.GetAllHoldings(symbol)
		futureSymbolHoldings, futureSymbolHoldingErr := api.GetPositionOkfuture(market, symbol)
		accountRights, realProfit, unrealProfit, accountErr := api.GetAccountOkfuture(model.AppAccounts, symbol)
		if allHoldingErr != nil || accountErr != nil || futureSymbolHoldingErr != nil || futureSymbolHoldings == nil {
			util.Notice(fmt.Sprintf(`fail to get allholdings and position and holding`))
			return 0
		}
		keepShort := math.Round((realProfit + unrealProfit) * bidPrice / faceValue)
		if allHoldings <= keepShort {
			//util.Notice(fmt.Sprintf(`allholding <= keep %f %f`, allHoldings, keepShort))
			return 0
		}
		liquidAmount := math.Round(accountRights * bidPrice / faceValue)
		if realProfit+unrealProfit > 0 {
			liquidAmount = math.Round((accountRights - realProfit - unrealProfit) * bidPrice / faceValue)
		}
		if liquidAmount > futureSymbolHoldings.OpenedShort {
			liquidAmount = futureSymbolHoldings.OpenedShort
		}
		if liquidAmount > model.ArbitraryCarryUSDT/faceValue {
			liquidAmount = math.Round(model.ArbitraryCarryUSDT / faceValue)
		}
		return liquidAmount
	}
	return 0
}

func openShort(carry *model.Carry, faceValue float64) {
	util.Notice(`[open short]` + carry.ToString())
	carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus, carry.DealBidAmount, carry.DealBidPrice =
		api.PlaceOrder(model.OrderSideBuy, model.OrderTypeMarket, carry.BidWeb, carry.BidSymbol, ``,
			carry.BidPrice, carry.BidAmount)
	if carry.DealBidOrderId == `` || carry.DealBidOrderId == `0` {
		util.Notice(fmt.Sprintf(`[bid fail]%s %s price%f amount in usd %f`, carry.BidWeb, carry.BidSymbol,
			carry.AskPrice, model.ArbitraryCarryUSDT))
		return
	}
	carry.DealBidAmount, carry.DealBidPrice, carry.DealBidStatus = api.SyncQueryOrderById(carry.BidWeb, carry.BidSymbol,
		carry.DealBidOrderId)
	api.RefreshAccount(carry.BidWeb)
	transferAmount := carry.DealBidAmount
	if transferAmount*carry.DealBidPrice <= model.AppConfig.MinUsdt {
		util.Notice(fmt.Sprintf(`%s transferAble %f <= %f in usd`, carry.BidSymbol, transferAmount,
			model.AppConfig.MinUsdt))
		return
	}
	transfer, errCode := api.MustFundTransferOkex(carry.BidSymbol, transferAmount, `1`, `3`)
	util.Notice(fmt.Sprintf(`%s transfer %f result %v %s`, carry.BidSymbol, transferAmount, transfer, errCode))
	if transfer {
		buyShort(carry, faceValue)
	}
	recordCarry(carry)
}

func buyShort(carry *model.Carry, faceValue float64) bool {
	accountRights, _, _, accountErr := api.GetAccountOkfuture(model.AppAccounts, carry.AskSymbol)
	allHoldings, allHoldingErr := api.GetAllHoldings(carry.AskSymbol)
	if accountErr != nil || allHoldingErr != nil {
		util.Notice(fmt.Sprintf(`fail to get allholdings and position`))
		return false
	}
	sellAmount := accountRights - allHoldings*faceValue/carry.AskPrice
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
		return true
	}
	return false
}

func liquidShort(carry *model.Carry, faceValue float64) bool {
	util.Notice(`[liquid short]` + carry.ToString())
	carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus, carry.DealBidAmount, carry.DealBidPrice =
		api.PlaceOrder(model.OrderSideLiquidateShort, model.OrderTypeMarket, carry.BidWeb, carry.BidSymbol,
			model.AmountTypeContractNumber, carry.BidPrice, carry.BidAmount)
	if carry.DealBidOrderId == `` || carry.DealBidOrderId == `0` {
		util.Notice(fmt.Sprintf(`[bid fail]%s %s price%f amount in contract number %f`,
			carry.BidWeb, carry.BidSymbol, carry.BidPrice, carry.BidAmount))
		return false
	}
	carry.DealBidAmount, carry.DealBidPrice, carry.DealBidStatus = api.SyncQueryOrderById(carry.BidWeb, carry.BidSymbol,
		carry.DealBidOrderId)
	if carry.DealBidPrice > 0 {
		carry.DealBidAmount = carry.DealBidAmount * faceValue / carry.DealBidPrice
	}
	return true
}

func jumpShort(carry *model.Carry, faceValue float64) {
	util.Notice(`[jump short]` + carry.ToString())
	if !liquidShort(carry, faceValue) {
		return
	}
	buyShort(carry, faceValue)
	recordCarry(carry)
}

func closeShort(carry *model.Carry, faceValue float64) {
	util.Notice(`[close short]` + carry.ToString())
	if !liquidShort(carry, faceValue) {
		return
	}
	defer recordCarry(carry)
	allHoldings, allHoldingErr := api.GetAllHoldings(carry.BidSymbol)
	accountRights, realProfit, unrealProfit, accountErr := api.GetAccountOkfuture(model.AppAccounts, carry.BidSymbol)
	if allHoldingErr != nil || accountErr != nil {
		util.Notice(fmt.Sprintf(`fail to get allholdings and position`))
		return
	}
	transferAble := accountRights - allHoldings*faceValue/carry.DealBidPrice
	if transferAble > accountRights-(realProfit+unrealProfit) {
		transferAble = accountRights - (realProfit + unrealProfit)
	}
	if transferAble*carry.DealBidPrice <= model.AppConfig.MinUsdt {
		util.Notice(fmt.Sprintf(`%s transferAble %f <= %f`, carry.BidWeb, transferAble, model.AppConfig.MinUsdt))
		return
	}
	transfer, errCode := api.MustFundTransferOkex(carry.AskSymbol, transferAble, `3`, `1`)
	util.Notice(fmt.Sprintf(`%s transfer %f result %v %s`, carry.AskSymbol, transferAble, transfer, errCode))
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
			carry.DealAskAmount, carry.DealAskPrice, _ = api.SyncQueryOrderById(carry.AskWeb, carry.AskSymbol,
				carry.DealAskOrderId)
		} else {
			util.Notice(`[!!Ask Fail]` + carry.DealAskErrCode + carry.DealAskStatus)
		}
		api.RefreshAccount(carry.AskWeb)
	}
}

func checkPending(symbol string) bool {
	pendingAmount, _ := api.QueryPendingOrderAmount(symbol)
	if pendingAmount > 0 {
		util.Notice(fmt.Sprintf(`[wait pending future orders] %d`, pendingAmount))
		return true
	}
	return false
}

func createCarry(symbol, futureSymbol, futureMarket string, faceValue float64) *model.Carry {
	index := strings.Index(futureSymbol, `_`)
	if futureMarket != model.OKFUTURE || index < 0 {
		return nil
	}
	settings := model.GetSettings(futureMarket, futureSymbol[0:index+1])
	symbolSetting := model.GetSetting(model.OKEX, symbol)
	if symbolSetting != nil {
		settings = append(settings, symbolSetting)
	}
	var askSetting *model.Setting
	bestSellPrice := 0.0
	for _, value := range settings {
		if model.AppMarkets.BidAsks[value.Symbol] == nil || model.AppMarkets.BidAsks[value.Symbol][value.Market] == nil {
			continue
		}
		if model.AppMarkets.BidAsks[value.Symbol][value.Market].Bids[0].Price > bestSellPrice {
			bestSellPrice = model.AppMarkets.BidAsks[value.Symbol][value.Market].Bids[0].Price
			askSetting = value
		}
	}
	bidSetting := askSetting
	bestBuyPrice := model.AppMarkets.BidAsks[askSetting.Symbol][askSetting.Market].Asks[0].Price
	bidAmount := 0.0
	margin := 0.0
	for _, value := range settings {
		if model.AppMarkets.BidAsks[value.Symbol] == nil || model.AppMarkets.BidAsks[value.Symbol][value.Market] == nil {
			continue
		}
		if bestBuyPrice > model.AppMarkets.BidAsks[value.Symbol][value.Market].Asks[0].Price {
			margin = (bestSellPrice - bestBuyPrice) / bestSellPrice
			if margin < askSetting.CloseShortMargin || margin < bidSetting.OpenShortMargin {
				if util.GetNow().Second() == 0 {
					util.Info(fmt.Sprintf(`[no margin]%f %f/%f %s/%s->%s/%s`, margin, bidSetting.OpenShortMargin,
						askSetting.CloseShortMargin, bidSetting.Market, bidSetting.Symbol, askSetting.Market, askSetting.Symbol))
				}
			} else {
				checkBidAmount := getBidAmount(value.Market, value.Symbol, faceValue,
					model.AppMarkets.BidAsks[value.Symbol][value.Market].Asks[0].Price)
				if checkBidAmount > 0 {
					bidAmount = checkBidAmount
					bestBuyPrice = model.AppMarkets.BidAsks[value.Symbol][value.Market].Asks[0].Price
					bidSetting = value
				} else {
					util.Notice(fmt.Sprintf(`[no amount bid]%s-%s %f`, value.Market, value.Symbol, bidAmount))
				}
			}
		}
	}
	if bidSetting == nil || askSetting == nil || bidAmount == 0 ||
		(futureSymbol != bidSetting.Symbol && futureSymbol != askSetting.Symbol) {
		return nil
	}
	carry := &model.Carry{AskSymbol: askSetting.Symbol, BidSymbol: bidSetting.Symbol, AskWeb: askSetting.Market,
		BidWeb: bidSetting.Market, AskPrice: bestSellPrice, BidPrice: bestBuyPrice, SideType: model.CarryTypeFuture,
		BidAmount: bidAmount, AskTime: int64(model.AppMarkets.BidAsks[askSetting.Symbol][askSetting.Market].Ts),
		BidTime: int64(model.AppMarkets.BidAsks[bidSetting.Symbol][bidSetting.Market].Ts)}
	checkTime, msg := carry.CheckWorthCarryTime()
	if !checkTime {
		util.Notice(`[not in time]` + msg.Error())
		return nil
	}
	return carry
}

var lastArbitraryTime map[string]int64 // currency - time in million second

func needArbitrary(currency string) bool {
	if lastArbitraryTime == nil {
		lastArbitraryTime = make(map[string]int64)
	}
	if util.GetNowUnixMillion()-lastArbitraryTime[currency] > 600000 {
		lastArbitraryTime[currency] = util.GetNowUnixMillion()
		return true
	}
	return false
}

var ProcessContractArbitrage = func(futureSymbol, futureMarket string) {
	if contractArbitraging || futureMarket != model.OKFUTURE {
		return
	}
	setContractArbitraging(true)
	defer setContractArbitraging(false)
	index := strings.Index(futureSymbol, `_`)
	if index < 0 {
		return
	}
	currency := futureSymbol[0:index]
	symbol := currency + `_usdt`
	faceValue := model.OKEXOtherContractFaceValue
	if strings.Contains(futureSymbol, `btc`) {
		faceValue = model.OKEXBTCContractFaceValue
	}
	if needArbitrary(currency) {
		arbitraryFutureMarket(futureSymbol, model.AppMarkets.BidAsks[futureSymbol][model.OKFUTURE], faceValue)
		arbitraryMarket(model.OKEX, symbol, model.AppMarkets.BidAsks[symbol][model.OKEX])
	}
	carry := createCarry(symbol, futureSymbol, futureMarket, faceValue)
	if carry == nil || checkPending(futureSymbol) {
		return
	}
	if carry.AskWeb == model.OKEX && carry.BidWeb == model.OKFUTURE {
		closeShort(carry, faceValue)
	} else if carry.AskWeb == model.OKFUTURE && carry.BidWeb == model.OKEX {
		openShort(carry, faceValue)
	} else if carry.AskWeb == model.OKFUTURE && carry.BidWeb == model.OKFUTURE {
		jumpShort(carry, faceValue)
	}
	time.Sleep(time.Second * 3)
}
