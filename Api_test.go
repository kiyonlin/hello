package main

import (
	"fmt"
	"github.com/jinzhu/configor"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/api"
	"hello/model"
	"hello/util"
	"math"
	"sort"
	"strings"
	"testing"
	"time"
)

func loadLazySettings() {
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

func getBidAmount(market, symbol string, faceValue, bidPrice float64) (amount float64) {
	if market == model.OKEX {
		index := strings.Index(symbol, `_`)
		if index == -1 {
			return 0
		}
		accountUsdt := model.AppAccounts.GetAccount(market, `usdt`)
		if accountUsdt == nil {
			util.Info(`account nil`)
			api.RefreshAccount(market)
			return 0
		}
		if accountUsdt.Free <= model.AppConfig.MinUsdt || accountUsdt.Free <= model.ArbitraryCarryUSDT {
			//util.Info(fmt.Sprintf(`账户usdt余额usdt%f不够买%f个%s`, account.Free, carry.Amount+1, symbol))
			return 0
		}
		return model.ArbitraryCarryUSDT
	} else if market == model.OKFUTURE {
		//allHoldings, allHoldingErr := api.GetAllHoldings(symbol)
		futureSymbolHoldings, futureSymbolHoldingErr := api.GetPositionOkfuture(market, symbol)
		accountRights, realProfit, unrealProfit, accountErr := api.GetAccountOkfuture(model.AppAccounts, symbol)
		if accountErr != nil || futureSymbolHoldingErr != nil || futureSymbolHoldings == nil || bidPrice == 0 {
			util.Notice(fmt.Sprintf(`fail to get allholdings and position and holding`))
			return 0
		}
		// 在劇烈震蕩的時候需要關注盈虧的開空
		//keepShort := math.Round((realProfit + unrealProfit) * bidPrice / faceValue)
		//if allHoldings <= keepShort {
		//	//util.Notice(fmt.Sprintf(`allholding <= keep %f %f`, allHoldings, keepShort))
		//	return 0
		//}
		liquidAmount := math.Round(accountRights * bidPrice / faceValue)
		if realProfit+unrealProfit > 0 {
			liquidAmount = math.Round((accountRights - realProfit - unrealProfit) * bidPrice / faceValue)
		}
		if liquidAmount > futureSymbolHoldings.OpenedShort {
			liquidAmount = futureSymbolHoldings.OpenedShort
		}
		if liquidAmount > model.ArbitraryCarryUSDT/faceValue {
			liquidAmount = math.Round(model.ArbitraryCarryUSDT / faceValue)
		}
		return liquidAmount
	}
	return 0
}


func Test_RefreshAccount(t *testing.T) {
	model.NewConfig()
	err := configor.Load(model.AppConfig, "./config.yml")
	if err != nil {
		util.Notice(err.Error())
		return
	}
	carry := model.Carry{}
	carry.BidAmount = getBidAmount(model.OKFUTURE, `etc_quarter`, 10, 11)
	loadLazySettings()
	for i := 0; i < 50; i++ {
		api.GetKLineOkexFuture(`btc_this_week`, `1min`, 100)
	}
	timeSlot := `1min`
	size := int64(1560)
	currency := `eos`
	kpointsFuture := api.GetKLineOkexFuture(currency+`_this_week`, timeSlot, size)
	kpoints := api.GetKLineOkex(currency+`_usdt`, timeSlot, size)
	percentage := 0.0
	ticks := make([]model.Tick, len(kpointsFuture))
	i := 0
	for key, value := range kpointsFuture {
		futureTime := time.Unix(value.TS/1000, 0).Format("Mon Jan 2 2006-01-02 15:04:05")
		currentTime := time.Unix(kpoints[key].TS/1000, 0).Format("Mon Jan 2 2006-01-02 15:04:05")
		percentage += (value.EndPrice - kpoints[key].EndPrice) / kpoints[key].EndPrice
		tick := model.Tick{Id: futureTime, Price: (value.EndPrice - kpoints[key].EndPrice) / kpoints[key].EndPrice}
		ticks[i] = tick
		i++
		fmt.Println(fmt.Sprintf(`%d: %s - %s = %f `, key, futureTime, currentTime,
			(value.EndPrice-kpoints[key].EndPrice)/kpoints[key].EndPrice))

	}
	var sortedKLine model.Ticks
	sortedKLine = ticks
	percentage = 100 * percentage / float64(size)
	fmt.Println(percentage)
	sort.Sort(sortedKLine)
	for _, value := range sortedKLine {
		fmt.Println(fmt.Sprintf(`%s %f`, value.Id, value.Price))
	}
}
