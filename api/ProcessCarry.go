package api

import (
	"strings"
	"strconv"
	"time"
	"github.com/haoweizh/hello/model"
	"github.com/haoweizh/hello/util"
)

type CarryHandler func(carry *model.Carry)

var Carrying = false

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
	carry.DealAskAmount, _ = strconv.ParseFloat(amount, 64)
	carry.DealAskErrCode = errCode
	carry.DealAskOrderId = orderId
	if orderId == "0" || orderId == "" {
		carry.DealAskStatus = model.CarryStatusFail
	} else {
		carry.DealAskStatus = model.CarryStatusWorking
	}
	model.CarryChannel <- *carry
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
	carry.DealBidAmount, _ = strconv.ParseFloat(amount, 64)
	carry.DealBidErrCode = errCode
	carry.DealBidOrderId = orderId
	if orderId == "0" || orderId == "" {
		carry.DealBidStatus = model.CarryStatusFail
	} else {
		carry.DealBidStatus = model.CarryStatusWorking
	}
	model.CarryChannel <- *carry
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
	if Carrying {
		util.Info("别人正在搬砖，我先退出")
		return
	}
	util.Info("开始搬砖" + carry.ToString())
	Carrying = true
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
	util.Info("计划从" + carry.AskWeb + "搬运" + carry.Symbol + strconv.FormatFloat(leftBalance, 'f', -1, 64) + "到" + carry.BidWeb)
	if leftBalance > carry.Amount {
		leftBalance = carry.Amount
	}
	if leftBalance*carry.BidPrice > rightBalance {
		leftBalance = rightBalance / carry.BidPrice
	}
	leftBalance, _ = calcAmount(leftBalance)
	util.Info("实际从" + carry.AskWeb + "搬运" + carry.Symbol + strconv.FormatFloat(leftBalance, 'f', -1, 64) + "到" + carry.BidWeb)
	if leftBalance == 0 {
		util.Info("数量为0,退出")
		Carrying = false
		return
	}
	strLeftBalance := strconv.FormatFloat(leftBalance, 'f', -1, 64)
	strAskPrice := strconv.FormatFloat(carry.AskPrice, 'f', -1, 64)
	strBidPrice := strconv.FormatFloat(carry.BidPrice, 'f', -1, 64)
	go doAsk(carry, strAskPrice, strLeftBalance)
	go doBid(carry, strBidPrice, strLeftBalance)
	time.Sleep(time.Second * 30)
	Carrying = false
	util.Info("搬砖结束")
}
