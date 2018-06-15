package api

import (
	"time"
	"hello/model"
	"hello/util"
)

func RefreshAccounts() {
	model.HuobiAccountId, _ = GetSpotAccountId(model.ApplicationConfig)
	for true {
		GetAccountHuobi(model.ApplicationAccounts)
		GetAccountOkex(model.ApplicationAccounts)
		GetAccountBinance(model.ApplicationAccounts)
		GetAccountFcoin(model.ApplicationAccounts)
		time.Sleep(time.Minute * 1)
	}
}

func DoAsk(carry *model.Carry, price string, amount string) (orderId, errCode string) {
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
	case model.Fcoin:
		orderId, errCode = PlaceOrderFcoin(carry.Symbol, "sell", price, amount)
		GetAccountFcoin(model.ApplicationAccounts)
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

func DoBid(carry *model.Carry, price string, amount string) (orderId, errCode string) {

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
	case model.Fcoin:
		orderId, errCode = PlaceOrderFcoin(carry.Symbol, "buy", price, amount)
		GetAccountFcoin(model.ApplicationAccounts)
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

