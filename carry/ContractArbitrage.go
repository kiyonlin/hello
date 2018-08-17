package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"strings"
	"time"
)

var contractArbitraging = false

func setContractArbitraging(status bool) {
	contractArbitraging = status
}

func closeShort(symbol, market, futureSymbol, futureMarket string, askPrice, bidPrice float64) {
	if model.AppFutureAccount[futureMarket] == nil ||
		model.AppFutureAccount[futureMarket][futureSymbol] == nil {
		util.Notice(futureMarket + ` fail to get future account ` + futureSymbol)
		return
	}
	futureAccount := model.AppFutureAccount[futureMarket][futureSymbol]
	if futureAccount.OpenedShort < 1 {
		util.Notice(`[No opened short]`)
	}
	carry := &model.Carry{}
	carry.Symbol = futureSymbol
	carry.AskWeb = market
	carry.BidWeb = futureMarket
	carry.AskPrice = askPrice
	carry.BidPrice = bidPrice
	if strings.Contains(futureSymbol, `btc`) {
		carry.Amount = model.OKLever * model.OKEXBTCContractFaceValue * 1.01 / carry.BidPrice
	} else {
		carry.Amount = model.OKLever * model.OKEXOtherContractFaceValue * 1.01 / carry.BidPrice
	}
	carry.AskAmount = carry.Amount
	carry.BidAmount = carry.Amount / model.OKLever // 10倍杠杆
	carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus = api.PlaceOrder(model.OrderSideLiquidateShort,
		model.OrderTypeMarket, futureMarket, futureSymbol, carry.BidPrice, carry.BidAmount)
	if carry.DealBidOrderId == `` || carry.DealBidOrderId == `0` {
		util.Notice(fmt.Sprintf(`[bid fail]%s %s price%f amount%f`, futureMarket, futureSymbol, carry.BidPrice, carry.BidAmount))
		return
	}
	time.Sleep(time.Second)
	api.RefreshAccount(futureMarket)
	carry.DealBidAmount, carry.BidPrice, _ = api.QueryOrderById(futureMarket, futureSymbol, carry.DealBidOrderId)
	if carry.DealBidAmount > 0 {
		transferAmount := carry.DealBidAmount * model.OKLever * model.OKEXOtherContractFaceValue / carry.BidPrice
		if strings.Contains(futureSymbol, `btc`) {
			transferAmount = carry.DealBidAmount * model.OKLever * model.OKEXBTCContractFaceValue / carry.BidPrice
		}
		transfer, _ := api.FundTransferOkex(symbol, transferAmount, `3`, `1`)
		if transfer {
			carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskStatus = api.PlaceOrder(model.OrderSideSell,
				model.OrderTypeMarket, market, symbol, carry.AskPrice, transferAmount)
			time.Sleep(time.Second)
			if carry.DealAskOrderId != `` && carry.DealAskOrderId != `0` {
				time.Sleep(time.Second)
				api.RefreshAccount(market)
				carry.DealAskAmount, carry.AskPrice, _ = api.QueryOrderById(market, symbol, carry.DealAskOrderId)
			} else {
				util.Notice(`[!!Ask Fail]` + carry.DealAskErrCode + carry.DealAskStatus)
			}
		} else {
			util.Notice(`[transfer fail]`)
		}
	}
	model.CarryChannel <- *carry
}

func openShort(symbol, market, futureSymbol, futureMarket string, askPrice, bidPrice float64) {
	carry := &model.Carry{}
	carry.Symbol = futureSymbol
	carry.AskWeb = futureMarket
	carry.BidWeb = market
	carry.AskPrice = askPrice
	carry.BidPrice = bidPrice
	// btc期貨面值100usd,十倍保证金为1000usd，按照101%買入，其他面值10usd
	if strings.Contains(symbol, `btc`) {
		carry.Amount = model.OKLever * model.OKEXBTCContractFaceValue * 1.01 / carry.BidPrice
	} else {
		carry.Amount = model.OKLever * model.OKEXOtherContractFaceValue * 1.01 / carry.BidPrice
	}
	account := model.AppAccounts.GetAccount(market, `usdt`)
	if account.Free < carry.Amount*carry.BidPrice {
		time.Sleep(time.Minute)
		util.Notice(fmt.Sprintf(`账户usdt余额usdt%f不够买%f个%s`, account.Free, carry.Amount, symbol))
		return
	}
	carry.BidAmount = carry.Amount
	carry.AskAmount = carry.Amount / model.OKLever // 10倍杠杆
	carry.DealBidOrderId, carry.DealBidErrCode, carry.DealBidStatus = api.PlaceOrder(model.OrderSideBuy,
		model.OrderTypeMarket, market, symbol, carry.BidPrice, carry.BidAmount)
	if carry.DealBidOrderId == `` || carry.DealBidOrderId == `0` {
		util.Notice(fmt.Sprintf(`[bid fail]%s %s price%f amount%f`, market, symbol, carry.BidPrice, carry.BidAmount))
		return
	}
	time.Sleep(time.Second)
	api.RefreshAccount(market)
	carry.DealBidAmount, carry.BidPrice, _ = api.QueryOrderById(market, symbol, carry.DealBidOrderId)
	if carry.DealBidAmount > 0 {
		transfer, _ := api.FundTransferOkex(symbol, carry.DealBidAmount, `1`, `3`)
		if transfer {
			carry.DealAskOrderId, carry.DealAskErrCode, carry.DealAskStatus = api.PlaceOrder(model.OrderSideSell,
				model.OrderTypeMarket, futureMarket, futureSymbol, carry.AskPrice, carry.AskAmount)
			time.Sleep(time.Second)
			if carry.DealAskOrderId != `` && carry.DealAskOrderId != `0` {
				time.Sleep(time.Second)
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
	if util.GetNowUnixMillion()-int64(futureBidAsk.Ts) > int64(model.AppConfig.Delay) ||
		util.GetNowUnixMillion()-int64(bidAsk.Ts) > int64(model.AppConfig.Delay) {
		util.Info(`bid ask not in time`)
		return
	}
	openShortMargin := (futureBidAsk.Bids[0].Price - bidAsk.Asks[0].Price) / bidAsk.Asks[0].Price
	closeShortMargin := (futureBidAsk.Asks[0].Price - bidAsk.Bids[0].Price) / bidAsk.Bids[0].Price
	if setting.OpenShortMargin < openShortMargin {
		openShort(symbol, model.OKEX, futureSymbol, futureMarket, futureBidAsk.Bids[0].Price, bidAsk.Asks[0].Price)
	} else if setting.CloseShortMargin > closeShortMargin {
		closeShort(symbol, model.OKEX, futureSymbol, futureMarket, bidAsk.Bids[0].Price, futureBidAsk.Asks[0].Price)
	}
	fmt.Println(fmt.Sprintf(`%f %f %f %f`, openShortMargin, closeShortMargin, setting.OpenShortMargin, setting.CloseShortMargin))
}
