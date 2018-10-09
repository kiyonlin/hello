package carry

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"hello/api"
	"hello/model"
	"hello/util"
	"strings"
	"time"
)

func MaintainOrders() {
	for true {
		var carries []model.Carry
		model.AppDB.Where(
			"deal_bid_status = ? OR deal_ask_status = ?", model.CarryStatusWorking, model.CarryStatusWorking).
			Find(&carries)
		util.Notice(fmt.Sprintf("deal with working carries %d", len(carries)))
		for _, carry := range carries {
			if util.GetNowUnixMillion()-carry.BidTime > 60000 && carry.DealBidStatus == model.CarryStatusWorking {
				if carry.SideType != `turtle` {
					api.CancelOrder(carry.BidWeb, carry.BidSymbol, carry.DealBidOrderId)
				}
				carry.DealBidAmount, _, carry.DealBidStatus = api.QueryOrderById(carry.BidWeb, carry.BidSymbol,
					carry.DealBidOrderId)
			}
			if util.GetNowUnixMillion()-carry.AskTime > 60000 && carry.DealAskStatus == model.CarryStatusWorking {
				if carry.SideType != `turtle` {
					api.CancelOrder(carry.AskWeb, carry.AskSymbol, carry.DealAskOrderId)
				}
				carry.DealAskAmount, _, carry.DealAskStatus = api.QueryOrderById(carry.AskWeb, carry.AskSymbol,
					carry.DealAskOrderId)
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

var accountServing = false

func AccountHandlerServe() {
	for true {
		accounts := <-model.AccountChannel
		if accountServing {
			continue
		}
		accountServing = true
		cleared := false
		for _, value := range accounts {
			//util.Info(fmt.Sprintf(`%s add account %s %f`, value.Market, value.Currency, value.PriceInUsdt))
			if !cleared {
				//util.Info(`remove accounts ` + value.Market + util.GetNow().Format("2006-01-02"))
				model.AppDB.Delete(model.Account{}, "market = ? AND date(created_at) = ?",
					value.Market, util.GetNow().Format("2006-01-02"))
				cleared = true
			}
			model.AppDB.Create(value)
		}
		accountServing = false
	}
}

func dealLazySettings() {
	createdAt := util.GetNow().Add(time.Duration(-86400) * time.Second)
	model.AppDB, _ = gorm.Open("postgres", model.AppConfig.DBConnection)
	settings := model.LoadLazySettings(model.OKFUTURE, model.CarryTypeFuture, createdAt)
	openShort := 0.0
	var setting *model.Setting
	for _, value := range settings {
		futureAccount, _ := api.GetPositionOkfuture(value.Market, value.Symbol)
		if futureAccount != nil {
			short := futureAccount.OpenedShort
			if strings.Contains(futureAccount.Symbol, `btc`) {
				short = short * 10
			}
			if openShort < short {
				openShort = short
				setting = value
			}
		}
	}
	setting.CloseShortMargin += 0.009
	setting.OpenShortMargin -= 0.0001
	model.AppDB.Save(setting)
	model.LoadSettings()
}

func MaintainArbitrarySettings() {
	for true {
		dealLazySettings()
		time.Sleep(time.Hour)
	}
}

func RefreshAccounts() {
	for true {
		markets := model.GetMarkets()
		for _, value := range markets {
			api.RefreshAccount(value)
		}
		time.Sleep(time.Second * 300)
	}
}

func OuterCarryServe() {
	for true {
		var carryInDb model.Carry
		carry := <-model.CarryChannel
		model.AppDB.Where("bid_time = ? AND ask_time = ?", carry.BidTime, carry.AskTime).First(&carryInDb)
		if model.AppDB.NewRecord(&carryInDb) {
			model.AppDB.Create(&carry)
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
			model.AppDB.Save(&carryInDb)
		}
	}
}
