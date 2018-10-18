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

const tradeFee = 0.00025

func initMoney(priceKLine []*model.KLinePoint) {
	doShort = true
	money = 5000
	balance = 10000
	coin = 5000 / priceKLine[0].EndPrice
	lastPrice = priceKLine[0].EndPrice
	fmt.Println(fmt.Sprintf(`buy%f`, lastPrice))
}

func rsiSell(kPoint *model.KLinePoint, price float64) {
	//worth := coin*price + money
	if coin > 0 {
		money += coin * price * (1 - tradeFee)
		coin = 0
	}
	//if money > 7*worth/4 {
	//	return
	//}
	//money += worth / 4 * (1 - tradeFee)
	//coin -= worth / 4 / price
}

func rsiBuy(kPoint *model.KLinePoint, price float64) {
	worth := coin*price + money
	if money >= worth/4 {
		money -= worth / 4
		coin += worth / 4 / price * (1 - tradeFee)
	} else {
		coin += money / price * (1 - tradeFee)
		money = 0
	}
}

func sell(klPoint *model.KLinePoint, price float64) {
	diff := coin*price - money
	coin -= lever * diff / 2 / price
	money += (lever * diff / 2) * (1 - tradeFee)
	balance = money + coin*price
	strTime := time.Unix(klPoint.TS/1000, 0).Format("2006-01-02 15:04:05")
	fmt.Println(fmt.Sprintf(`%s sell %f  money %f coin %f coin money %f in all %f`,
		strTime, price, money, coin, coin*price, balance))
	lastPrice = price
	totalSell += lastPrice
	countSell++
}

func buy(klPoint *model.KLinePoint, price float64) {
	diff := money - coin*price
	coin += (lever * diff / 2 / price) * (1 - tradeFee)
	money -= lever * diff / 2
	balance = money + coin*price
	strTime := time.Unix(klPoint.TS/1000, 0).Format("2006-01-02 15:04:05")
	fmt.Println(fmt.Sprintf(`%s buy %f  money %f coin %f coin money %f in all %f`,
		strTime, price, money, coin, coin*price, balance))
	lastPrice = price
	totalBuy += lastPrice
	countBuy++
}

func printBalance() {
	fmt.Println(fmt.Sprintf(`buy1 %f sell1 %f count buy1 %d count sell1 %d avg buy1 %f avg sell1 %f`,
		totalBuy, totalSell, countBuy, countSell, totalBuy/float64(countBuy), totalSell/float64(countSell)))
	//fmt.Println(fmt.Sprintf(`条数%d 净值%f`, len(priceKLine), coin*priceKLine[0].EndPrice+money))
}

func analyzeKLine(priceKLine []*model.KLinePoint, percentage float64) {
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
	for i := 300; i < len(priceKLine); i++ {
		upPercentage := (priceKLine[i].HighPrice - priceKLine[i-1].EndPrice) / priceKLine[i-1].EndPrice
		//downPercentage := (priceKLine[i-1].EndPrice - priceKLine[i].LowPrice) / priceKLine[i].LowPrice
		//if upPercentage > percentage {
		//	money += coin * (priceKLine[i-1].EndPrice * (1 + percentage))
		//	coin = 0
		//} else if downPercentage > percentage {
		//	money += coin * (priceKLine[i-1].EndPrice * (1 - percentage))
		//	coin = 0
		//} else
		if priceKLine[i].RSI < 25 {
			rsiBuy(priceKLine[i], priceKLine[i].EndPrice)
		} else if priceKLine[i].RSI > 70 && percentage >= upPercentage {
			rsiSell(priceKLine[i], priceKLine[i].EndPrice)
		}
	}
}

func printKLine(kline []*model.KLinePoint) {
	for _, kPoint := range kline {
		strTime := time.Unix(kPoint.TS/1000, 0).Format("2006-01-02 15:04:05")
		fmt.Println(fmt.Sprintf(`%s %f`, strTime, kPoint.EndPrice))
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
		avgUp := 0.0
		avgDown := 0.0
		for i := 5; i < len(diff); i++ {
			up := 0.0
			down := 0.0
			j := i - 5
			for ; j <= 5; j++ {
				if diff[j] > 0 {
					up += diff[j]
				} else {
					down += diff[j]
				}
			}
			if i > 5 {
				if diff[i] > 0 {
					up = diff[i]
				} else {
					down = diff[i]
				}
				avgUp = (avgUp*5 + up) / 6
				avgDown = (avgDown*5 + down) / 6
				data[symbol][timeSlot][i+1].RSI = 100 * avgUp / (avgUp - avgDown)
			} else {
				avgUp = up / 6
				avgDown = down / 6
				data[symbol][timeSlot][i+1].RSI = 100 * avgUp / (avgUp - avgDown)
			}
		}
	}
	return data[symbol][timeSlot]
}

func testBalance() {
	symbols := []string{`eth_usdt`}
	slots := []string{`15min`}
	percentages := []float64{0.01}
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
					//results[slot][percentage][symbol] += (beginPrice - endPrice) / beginPrice * balance / 2
					results[slot][percentage][symbol] = results[slot][percentage][symbol] - 5000/beginPrice*endPrice - 5000
				}
				fmt.Print(fmt.Sprintf(`	===========%s:%f `, symbol, results[slot][percentage][symbol]))
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
	symbols := []string{`btc_usdt`, `eth_usdt`, `eos_usdt`}
	slots := []string{`15min`, `30min`}
	percentages := []float64{0.0025, 0.003, 0.005, 0.01, 0.03, 0.05}
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
	testBalance()
	//testRSI()
}
