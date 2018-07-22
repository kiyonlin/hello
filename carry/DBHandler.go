package carry

import (
	"fmt"
	"hello/model"
	"hello/util"
	"time"
	"hello/api"
)

func MaintainOrders() {
	for true {
		var carries []model.Carry
		model.ApplicationDB.Where(
			"deal_bid_status = ? OR deal_ask_status = ?", model.CarryStatusWorking, model.CarryStatusWorking).
			Find(&carries)
		util.Notice(fmt.Sprintf("deal with working carries %d", len(carries)))
		for _, carry := range carries {
			if util.GetNowUnixMillion()-carry.BidTime > 60000 && carry.DealBidStatus == model.CarryStatusWorking {
				if carry.SideType != `turtle` {
					api.CancelOrder(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
				}
				carry.DealBidAmount, carry.DealBidStatus = api.QueryOrderById(carry.BidWeb, carry.Symbol, carry.DealBidOrderId)
			}
			if util.GetNowUnixMillion()-carry.AskTime > 60000 && carry.DealAskStatus == model.CarryStatusWorking {
				if carry.SideType != `turtle` {
					api.CancelOrder(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
				}
				carry.DealAskAmount, carry.DealAskStatus = api.QueryOrderById(carry.AskWeb, carry.Symbol, carry.DealAskOrderId)
			}
			model.CarryChannel <- carry
			time.Sleep(time.Second * 1)
		}
		time.Sleep(time.Minute * 5)
	}
}

func InnerCarryServe() {
	for true {
		orderCarry := <-model.InnerCarryChannel
		util.Notice(fmt.Sprintf(`||||||[bid-ask] [%s %s] [%s %s]`, orderCarry.DealBidOrderId,
			orderCarry.DealAskOrderId, orderCarry.DealBidStatus, orderCarry.DealAskStatus))
		if orderCarry.DealBidStatus == `` || orderCarry.DealAskStatus == `` {
			continue
		}
		model.CarryChannel <- orderCarry
	}
}

func AccountHandlerServe() {
	for true {
		accounts := <-model.AccountChannel
		cleared := false
		for _, value := range accounts {
			//util.Info(fmt.Sprintf(`%s add account %s %f`, value.Market, value.Currency, value.PriceInUsdt))
			if !cleared {
				//util.Info(`remove accounts ` + value.Market + util.GetNow().Format("2006-01-02"))
				model.ApplicationDB.Delete(model.Account{}, "market = ? AND date(created_at) = ?",
					value.Market, util.GetNow().Format("2006-01-02"))
				cleared = true
			}
			model.ApplicationDB.Create(value)
		}
	}
}

func RefreshAccounts() {
	for true {
		markets := model.GetMarkets()
		for _, value := range markets {
			api.RefreshAccount(value)
		}
		time.Sleep(time.Second * 100)
	}
}

func OuterCarryServe() {
	for true {
		var carryInDb model.Carry
		carry := <-model.CarryChannel
		model.ApplicationDB.Where("bid_time = ? AND ask_time = ?", carry.BidTime, carry.AskTime).First(&carryInDb)
		if model.ApplicationDB.NewRecord(&carryInDb) {
			model.ApplicationDB.Create(&carry)
		} else {
			if carry.DealAskAmount != 0 {
				carryInDb.DealAskAmount = carry.DealAskAmount
			}
			if carry.DealBidAmount != 0 {
				carryInDb.DealBidAmount = carry.DealBidAmount
			}
			if carry.DealAskStatus != `` {
				carryInDb.DealAskStatus = carry.DealAskStatus
			}
			if carry.DealBidStatus != `` {
				carryInDb.DealBidStatus = carry.DealBidStatus
			}
			if carry.DealAskOrderId != `` {
				carryInDb.DealAskOrderId = carry.DealAskOrderId
			}
			if carry.DealBidOrderId != `` {
				carryInDb.DealBidOrderId = carry.DealBidOrderId
			}
			model.ApplicationDB.Save(&carryInDb)
		}
	}
}
