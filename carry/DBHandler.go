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
		util.Notice(fmt.Sprintf("deal with working carries %d", len(carries)))
		for _, carry := range carries {
			if util.GetNowUnixMillion()-carry.BidTime > 60000 && carry.DealBidStatus == model.CarryStatusWorking &&
				util.GetNowUnixMillion()-carry.BidTime < 600000{ // 忽略十分钟以前的
				cancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
				queryOrder(&carry)
			}
			if util.GetNowUnixMillion()-carry.AskTime > 60000 && carry.DealAskStatus == model.CarryStatusWorking &&
				util.GetNowUnixMillion()-carry.AskTime < 600000{
				cancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
				queryOrder(&carry)
			}
			time.Sleep(time.Second * 1)
		}
		time.Sleep(time.Minute * 1)
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

func BidAskUpdate() {
	for true {
		var carryInDb model.Carry
		carry := <-model.BidAskChannel
		model.ApplicationDB.Where("ask_time = ? AND bid_time = ?", carry.AskTime, carry.BidTime).First(&carryInDb)
		if model.ApplicationDB.NewRecord(&carryInDb) {
			util.Notice("create new ask " + carry.ToString())
			model.ApplicationDB.Create(&carry)
		} else {
			util.Notice(carry.DealAskOrderId + " update old " + carry.SideType + carry.ToString())
			if carry.SideType == `ask` {
				model.ApplicationDB.Model(&carryInDb).Updates(map[string]interface{}{
					"deal_ask_order_id": carry.DealAskOrderId, "deal_ask_err_code": carry.DealAskErrCode,
					"deal_ask_status": carry.DealAskStatus})
			} else if carry.SideType == `bid` {
				model.ApplicationDB.Model(&carryInDb).Updates(map[string]interface{}{
					"deal_bid_order_id": carry.DealBidOrderId, "deal_bid_err_code": carry.DealBidErrCode,
					"deal_bid_status": carry.DealBidStatus})
			} else {
				// TODO 处理其他更新类型
			}
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
