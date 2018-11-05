package carry

import (
	"fmt"
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
			current := util.GetNow()
			//util.Info(fmt.Sprintf(`%s add account %s %f`, value.Market, value.Currency, value.PriceInUsdt))
			if !cleared {
				//util.Info(`remove accounts ` + value.Market + util.GetNow().Format("2006-01-02"))
				model.AppDB.Delete(model.Account{}, "market = ? AND date(created_at) > ?",
					value.Market, current.Format("2006-01-02"))
				cleared = true
			}
			model.AppDB.Create(value)
		}
		accountServing = false
	}
}

//每个小时查询过去一天内有没有从这种setting里面离开的，
//       如果有，那么这是一个勤快的setting
//              如果离开利润低于百分之1.5那么买入利润增加万分之一，
//              如果进入利润高于千分之1.5，那么进入利润减万分之一
//       如果没有，那么这是一个懒惰的setting，在懒惰的里面寻找所有持仓量最大的setting
//              如果进入利润低于百分之1.5，那么进入利润加万分之一
//              如果离开利润大于万分之5，那么离开利润减万分之一
func dealDiligentSettings() {
	createdAt := util.GetNow().Add(time.Duration(-3600) * time.Second)
	settings := model.LoadDiligentSettings(model.OKFUTURE, model.CarryTypeFuture, createdAt)
	for _, setting := range settings {
		if setting == nil {
			continue
		}
		util.Notice(fmt.Sprintf(`[modify setting]%s %s`, setting.Market, setting.Symbol))
		if setting.OpenShortMargin < 0.015 {
			util.Notice(fmt.Sprintf(`open margin %f < 0.015, + 0.0001`, setting.OpenShortMargin))
			setting.OpenShortMargin += 0.0001
		}
		if setting.CloseShortMargin > 0.0015 {
			util.Notice(fmt.Sprintf(`close margin %f > 0.0015, - 0.0001`, setting.CloseShortMargin))
			setting.CloseShortMargin -= 0.0001
		}
		model.AppDB.Save(setting)
	}
}

func dealLazySettings() {
	createdAt := util.GetNow().Add(time.Duration(-86400) * time.Second)
	settings := model.GetMarketSettings(model.OKFUTURE)
	diligentSettings := model.LoadDiligentSettings(model.OKFUTURE, model.CarryTypeFuture, createdAt)
	openShort := 0.0
	var setting *model.Setting
	for _, value := range settings {
		if diligentSettings[value.Symbol] != nil {
			continue
		}
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
	if setting != nil {
		changed := false
		util.Notice(fmt.Sprintf(`[modify setting]%s %s`, setting.Market, setting.Symbol))
		if setting.CloseShortMargin < 0.015 {
			changed = true
			util.Notice(fmt.Sprintf(`close margin %f < 0.015, + 0.0001`, setting.CloseShortMargin))
			setting.CloseShortMargin += 0.0001
		}
		if setting.OpenShortMargin > 0.0005 {
			changed = true
			util.Notice(fmt.Sprintf(`open margin %f > 0.0005, - 0.0001`, setting.OpenShortMargin))
			setting.OpenShortMargin -= 0.0001
		}
		if changed {
			model.AppDB.Save(setting)
		}
	}
}

func MaintainArbitrarySettings() {
	for true {
		dealLazySettings()
		dealDiligentSettings()
		time.Sleep(time.Hour)
	}
}

func RefreshAccounts() {
	for true {
		markets := model.GetMarkets()
		timestamp := util.GetNow()
		for _, value := range markets {
			api.RefreshAccount(value)
			if model.AppAccounts.Data[value] == nil {
				continue
			}
			//accounts.MarketTotal[marketName] = 0
			for key, value := range model.AppAccounts.Data[value] {
				value.PriceInUsdt, _ = api.GetPrice(key + "_usdt")
				value.Timestamp = timestamp
				//util.Info(fmt.Sprintf(`%s price %f`, key, value.PriceInUsdt))
			}
			model.AccountChannel <- model.AppAccounts.Data[value]
		}
		time.Sleep(time.Hour * 1)
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
