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

var lastArbitraryTime map[string]int64 // currency - time in million second

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
		openSetting.OpenShortMargin = openSetting.OpenShortMargin + math.Abs(delta)
		closeSetting.CloseShortMargin = openSetting.CloseShortMargin + math.Abs(delta)
		model.AppDB.Save(openSetting)
		model.AppDB.Save(closeSetting)
	}
	model.CarryChannel <- *carry
}

func arbitraryFutureMarket(futureSymbol string, futureBidAsk *model.BidAsk, faceValue float64) {
	currency, err := util.GetCurrencyFromSymbol(futureSymbol)
	if checkPending(futureSymbol) || err != nil {
		return
	}
	util.Info(fmt.Sprintf(`check to arbitrary %s`, futureSymbol))
	accountErr := api.GetAccountOkfuture(model.AppAccounts)
	allHoldings, allHoldingsErr := api.GetAllHoldings(futureSymbol)
	account := model.AppAccounts.GetAccount(model.OKFUTURE, currency)
	if futureBidAsk == nil || futureBidAsk.Bids == nil || len(futureBidAsk.Bids) < 1 ||
		accountErr != nil || allHoldingsErr != nil || account == nil {
		util.Notice(fmt.Sprintf(`fail to get allholdings and position and holding`))
		return
	}
	transferAble := account.Free - allHoldings*faceValue/futureBidAsk.Bids[0].Price
	if transferAble > 0 {
		if transferAble > account.Free-(account.ProfitReal+account.ProfitUnreal) {
			transferAble = account.Free - (account.ProfitReal + account.ProfitUnreal)
		}
		if transferAble*futureBidAsk.Bids[0].Price <= model.AppConfig.MinUsdt {
			util.Info(fmt.Sprintf(`%s transferAble %f <= %f`, futureSymbol, transferAble, model.AppConfig.MinUsdt))
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
		order := api.PlaceOrder(model.OrderSideSell, model.OrderTypeMarket,
			market, symbol, model.AmountTypeCoinNumber, marketBidAsk.Bids[0].Price, accountCoin.Free)
		//orderId, errCode, status, actualAmount, actualPrice :=
		if order.OrderId == `` || order.OrderId == `0` {
			util.Notice(fmt.Sprintf(`[arbitrary sell fail]%s %s price%f amount%f`, market, symbol,
				marketBidAsk.Bids[0].Price, accountCoin.Free))
			return
		}
		order = api.SyncQueryOrderById(market, symbol, order.OrderId)
		carry := &model.Carry{AskSymbol: symbol, AskWeb: market, DealAskPrice: order.DealPrice,
			AskPrice: marketBidAsk.Bids[0].Price, DealAskStatus: order.Status, AskTime: int64(marketBidAsk.Ts),
			SideType: model.CarryTypeArbitrarySell, DealAskAmount: order.DealAmount}
		recordCarry(carry)
		api.RefreshAccount(market)
		util.Notice(fmt.Sprintf(`[!arbitrary!]orderid:%s errCode:%s status:%s dealAmount:%f at price:%f`,
			order.OrderId, order.ErrCode, order.Status, order.DealAmount, order.DealPrice))
	}
}

func getBidAmount(market, symbol string, faceValue, bidPrice float64) (amount float64) {
	if market == model.OKEX {
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
		currency, err := util.GetCurrencyFromSymbol(symbol)
		futureSymbolHoldings, futureSymbolHoldingErr := api.GetPositionOkfuture(market, symbol)
		if err != nil || futureSymbolHoldingErr != nil || futureSymbolHoldings == nil ||
			bidPrice == 0 {
			util.Info(fmt.Sprintf(`fail to get allholdings and position and holding`))
			return 0
		}
		account := model.AppAccounts.GetAccount(market, currency)
		if account == nil {
			return 0
		}
		liquidAmount := math.Round(account.Free * bidPrice / faceValue)
		if account.ProfitReal+account.ProfitUnreal > 0 {
			liquidAmount = math.Round((account.Free - account.ProfitReal - account.ProfitUnreal) * bidPrice / faceValue)
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
	order := api.PlaceOrder(model.OrderSideBuy, model.OrderTypeMarket, carry.BidWeb, carry.BidSymbol, ``,
		carry.BidPrice, carry.BidAmount)
	carry.DealBidOrderId = order.OrderId
	carry.DealBidErrCode = order.ErrCode
	carry.DealBidStatus = order.Status
	carry.DealBidAmount = order.DealAmount
	carry.DealBidPrice = order.DealPrice
	if carry.DealBidOrderId == `` || carry.DealBidOrderId == `0` {
		util.Notice(fmt.Sprintf(`[bid fail]%s %s price%f amount in usd %f`, carry.BidWeb, carry.BidSymbol,
			carry.AskPrice, model.ArbitraryCarryUSDT))
		return
	}
	order = api.SyncQueryOrderById(carry.BidWeb, carry.BidSymbol, carry.DealBidOrderId)
	carry.DealBidAmount = order.DealAmount
	carry.DealBidPrice = order.DealPrice
	carry.DealBidStatus = order.Status
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
	currency, err := util.GetCurrencyFromSymbol(carry.AskSymbol)
	if err != nil {
		return false
	}
	accountErr := api.GetAccountOkfuture(model.AppAccounts)
	allHoldings, allHoldingErr := api.GetAllHoldings(carry.AskSymbol)
	if accountErr != nil || allHoldingErr != nil {
		util.Notice(fmt.Sprintf(`fail to get allholdings and position`))
		return false
	}
	account := model.AppAccounts.GetAccount(carry.AskWeb, currency)
	if account == nil {
		return false
	}
	sellAmount := account.Free - allHoldings*faceValue/carry.AskPrice
	if sellAmount >= 0 {
		order := api.PlaceOrder(model.OrderSideSell, model.OrderTypeMarket, carry.AskWeb, carry.AskSymbol,
			model.AmountTypeCoinNumber, carry.AskPrice, sellAmount)
		carry.DealAskOrderId = order.OrderId
		carry.DealAskErrCode = order.ErrCode
		carry.DealAskStatus = order.Status
		carry.DealAskAmount = order.DealAmount
		carry.DealAskPrice = order.DealPrice
		if carry.DealAskOrderId != `` && carry.DealAskOrderId != `0` {
			order = api.SyncQueryOrderById(carry.AskWeb, carry.AskSymbol, carry.DealAskOrderId)
			carry.DealAskAmount = order.DealAmount
			carry.DealAskPrice = order.DealPrice
			carry.DealAskStatus = order.Status
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
	order := api.PlaceOrder(model.OrderSideLiquidateShort, model.OrderTypeMarket, carry.BidWeb, carry.BidSymbol,
		model.AmountTypeContractNumber, carry.BidPrice, carry.BidAmount)
	carry.DealBidOrderId = order.OrderId
	carry.DealBidErrCode = order.ErrCode
	carry.DealBidStatus = order.Status
	carry.DealBidAmount = order.DealAmount
	carry.DealBidPrice = order.DealPrice
	if carry.DealBidOrderId == `` || carry.DealBidOrderId == `0` {
		util.Notice(fmt.Sprintf(`[bid fail]%s %s price%f amount in contract number %f`,
			carry.BidWeb, carry.BidSymbol, carry.BidPrice, carry.BidAmount))
		return false
	}
	order = api.SyncQueryOrderById(carry.BidWeb, carry.BidSymbol, carry.DealBidOrderId)
	carry.DealBidAmount = order.DealAmount
	carry.DealBidPrice = order.DealPrice
	carry.DealBidStatus = order.Status
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
	currency, err := util.GetCurrencyFromSymbol(carry.BidSymbol)
	if err != nil {
		return
	}
	allHoldings, allHoldingErr := api.GetAllHoldings(carry.BidSymbol)
	accountErr := api.GetAccountOkfuture(model.AppAccounts)
	if allHoldingErr != nil || accountErr != nil {
		util.Notice(fmt.Sprintf(`fail to get allholdings and position`))
		return
	}
	account := model.AppAccounts.GetAccount(carry.BidWeb, currency)
	if account == nil {
		return
	}
	transferAble := account.Free - allHoldings*faceValue/carry.DealBidPrice
	if transferAble > account.Free-(account.ProfitReal+account.ProfitUnreal) {
		transferAble = account.Free - (account.ProfitReal + account.ProfitUnreal)
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
		order := api.PlaceOrder(model.OrderSideSell, model.OrderTypeMarket, carry.AskWeb, carry.AskSymbol,
			model.AmountTypeCoinNumber, carry.AskPrice, account.Free)
		carry.DealAskOrderId = order.OrderId
		carry.DealAskErrCode = order.ErrCode
		carry.DealAskStatus = order.Status
		carry.DealAskAmount = order.DealAmount
		carry.DealAskPrice = order.DealPrice
		if carry.DealAskOrderId != `` && carry.DealAskOrderId != `0` {
			order = api.SyncQueryOrderById(carry.AskWeb, carry.AskSymbol,
				carry.DealAskOrderId)
			carry.DealAskAmount, carry.DealAskPrice = order.DealAmount, order.DealPrice
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

func createCarryByMargin(settings []*model.Setting) (carries []*model.Carry) {
	carries = make([]*model.Carry, 0)
	var bidSetting, askSetting *model.Setting
	for i := 0; i < len(settings); i++ {
		bidSetting = settings[i]
		if model.AppMarkets.BidAsks[bidSetting.Symbol] == nil ||
			model.AppMarkets.BidAsks[bidSetting.Symbol][bidSetting.Market] == nil {
			continue
		}
		bidPrice := model.AppMarkets.BidAsks[bidSetting.Symbol][bidSetting.Market].Asks[0].Price
		for j := 0; j < len(settings); j++ {
			askSetting = settings[j]
			if i == j || model.AppMarkets.BidAsks[askSetting.Symbol] == nil ||
				model.AppMarkets.BidAsks[askSetting.Symbol][askSetting.Market] == nil {
				continue
			}
			askPrice := model.AppMarkets.BidAsks[askSetting.Symbol][askSetting.Market].Bids[0].Price
			margin := (askPrice - bidPrice) / askPrice
			if margin > bidSetting.OpenShortMargin && margin > askSetting.CloseShortMargin {
				carry := &model.Carry{AskSymbol: askSetting.Symbol, BidSymbol: bidSetting.Symbol, AskWeb: askSetting.Market,
					BidWeb: bidSetting.Market, AskPrice: askPrice, BidPrice: bidPrice, SideType: model.CarryTypeFuture,
					AskTime: int64(model.AppMarkets.BidAsks[askSetting.Symbol][askSetting.Market].Ts),
					BidTime: int64(model.AppMarkets.BidAsks[bidSetting.Symbol][bidSetting.Market].Ts)}
				checkTime, _ := carry.CheckWorthCarryTime()
				if checkTime {
					carries = append(carries, carry)
				}
			}
			if util.GetNow().Second() == 0 {
				util.Info(fmt.Sprintf(`[record margin %v]%s/%s->%s/%s margin: %f %f-%f`,
					margin > bidSetting.OpenShortMargin && margin > askSetting.CloseShortMargin,
					bidSetting.Market, bidSetting.Symbol, askSetting.Market, askSetting.Symbol,
					margin, bidSetting.OpenShortMargin, askSetting.CloseShortMargin))
			}
		}
	}
	return carries
}

func filterCarry(carries []*model.Carry, faceValue float64) *model.Carry {
	if carries == nil || len(carries) == 0 {
		return nil
	}
	margin := 0.0
	var bestCarry *model.Carry
	accountErr := api.GetAccountOkfuture(model.AppAccounts)
	if accountErr != nil {
		return nil
	}
	for _, carry := range carries {
		carry.BidAmount = getBidAmount(carry.BidWeb, carry.BidSymbol, faceValue, carry.BidPrice)
		//util.Info(fmt.Sprintf(`[filter carry]%s/%s->%s/%s have margin %f amount %f`,
		//	carry.BidWeb, carry.BidSymbol, carry.AskWeb, carry.AskSymbol,
		//	(carry.AskPrice-carry.BidPrice)/carry.AskPrice, carry.BidAmount))
		if carry.BidAmount <= 0 {
			setting := model.GetSetting(carry.BidWeb, carry.BidSymbol)
			//util.Info(fmt.Sprintf(`[add chance]%s/%s %d++`, carry.BidWeb, carry.BidSymbol, setting.Chance))
			setting.Chance += (carry.AskPrice - carry.BidPrice) / carry.AskPrice
			model.AppDB.Save(setting)
		}
		if carry.BidAmount > 0 && margin < carry.AskPrice-carry.BidPrice {
			bestCarry = carry
			margin = carry.AskPrice - carry.BidPrice
		}
	}
	return bestCarry
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
	carries := createCarryByMargin(settings)
	carry := filterCarry(carries, faceValue)
	return carry
}

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

var ProcessContractArbitrage = func(futureMarket, futureSymbol string) {
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
}
