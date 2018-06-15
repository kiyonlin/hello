package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"time"
)

func cancelOrder(market string, symbol string, orderId string) {
	switch market {
	case model.Huobi:
		api.CancelOrderHuobi(orderId)
	case model.OKEX:
		api.CancelOrderOkex(symbol, orderId)
	case model.Binance:
		api.CancelOrderBinance(symbol, orderId)
	case model.Fcoin:
		api.CancelOrderFcoin(orderId)
	}
}

func queryOrder(carry *model.Carry) {
	if carry.DealBidOrderId != "" {
		switch carry.BidWeb {
		case model.Huobi:
			carry.DealBidAmount, carry.DealBidStatus = api.QueryOrderHuobi(carry.DealBidOrderId)
		case model.OKEX:
			carry.DealBidAmount, carry.DealBidStatus = api.QueryOrderOkex(carry.Symbol, carry.DealBidOrderId)
		case model.Binance:
			carry.DealBidAmount, carry.DealBidStatus = api.QueryOrderBinance(carry.Symbol, carry.DealBidOrderId)
		case model.Fcoin:
			carry.DealBidAmount, carry.DealBidStatus = api.QueryOrderFcoin(carry.Symbol, carry.DealBidOrderId)
		}
	}
	if carry.DealAskOrderId != "" {
		switch carry.AskWeb {
		case model.Huobi:
			carry.DealAskAmount, carry.DealAskStatus = api.QueryOrderHuobi(carry.DealAskOrderId)
		case model.OKEX:
			carry.DealAskAmount, carry.DealAskStatus = api.QueryOrderOkex(carry.Symbol, carry.DealAskOrderId)
		case model.Binance:
			carry.DealAskAmount, carry.DealAskStatus = api.QueryOrderBinance(carry.Symbol, carry.DealAskOrderId)
		case model.Fcoin:
			carry.DealAskAmount, carry.DealAskStatus = api.QueryOrderFcoin(carry.Symbol, carry.DealAskOrderId)
		}
	}
	model.CarryChannel <- *carry
}

func MaintainOrders() {
	for true {
		var carries []model.Carry
		model.ApplicationDB.Where(
			"deal_bid_status = ? OR deal_ask_status = ?", model.CarryStatusWorking, model.CarryStatusWorking).
			Find(&carries)
		model.WorkingCarryNum = len(carries)
		util.Notice(fmt.Sprintf("deal with working carries %d", len(carries)))
		for _, carry := range carries {
			// cancel order if delay too long (180 seconds)
			if util.GetNowUnixMillion()-carry.BidTime > 180000 || util.GetNowUnixMillion()-carry.AskTime > 180000 {
				cancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
				cancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
				queryOrder(&carry)
			}
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
			util.SocketInfo("create new bid " + carry.ToString())
			model.ApplicationDB.Create(&carry)
		} else {
			util.Notice("update old bid " + carry.ToString())
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

func DBHandlerServe() {
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
