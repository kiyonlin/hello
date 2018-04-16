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
			util.SocketInfo("Pause carry processor for 10 minutes")
			time.Sleep(time.Minute * 10)
			index = 0
		} else {
			// cancel order if delay too long (5 minutes)
			if carry.DealBidStatus == model.CarryStatusWorking && util.GetNowUnixMillion()-carry.BidTime > 300000 {
				orderId := carry.DealBidOrderId
				cancelOrder(carry.BidWeb, carry.Symbol, orderId)
			}
			if carry.DealAskStatus == model.CarryStatusWorking && util.GetNowUnixMillion()-carry.AskTime > 300000 {
				orderId := carry.DealAskOrderId
				cancelOrder(carry.AskWeb, carry.Symbol, orderId)
			}
			queryOrder(&carry)
			index++
			time.Sleep(time.Second * 10)
		}
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
			util.SocketInfo("insert carry" + carryInDb.Symbol)
			model.ApplicationDB.Create(&carry)
		} else {
			util.SocketInfo("update bid carry<<<<<<<<<<<<<<<" + carryInDb.Symbol)
			carryInDb.DealBidOrderId = carry.DealBidOrderId
			carryInDb.DealBidErrCode = carry.DealBidErrCode
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
			util.SocketInfo("insert carry" + carryInDb.Symbol)
			model.ApplicationDB.Create(&carry)
		} else {
			util.SocketInfo("update ask carry>>>>>>>>>>>>>>>>" + carryInDb.Symbol)
			carryInDb.DealAskOrderId = carry.DealAskOrderId
			carryInDb.DealAskErrCode = carry.DealAskErrCode
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
			util.SocketInfo("insert carry" + carryInDb.Symbol)
			model.ApplicationDB.Create(&carry)
		} else {
			util.SocketInfo("update carry")
			carryInDb.DealAskAmount = carry.DealAskAmount
			carryInDb.DealBidAmount = carry.DealBidAmount
			carryInDb.DealAskStatus = carry.DealAskStatus
			carryInDb.DealBidStatus = carry.DealBidStatus
			model.ApplicationDB.Save(&carryInDb)
		}
	}
}
