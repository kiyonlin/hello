package main

import (
	"fmt"
	"github.com/jinzhu/configor"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"hello/api"
	"hello/model"
	"hello/util"
	"testing"
	"time"
)

var lastPrice float64
var money = 5000.0
var coin = 0.0

//var priceKLine []KLinePoint
var totalBuy = 0.0
var countBuy = 0
var totalSell = 0.0
var countSell = 0
var balance = 10000.0
var lever = 1.0
var doShort = true
var size = 1560
var data = make(map[string]map[string][]*model.KLinePoint) // symbol - slot - kline data

const tradeFee = 0.00035

func initMoney(priceKLine []*model.KLinePoint) {
	doShort = true
	money = 5000
	balance = 10000
	coin = 5000 / priceKLine[0].EndPrice
	lastPrice = priceKLine[0].EndPrice
	//fmt.Println(fmt.Sprintf(`buy%f`, lastPrice))
}

func rsiSell(kPoint *model.KLinePoint, price float64) {
	if coin > 0 {
		money += coin * price
		coin = 0
		strTime := time.Unix(kPoint.TS/1000, 0).Format("2006-01-02 15:04:05")
		fmt.Println(fmt.Sprintf(`%f sell %s %f at %f`, money, strTime, kPoint.RSI, price))
	}
}

func rsiBuy(kPoint *model.KLinePoint) {
	if money > 0 {
		coin += money / kPoint.EndPrice
		money = 0
		strTime := time.Unix(kPoint.TS/1000, 0).Format("2006-01-02 15:04:05")
		fmt.Println(fmt.Sprintf(`%f buy %s %f at %f`, coin*kPoint.EndPrice, strTime, kPoint.RSI, kPoint.EndPrice))
	}
}

func sell(_ *model.KLinePoint, price float64) {
	diff := coin*price - money
	coin -= lever * diff / 2 / price
	money += (lever * diff / 2) * (1 - tradeFee)
	balance = money + coin*price
	//strTime := time.Unix(klPoint.TS/1000, 0).Format("2006-01-02 15:04:05")
	//fmt.Println(fmt.Sprintf(`%s sell %f  money %f coin %f coin money %f in all %f`,
	//	strTime, price, money, coin, coin*price, balance))
	lastPrice = price
	totalSell += lastPrice
	countSell++
}

func buy(_ *model.KLinePoint, price float64) {
	diff := money - coin*price
	coin += (lever * diff / 2 / price) * (1 - tradeFee)
	money -= lever * diff / 2
	balance = money + coin*price
	//strTime := time.Unix(klPoint.TS/1000, 0).Format("2006-01-02 15:04:05")
	//fmt.Println(fmt.Sprintf(`%s buy %f  money %f coin %f coin money %f in all %f`,
	//	strTime, price, money, coin, coin*price, balance))
	lastPrice = price
	totalBuy += lastPrice
	countBuy++
}

func printBalance() {
	//fmt.Println(fmt.Sprintf(`buy1 %f sell1 %f count buy1 %d count sell1 %d avg buy1 %f avg sell1 %f`,
	//	totalBuy, totalSell, countBuy, countSell, totalBuy/float64(countBuy), totalSell/float64(countSell)))
	//fmt.Println(fmt.Sprintf(`条数%d 净值%f`, len(priceKLine), coin*priceKLine[0].EndPrice+money))
}

func analyzeKLine(priceKLine []*model.KLinePoint, percentage float64) {
	//initMoney()
	//for i := 2; i < len(data)-1; i++ {
	//	if priceKLine[i-1].EndPrice > priceKLine[i-2].EndPrice && priceKLine[i].EndPrice < priceKLine[i-1].EndPrice &&
	//		priceKLine[i].EndPrice > priceKLine[i-2].EndPrice && (priceKLine[i].EndPrice-lastPrice)*coin/balance > percentage {
	//		sell(priceKLine[i])
	//	}
	//	if priceKLine[i-1].EndPrice < priceKLine[i-2].EndPrice && priceKLine[i].EndPrice > priceKLine[i-1].EndPrice &&
	//		priceKLine[i].EndPrice < priceKLine[i-2].EndPrice &&
	//		(lastPrice-priceKLine[i].EndPrice)*coin/balance > percentage {
	//		buy(priceKLine[i])
	//	}
	//}
	//printBalance()
	initMoney(priceKLine)
	for i := 1; i < len(priceKLine)-1; i++ {
		if (priceKLine[i].HighPrice-lastPrice)*coin/balance > percentage {
			sell(priceKLine[i], percentage*balance/coin+lastPrice)
		}
		if (lastPrice-priceKLine[i].LowPrice)*coin/balance > percentage {
			buy(priceKLine[i], lastPrice-percentage*balance/coin)
		}
	}
	printBalance()
}

func analyzeRSI(priceKLine []*model.KLinePoint, percentage float64) {
	money = 10000
	coin = 0
	for i := 7; i < len(priceKLine); i++ {
		upPercentage := (priceKLine[i].HighPrice - priceKLine[i-1].EndPrice) / priceKLine[i-1].EndPrice
		downPercentage := (priceKLine[i-1].EndPrice - priceKLine[i].LowPrice) / priceKLine[i].LowPrice
		if upPercentage > percentage {
			rsiSell(priceKLine[i], priceKLine[i-1].EndPrice*(1+percentage))
		}
		if downPercentage > percentage {
			rsiSell(priceKLine[i], priceKLine[i-1].EndPrice*(1-percentage))
		}
		if priceKLine[i].RSI < 20 {
			rsiBuy(priceKLine[i])
		} else if priceKLine[i].RSI > 80 {
			rsiSell(priceKLine[i], priceKLine[i].EndPrice)
		}
	}
}

func getData(symbol, timeSlot string) []*model.KLinePoint {
	if data[symbol] == nil {
		data[symbol] = make(map[string][]*model.KLinePoint)
	}
	if data[symbol][timeSlot] == nil {
		priceKLine := api.GetKLineOkex(symbol, timeSlot, int64(size))
		data[symbol][timeSlot] = priceKLine

		diff := make([]float64, len(data[symbol][timeSlot])-1)
		for i := 0; i < len(diff); i++ {
			diff[i] = data[symbol][timeSlot][i+1].EndPrice - data[symbol][timeSlot][i].EndPrice
		}
		for i := 5; i < len(diff); i++ {
			up := 0.0
			down := 0.0
			for j := i - 5; j <= i; j++ {
				if diff[j] > 0 {
					up += diff[j]
				} else {
					down -= diff[j]
				}
			}
			data[symbol][timeSlot][i+1].RSI = 100 * up / (up + down)
		}
	}
	return data[symbol][timeSlot]
}

func testBalance() {
	//model.AppDB, err = gorm.Open("postgres", model.AppConfig.DBConnection)
	//if err != nil {
	//	util.Notice(err.Error())
	//	return
	//}
	//defer model.AppDB.Close()
	//model.LoadSettings()
	//setting := model.AppFutureAccount[model.OKFUTURE][`btc_this_week`]
	//symbols := []string{`btc_usdt`, `eth_usdt`, `eos_usdt`}
	symbols := []string{`btc_usdt`, `eos_eth`, `eos_usdt`}
	slots := []string{`1min`, `5min`, `30min`, `1hour`, `6hour`}
	percentages := []float64{0.001, 0.003, 0.005, 0.01, 0.015, 0.02, 0.03, 0.04, 0.05, 0.1, 0.9}
	results := make(map[string]map[float64]map[string]float64)
	for _, slot := range slots {
		fmt.Print(fmt.Sprintf("\n%s 做空：%t", slot, doShort))
		results[slot] = make(map[float64]map[string]float64)
		for _, percentage := range percentages {
			results[slot][percentage] = make(map[string]float64)
			fmt.Print(fmt.Sprintf("\n %f", percentage))
			for _, symbol := range symbols {
				data := getData(symbol, slot)
				analyzeKLine(data, percentage)
				beginPrice := data[0].EndPrice
				endPrice := data[len(data)-1].EndPrice
				//results[slot][percentage][symbol] = coin*priceKLine[0].EndPrice + money
				results[slot][percentage][symbol] = coin*endPrice + money
				if doShort {
					results[slot][percentage][symbol] += (beginPrice - endPrice) / beginPrice * balance / 2
				}
				fmt.Print(fmt.Sprintf(`	%s:%f `, symbol, results[slot][percentage][symbol]))
			}
		}
	}
	fmt.Println()
	// 5min eth 0.009> eos 0.007 > btc 0.0018
	// 1min btc 净值10004 eth 净值10008 eos 净值10005
	// 30min eos 净值10638.965867
	//result, errCode := api.FundTransferOkex(`eos_usd`, 21.3711, `3`, `1`)
	//fmt.Println(fmt.Sprintf(`return %t %s`, result, errCode))
	//fmt.Println(fmt.Sprintf(`market %s symbol %s %f %f`, setting.Market, setting.Symbol, setting.OpenedShort, setting.OpenedLong))
	//api.CancelOrder(`binance`, `eos_usdt`, `184201444`)
	//api.CancelOrder(`binance`, `eos_usdt`, `184201445`)
}

func testRSI() {
	symbols := []string{`eos_usdt`}
	slots := []string{`1min`}
	percentages := []float64{0.001, 0.003, 0.005, 0.01, 0.015, 0.02, 0.03, 0.04, 0.05, 0.1, 0.9}
	for _, slot := range slots {
		for _, percentage := range percentages {
			for _, symbol := range symbols {
				data := getData(symbol, slot)
				analyzeRSI(data, percentage)
				fmt.Println(fmt.Sprintf(`%f %s %s %f`, percentage, slot, symbol, money+data[len(data)-1].EndPrice*coin))
			}
		}
	}

}

func Test_simulation(t *testing.T) {
	model.NewConfig()
	err := configor.Load(model.AppConfig, "./config.yml")
	if err != nil {
		util.Notice(err.Error())
		return
	}
	//testBalance()
	testRSI()
}
