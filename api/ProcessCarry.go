package api

import (
	"strings"
	"strconv"
	"hello/model"
	"hello/util"
	"time"
)

type CarryHandler func(carry *model.Carry)

//var Carrying = false

func doAsk(carry *model.Carry, price string, amount string) (orderId, errCode string) {
	util.Notice(carry.AskWeb + "ask" + carry.Symbol + " with price: " + price + " amount:" + amount)
	switch carry.AskWeb {
	case model.Huobi:
		orderId, errCode = PlaceOrderHuobi(carry.Symbol, "sell-limit", price, amount)
		GetAccountHuobi(model.ApplicationAccounts)
	case model.OKEX:
		orderId, errCode = PlaceOrderOkex(carry.Symbol, "sell", price, amount)
		GetAccountOkex(model.ApplicationAccounts)
	case model.Binance:
		orderId, errCode = PlaceOrderBinance(carry.Symbol, "SELL", price, amount)
		GetAccountBinance(model.ApplicationAccounts)
	}
	//carry.DealAskAmount, _ = strconv.ParseFloat(amount, 64)
	carry.DealAskErrCode = errCode
	carry.DealAskOrderId = orderId
	if orderId == "0" || orderId == "" {
		carry.DealAskStatus = model.CarryStatusFail
	} else {
		carry.DealAskStatus = model.CarryStatusWorking
	}
	model.AskChannel <- *carry
	return orderId, errCode
}

func doBid(carry *model.Carry, price string, amount string) (orderId, errCode string) {

	util.Notice(carry.BidWeb + "bid" + carry.Symbol + " with price: " + price + " amount:" + amount)
	switch carry.BidWeb {
	case model.Huobi:
		orderId, errCode = PlaceOrderHuobi(carry.Symbol, "buy-limit", price, amount)
		GetAccountHuobi(model.ApplicationAccounts)
	case model.OKEX:
		orderId, errCode = PlaceOrderOkex(carry.Symbol, "buy", price, amount)
		GetAccountOkex(model.ApplicationAccounts)
	case model.Binance:
		orderId, errCode = PlaceOrderBinance(carry.Symbol, "BUY", price, amount)
		GetAccountBinance(model.ApplicationAccounts)
	}
	//carry.DealBidAmount, _ = strconv.ParseFloat(amount, 64)
	carry.DealBidErrCode = errCode
	carry.DealBidOrderId = orderId
	if orderId == "0" || orderId == "" {
		carry.DealBidStatus = model.CarryStatusFail
	} else {
		carry.DealBidStatus = model.CarryStatusWorking
	}
	model.BidChannel <- *carry
	return orderId, errCode
}

// 只取第一位
func calcAmount(originalAmount float64) (num float64, err error) {
	str := strconv.FormatFloat(originalAmount, 'f', -1, 64)
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

var ProcessCarry = func(carry *model.Carry) {
	util.Info(carry.ToString())
	currencies := strings.Split(carry.Symbol, "_")
	leftBalance := 0.0
	rightBalance := 0.0
	account := model.ApplicationAccounts.GetAccount(carry.AskWeb, currencies[0])
	if account != nil {
		leftBalance = account.Free
	}
	account = model.ApplicationAccounts.GetAccount(carry.BidWeb, currencies[1])
	if account != nil {
		rightBalance = account.Free
	}
	priceInUsdt, _ := model.GetBuyPriceOkex(currencies[0] + "_usdt")
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
	leftBalance, _ = calcAmount(leftBalance)
	if leftBalance < minAmount {
		return
	}
	strLeftBalance := strconv.FormatFloat(leftBalance, 'f', -1, 64)
	strAskPrice := strconv.FormatFloat(carry.AskPrice, 'f', -1, 64)
	strBidPrice := strconv.FormatFloat(carry.BidPrice, 'f', -1, 64)

	timeOk, _ := carry.CheckWorthCarryTime(model.ApplicationMarkets, model.ApplicationConfig)
	marginOk, _ := carry.CheckWorthCarryMargin(model.ApplicationMarkets, model.ApplicationConfig)
	util.Info(carry.ToString())
	if timeOk && marginOk {
		if model.ApplicationConfig.Env != `test` {
			go doAsk(carry, strAskPrice, strLeftBalance)
			go doBid(carry, strBidPrice, strLeftBalance)
		}
		model.ApplicationMarkets.BidAsks[carry.Symbol][carry.AskWeb] = nil
		model.ApplicationMarkets.BidAsks[carry.Symbol][carry.BidWeb] = nil
	} else {
		if carry.CheckWorthSaveMargin() {
			carry.DealAskStatus = `NotWorth`
			carry.DealBidStatus = `NotWorth`
			model.BidChannel <- *carry
			model.ApplicationMarkets.BidAsks[carry.Symbol][carry.AskWeb] = nil
			model.ApplicationMarkets.BidAsks[carry.Symbol][carry.BidWeb] = nil
		}
	}
	time.Sleep(time.Second * 3)
}
