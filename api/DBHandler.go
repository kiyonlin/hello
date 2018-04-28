package api

import (
	"time"
	"hello/model"
	"hello/util"
)

func cancelOrder(market string, symbol string, orderId string) {
	switch market {
	case model.Huobi:
		CancelOrderHuobi(orderId)
	case model.OKEX:
		CancelOrderOkex(symbol, orderId)
	case model.Binance:
		CancelOrderBinance(symbol, orderId)
	}
}

func queryOrder(carry *model.Carry) {
	if carry.DealBidStatus == model.CarryStatusWorking && carry.DealBidOrderId != "" {
		switch carry.BidWeb {
		case model.Huobi:
			carry.DealBidAmount, carry.DealBidStatus = QueryOrderHuobi(carry.DealBidOrderId)
		case model.OKEX:
			carry.DealBidAmount, carry.DealBidStatus = QueryOrderOkex(carry.Symbol, carry.DealBidOrderId)
		case model.Binance:
			carry.DealBidAmount, carry.DealBidStatus = QueryOrderBinance(carry.Symbol, carry.DealBidOrderId)
		}
	}
	if carry.DealAskStatus == model.CarryStatusWorking && carry.DealAskOrderId != "" {
		switch carry.AskWeb {
		case model.Huobi:
			carry.DealAskAmount, carry.DealAskStatus = QueryOrderHuobi(carry.DealAskOrderId)
		case model.OKEX:
			carry.DealAskAmount, carry.DealAskStatus = QueryOrderOkex(carry.Symbol, carry.DealAskOrderId)
		case model.Binance:
			carry.DealAskAmount, carry.DealAskStatus = QueryOrderBinance(carry.Symbol, carry.DealAskOrderId)
		}
	}
	model.CarryChannel <- *carry
}

func CarryProcessor() {
	index := 0
	for true {
		var carry model.Carry
		model.ApplicationDB.Where("deal_bid_status = ? OR deal_ask_status = ?", model.CarryStatusWorking, model.CarryStatusWorking).Offset(index).First(&carry)
		if model.ApplicationDB.NewRecord(&carry) {
			index = 0
		} else {
			// cancel order if delay too long (180 seconds)
			if carry.DealBidStatus == model.CarryStatusWorking && util.GetNowUnixMillion()-carry.BidTime > 180000 {
				cancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
			}
			if carry.DealAskStatus == model.CarryStatusWorking && util.GetNowUnixMillion()-carry.AskTime > 180000 {
				cancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
			}
			queryOrder(&carry)
			index++
		}
		time.Sleep(time.Minute * 3)
	}
}

func AccountDBHandlerServe() {
	for true {
		accounts := <-model.AccountChannel
		cleared := false
		for _, value := range accounts {
			if !cleared {
				model.ApplicationDB.Delete(model.Account{}, "market = ? AND date(created_at) = ?",
					value.Market, util.GetNow().Format("2006-01-02"))
				cleared = true
			}
			model.ApplicationDB.Create(value)
		}
	}
}

func BidUpdate() {
	for true {
		var carryInDb model.Carry
		carry := <-model.BidChannel
		model.ApplicationDB.Where("bid_time = ? AND ask_time = ?", carry.BidTime, carry.AskTime).First(&carryInDb)
		if model.ApplicationDB.NewRecord(&carryInDb) {
			model.ApplicationDB.Create(&carry)
		} else {
			carryInDb.DealBidOrderId = carry.DealBidOrderId
			carryInDb.DealBidErrCode = carry.DealBidErrCode
			carryInDb.DealBidStatus = carry.DealBidStatus
			model.ApplicationDB.Save(&carryInDb)
		}
	}
}

func AskUpdate() {
	for true {
		var carryInDb model.Carry
		carry := <-model.AskChannel
		model.ApplicationDB.Where("bid_time = ? AND ask_time = ?", carry.BidTime, carry.AskTime).First(&carryInDb)
		if model.ApplicationDB.NewRecord(&carryInDb) {
			model.ApplicationDB.Create(&carry)
		} else {
			carryInDb.DealAskOrderId = carry.DealAskOrderId
			carryInDb.DealAskErrCode = carry.DealAskErrCode
			carryInDb.DealAskStatus = carry.DealAskStatus
			model.ApplicationDB.Save(&carryInDb)
		}
	}
}

func CarryDBHandlerServe() {
	for true {
		carry := <-model.CarryChannel
		var carryInDb model.Carry
		model.ApplicationDB.Where("bid_time = ? AND ask_time = ?", carry.BidTime, carry.AskTime).First(&carryInDb)
		if model.ApplicationDB.NewRecord(&carryInDb) {
			model.ApplicationDB.Create(&carry)
		} else {
			carryInDb.DealAskAmount = carry.DealAskAmount
			carryInDb.DealBidAmount = carry.DealBidAmount
			carryInDb.DealAskStatus = carry.DealAskStatus
			carryInDb.DealBidStatus = carry.DealBidStatus
			model.ApplicationDB.Save(&carryInDb)
		}
	}
}
