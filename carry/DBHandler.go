package carry

import (
	"fmt"
	"hello/api"
	"hello/model"
	"hello/util"
	"strings"
	"time"
)

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
	symbols := model.GetMarketSymbols(model.OKFUTURE)
	diligentSettings := model.LoadDiligentSettings(model.OKFUTURE, model.CarryTypeFuture, createdAt)
	openShort := 0.0
	var setting *model.Setting
	for symbol := range symbols {
		if diligentSettings[symbol] != nil {
			continue
		}
		futureAccount, _ := api.GetPositionOkfuture(model.OKFUTURE, symbol)
		if futureAccount != nil {
			short := futureAccount.OpenedShort
			if strings.Contains(futureAccount.Symbol, `btc`) {
				short = short * 10
			}
			if openShort < short {
				openShort = short
				setting = model.GetSetting(model.FunctionArbitrary, model.OKFUTURE, symbol)
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

func CancelOldWorkingOrders() {
	d, _ := time.ParseDuration("-1h")
	for true {
		timeLine := util.GetNow().Add(d)
		markets := model.GetFunctionMarkets(model.FunctionGrid)
		for _, market := range markets {
			symbols := model.GetMarketSymbols(market)
			for symbol := range symbols {
				orders := api.QueryOrders(market, symbol, model.CarryStatusWorking)
				for orderId, order := range orders {
					if orderId != `` && order.OrderTime.Before(timeLine) {
						result, errCode, msg := api.CancelOrder(market, symbol, orderId)
						util.Notice(fmt.Sprintf(`[cancel old]%v %s %s`, result, errCode, msg))
					}
				}
			}
		}
		time.Sleep(time.Hour)
	}
}

func RefreshAccounts() {
	for true {
		if model.AppConfig.Handle == `1` {
			model.AppConfig.Handle = `0`
			time.Sleep(time.Second * 5)
			model.AppConfig.Handle = `1`
		}
		markets := model.GetMarkets()
		timestamp := util.GetNow()
		for _, value := range markets {
			api.RefreshAccount(value)
			if model.AppAccounts.Data[value] == nil {
				continue
			}
			//accounts.MarketTotal[marketName] = 0
			symbols := model.GetMarketSymbols(value)
			for key, account := range model.AppAccounts.Data[value] {
				if symbols[key+"_usdt"] || key == `usdt` {
					account.PriceInUsdt, _ = api.GetPrice(key + "_usdt")
					account.Timestamp = timestamp
				}
			}
			model.AccountChannel <- model.AppAccounts.Data[value]
		}
		time.Sleep(time.Hour)
	}
}

var feeIndex int

func MaintainTransFee() {
	for true {
		d, _ := time.ParseDuration("-48h")
		timeLine := util.GetNow().Add(d)
		var orders []model.Order
		for true {
			model.AppDB.Limit(100).Offset(feeIndex).Where(`fee=? and fee_income=? and date(order_time)>?`,
				0, 0, timeLine).Find(&orders)
			if len(orders) == 0 {
				break
			}
			feeIndex += len(orders)
			for _, value := range orders {
				order := api.QueryOrderById(value.Market, value.Symbol, value.OrderId)
				value.Fee = order.Fee
				value.FeeIncome = order.FeeIncome
				model.AppDB.Save(&value)
				if model.AppConfig.Handle == `1` {
					time.Sleep(time.Second)
				} else {
					time.Sleep(time.Millisecond * 120)
				}
			}
		}
		feeIndex = 0
		time.Sleep(time.Minute * 5)
	}
}
